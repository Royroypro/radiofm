package video

import (
	"bytes"
	"context"
	"encoding/binary"
	"image"
	"image/color"
	"image/jpeg"
	"log/slog"
	"math"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"radioclub25/estudio/internal/platform/ffmpeg"
)

// Scene es lo que se ve de fondo (las capas de grafismo van siempre encima).
type Scene string

const (
	SceneVisual Scene = "visual" // fondo + espectro animado (por defecto)
	SceneImage  Scene = "image"  // placa: imagen a pantalla completa
	SceneClip   Scene = "clip"   // videoclip de lo que suena (lo elige el director, no el operador)
)

// Config es todo lo que necesita el modo TV para arrancar.
type Config struct {
	Output     ffmpeg.VideoConfig
	Look       Look
	Background image.Image // nil = degradado con los colores de la marca
	Plate      image.Image // imagen de la escena «placa»
	// ClipOffset adelanta (positivo) o atrasa el video respecto del audio, para
	// compensar la latencia de la tarjeta.
	ClipOffset time.Duration
}

// AudioSource es de donde sale el audio (Engine.TVOutput).
type AudioSource interface {
	Read(p []float32) int
	Len() int
	Discard(n int) int
}

// Status es el estado del modo TV para la interfaz.
type Status struct {
	Running bool               `json:"running"`
	Scene   Scene              `json:"scene"`
	Showing Scene              `json:"showing"` // lo que se ve ahora (el videoclip si hay)
	Clip    string             `json:"clip"`    // archivo del videoclip que suena
	Encoder ffmpeg.VideoStatus `json:"encoder"`
	FPS     float64            `json:"fps"`  // cuadros por segundo generados (medidos)
	Late    int64              `json:"late"` // cuadros que se generaron tarde (PC lenta)
	Size    string             `json:"size"`
}

// Service es el modo TV: genera los cuadros a ritmo fijo y los entrega al codificador.
type Service struct {
	tools  ffmpeg.Tools
	audio  AudioSource
	tap    func(on bool)
	log    *slog.Logger
	decode decodeFunc

	mu      sync.Mutex
	cancel  context.CancelFunc
	enc     *ffmpeg.VideoEncoder
	gfx     *Graphics
	cfg     Config
	scene   Scene
	pending struct {
		set           bool
		artist, title string
		brk           bool
	}
	layers  struct{ set, logo, now, clk, tick bool }
	clipReq struct {
		set  bool
		path string // "" = quitar el video
		pos  time.Duration
		at   time.Time
	}
	clipName string // video que suena (para la interfaz)

	snapMu  sync.Mutex
	snap    []byte // último JPEG del programa (monitor)
	snapAt  time.Time
	want    atomic.Int64 // último pedido del monitor (unix ms): solo se codifica si alguien mira
	fps     atomic.Uint64
	showing atomic.Value // Scene que se ve (string)
	late    atomic.Int64
	running atomic.Bool
}

// New crea el servicio. tap enciende o apaga la copia del audio hacia la TV.
func New(tools ffmpeg.Tools, audio AudioSource, tap func(bool), log *slog.Logger) *Service {
	s := &Service{tools: tools, audio: audio, tap: tap, log: log, scene: SceneVisual, decode: tools.DecodeVideo}
	s.showing.Store(string(SceneVisual))
	return s
}

// PlayClip pone el video path desde pos, sincronizado con el audio que empezó
// a sonar en at (el director lo llama al empezar una canción con video).
func (s *Service) PlayClip(path string, pos time.Duration, at time.Time) {
	s.mu.Lock()
	s.clipReq.set, s.clipReq.path, s.clipReq.pos, s.clipReq.at = true, path, pos, at
	s.mu.Unlock()
}

// StopClip quita el video (vuelve el visualizador).
func (s *Service) StopClip() { s.PlayClip("", 0, time.Time{}) }

// Start (re)arranca el modo TV con cfg.
func (s *Service) Start(ctx context.Context, cfg Config) {
	s.Stop()
	s.mu.Lock()
	defer s.mu.Unlock()
	o := cfg.Output
	o.W, o.H = o.W&^1, o.H&^1
	cfg.Output = o
	s.cfg = cfg
	s.enc = s.tools.NewVideoEncoder(o)
	s.gfx = NewGraphics(o.W, o.H, cfg.Look)
	ctx, s.cancel = context.WithCancel(ctx)
	s.tap(true)
	s.running.Store(true)
	go s.enc.Run(ctx)
	go s.loop(ctx, s.enc, s.gfx, cfg)
	s.log.Info("modo TV iniciado", "salida", redact(o.URL), "tamaño", o.W, "x", o.H, "fps", o.FPS)
}

// Stop detiene el modo TV.
func (s *Service) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cancel != nil {
		s.cancel()
		s.cancel = nil
		s.tap(false)
	}
	s.running.Store(false)
	s.enc = nil
}

// SetNowPlaying actualiza el cintillo (se aplica en el próximo cuadro).
func (s *Service) SetNowPlaying(artist, title string, brk bool) {
	s.mu.Lock()
	s.pending.set, s.pending.artist, s.pending.title, s.pending.brk = true, artist, title, brk
	s.mu.Unlock()
}

// SetScene cambia la escena (con fundido).
func (s *Service) SetScene(sc Scene) {
	s.mu.Lock()
	s.scene = sc
	s.mu.Unlock()
}

// SetLayers enciende o apaga las capas de grafismo al instante.
func (s *Service) SetLayers(logo, now, clk, tick bool) {
	s.mu.Lock()
	s.layers.set, s.layers.logo, s.layers.now, s.layers.clk, s.layers.tick = true, logo, now, clk, tick
	s.mu.Unlock()
}

// Status devuelve el estado.
func (s *Service) Status() Status {
	s.mu.Lock()
	defer s.mu.Unlock()
	st := Status{Running: s.running.Load(), Scene: s.scene, FPS: math.Float64frombits(s.fps.Load()), Late: s.late.Load(),
		Clip: s.clipName, Showing: Scene(s.showing.Load().(string))}
	if s.enc != nil {
		st.Encoder = s.enc.Status()
		st.Size = strconv.Itoa(s.cfg.Output.W) + "×" + strconv.Itoa(s.cfg.Output.H)
	}
	return st
}

// Snapshot devuelve el último JPEG del programa (para el monitor). Pedirlo
// activa la captura durante unos segundos.
func (s *Service) Snapshot() []byte {
	s.want.Store(time.Now().UnixMilli())
	s.snapMu.Lock()
	defer s.snapMu.Unlock()
	return s.snap
}

func (s *Service) loop(ctx context.Context, enc *ffmpeg.VideoEncoder, gfx *Graphics, cfg Config) {
	o := cfg.Output
	w, h, fps := o.W, o.H, o.FPS
	spf := 44100 / fps // muestras por cuadro (1470 a 30 fps)
	bg := cfg.Background
	var bgFrame *Frame
	if bg != nil {
		bgFrame = FrameFromImage(bg, w, h)
	} else {
		panel := cfg.Look.Panel
		panel.A = 255
		bgFrame = GradientFrame(w, h, shade(panel, 0.55), panel)
	}
	var plate *Frame
	if cfg.Plate != nil {
		plate = FrameFromImage(cfg.Plate, w, h)
	}
	vis := NewVisualizer(56, cfg.Look.Accent, cfg.Look.Accent2)
	prog, a, b := NewFrame(w, h), NewFrame(w, h), NewFrame(w, h)
	samples := make([]float32, spf*2)
	abytes := make([]byte, spf*2*4)
	vbytes := make([]byte, 0, prog.Size())

	cur, prev := SceneVisual, SceneVisual
	fadeLeft, fadeLen := 0, fps/2 // fundido de medio segundo
	var cl *clip
	var clipFrame *Frame
	defer func() {
		if cl != nil {
			cl.close()
		}
	}()
	render := func(sc Scene, dst *Frame) {
		if sc == SceneImage && plate != nil {
			dst.CopyFrom(plate)
			return
		}
		if sc == SceneClip && clipFrame != nil {
			dst.CopyFrom(clipFrame)
			return
		}
		dst.CopyFrom(bgFrame)
		vis.Draw(dst, w*8/100, h*18/100, w*84/100, h*60/100)
	}

	s.audio.Discard(s.audio.Len())
	start := time.Now()
	var made int64
	tick := time.NewTicker(time.Second / time.Duration(fps))
	defer tick.Stop()
	measureAt, measureN := start, int64(0)
	snapBusy := atomic.Bool{}
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
		// cuántos cuadros tocan según el reloj (si la PC se atrasó, se recuperan)
		due := int64(time.Since(start).Seconds() * float64(fps))
		if due-made > int64(fps) { // más de 1 s de atraso: no recuperar, saltar
			s.late.Add(due - made)
			made = due - 1
		}
		for made < due {
			made++
			// novedades de la interfaz
			s.mu.Lock()
			if s.pending.set {
				gfx.SetNowPlaying(s.pending.artist, s.pending.title, s.pending.brk)
				s.pending.set = false
			}
			if s.layers.set {
				gfx.look.ShowLogo, gfx.look.ShowNow, gfx.look.ShowClk, gfx.look.ShowTick =
					s.layers.logo, s.layers.now, s.layers.clk, s.layers.tick
				s.layers.set = false
			}
			want := s.scene
			req := s.clipReq
			s.clipReq.set = false
			s.mu.Unlock()

			// videoclip: el director pide uno nuevo o quitarlo
			if req.set {
				if cl != nil {
					cl.close()
					cl = nil
				}
				name := ""
				if req.path != "" {
					c, err := startClip(ctx, s.decode, req.path, req.pos, req.at.Add(-cfg.ClipOffset), w, h, fps)
					if err != nil {
						s.log.Error("TV: abrir video", "archivo", req.path, "err", err)
					} else {
						cl, name = c, req.path
					}
				}
				s.mu.Lock()
				s.clipName = name
				s.mu.Unlock()
			}
			clipFrame = nil
			if cl != nil {
				f, ok := cl.frame(time.Now())
				if ok {
					clipFrame = f
				} else if cl.ended.Load() {
					cl.close()
					cl = nil
					s.mu.Lock()
					s.clipName = ""
					s.mu.Unlock()
				}
			}
			// la escena del operador manda; si no, el videoclip; si no, el visualizador
			if want == SceneVisual && clipFrame != nil {
				want = SceneClip
			}
			if want == SceneImage && plate == nil {
				want = SceneVisual
			}
			s.showing.Store(string(want))

			// audio del cuadro: exactamente spf muestras (silencio si falta; si
			// sobra mucho por la deriva de relojes, se descarta)
			// (poco atraso: el videoclip se sincroniza con la hora, así que el audio de la
			// TV no debe quedar más de ~4 cuadros detrás de lo que suena)
			if l := s.audio.Len(); l > spf*2*4 {
				s.audio.Discard(l - spf*2*2)
			}
			n := s.audio.Read(samples)
			clear(samples[n:])
			vis.Push(samples)
			vis.Update()

			if want != cur {
				prev, cur, fadeLeft = cur, want, fadeLen
			}
			if fadeLeft > 0 {
				render(prev, a)
				render(cur, b)
				prog.Mix(a, b, 1-float32(fadeLeft)/float32(fadeLen))
				fadeLeft--
			} else {
				render(cur, prog)
			}
			gfx.Draw(prog, time.Now(), fps)

			for i, v := range samples {
				binary.LittleEndian.PutUint32(abytes[i*4:], math.Float32bits(v))
			}
			vbytes = prog.AppendTo(vbytes[:0])
			enc.Push(vbytes, abytes)

			// monitor: un JPEG cada 200 ms mientras alguien mire
			if made%int64(max(fps/5, 1)) == 0 && time.Now().UnixMilli()-s.want.Load() < 5000 && snapBusy.CompareAndSwap(false, true) {
				cp := NewFrame(w, h)
				cp.CopyFrom(prog)
				go func() {
					defer snapBusy.Store(false)
					s.storeSnap(cp)
				}()
			}
		}
		if el := time.Since(measureAt); el >= 2*time.Second {
			s.fps.Store(math.Float64bits(float64(made-measureN) / el.Seconds()))
			measureAt, measureN = time.Now(), made
		}
	}
}

func (s *Service) storeSnap(f *Frame) {
	img := &image.YCbCr{Y: f.Y, Cb: f.U, Cr: f.V, YStride: f.W, CStride: f.W / 2,
		SubsampleRatio: image.YCbCrSubsampleRatio420, Rect: image.Rect(0, 0, f.W, f.H)}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 70}); err != nil {
		return
	}
	s.snapMu.Lock()
	s.snap, s.snapAt = buf.Bytes(), time.Now()
	s.snapMu.Unlock()
}

// redact oculta la clave de la URL para el registro.
func redact(u string) string {
	if i := strings.Index(u, "pass="); i >= 0 {
		return u[:i] + "pass=***"
	}
	return u
}

// shade mezcla c con negro (k = 0 → igual, 1 → negro).
func shade(c color.NRGBA, k float64) color.NRGBA {
	f := func(v uint8) uint8 { return uint8(float64(v) * (1 - k)) }
	return color.NRGBA{f(c.R), f(c.G), f(c.B), 255}
}
