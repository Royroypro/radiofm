package integration

import (
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"radioclub25/emisor/internal/config"
	"radioclub25/emisor/internal/ffmpeg"
	"radioclub25/emisor/internal/runner"
)

// makeClip genera con ffmpeg un video de prueba (barras + tono) en la carpeta dir:
// clip.mp4 y, en hls/, una lista HLS (index.m3u8 + segmentos).
func makeClip(t *testing.T, ff, dir string) {
	t.Helper()
	run := func(args ...string) {
		out, err := exec.Command(ff, args...).CombinedOutput()
		if err != nil {
			t.Fatalf("ffmpeg %v: %v\n%s", args, err, out)
		}
	}
	src := []string{"-hide_banner", "-loglevel", "error", "-y", "-f", "lavfi", "-i", "testsrc2=size=640x360:rate=25",
		"-f", "lavfi", "-i", "sine=frequency=440:sample_rate=44100", "-t", "20", "-c:v", "libx264", "-preset", "ultrafast",
		"-g", "50", "-c:a", "aac"}
	run(append(src, filepath.Join(dir, "clip.mp4"))...)
	os.MkdirAll(filepath.Join(dir, "hls"), 0o755)
	run(append(src, "-f", "hls", "-hls_time", "2", "-hls_playlist_type", "vod", filepath.Join(dir, "hls", "index.m3u8"))...)
}

// TestTVFromURL envía a la TV un video leído por http y por https (MP4 y HLS),
// con el audio que trae el propio video.
func TestTVFromURL(t *testing.T) {
	ff, probe := need(t)
	dir := t.TempDir()
	makeClip(t, ff, dir)
	plain := httptest.NewServer(http.FileServer(http.Dir(dir)))
	defer plain.Close()
	tls := httptest.NewTLSServer(http.FileServer(http.Dir(dir))) // certificado propio: ffmpeg no lo verifica por defecto
	defer tls.Close()

	for _, tc := range []struct{ name, url string }{
		{"http-mp4", plain.URL + "/clip.mp4"},
		{"https-hls", tls.URL + "/hls/index.m3u8"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, out, waitRecv := receiver(t, ff, config.ProtoRTMP)
			c.Video = config.Source{Kind: config.SourceURL, Name: tc.url}
			c.Audio = config.Source{Kind: config.SourceFromVideo}
			w, stop := emitTV(t, ff, c)
			s := w.wait(t, "TV en vivo desde "+tc.name, 30*time.Second, func(s runner.Status) bool {
				return s.State == runner.Live && s.SentBytes > 300_000 && len(s.Levels) == 2
			})
			stop()
			waitRecv()
			checkVideo(t, probe, out, 1280, 720)
			if s.Levels[0].PeakDB < -30 {
				t.Errorf("el audio del video no llegó (nivel %.1f dB)", s.Levels[0].PeakDB)
			}
			t.Logf("%s: %.0f kbps, nivel %.1f dB", tc.name, s.BitrateKbps, s.Levels[0].PeakDB)
		})
	}

	// Una dirección que no existe explica el 404.
	cfg := config.Defaults().TV
	cfg.Host, cfg.Password, cfg.RTMPPort = "127.0.0.1", "x", freePort(t, "tcp")
	cfg.Video = config.Source{Kind: config.SourceURL, Name: plain.URL + "/no-existe.mp4"}
	cfg.Audio = config.Source{Kind: config.SourceNone}
	w, stop := emitTV(t, ff, cfg)
	s := w.wait(t, "reconectando con explicación", 20*time.Second, func(s runner.Status) bool { return s.State == runner.Reconnecting })
	stop()
	if want := ffmpeg.Hint("Server returned 404 Not Found"); s.Message != want {
		t.Errorf("motivo %q, esperaba %q", s.Message, want)
	}
}
