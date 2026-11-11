package video

import (
	"context"
	"encoding/binary"
	"io"
	"math"
	"sync"
	"sync/atomic"
	"time"

	"radioclub25/estudio/internal/audio"
	"radioclub25/estudio/internal/platform/ffmpeg"
)

// SourceSpec es una fuente de la TV, como el «Origen multimedia» de OBS: un
// archivo local o una URL en vivo (http/https, HLS, rtmp, rtsp, srt).
type SourceSpec struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Path  string `json:"path"`  // ruta del archivo o URL
	Loop  bool   `json:"loop"`  // archivos: repetir al terminar
	Audio string `json:"audio"` // radio (el audio de la TV sigue siendo la radio) | source (el de la fuente)
}

// sourceRun es una fuente al aire: se abre en segundo plano (consultar si trae
// audio y conectar puede tardar unos segundos; mientras, sigue lo anterior) y,
// si es una URL y se corta, se reconecta sola.
type sourceRun struct {
	spec    SourceSpec
	w, h    int
	fps     int
	open    sourceOpener
	hasAud  func(ctx context.Context, path string) bool
	ctx     context.Context
	cancel  context.CancelFunc
	mu      sync.Mutex
	clip    *clip
	stream  sourceStream
	ring    *audio.RingBuffer // audio de la fuente (si se usa)
	audioOn atomic.Bool       // el audio de la fuente está llegando
	silent  atomic.Bool
	err     atomic.Value // string
	ended   atomic.Bool  // archivo sin repetir que terminó
	retryAt time.Time
}

// sourceStream es lo que entrega ffmpeg.Tools.OpenSource (los tests usan un falso).
type sourceStream interface {
	io.Closer
	video() io.ReadCloser
	audioConn(wait time.Duration) io.ReadCloser
}

type sourceOpener func(ctx context.Context, src string, loop, withAudio bool, w, h, fps int) (sourceStream, error)

// ffmpegOpener adapta Tools.OpenSource.
func ffmpegOpener(t ffmpeg.Tools) sourceOpener {
	return func(ctx context.Context, src string, loop, withAudio bool, w, h, fps int) (sourceStream, error) {
		s, err := t.OpenSource(ctx, src, loop, withAudio, w, h, fps)
		if err != nil {
			return nil, err
		}
		return ffStream{s}, nil
	}
}

type ffStream struct{ s *ffmpeg.SourceStream }

func (f ffStream) Close() error         { return f.s.Close() }
func (f ffStream) video() io.ReadCloser { return f.s.Video }
func (f ffStream) audioConn(wait time.Duration) io.ReadCloser {
	if c := f.s.Audio(wait); c != nil {
		return c
	}
	return nil
}

func newSourceRun(parent context.Context, spec SourceSpec, open sourceOpener, hasAudio func(context.Context, string) bool, w, h, fps int) *sourceRun {
	ctx, cancel := context.WithCancel(parent)
	r := &sourceRun{spec: spec, w: w, h: h, fps: fps, open: open, hasAud: hasAudio, ctx: ctx, cancel: cancel}
	r.err.Store("")
	go r.connect()
	return r
}

func (r *sourceRun) connect() {
	r.audioOn.Store(false)
	has := r.hasAud(r.ctx, r.spec.Path)
	r.silent.Store(!has)
	withAudio := has && r.spec.Audio == "source"
	st, err := r.open(r.ctx, r.spec.Path, r.spec.Loop, withAudio, r.w, r.h, r.fps)
	if err != nil {
		r.fail(err.Error())
		return
	}
	cctx, ccancel := context.WithCancel(r.ctx)
	c := newClip(cctx, ccancel, st.video(), time.Now(), true, r.w, r.h, r.fps)
	r.mu.Lock()
	r.clip, r.stream = c, st
	r.mu.Unlock()
	r.err.Store("")
	if withAudio {
		if conn := st.audioConn(15 * time.Second); conn != nil {
			r.mu.Lock()
			if r.ring == nil {
				r.ring = audio.NewRingBuffer(44100 * 2 * 2) // 2 s
			}
			ring := r.ring
			r.mu.Unlock()
			r.audioOn.Store(true)
			go r.readAudio(conn, ring)
		}
	}
}

// readAudio pasa el audio de la fuente (f32le) al ring buffer.
func (r *sourceRun) readAudio(conn io.ReadCloser, ring *audio.RingBuffer) {
	defer conn.Close()
	defer r.audioOn.Store(false)
	buf := make([]byte, 4096*8)
	samples := make([]float32, 4096*2)
	rest := 0
	for {
		n, err := conn.Read(buf[rest:])
		n += rest
		k := n / 4
		for i := 0; i < k; i++ {
			samples[i] = math.Float32frombits(binary.LittleEndian.Uint32(buf[i*4:]))
		}
		ring.Write(samples[:k]) // si la TV no lo lee a tiempo, se descarta
		rest = copy(buf, buf[k*4:n])
		if err != nil {
			return
		}
	}
}

func (r *sourceRun) fail(msg string) {
	r.err.Store(msg)
	r.mu.Lock()
	r.retryAt = time.Now().Add(5 * time.Second)
	r.mu.Unlock()
}

// frame devuelve el cuadro actual de la fuente. Si una URL se cortó, la
// reconecta (cada 5 s); si un archivo sin repetir terminó, marca ended.
func (r *sourceRun) frame(now time.Time) (*Frame, bool) {
	r.mu.Lock()
	c, retry := r.clip, r.retryAt
	r.mu.Unlock()
	if c == nil {
		if !retry.IsZero() && now.After(retry) && ffmpeg.IsNetwork(r.spec.Path) {
			r.mu.Lock()
			r.retryAt = time.Time{}
			r.mu.Unlock()
			go r.connect()
		} else if !retry.IsZero() && !ffmpeg.IsNetwork(r.spec.Path) {
			r.ended.Store(true) // archivo que no abre: se deja
		}
		return nil, false
	}
	f, ok := c.frame(now)
	if !ok && c.ended.Load() {
		r.drop()
		if ffmpeg.IsNetwork(r.spec.Path) {
			r.fail("la señal se cortó; reconectando…")
		} else {
			r.ended.Store(true)
		}
	}
	return f, ok
}

// readAudioInto llena p con el audio de la fuente (silencio si falta). ok =
// false si la fuente no aporta audio (se usa el de la radio).
func (r *sourceRun) readAudioInto(p []float32) bool {
	if r.spec.Audio != "source" || !r.audioOn.Load() {
		return false
	}
	r.mu.Lock()
	ring := r.ring
	r.mu.Unlock()
	if ring == nil {
		return false
	}
	if l := ring.Len(); l > len(p)*4 { // poco atraso, como el audio de la radio
		ring.Discard(l - len(p)*2)
	}
	n := ring.Read(p)
	clear(p[n:])
	return true
}

func (r *sourceRun) drop() {
	r.mu.Lock()
	c, st := r.clip, r.stream
	r.clip, r.stream = nil, nil
	r.mu.Unlock()
	if c != nil {
		c.close()
	}
	if st != nil {
		_ = st.Close()
	}
}

func (r *sourceRun) close() {
	r.cancel()
	r.drop()
}

func (r *sourceRun) errText() string { return r.err.Load().(string) }
