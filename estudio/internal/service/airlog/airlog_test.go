package airlog

import (
	"context"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"radioclub25/estudio/internal/audio"
	"radioclub25/estudio/internal/platform/ffmpeg"
)

type fakeEngine struct {
	ring *audio.RingBuffer
	on   bool
}

func (f *fakeEngine) SetLoggerTap(on bool)            { f.on = on }
func (f *fakeEngine) LoggerOutput() *audio.RingBuffer { return f.ring }

func quiet() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func writeFile(t *testing.T, dir string, at time.Time, size int) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, at.Format(fileLayout)+".mp3"), make([]byte, size), 0o644); err != nil {
		t.Fatal(err)
	}
}

// Retención: primero lo más viejo que MaxDays, luego lo más viejo hasta quedar
// bajo MaxMB; nunca el archivo en curso.
func TestPrune(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 11, 9, 15, 30, 0, 0, time.Local)
	for h := 0; h < 6; h++ { // 6 archivos de 1 MB: hace 40 días (1) y hoy de 10:00 a 14:00 (5)
		at := now.Add(-time.Duration(6-h) * time.Hour).Truncate(time.Hour) // 10:00 … 14:00
		if h == 0 {
			at = now.AddDate(0, 0, -40)
		}
		writeFile(t, dir, at, 1<<20)
	}
	writeFile(t, dir, now.Truncate(time.Hour), 1<<20) // en curso
	if err := os.WriteFile(filepath.Join(dir, "otra-cosa.txt"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	s := New(&fakeEngine{}, nil, func() Settings { return Settings{Dir: dir, MaxMB: 4, MaxDays: 30} }, quiet())
	s.now = func() time.Time { return now }
	if n := s.Prune(); n != 3 { // el de hace 40 días + 2 más viejos para bajar de 7 MB a 4 MB
		t.Fatalf("borrados = %d", n)
	}
	files, _ := s.Files()
	if len(files) != 4 || files[len(files)-1].Start != now.Truncate(time.Hour) {
		t.Fatalf("quedan %+v", files)
	}
	if _, err := os.Stat(filepath.Join(dir, "otra-cosa.txt")); err != nil {
		t.Fatal("solo se borran grabaciones")
	}
	if _, err := s.FilePath("../x.mp3"); err == nil {
		t.Fatal("no debe aceptar rutas")
	}
}

func TestWindowsAndFileName(t *testing.T) {
	now := time.Date(2026, 11, 9, 9, 5, 0, 0, time.Local) // lunes 09:05
	r := Recording{Name: "Mañanas: Club", Days: 0b0111110, Start: "06:00", Minutes: 180}
	w := windows(r, now)
	if len(w) != 1 || w[0].from.Hour() != 6 || w[0].to.Hour() != 9 {
		t.Fatalf("franjas = %+v", w)
	}
	if w := windows(r, now.Add(-10*time.Minute)); len(w) != 0 { // 08:55: todavía no terminó
		t.Fatalf("no debe exportar antes de terminar: %+v", w)
	}
	if w := windows(Recording{Name: "x", Days: 1, Start: "06:00", Minutes: 60}, now); len(w) != 0 {
		t.Fatalf("solo domingos: un lunes no corresponde: %+v", w)
	}
	if got := fileName(r.Name, w[0].from); got != "Mañanas_ Club 2026-11-09 0600.mp3" {
		t.Fatalf("nombre = %q", got)
	}
}

// De punta a punta con ffmpeg real: graba el bus y exporta un tramo.
func TestRecordAndExport(t *testing.T) {
	tools, err := ffmpeg.Locate()
	if err != nil {
		t.Skip("ffmpeg no disponible")
	}
	dir := t.TempDir()
	eng := &fakeEngine{ring: audio.NewRingBuffer(audio.SampleRate * audio.Channels * 5)}
	s := New(eng, tools.Command, func() Settings { return Settings{Enabled: true, Dir: dir, Kbps: 64} }, quiet())
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { s.Run(ctx); close(done) }()
	deadline := time.Now().Add(5 * time.Second)
	for !eng.on && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	start := time.Now()
	sig := audio.Sine(440, 0.3, 3)
	for i := 0; i < len(sig); i += 8820 { // 100 ms por vez, a tiempo real
		eng.ring.Write(sig[i:min(i+8820, len(sig))])
		time.Sleep(100 * time.Millisecond)
	}
	cancel()
	<-done
	files, _ := s.Files()
	if len(files) != 1 || files[0].Size < 10_000 {
		t.Fatalf("grabación = %+v (error %q)", files, s.Status().Error)
	}
	dst := filepath.Join(dir, "programas", "tramo.mp3")
	if err := s.Export(context.Background(), start.Add(500*time.Millisecond), start.Add(2*time.Second), dst); err != nil {
		t.Fatal(err)
	}
	info, err := tools.Probe(context.Background(), dst)
	if err != nil || info.Duration < time.Second || info.Duration > 2*time.Second {
		t.Fatalf("tramo exportado: %+v, %v", info, err)
	}
	if err := s.Export(context.Background(), start.Add(-time.Hour), start.Add(-50*time.Minute), dst); err == nil {
		t.Fatal("sin grabación de esa franja debe dar error")
	}
	_ = exec.ErrNotFound
}
