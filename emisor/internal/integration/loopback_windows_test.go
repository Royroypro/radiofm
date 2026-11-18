//go:build windows && cgo

package integration

import (
	"encoding/binary"
	"math"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gen2brain/malgo"

	"radioclub25/emisor/internal/config"
	"radioclub25/emisor/internal/ffmpeg"
	"radioclub25/emisor/internal/loopback"
	"radioclub25/emisor/internal/runner"
)

// virtualOutput busca una salida de audio virtual (VB-Cable, Virtual Audio Cable…)
// para que la prueba no suene en los altavoces de la PC.
func virtualOutput(t *testing.T) string {
	names, err := loopback.ListPlayback()
	if err != nil {
		t.Skipf("no se pudieron listar las salidas de audio: %v", err)
	}
	for _, n := range names {
		l := strings.ToLower(n)
		if strings.Contains(l, "cable input") || strings.Contains(l, "virtual audio cable") || strings.Contains(l, "line 1 (virtual") {
			return n
		}
	}
	t.Skip("no hay una salida de audio virtual (VB-Cable); se omite para no hacer sonar los altavoces")
	return ""
}

// playTone reproduce un tono de 1 kHz a -12 dBFS en la salida name hasta que se llame a la función devuelta.
func playTone(t *testing.T, name string) func() {
	t.Helper()
	ctx, err := malgo.InitContext([]malgo.Backend{malgo.BackendWasapi}, malgo.ContextConfig{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	infos, err := ctx.Devices(malgo.Playback)
	if err != nil {
		t.Fatal(err)
	}
	cfg := malgo.DefaultDeviceConfig(malgo.Playback)
	cfg.Playback.Format, cfg.Playback.Channels, cfg.SampleRate = malgo.FormatS16, 2, 48000
	for i := range infos {
		if infos[i].Name() == name {
			cfg.Playback.DeviceID = infos[i].ID.Pointer()
		}
	}
	var phase float64
	amp := 32767 * math.Pow(10, -12.0/20)
	dev, err := malgo.InitDevice(ctx.Context, cfg, malgo.DeviceCallbacks{
		Data: func(out, _ []byte, frames uint32) {
			for i := 0; i < int(frames); i++ {
				v := int16(amp * math.Sin(phase))
				phase += 2 * math.Pi * 1000 / 48000
				binary.LittleEndian.PutUint16(out[i*4:], uint16(v))
				binary.LittleEndian.PutUint16(out[i*4+2:], uint16(v))
			}
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := dev.Start(); err != nil {
		t.Fatal(err)
	}
	var once sync.Once
	return func() { once.Do(func() { dev.Uninit(); _ = ctx.Uninit(); ctx.Free() }) }
}

// TestRadioFromPCPlayback emite a la radio lo que suena en una salida de la PC
// (captura loopback) y comprueba que llega el tono con su nivel.
func TestRadioFromPCPlayback(t *testing.T) {
	ff, _ := need(t)
	out := virtualOutput(t)
	stopTone := playTone(t, out)
	defer stopTone()

	srv, err := loopback.NewServer()
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()
	stopCap, err := loopback.Capture(out, srv.Write)
	if err != nil {
		t.Fatal(err)
	}
	defer stopCap()

	ice, port := startIcecast(t, "fuente")
	c := radioConfig(port, "fuente")
	c.Radio.Input = config.Source{Kind: config.SourceLoopback, Name: out, Addr: srv.Addr()}
	if err := c.ValidateRadio(); err != nil {
		t.Fatal(err)
	}
	w := &watcher{}
	r := newRunner("radio", ff, func() ([]string, error) { return ffmpeg.RadioArgs(c.Radio), nil }, w)
	if err := r.Start(); err != nil {
		t.Fatal(err)
	}
	defer r.Stop()
	s := w.wait(t, "radio en vivo con el tono de la PC", 20*time.Second, func(s runner.Status) bool {
		return s.State == runner.Live && len(s.Levels) == 2 && s.Levels[0].PeakDB > -20 && s.SentBytes > 30_000
	})
	// Tono a -12 dBFS (el mezclador de Windows puede cambiarlo un poco).
	if s.Levels[0].PeakDB < -18 || s.Levels[0].PeakDB > -6 {
		t.Errorf("nivel capturado %.1f dB, esperaba ≈ -12", s.Levels[0].PeakDB)
	}
	r.Stop()
	if _, data, _, _ := ice.Get("/radio"); len(data) < 30_000 {
		t.Fatalf("Icecast recibió poco: %d bytes", len(data))
	}
	t.Logf("salida %q: nivel %.1f dB, %.0f kbps", out, s.Levels[0].PeakDB, s.BitrateKbps)

	// Al parar el tono, la emisión sigue (silencio relleno) en lugar de cortarse.
	stopTone()
	w2 := &watcher{}
	r2 := newRunner("radio", ff, func() ([]string, error) { return ffmpeg.RadioArgs(c.Radio), nil }, w2)
	_ = r2.Start()
	defer r2.Stop()
	w2.wait(t, "en vivo con la salida en silencio", 20*time.Second, func(s runner.Status) bool {
		return s.State == runner.Live && s.SentBytes > 40_000
	})
}
