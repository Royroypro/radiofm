package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"radioclub25/emisor/internal/config"
	"radioclub25/emisor/internal/ffmpeg"
	"radioclub25/emisor/internal/icefake"
	"radioclub25/emisor/internal/runner"
)

func need(t *testing.T) (ff, probe string) {
	t.Helper()
	if testing.Short() {
		t.Skip("prueba con ffmpeg real (sin -short)")
	}
	ff, err1 := exec.LookPath("ffmpeg")
	probe, err2 := exec.LookPath("ffprobe")
	if err1 != nil || err2 != nil {
		t.Skip("no hay ffmpeg/ffprobe en el PATH")
	}
	return ff, probe
}

func encoders(t *testing.T, ff string) map[string]bool {
	out, err := exec.Command(ff, "-hide_banner", "-encoders").Output()
	if err != nil {
		t.Fatal(err)
	}
	return ffmpeg.ParseEncoders(string(out))
}

func freePort(t *testing.T, network string) int {
	t.Helper()
	if network == "udp" {
		c, err := net.ListenPacket("udp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		defer c.Close()
		return c.LocalAddr().(*net.UDPAddr).Port
	}
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

// startIcecast levanta icefake con la contraseña de fuente dada.
func startIcecast(t *testing.T, pass string) (*icefake.Server, int) {
	srv := icefake.New("source", pass)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	go srv.Serve(ln)
	return srv, ln.Addr().(*net.TCPAddr).Port
}

// watcher junta los estados de un runner.
type watcher struct {
	mu   sync.Mutex
	last runner.Status
	seen []runner.Status
	logs []string
}

func (w *watcher) status(s runner.Status) {
	w.mu.Lock()
	w.last = s
	w.seen = append(w.seen, s)
	w.mu.Unlock()
}
func (w *watcher) log(l string)       { w.mu.Lock(); w.logs = append(w.logs, l); w.mu.Unlock() }
func (w *watcher) get() runner.Status { w.mu.Lock(); defer w.mu.Unlock(); return w.last }

func (w *watcher) wait(t *testing.T, what string, d time.Duration, ok func(runner.Status) bool) runner.Status {
	t.Helper()
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if s := w.get(); ok(s) {
			return s
		}
		time.Sleep(50 * time.Millisecond)
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	t.Fatalf("no llegó %q; último estado %+v\nregistro:\n%s", what, w.last, strings.Join(w.logs, "\n"))
	return runner.Status{}
}

func newRunner(name, ff string, args func() ([]string, error), w *watcher) *runner.Runner {
	return runner.New(runner.Options{Name: name, FFmpeg: ff, Args: args, OnStatus: w.status, OnLog: w.log,
		MinBackoff: 300 * time.Millisecond, MaxBackoff: time.Second})
}

// probe devuelve los streams de un archivo según ffprobe.
type stream struct {
	CodecType string `json:"codec_type"`
	CodecName string `json:"codec_name"`
	Profile   string `json:"profile"`
	Width     int    `json:"width"`
	Height    int    `json:"height"`
	Channels  int    `json:"channels"`
	Rate      string `json:"sample_rate"`
}

func probeFile(t *testing.T, probe, path string) []stream {
	t.Helper()
	out, err := exec.Command(probe, "-v", "error", "-show_streams", "-of", "json", path).Output()
	if err != nil {
		t.Fatalf("ffprobe %s: %v", path, err)
	}
	var r struct{ Streams []stream }
	if err := json.Unmarshal(out, &r); err != nil {
		t.Fatal(err)
	}
	return r.Streams
}

func radioConfig(port int, pass string) config.Config {
	c := config.Defaults()
	c.Radio.Host, c.Radio.Port, c.Radio.Password = "127.0.0.1", port, pass
	c.Radio.Input = config.Source{Kind: config.SourceTest}
	return c
}

// emitRadio emite la radio con la configuración dada hasta tener ~2 s de audio.
func emitRadio(t *testing.T, ff string, c config.Config) (*watcher, func()) {
	t.Helper()
	if err := c.ValidateRadio(); err != nil {
		t.Fatalf("configuración inválida: %v", err)
	}
	w := &watcher{}
	r := newRunner("radio", ff, func() ([]string, error) { return ffmpeg.RadioArgs(c.Radio), nil }, w)
	if err := r.Start(); err != nil {
		t.Fatal(err)
	}
	return w, r.Stop
}

func TestRadioFormats(t *testing.T) {
	ff, probe := need(t)
	enc := encoders(t, ff)
	cases := []struct {
		codec, ctype, probeCodec, profile string
		kbps                              int
	}{
		{config.CodecMP3, "audio/mpeg", "mp3", "", 320},
		{config.CodecAAC, "audio/aac", "aac", "LC", 320},
		{config.CodecHEAAC, "audio/aac", "aac", "HE-AAC", 64},
		{config.CodecHEAACv2, "audio/aac", "aac", "HE-AACv2", 48},
		{config.CodecOpus, "audio/ogg", "opus", "", 320},
		{config.CodecVorbis, "audio/ogg", "vorbis", "", 192},
	}
	for _, tc := range cases {
		t.Run(tc.codec, func(t *testing.T) {
			if need := map[string]string{config.CodecHEAAC: "libfdk_aac", config.CodecHEAACv2: "libfdk_aac"}[tc.codec]; need != "" && !enc[need] {
				t.Skipf("este ffmpeg no trae %s (AAC+ necesita un ffmpeg con libfdk_aac)", need)
			}
			ice, port := startIcecast(t, "fuente:con@raros")
			c := radioConfig(port, "fuente:con@raros")
			c.Radio.Codec, c.Radio.BitrateKbps = tc.codec, tc.kbps
			w, stop := emitRadio(t, ff, c)
			live := w.wait(t, "en vivo", 15*time.Second, func(s runner.Status) bool { return s.State == runner.Live && s.SentBytes > 0 })
			w.wait(t, "2 s de audio con niveles", 15*time.Second, func(s runner.Status) bool {
				return s.SentBytes > int64(tc.kbps*1000/8) && len(s.Levels) == 2
			})
			lv := w.get().Levels
			stop()
			// Tono de prueba: -18 dBFS en mono, ≈ -21 dBFS por canal en estéreo.
			if lv[0].PeakDB > -17 || lv[0].PeakDB < -23 {
				t.Errorf("nivel del tono = %.1f dB, esperaba ≈ -21", lv[0].PeakDB)
			}
			ctype, data, headers, ok := ice.Get("/radio")
			if !ok || len(data) == 0 {
				t.Fatal("Icecast no recibió nada en /radio")
			}
			if ctype != tc.ctype || headers.Get("Ice-Name") != "Radio Club 25" {
				t.Errorf("Content-Type %q / Ice-Name %q", ctype, headers.Get("Ice-Name"))
			}
			f := filepath.Join(t.TempDir(), "recibido")
			os.WriteFile(f, data, 0o600)
			st := probeFile(t, probe, f)
			if len(st) != 1 || st[0].CodecName != tc.probeCodec || (tc.profile != "" && st[0].Profile != tc.profile) {
				t.Fatalf("lo recibido no es %s %s: %+v", tc.probeCodec, tc.profile, st)
			}
			t.Logf("%s %d kbps: en vivo en %s, %d bytes recibidos, %+v", tc.codec, tc.kbps, time.Since(live.LiveSince).Round(time.Millisecond), len(data), st[0])
		})
	}
}

func TestRadioWrongPasswordRetries(t *testing.T) {
	ff, _ := need(t)
	ice, port := startIcecast(t, "la-buena")
	c := radioConfig(port, "la-mala")
	w, stop := emitRadio(t, ff, c)
	defer stop()
	s := w.wait(t, "detenido tras rechazos de la clave", 30*time.Second, func(s runner.Status) bool {
		return s.State == runner.Stopped && s.Message != ""
	})
	if !strings.Contains(s.Message, "401") {
		t.Errorf("el motivo debía hablar de la clave: %q", s.Message)
	}
	if ice.Rejected() != runner.MaxAuthFails {
		t.Errorf("debía intentar %d veces: %d rechazos", runner.MaxAuthFails, ice.Rejected())
	}
}

// receiver es ffmpeg escuchando como servidor RTMP o SRT (en lugar de MediaMTX).
func receiver(t *testing.T, ff, proto string) (c config.TV, out string, wait func()) {
	t.Helper()
	dir := t.TempDir()
	c = config.Defaults().TV
	c.Host, c.Password, c.Protocol = "127.0.0.1", "Clave123", proto
	c.Video, c.Audio = config.Source{Kind: config.SourceTest}, config.Source{Kind: config.SourceTest}
	var in string
	if proto == config.ProtoSRT {
		c.SRTPort = freePort(t, "udp")
		in = fmt.Sprintf("srt://127.0.0.1:%d?mode=listener", c.SRTPort)
		out = filepath.Join(dir, "recibido.ts")
	} else {
		c.RTMPPort = freePort(t, "tcp")
		in = fmt.Sprintf("rtmp://127.0.0.1:%d/tv", c.RTMPPort)
		out = filepath.Join(dir, "recibido.flv")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	args := []string{"-hide_banner", "-loglevel", "error", "-y"}
	if proto == config.ProtoRTMP {
		args = append(args, "-listen", "1")
	}
	args = append(args, "-i", in, "-c", "copy", out)
	cmd := exec.CommandContext(ctx, ff, args...)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	time.Sleep(500 * time.Millisecond) // que el receptor ya escuche
	return c, out, func() {
		done := make(chan error, 1)
		go func() { done <- cmd.Wait() }()
		select {
		case <-done:
		case <-time.After(10 * time.Second):
			cancel()
			<-done
		}
		cancel()
		if stderr.Len() > 0 {
			t.Logf("receptor: %s", stderr.String())
		}
	}
}

func emitTV(t *testing.T, ff string, c config.TV) (*watcher, func()) {
	t.Helper()
	cfg := config.Defaults()
	cfg.TV = c
	if err := cfg.ValidateTV(); err != nil {
		t.Fatalf("configuración inválida: %v", err)
	}
	w := &watcher{}
	r := newRunner("tv", ff, func() ([]string, error) { return ffmpeg.TVArgs(c), nil }, w)
	if err := r.Start(); err != nil {
		t.Fatal(err)
	}
	return w, r.Stop
}

func checkVideo(t *testing.T, probe, path string, wantW, wantH int) {
	t.Helper()
	var v, a bool
	for _, s := range probeFile(t, probe, path) {
		switch {
		case s.CodecType == "video" && s.CodecName == "h264" && s.Width == wantW && s.Height == wantH:
			v = true
		case s.CodecType == "audio" && s.CodecName == "aac" && s.Channels == 2:
			a = true
		}
	}
	if !v || !a {
		t.Fatalf("%s: esperaba H.264 %dx%d + AAC estéreo, ffprobe: %+v", filepath.Base(path), wantW, wantH, probeFile(t, probe, path))
	}
}

func TestTVOnly(t *testing.T) {
	ff, probe := need(t)
	for _, proto := range []string{config.ProtoRTMP, config.ProtoSRT} {
		t.Run(proto, func(t *testing.T) {
			c, out, waitRecv := receiver(t, ff, proto)
			c.VideoKbps = config.MaxVideoKbps // 6000 kbps
			c.Audio = config.Source{Kind: config.SourceNone}
			w, stop := emitTV(t, ff, c)
			w.wait(t, "TV en vivo 2 s", 30*time.Second, func(s runner.Status) bool {
				return s.State == runner.Live && s.SentBytes > 600_000 && len(s.Levels) == 2
			})
			s := w.get()
			stop()
			waitRecv()
			checkVideo(t, probe, out, 1280, 720)
			t.Logf("TV por %s: %.0f kbps, %.0f fps, %d bytes", proto, s.BitrateKbps, s.FPS, s.SentBytes)
		})
	}
}

// TestRadioAndTVTogether: las dos salidas a la vez, cada una con su proceso.
func TestRadioAndTVTogether(t *testing.T) {
	ff, probe := need(t)
	ice, port := startIcecast(t, "fuente")
	rc := radioConfig(port, "fuente")
	tc, out, waitRecv := receiver(t, ff, config.ProtoRTMP)
	tc.Width, tc.Height, tc.VideoKbps = 854, 480, 1500
	rw, stopRadio := emitRadio(t, ff, rc)
	tw, stopTV := emitTV(t, ff, tc)
	rw.wait(t, "radio en vivo", 20*time.Second, func(s runner.Status) bool { return s.State == runner.Live && s.SentBytes > 40_000 })
	tw.wait(t, "TV en vivo", 30*time.Second, func(s runner.Status) bool { return s.State == runner.Live && s.SentBytes > 300_000 })
	// Detener la TV no debe cortar la radio.
	stopTV()
	waitRecv()
	before := rw.get().SentBytes
	time.Sleep(1500 * time.Millisecond)
	if s := rw.get(); s.State != runner.Live || s.SentBytes <= before {
		t.Errorf("la radio debía seguir al aire tras detener la TV: %+v", s)
	}
	stopRadio()
	checkVideo(t, probe, out, 854, 480)
	if _, data, _, _ := ice.Get("/radio"); len(data) < 40_000 {
		t.Errorf("la radio envió poco: %d bytes", len(data))
	}
}
