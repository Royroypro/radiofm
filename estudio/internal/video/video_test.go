package video

import (
	"context"
	"encoding/json"
	"image"
	"image/color"
	"io"
	"log/slog"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"

	"radioclub25/estudio/internal/platform/ffmpeg"
)

func TestMixIsExact(t *testing.T) {
	a, b, f := NewFrame(64, 32), NewFrame(64, 32), NewFrame(64, 32)
	a.Fill(YUV{20, 100, 100})
	b.Fill(YUV{220, 160, 160})
	f.Mix(a, b, 0)
	if f.Y[0] != 20 || f.U[0] != 100 {
		t.Fatalf("t=0 debe ser a: %d %d", f.Y[0], f.U[0])
	}
	f.Mix(a, b, 1)
	if f.Y[0] != 220 || f.V[0] != 160 {
		t.Fatalf("t=1 debe ser b: %d %d", f.Y[0], f.V[0])
	}
	f.Mix(a, b, 0.5)
	if d := int(f.Y[0]) - 120; d < -1 || d > 1 {
		t.Fatalf("t=0,5 debe ser el promedio: %d", f.Y[0])
	}
}

func TestLayerAlpha(t *testing.T) {
	f := NewFrame(64, 64)
	f.Fill(ToYUV(0, 0, 0))
	img := image.NewNRGBA(image.Rect(0, 0, 16, 16))
	for y := 0; y < 16; y++ {
		for x := 0; x < 16; x++ {
			a := uint8(255)
			if x >= 8 {
				a = 0 // mitad derecha transparente
			}
			img.SetNRGBA(x, y, color.NRGBA{255, 255, 255, a})
		}
	}
	f.Draw(NewLayer(img), 10, 10, 0, 0, 1)
	white := ToYUV(255, 255, 255).Y
	if got := f.Y[12*64+12]; got != white {
		t.Fatalf("parte opaca: Y=%d, quería %d", got, white)
	}
	if got := f.Y[12*64+20]; got != 16 {
		t.Fatalf("parte transparente no debe cambiar: Y=%d", got)
	}
	if got := f.Y[40*64+40]; got != 16 {
		t.Fatalf("fuera de la capa no debe cambiar: Y=%d", got)
	}
	// media opacidad
	g := NewFrame(64, 64)
	g.Fill(ToYUV(0, 0, 0))
	g.Draw(NewLayer(img), 0, 0, 0, 0, 0.5)
	if d := int(g.Y[2*64+2]) - (16+int(white))/2; d < -2 || d > 2 {
		t.Fatalf("opacidad 0,5: Y=%d", g.Y[2*64+2])
	}
	// recorte fuera del cuadro: no debe fallar
	g.Draw(NewLayer(img), 60, 60, 0, 0, 1)
	g.Draw(NewLayer(img), -8, -8, 0, 0, 1)
}

func sine(freq float64, n, offset int) []float32 {
	out := make([]float32, n*2)
	for i := 0; i < n; i++ {
		v := float32(0.5 * math.Sin(2*math.Pi*freq*float64(i+offset)/44100))
		out[2*i], out[2*i+1] = v, v
	}
	return out
}

func TestVisualizerFollowsTone(t *testing.T) {
	v := NewVisualizer(40, color.NRGBA{255, 0, 0, 255}, color.NRGBA{255, 200, 0, 255})
	for i := 0; i < 10; i++ {
		v.Push(sine(1000, 1470, i*1470))
		v.Update()
	}
	lv := v.Levels()
	best := 0
	for i := range lv {
		if lv[i] > lv[best] {
			best = i
		}
	}
	f := math.Exp(math.Log(40) + (math.Log(16000)-math.Log(40))*(float64(best)+0.5)/40)
	if f < 700 || f > 1400 {
		t.Fatalf("la barra más alta está en ~%.0f Hz, quería ~1 kHz (niveles %v)", f, lv)
	}
	if lv[0] > 0.3 || lv[len(lv)-1] > 0.3 {
		t.Fatalf("graves y agudos deben quedar bajos con un tono de 1 kHz: %v", lv)
	}
	// silencio: las barras bajan
	for i := 0; i < 60; i++ {
		v.Push(make([]float32, 2940))
		v.Update()
	}
	for _, l := range v.Levels() {
		if l > 0.05 {
			t.Fatalf("en silencio las barras deben bajar: %v", v.Levels())
		}
	}
	fr := NewFrame(320, 180)
	v.Draw(fr, 10, 10, 300, 160) // no debe fallar
}

func TestGraphicsDraw(t *testing.T) {
	g := NewGraphics(1280, 720, Look{Station: "Radio Club 25", ShowLogo: true, ShowNow: true, ShowClk: true, ShowTick: true,
		Ticker: "Bienvenidos a Radio Club 25", Accent: color.NRGBA{230, 40, 40, 255}, Accent2: color.NRGBA{250, 150, 30, 255},
		Panel: color.NRGBA{10, 20, 60, 220}, Text: color.NRGBA{255, 255, 255, 255}})
	g.SetNowPlaying("Artista", "Una canción con un título muy largo que no entra en el cintillo de ninguna manera posible", false)
	f := NewFrame(1280, 720)
	before := append([]byte(nil), f.Y...)
	for i := 0; i < 40; i++ {
		g.Draw(f, time.Now().Add(time.Duration(i)*100*time.Millisecond), 30)
	}
	changed := 0
	for i := range f.Y {
		if f.Y[i] != before[i] {
			changed++
		}
	}
	if changed < 20000 {
		t.Fatalf("el grafismo casi no dibujó nada (%d píxeles)", changed)
	}
	if g.now == nil || g.now.W > 1280*62/100+200 {
		t.Fatal("el cintillo debe recortarse al 62 % del ancho")
	}
	g.SetNowPlaying("Ferretería El Sol", "Cuña", true)
	g.Draw(f, time.Now(), 30)
	if g.brk == nil {
		t.Fatal("en tanda debe aparecer la placa «Publicidad»")
	}
}

// fakeAudio entrega un tono a ritmo real (como el motor de audio).
type fakeAudio struct {
	mu    sync.Mutex
	start time.Time
	read  int
	phase float64
}

func (f *fakeAudio) avail() int {
	return int(time.Since(f.start).Seconds()*44100)*2 - f.read
}
func (f *fakeAudio) Len() int { f.mu.Lock(); defer f.mu.Unlock(); return max(f.avail(), 0) }
func (f *fakeAudio) Discard(n int) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n = min(n, max(f.avail(), 0))
	f.read += n
	return n
}
func (f *fakeAudio) Read(p []float32) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := min(len(p), max(f.avail(), 0)) &^ 1
	for i := 0; i < n; i += 2 {
		v := float32(0.3 * math.Sin(f.phase))
		f.phase += 2 * math.Pi * 440 / 44100
		p[i], p[i+1] = v, v
	}
	f.read += n
	return n
}

// Codifica unos segundos a un archivo con el ffmpeg real y revisa con ffprobe
// que audio y video duren lo mismo (paso fijo), el tamaño y los cuadros por segundo.
func TestEncodeToFile(t *testing.T) {
	tools, err := ffmpeg.Locate()
	if err != nil {
		t.Skip("sin ffmpeg")
	}
	out := filepath.Join(t.TempDir(), "tv.flv")
	src := &fakeAudio{start: time.Now()}
	s := New(tools, src, func(bool) {}, slog.Default())
	s.Start(context.Background(), Config{
		Output: ffmpeg.VideoConfig{URL: out, W: 640, H: 360, FPS: 30, BitrateKbps: 800},
		Look: Look{Station: "Club 25", ShowLogo: true, ShowNow: true, ShowClk: true, Ticker: "Prueba",
			ShowTick: true, Accent: color.NRGBA{230, 40, 40, 255}, Accent2: color.NRGBA{250, 150, 30, 255},
			Panel: color.NRGBA{10, 20, 60, 220}, Text: color.NRGBA{255, 255, 255, 255}},
	})
	s.SetNowPlaying("Artista", "Canción", false)
	_ = s.Snapshot()
	// a los 2 s entra un videoclip (generado) que dura 10 s
	clipFile := filepath.Join(t.TempDir(), "clip.mp4")
	if out, err := exec.Command(tools.FFmpeg, "-v", "error", "-f", "lavfi", "-i", "testsrc2=size=640x360:rate=30:duration=10",
		"-pix_fmt", "yuv420p", clipFile).CombinedOutput(); err != nil {
		t.Fatalf("generar clip: %v %s", err, out)
	}
	time.Sleep(2 * time.Second)
	s.PlayClip(clipFile, 0, time.Now())
	time.Sleep(4 * time.Second)
	st := s.Status()
	if st.Showing != SceneClip {
		t.Fatalf("con un videoclip sonando debe verse el clip, se ve %q", st.Showing)
	}
	snap := s.Snapshot()
	s.Stop()
	time.Sleep(1500 * time.Millisecond)     // ffmpeg cierra el archivo
	if p := os.Getenv("TV_SNAP"); p != "" { // para mirar el resultado: TV_SNAP=archivo.jpg
		_ = os.WriteFile(p, snap, 0o644)
	}
	if len(snap) < 1000 {
		t.Fatalf("el monitor no generó la imagen (%d bytes)", len(snap))
	}
	if st.FPS < 25 {
		t.Fatalf("se generan %.1f cuadros por segundo, quería ~30 (estado %+v)", st.FPS, st)
	}
	// FLV no guarda duración por pista: se cuentan los cuadros de video y de audio (AAC = 1024 muestras)
	cmd := exec.Command(tools.FFprobe, "-v", "error", "-count_frames",
		"-show_entries", "stream=codec_type,width,height,avg_frame_rate,nb_read_frames", "-of", "json", out)
	b, err := cmd.Output()
	if err != nil {
		t.Fatalf("ffprobe: %v (estado del codificador %+v)", err, st.Encoder)
	}
	var probe struct {
		Streams []struct {
			Type   string `json:"codec_type"`
			W      int    `json:"width"`
			H      int    `json:"height"`
			Rate   string `json:"avg_frame_rate"`
			Frames string `json:"nb_read_frames"`
		} `json:"streams"`
	}
	if err := json.Unmarshal(b, &probe); err != nil {
		t.Fatal(err)
	}
	var vd, ad float64
	for _, x := range probe.Streams {
		n, _ := strconv.ParseFloat(x.Frames, 64)
		switch x.Type {
		case "video":
			vd = n / 30
			if x.W != 640 || x.H != 360 || x.Rate != "30/1" {
				t.Fatalf("video %dx%d a %s, quería 640x360 a 30/1", x.W, x.H, x.Rate)
			}
		case "audio":
			ad = n * 1024 / 44100
		}
	}
	t.Logf("duración video %.3f s, audio %.3f s, %s", vd, ad, string(b))
	if vd < 3 {
		t.Fatalf("se codificó muy poco: %.2f s", vd)
	}
	if math.Abs(vd-ad) > 0.1 {
		t.Fatalf("audio y video deben durar lo mismo: video %.3f s, audio %.3f s", vd, ad)
	}
}

// fakeVideo entrega n cuadros: el cuadro k tiene Y = k (para saber cuál se muestra).
type fakeVideo struct {
	w, h, n, k int
	buf        []byte
}

func (f *fakeVideo) Read(p []byte) (int, error) {
	if len(f.buf) == 0 {
		if f.k >= f.n {
			return 0, io.EOF
		}
		fr := NewFrame(f.w, f.h)
		for i := range fr.Y {
			fr.Y[i] = byte(f.k)
		}
		f.buf = fr.AppendTo(nil)
		f.k++
	}
	n := copy(p, f.buf)
	f.buf = f.buf[n:]
	return n, nil
}
func (f *fakeVideo) Close() error { return nil }

// El videoclip se sincroniza con la hora del audio: si arranca «tarde» salta
// cuadros hasta alcanzarlo, y al terminar avisa para volver al visualizador.
func TestClipFollowsAudioClock(t *testing.T) {
	dec := func(ctx context.Context, path string, start time.Duration, w, h, fps int) (io.ReadCloser, error) {
		return &fakeVideo{w: w, h: h, n: 40}, nil
	}
	at := time.Now().Add(-500 * time.Millisecond) // el audio empezó hace medio segundo
	c, err := startClip(context.Background(), dec, "x.mp4", 0, at, 32, 18, 30)
	if err != nil {
		t.Fatal(err)
	}
	defer c.close()
	var f *Frame
	var ok bool
	for i := 0; i < 100 && (!ok || f.Y[0] < 15); i++ { // el decodificador entrega de a poco: alcanza al audio
		time.Sleep(10 * time.Millisecond)
		f, ok = c.frame(at.Add(500 * time.Millisecond))
	}
	if !ok || f.Y[0] != 15 {
		t.Fatalf("a los 0,5 s a 30 fps debe verse el cuadro 15 (se ve %v, ok=%v)", f, ok)
	}
	// adelantado: pedir un instante anterior no retrocede (repite el cuadro)
	if f, _ := c.frame(at.Add(100 * time.Millisecond)); f.Y[0] != 15 {
		t.Fatalf("no debe retroceder: %d", f.Y[0])
	}
	// después del final: termina
	for i := 0; i < 100; i++ {
		if _, ok := c.frame(at.Add(10 * time.Second)); !ok {
			if !c.ended.Load() {
				t.Fatal("debe marcar que terminó")
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("el clip no terminó")
}

// Decodifica un video real (generado con ffmpeg): sale al tamaño y ritmo pedidos.
func TestDecodeVideoReal(t *testing.T) {
	tools, err := ffmpeg.Locate()
	if err != nil {
		t.Skip("sin ffmpeg")
	}
	src := filepath.Join(t.TempDir(), "clip.mp4")
	gen := exec.Command(tools.FFmpeg, "-v", "error", "-f", "lavfi", "-i", "testsrc=size=320x240:rate=25:duration=2",
		"-pix_fmt", "yuv420p", src)
	if out, err := gen.CombinedOutput(); err != nil {
		t.Fatalf("generar video: %v %s", err, out)
	}
	rc, err := tools.DecodeVideo(context.Background(), src, 500*time.Millisecond, 64, 36, 30)
	if err != nil {
		t.Fatal(err)
	}
	defer rc.Close()
	f := NewFrame(64, 36)
	n := 0
	for {
		if _, err := io.ReadFull(rc, f.Y); err != nil {
			break
		}
		if _, err := io.ReadFull(rc, f.U); err != nil {
			break
		}
		if _, err := io.ReadFull(rc, f.V); err != nil {
			break
		}
		n++
	}
	// 1,5 s (desde 0,5 s) a 30 fps ≈ 45 cuadros (4:3 en 16:9 con bandas negras)
	if n < 42 || n > 48 {
		t.Fatalf("cuadros = %d, quería ~45", n)
	}
	if f.Y[0] > 20 { // la esquina es banda negra (pad)
		t.Fatalf("la esquina debe ser negra (banda lateral): Y=%d", f.Y[0])
	}
}
