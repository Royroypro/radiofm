package integration

import (
	"testing"
	"time"

	"radioclub25/emisor/internal/config"
	"radioclub25/emisor/internal/feed"
	"radioclub25/emisor/internal/ffmpeg"
	"radioclub25/emisor/internal/runner"
)

// TestInputSwitchLive: la radio lee del bus de audio; cambiar la entrada
// (tono → silencio → «audio de la PC» → tono) no corta la conexión con Icecast.
func TestInputSwitchLive(t *testing.T) {
	ff, _ := need(t)
	ice, port := startIcecast(t, "fuente")
	w := &watcher{}
	// «Audio de la PC» falso: un tono de 440 Hz a -6 dBFS.
	f, err := feed.New(feed.Options{Name: "radio", FFmpeg: ff, OnLog: w.log,
		Capture: func(_ string, on func([]byte)) (func(), error) { return feed.Tone(on, 440, -6), nil }})
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := f.Switch(config.Source{Kind: config.SourceTest}); err != nil {
		t.Fatal(err)
	}
	c := radioConfig(port, "fuente")
	c.Radio.Input = config.Source{Kind: config.SourceLoopback, Addr: f.Addr()}
	r := newRunner("radio", ff, func() ([]string, error) { return ffmpeg.RadioArgs(c.Radio), nil }, w)
	if err := r.Start(); err != nil {
		t.Fatal(err)
	}
	defer r.Stop()
	first := w.wait(t, "en vivo", 15*time.Second, func(s runner.Status) bool { return s.State == runner.Live && len(s.Levels) == 2 })

	steps := []struct {
		src      config.Source
		min, max float64
	}{
		{config.Source{Kind: config.SourceTest}, -24, -15},    // tono a -18 dBFS
		{config.Source{Kind: config.SourceNone}, -200, -60},   // silencio
		{config.Source{Kind: config.SourceLoopback}, -10, -3}, // -6 dBFS
		{config.Source{Kind: config.SourceTest}, -24, -15},
	}
	var sizes []int
	for i, st := range steps {
		if i > 0 {
			if err := f.Switch(st.src); err != nil {
				t.Fatal(err)
			}
		}
		time.Sleep(1500 * time.Millisecond) // el medidor promedia y el búfer se vacía
		p := avgPeak(w, time.Second)
		if p < st.min || p > st.max {
			t.Fatalf("%s: pico %.1f dB, esperaba entre %.0f y %.0f", st.src.Kind, p, st.min, st.max)
		}
		_, data, _, _ := ice.Get("/radio")
		sizes = append(sizes, len(data))
		t.Logf("%-8s pico %.1f dB · Icecast recibió %d bytes", st.src.Kind, p, len(data))
	}
	for i := 1; i < len(sizes); i++ {
		if sizes[i] <= sizes[i-1] {
			t.Fatalf("Icecast dejó de recibir audio: %v", sizes)
		}
	}
	if s := w.get(); s.State != runner.Live || !s.LiveSince.Equal(first.LiveSince) {
		t.Fatalf("cambiar la entrada no debía reconectar: %+v", s)
	}
}
