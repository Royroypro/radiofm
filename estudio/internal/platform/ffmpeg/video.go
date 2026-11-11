package ffmpeg

import (
	"context"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// VideoConfig es la salida del modo TV.
type VideoConfig struct {
	URL         string // rtmp://servidor:1935/tv?user=..&pass=.. (o srt://, o un archivo para pruebas)
	W, H, FPS   int
	BitrateKbps int    // video (2500 por defecto)
	AudioKbps   int    // AAC (128 por defecto)
	Codec       string // x264 (por defecto) | nvenc | qsv | amf
}

func (c VideoConfig) withDefaults() VideoConfig {
	if c.BitrateKbps <= 0 {
		c.BitrateKbps = 2500
	}
	if c.AudioKbps <= 0 {
		c.AudioKbps = 128
	}
	if c.FPS <= 0 {
		c.FPS = 30
	}
	return c
}

// videoArgs arma la línea de ffmpeg: video crudo YUV 4:2:0 por TCP (entrada 0)
// y PCM float32 estéreo 44,1 kHz por TCP (entrada 1) → H.264 + AAC. Por TCP
// local porque en Windows ffmpeg no puede leer pipes extra además de stdin.
func (c VideoConfig) videoArgs(vport, aport int) []string {
	b := fmt.Sprintf("%dk", c.BitrateKbps)
	buf := fmt.Sprintf("%dk", 2*c.BitrateKbps)
	gop := fmt.Sprint(2 * c.FPS) // un keyframe cada 2 s (segmentos HLS de mediamtx)
	a := []string{"-hide_banner", "-loglevel", "error", "-nostdin",
		"-thread_queue_size", "1024", "-f", "rawvideo", "-pix_fmt", "yuv420p",
		"-video_size", fmt.Sprintf("%dx%d", c.W, c.H), "-framerate", fmt.Sprint(c.FPS),
		"-i", fmt.Sprintf("tcp://127.0.0.1:%d", vport),
		"-thread_queue_size", "1024", "-f", "f32le", "-ar", "44100", "-ac", "2",
		"-i", fmt.Sprintf("tcp://127.0.0.1:%d", aport),
		"-map", "0:v", "-map", "1:a"}
	switch c.Codec {
	case "nvenc":
		a = append(a, "-c:v", "h264_nvenc", "-preset", "p4", "-tune", "ll", "-rc", "cbr")
	case "qsv":
		a = append(a, "-c:v", "h264_qsv", "-preset", "veryfast")
	case "amf":
		a = append(a, "-c:v", "h264_amf", "-rc", "cbr")
	default:
		a = append(a, "-c:v", "libx264", "-preset", "veryfast", "-profile:v", "main")
	}
	a = append(a, "-b:v", b, "-maxrate", b, "-bufsize", buf, "-g", gop, "-keyint_min", gop, "-sc_threshold", "0",
		"-pix_fmt", "yuv420p", "-colorspace", "bt709", "-color_primaries", "bt709", "-color_trc", "bt709",
		"-color_range", "tv",
		"-c:a", "aac", "-b:a", fmt.Sprintf("%dk", c.AudioKbps), "-ar", "44100")
	switch {
	case strings.HasPrefix(c.URL, "rtmp://") || strings.HasPrefix(c.URL, "rtmps://"):
		a = append(a, "-f", "flv")
	case strings.HasPrefix(c.URL, "srt://"):
		a = append(a, "-f", "mpegts")
	default:
		a = append(a, "-y") // archivo (pruebas): el formato sale de la extensión
	}
	return append(a, c.URL)
}

// VideoStatus es el estado de la emisión de TV.
type VideoStatus struct {
	Connected bool      `json:"connected"`
	Since     time.Time `json:"since,omitempty"`
	Retries   int       `json:"retries"`
	LastError string    `json:"last_error,omitempty"`
	Frames    int64     `json:"frames"`  // cuadros enviados
	Dropped   int64     `json:"dropped"` // cuadros descartados (ffmpeg atrasado o desconectado)
	Kbps      int       `json:"kbps"`
	Codec     string    `json:"codec"`
}

// VideoEncoder envía video y audio a ffmpeg en paso fijo (un cuadro con su
// audio por vez, o se descartan los dos juntos: nunca se desincronizan) y se
// reconecta solo, con espera exponencial, si se corta.
type VideoEncoder struct {
	tools Tools
	cfg   VideoConfig

	cur     atomic.Pointer[videoSession]
	frames  atomic.Int64
	dropped atomic.Int64

	mu     sync.Mutex
	status VideoStatus

	MinBackoff, MaxBackoff time.Duration
}

// NewVideoEncoder crea el codificador de TV.
func (t Tools) NewVideoEncoder(cfg VideoConfig) *VideoEncoder {
	cfg = cfg.withDefaults()
	return &VideoEncoder{tools: t, cfg: cfg, MinBackoff: 2 * time.Second, MaxBackoff: time.Minute,
		status: VideoStatus{Kbps: cfg.BitrateKbps, Codec: cfg.Codec}}
}

// Status devuelve una copia del estado.
func (e *VideoEncoder) Status() VideoStatus {
	e.mu.Lock()
	defer e.mu.Unlock()
	s := e.status
	s.Frames, s.Dropped = e.frames.Load(), e.dropped.Load()
	return s
}

func (e *VideoEncoder) update(f func(*VideoStatus)) {
	e.mu.Lock()
	f(&e.status)
	e.mu.Unlock()
}

// Push entrega un cuadro (yuv420p) y su audio (f32le). No bloquea: si ffmpeg
// está atrasado o no hay conexión, descarta los dos y devuelve false.
func (e *VideoEncoder) Push(video, audio []byte) bool {
	s := e.cur.Load()
	if s == nil || !s.push(video, audio) {
		e.dropped.Add(1)
		return false
	}
	e.frames.Add(1)
	return true
}

// Run mantiene la emisión hasta que ctx se cancele.
func (e *VideoEncoder) Run(ctx context.Context) {
	backoff := e.MinBackoff
	for ctx.Err() == nil {
		started := time.Now()
		err := e.session(ctx)
		if ctx.Err() != nil {
			break
		}
		stable := time.Since(started) > 30*time.Second
		e.update(func(s *VideoStatus) {
			s.Connected, s.Since = false, time.Time{}
			if err != nil {
				s.LastError = err.Error()
			}
			if stable {
				s.Retries = 0
			}
			s.Retries++
		})
		if stable {
			backoff = e.MinBackoff
		}
		select {
		case <-ctx.Done():
		case <-time.After(backoff):
		}
		backoff = min(backoff*2, e.MaxBackoff)
	}
	e.update(func(s *VideoStatus) { s.Connected, s.Since = false, time.Time{} })
}

// videoSession es una ejecución de ffmpeg con sus dos conexiones.
type videoSession struct {
	vq, vfree chan []byte
	aq, afree chan []byte
}

const videoQueue = 12 // cuadros en cola (~0,4 s a 30 fps)

func (s *videoSession) push(video, audio []byte) bool {
	var vb, ab []byte
	select {
	case vb = <-s.vfree:
	default:
		return false
	}
	select {
	case ab = <-s.afree:
	default:
		s.vfree <- vb
		return false
	}
	vb = append(vb[:0], video...)
	ab = append(ab[:0], audio...)
	s.vq <- vb // no bloquea: hay tantos lugares como buffers
	s.aq <- ab
	return true
}

func (e *VideoEncoder) session(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	lv, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return err
	}
	defer lv.Close()
	la, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return err
	}
	defer la.Close()
	frameSize := e.cfg.W * e.cfg.H * 3 / 2
	audioSize := 44100 / e.cfg.FPS * 2 * 4
	s := &videoSession{vq: make(chan []byte, videoQueue), vfree: make(chan []byte, videoQueue),
		aq: make(chan []byte, videoQueue), afree: make(chan []byte, videoQueue)}
	for i := 0; i < videoQueue; i++ {
		s.vfree <- make([]byte, 0, frameSize)
		s.afree <- make([]byte, 0, audioSize+64)
	}

	// el proceso no se mata con ctx: al detener se cierran sus entradas para que
	// termine de escribir (un archivo de prueba queda completo) y, si no sale en 5 s, se mata
	cmd := command(context.Background(), e.tools.FFmpeg, e.cfg.videoArgs(lv.Addr().(*net.TCPAddr).Port, la.Addr().(*net.TCPAddr).Port)...)
	stderr := &limitedBuffer{max: 4096}
	cmd.Stderr = stderr
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("iniciar ffmpeg: %w", err)
	}
	exited := make(chan error, 1)
	go func() { exited <- cmd.Wait() }()
	e.cur.Store(s)
	defer e.cur.Store(nil)

	failed := make(chan error, 2)
	writer := func(l net.Listener, q, free chan []byte) {
		_ = l.(*net.TCPListener).SetDeadline(time.Now().Add(20 * time.Second))
		c, err := l.Accept()
		if err != nil {
			failed <- fmt.Errorf("ffmpeg no se conectó: %w", err)
			return
		}
		defer c.Close()
		for {
			select {
			case <-ctx.Done():
				return
			case b := <-q:
				_ = c.SetWriteDeadline(time.Now().Add(10 * time.Second))
				_, err := c.Write(b)
				free <- b
				if err != nil {
					failed <- err
					return
				}
			}
		}
	}
	go writer(lv, s.vq, s.vfree)
	go writer(la, s.aq, s.afree)

	connectedAt := time.Now()
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	finish := func(err error) error {
		cancel() // los escritores cierran sus conexiones: ffmpeg recibe fin de datos
		_ = lv.Close()
		_ = la.Close()
		select {
		case <-exited:
		case <-time.After(5 * time.Second):
			_ = cmd.Process.Kill()
			<-exited
		}
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return fmt.Errorf("%s", lastLine(msg))
		}
		return err
	}
	for {
		select {
		case <-ctx.Done():
			return finish(nil)
		case err := <-exited:
			exited <- err // para finish
			if err == nil {
				err = io.EOF
			}
			return finish(fmt.Errorf("ffmpeg terminó: %w", err))
		case err := <-failed:
			return finish(err)
		case <-tick.C:
			// sin aviso de conexión del RTMP: si ffmpeg sigue vivo a los 3 s, está emitiendo
			if time.Since(connectedAt) > 3*time.Second {
				e.update(func(st *VideoStatus) {
					if !st.Connected {
						st.Connected, st.Since, st.LastError = true, time.Now(), ""
					}
				})
			}
		}
	}
}

// DecodeVideo arranca ffmpeg para decodificar el video de path desde start a
// cuadros crudos yuv420p de w×h a fps cuadros por segundo (sin audio), sin
// deformar: se escala para que entre y se completa con negro. Lee lo más
// rápido posible: el que consume marca el ritmo (y la sincronía con el audio).
//
// loop = repetir sin fin (fondo en bucle).
func (t Tools) DecodeVideo(ctx context.Context, path string, start time.Duration, w, h, fps int, loop bool) (io.ReadCloser, error) {
	args := []string{"-hide_banner", "-loglevel", "error", "-nostdin"}
	if loop {
		args = append(args, "-stream_loop", "-1")
	}
	if start > 0 {
		args = append(args, "-ss", fmt.Sprintf("%.3f", start.Seconds()))
	}
	vf := fmt.Sprintf("scale=%d:%d:force_original_aspect_ratio=decrease:flags=bicubic,pad=%d:%d:(ow-iw)/2:(oh-ih)/2:black,fps=%d,format=yuv420p",
		w, h, w, h, fps)
	args = append(args, "-i", path, "-an", "-sn", "-vf", vf, "-f", "rawvideo", "-pix_fmt", "yuv420p", "pipe:1")
	ctx, cancel := context.WithCancel(ctx)
	cmd := command(ctx, t.FFmpeg, args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		return nil, err
	}
	stderr := &limitedBuffer{max: 4096}
	cmd.Stderr = stderr
	if err := cmd.Start(); err != nil {
		cancel()
		return nil, fmt.Errorf("ffmpeg: iniciar: %w", err)
	}
	return &procReader{r: stdout, cmd: cmd, cancel: cancel, stderr: stderr}, nil
}

// HasAudio dice si el archivo tiene alguna pista de audio (un video mudo no).
func (t Tools) HasAudio(ctx context.Context, path string) bool {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	out, err := command(ctx, t.FFprobe, "-v", "error", "-select_streams", "a", "-show_entries", "stream=index",
		"-of", "csv=p=0", path).Output()
	return err == nil && len(strings.TrimSpace(string(out))) > 0
}
