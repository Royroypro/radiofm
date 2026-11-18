package integration

import (
	"strings"
	"sync"
	"testing"
	"time"

	"radioclub25/emisor/internal/config"
	"radioclub25/emisor/internal/ffmpeg"
	"radioclub25/emisor/internal/runner"
)

// avgPeak promedia el pico del canal izquierdo durante d.
func avgPeak(w *watcher, d time.Duration) float64 {
	var sum float64
	var n int
	for end := time.Now().Add(d); time.Now().Before(end); time.Sleep(100 * time.Millisecond) {
		if s := w.get(); len(s.Levels) > 0 {
			sum += s.Levels[0].PeakDB
			n++
		}
	}
	if n == 0 {
		return ffmpeg.Silence
	}
	return sum / float64(n)
}

// TestEnhanceLiveAndApply: MEJORA se enciende y apaga al aire sin cortar, y
// Aplicar (Restart) reconecta en seguida con la configuración nueva.
func TestEnhanceLiveAndApply(t *testing.T) {
	ff, _ := need(t)
	ice, port := startIcecast(t, "fuente")
	var mu sync.Mutex
	c := radioConfig(port, "fuente")
	w := &watcher{}
	r := newRunner("radio", ff, func() ([]string, error) {
		mu.Lock()
		defer mu.Unlock()
		return ffmpeg.RadioArgs(c.Radio), nil
	}, w)
	if err := r.Start(); err != nil {
		t.Fatal(err)
	}
	defer r.Stop()
	first := w.wait(t, "en vivo", 15*time.Second, func(s runner.Status) bool { return s.State == runner.Live && len(s.Levels) == 2 })
	time.Sleep(time.Second)
	off := avgPeak(w, time.Second)

	// MEJORA al aire: sube el nivel (compresor con +8 dB), sin reconectar.
	if err := r.Send(ffmpeg.EnhanceCommands(true)); err != nil {
		t.Fatal(err)
	}
	time.Sleep(2 * time.Second) // 6 comandos, uno cada 0,25 s
	on := avgPeak(w, time.Second)
	if on-off < 5 {
		w.mu.Lock()
		t.Log(strings.Join(w.logs, "\n"))
		w.mu.Unlock()
		t.Fatalf("MEJORA debía subir el nivel: apagada %.1f dB, encendida %.1f dB", off, on)
	}
	if on > -0.4 {
		t.Fatalf("el limitador debía dejarlo bajo -0,5 dBFS: %.1f dB", on)
	}
	if err := r.Send(ffmpeg.EnhanceCommands(false)); err != nil {
		t.Fatal(err)
	}
	time.Sleep(2 * time.Second)
	back := avgPeak(w, time.Second)
	if d := back - off; d > 1 || d < -1 {
		t.Fatalf("al apagar MEJORA debía volver al nivel original: %.1f → %.1f dB", off, back)
	}
	if s := w.get(); s.State != runner.Live || !s.LiveSince.Equal(first.LiveSince) {
		t.Fatalf("MEJORA no debía cortar ni reconectar: %+v", s)
	}
	t.Logf("tono: %.1f dB sin MEJORA, %.1f dB con MEJORA, %.1f dB al apagarla", off, on, back)

	// Aplicar: cambia el formato a AAC y la radio se reconecta en seguida.
	mu.Lock()
	c.Radio.Codec, c.Radio.BitrateKbps = config.CodecAAC, 96
	mu.Unlock()
	start := time.Now()
	r.Restart()
	w.wait(t, "en vivo en AAC", 15*time.Second, func(s runner.Status) bool {
		ctype, _, _, _ := ice.Get("/radio")
		return s.State == runner.Live && ctype == "audio/aac" && s.SentBytes > 0
	})
	if el := time.Since(start); el > 5*time.Second {
		t.Fatalf("Aplicar tardó %s en volver al aire", el)
	}
	t.Logf("Aplicar: de MP3 a AAC en %s", time.Since(start).Round(10*time.Millisecond))
}

// TestEQLive: subir la banda de 1 kHz con un tono de 1 kHz sube el nivel al aire, sin reconectar.
func TestEQLive(t *testing.T) {
	ff, _ := need(t)
	_, port := startIcecast(t, "fuente")
	c := radioConfig(port, "fuente")
	w := &watcher{}
	r := newRunner("radio", ff, func() ([]string, error) { return ffmpeg.RadioArgs(c.Radio), nil }, w)
	if err := r.Start(); err != nil {
		t.Fatal(err)
	}
	defer r.Stop()
	first := w.wait(t, "en vivo", 15*time.Second, func(s runner.Status) bool { return s.State == runner.Live && len(s.Levels) == 2 })
	time.Sleep(time.Second)
	flat := avgPeak(w, time.Second)
	boost := [10]float64{0, 0, 0, 0, 0, 6, 0, 0, 0, 0} // banda 5 = 1 kHz
	if err := r.Send(ffmpeg.EQCommands([10]float64{}, boost)); err != nil {
		t.Fatal(err)
	}
	time.Sleep(1500 * time.Millisecond)
	up := avgPeak(w, time.Second)
	if d := up - flat; d < 5 || d > 7 {
		t.Fatalf("+6 dB en 1 kHz con un tono de 1 kHz: %.1f → %.1f dB (diferencia %.1f)", flat, up, d)
	}
	if s := w.get(); !s.LiveSince.Equal(first.LiveSince) {
		t.Fatal("el ecualizador no debía reconectar")
	}
	t.Logf("ecualizador al aire: %.1f dB plano → %.1f dB con +6 en 1 kHz", flat, up)
}

// TestGainLive: la ganancia se cambia al aire, sin reconectar.
func TestGainLive(t *testing.T) {
	ff, _ := need(t)
	_, port := startIcecast(t, "fuente")
	c := radioConfig(port, "fuente")
	w := &watcher{}
	r := newRunner("radio", ff, func() ([]string, error) { return ffmpeg.RadioArgs(c.Radio), nil }, w)
	if err := r.Start(); err != nil {
		t.Fatal(err)
	}
	defer r.Stop()
	first := w.wait(t, "en vivo", 15*time.Second, func(s runner.Status) bool { return s.State == runner.Live && len(s.Levels) == 2 })
	time.Sleep(time.Second)
	before := avgPeak(w, time.Second)
	if err := r.Send(ffmpeg.GainCommand(6)); err != nil {
		t.Fatal(err)
	}
	time.Sleep(time.Second)
	after := avgPeak(w, time.Second)
	if d := after - before; d < 5.5 || d > 6.5 {
		t.Fatalf("+6 dB de ganancia: %.1f → %.1f dB", before, after)
	}
	if s := w.get(); !s.LiveSince.Equal(first.LiveSince) {
		t.Fatal("la ganancia no debía reconectar")
	}
	t.Logf("ganancia al aire: %.1f → %.1f dB", before, after)
}
