package integration

import (
	"net/http"
	"net/http/httptest"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"radioclub25/emisor/internal/config"
	"radioclub25/emisor/internal/ffmpeg"
)

// needFFmpeg es como need, pero solo pide ffmpeg (sin ffprobe).
func needFFmpeg(t *testing.T) string {
	t.Helper()
	if testing.Short() {
		t.Skip("prueba con ffmpeg real (sin -short)")
	}
	ff, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("no hay ffmpeg en el PATH")
	}
	return ff
}

// TestTVAudioModesReal comprueba con el ffmpeg real (1.7.0):
//   - que se distingue un video con audio de uno sin audio (video de fondo);
//   - que los tres modos de audio de la TV arman un comando que ffmpeg acepta y
//     codifica (incluida la mezcla con amix), con el video leído por http.
func TestTVAudioModesReal(t *testing.T) {
	ff := needFFmpeg(t)
	dir := t.TempDir()
	gen := func(name string, withAudio bool) {
		args := []string{"-hide_banner", "-loglevel", "error", "-f", "lavfi", "-i", "testsrc2=size=320x240:rate=25"}
		if withAudio {
			args = append(args, "-f", "lavfi", "-i", "sine=frequency=440:sample_rate=44100")
		}
		args = append(args, "-t", "4", "-c:v", "libx264", "-pix_fmt", "yuv420p")
		if withAudio {
			args = append(args, "-c:a", "aac")
		}
		args = append(args, filepath.Join(dir, name))
		if out, err := exec.Command(ff, args...).CombinedOutput(); err != nil {
			t.Fatalf("no se pudo crear %s: %v\n%s", name, err, out)
		}
	}
	gen("con-audio.mp4", true)
	gen("fondo.mp4", false)
	srv := httptest.NewServer(http.FileServer(http.Dir(dir)))
	defer srv.Close()

	probe := func(name string) (hasVideo, hasAudio bool) {
		src := config.Source{Kind: config.SourceURL, Name: srv.URL + "/" + name}
		out, _ := exec.Command(ff, ffmpeg.ProbeArgs(src)...).CombinedOutput()
		return ffmpeg.HasVideoStream(string(out)), ffmpeg.HasAudioStream(string(out))
	}
	if v, a := probe("con-audio.mp4"); !v || !a {
		t.Errorf("video con audio: video=%v audio=%v", v, a)
	}
	if v, a := probe("fondo.mp4"); !v || a {
		t.Errorf("video de fondo sin audio: video=%v audio=%v (no debe tener audio)", v, a)
	}

	// Cada modo, codificado de verdad durante 2 s hacia la nada (-f null).
	run := func(name, mode string) {
		t.Helper()
		tv := config.Defaults().TV
		tv.Password = "x"
		tv.Protocol = config.ProtoSRT // salida mpegts: se cambia por -f null
		tv.Width, tv.Height, tv.FPS, tv.VideoKbps = 320, 240, 25, 500
		tv.Encoder = "libx264"
		tv.Video = config.Source{Kind: config.SourceURL, Name: srv.URL + "/" + name}
		tv.Audio = config.Source{Kind: config.SourceTest} // «el otro audio»: un tono
		tv.AudioMode, tv.VideoMixDB, tv.OtherMixDB = mode, -3, -6
		args := ffmpeg.TVArgs(tv)
		args = append(args[:len(args)-3], "-t", "2", "-f", "null", "-")
		cmd := exec.Command(ff, args...)
		cmd.Stdin = strings.NewReader("") // sin «q»: termina con -t
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Errorf("%s / modo %s: ffmpeg falló: %v\n%s", name, mode, err, tail(string(out), 1500))
		}
	}
	for _, mode := range []string{config.AudioModeOther, config.AudioModeVideo, config.AudioModeBoth} {
		run("con-audio.mp4", mode)
	}
	// El video de fondo solo funciona en «other»: es lo que elige internal/ui cuando
	// el video no trae audio (los otros modos pedirían una pista que no existe).
	run("fondo.mp4", config.AudioModeOther)
}

func tail(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return "…" + s[len(s)-n:]
}
