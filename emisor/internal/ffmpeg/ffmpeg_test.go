package ffmpeg

import (
	"bufio"
	"fmt"
	"math"
	"strings"
	"testing"

	"radioclub25/emisor/internal/config"
)

func radio() config.Radio {
	r := config.Defaults().Radio
	r.Password = "p@ss:w/rd"
	r.Input = config.Source{Kind: config.SourceDShow, Name: "Micrófono (USB PnP Audio Device)"}
	return r
}

func tv() config.TV {
	t := config.Defaults().TV
	t.Password = "Clave123"
	t.Video = config.Source{Kind: config.SourceDShow, Name: "Iriun Webcam"}
	t.Audio = config.Source{Kind: config.SourceDShow, Name: "Line 1/2 (M-Track)"}
	return t
}

func join(a []string) string { return strings.Join(a, " ") }

// after devuelve el argumento que sigue a flag (o "").
func after(args []string, flag string) string {
	for i := 0; i < len(args)-1; i++ {
		if args[i] == flag {
			return args[i+1]
		}
	}
	return ""
}

func TestRadioArgsCodecs(t *testing.T) {
	cases := []struct {
		codec, encoder, format, ctype string
	}{
		{config.CodecMP3, "libmp3lame", "mp3", "audio/mpeg"},
		{config.CodecAAC, "aac", "adts", "audio/aac"},
		{config.CodecHEAAC, "libfdk_aac", "adts", "audio/aac"},
		{config.CodecHEAACv2, "libfdk_aac", "adts", "audio/aac"},
		{config.CodecOpus, "libopus", "ogg", "audio/ogg"},
		{config.CodecVorbis, "libvorbis", "ogg", "audio/ogg"},
	}
	for _, tc := range cases {
		r := radio()
		r.Codec, r.BitrateKbps = tc.codec, 320
		a := RadioArgs(r)
		if after(a, "-c:a") != tc.encoder || after(a, "-f") == "" || after(a, "-content_type") != tc.ctype || after(a, "-b:a") != "320k" {
			t.Errorf("%s: %s", tc.codec, join(a))
		}
		// -f aparece dos veces (entrada dshow y salida): la de salida es la última.
		if a[len(a)-1] != "icecast://source:p%40ss%3Aw%2Frd@radioclub25peru.com:8010/radio" {
			t.Errorf("%s: URL de Icecast inesperada: %s", tc.codec, a[len(a)-1])
		}
		lastF := ""
		for i := range a[:len(a)-1] {
			if a[i] == "-f" {
				lastF = a[i+1]
			}
		}
		if lastF != tc.format {
			t.Errorf("%s: formato %q, esperaba %q", tc.codec, lastF, tc.format)
		}
	}
	r := radio()
	r.Codec = config.CodecHEAACv2
	if after(RadioArgs(r), "-profile:a") != "aac_he_v2" {
		t.Error("AAC+ v2 debe usar el perfil aac_he_v2")
	}
}

func TestRadioArgsDetails(t *testing.T) {
	r := radio()
	r.Codec, r.SampleRate, r.Channels, r.GainDB = config.CodecOpus, 44100, 1, -3
	a := RadioArgs(r)
	if after(a, "-ar") != "48000" || after(a, "-ac") != "1" {
		t.Fatalf("Opus debe ir a 48 kHz y respetar mono: %s", join(a))
	}
	fc := after(a, "-filter_complex")
	for _, want := range []string{"volume@ganancia=volume=-3.0dB", "channel_layouts=mono", "astats=", "file='pipe\\:1'", "asetnsamples=n=4800"} {
		if !strings.Contains(fc, want) {
			t.Errorf("filtro sin %q: %s", want, fc)
		}
	}
	if after(a, "-i") != "audio=Micrófono (USB PnP Audio Device)" || after(a, "-ice_name") != "Radio Club 25" {
		t.Errorf("entrada o metadatos: %s", join(a))
	}
	r.Input = config.Source{Kind: config.SourceLoopback, Addr: "tcp://127.0.0.1:6000"}
	if a := RadioArgs(r); after(a, "-i") != "tcp://127.0.0.1:6000" || !strings.Contains(join(a), "-f s16le -ar 48000 -ac 2") {
		t.Errorf("altavoces de la PC: %s", join(a))
	}
	r.Input = config.Source{Kind: config.SourceTest}
	if !strings.HasPrefix(after(RadioArgs(r), "-i"), "sine=") {
		t.Error("la señal de prueba debe ser un tono")
	}
}

func TestTVArgs(t *testing.T) {
	v := tv()
	v.VideoKbps = 6000
	a := TVArgs(v)
	s := join(a)
	for _, want := range []string{"video=Iriun Webcam", "audio=Line 1/2 (M-Track)", "-c:v libx264", "-b:v 6000k",
		"-maxrate 6000k", "-bufsize 12000k", "-g 60", "-c:a aac", "-b:a 128k", "-map [vout] -map [aout]", "-f flv"} {
		if !strings.Contains(s, want) {
			t.Errorf("falta %q en %s", want, s)
		}
	}
	if a[len(a)-1] != "rtmp://radioclub25peru.com:1935/tv?pass=Clave123&user=emisora" {
		t.Errorf("URL RTMP: %s", a[len(a)-1])
	}

	v.Protocol = config.ProtoSRT
	a = TVArgs(v)
	if a[len(a)-1] != "srt://radioclub25peru.com:8890?streamid=publish:tv:emisora:Clave123&pkt_size=1316" || a[len(a)-2] != "mpegts" {
		t.Errorf("URL SRT: %s", join(a[len(a)-3:]))
	}

	v.Video, v.Audio, v.Encoder = config.Source{Kind: config.SourceScreen}, config.Source{Kind: config.SourceNone}, "h264_nvenc"
	v.GainDB = 6
	s = join(TVArgs(v))
	for _, want := range []string{"-f gdigrab", "-i desktop", "anullsrc=", "-c:v h264_nvenc"} {
		if !strings.Contains(s, want) {
			t.Errorf("falta %q en %s", want, s)
		}
	}
	if !strings.Contains(s, "volume@ganancia=volume=0.0dB") {
		t.Error("el silencio no lleva ganancia (0 dB)")
	}
	// Video desde https con su propio audio: una sola entrada y el audio sale de 0:a.
	v.Video = config.Source{Kind: config.SourceURL, Name: "https://cdn.example/live/index.m3u8"}
	v.AudioMode = config.AudioModeVideo
	a = TVArgs(v)
	s = join(a)
	if strings.Count(s, " -i ") != 1 || !strings.Contains(s, "-reconnect 1") || !strings.Contains(s, "-re ") ||
		after(a, "-i") != "https://cdn.example/live/index.m3u8" || !strings.Contains(after(a, "-filter_complex"), "[0:a]aresample") {
		t.Errorf("video por URL: %s", s)
	}
	// Video por URL con el audio de los altavoces de la PC (el del video no sale).
	v.AudioMode = config.AudioModeOther
	v.Audio = config.Source{Kind: config.SourceLoopback, Addr: "tcp://127.0.0.1:5000"}
	s = join(TVArgs(v))
	if !strings.Contains(s, "-f s16le -ar 48000 -ac 2 -i tcp://127.0.0.1:5000") || !strings.Contains(s, "[1:a]aresample") {
		t.Errorf("URL + altavoces: %s", s)
	}
	v.Video = config.Source{Kind: config.SourceTest}
	if !strings.Contains(join(TVArgs(v)), "testsrc2=size=1280x720:rate=30") {
		t.Error("la señal de prueba de video debe ser testsrc2 al tamaño de salida")
	}
}

func TestRedact(t *testing.T) {
	r := radio()
	cmd := join(RadioArgs(r))
	if got := Redact(cmd, r.Password); strings.Contains(got, "p%40ss") || strings.Contains(got, r.Password) {
		t.Fatalf("la clave sigue visible: %s", got)
	}
	v := tv()
	target, _ := TVURL(v)
	if strings.Contains(Redact(target, v.Password), "Clave123") {
		t.Fatal("la clave de TV sigue visible")
	}
}

const dshowNew = `[dshow @ 000001] "Iriun Webcam" (video)
[dshow @ 000001]   Alternative name "@device_pnp_\\?\root#devgen"
[dshow @ 000001] "LSVCam" (none)
[dshow @ 000001] "Micrófono (USB PnP Audio Device)" (audio)
[dshow @ 000001]   Alternative name "@device_cm_{33D9}\wave_{42BB}"
[dshow @ 000001] "Line 1/2 (9- M-Audio M-Track Solo and Duo)" (audio)
[dshow @ 000001] "Line 1/2 (9- M-Audio M-Track Solo and Duo)" (audio)
dummy: Immediate exit requested`

const dshowOld = `[dshow @ 0000] DirectShow video devices (some may be both video and audio devices)
[dshow @ 0000]  "USB2.0 HD UVC WebCam"
[dshow @ 0000]     Alternative name "@device_pnp_\\?\usb"
[dshow @ 0000] DirectShow audio devices
[dshow @ 0000]  "Micrófono (Realtek(R) Audio)"
[dshow @ 0000]     Alternative name "@device_cm_{33D9}"
`

func TestParseDevices(t *testing.T) {
	got := ParseDevices(dshowNew)
	want := []Device{{"Iriun Webcam", "video"}, {"Micrófono (USB PnP Audio Device)", "audio"},
		{"Line 1/2 (9- M-Audio M-Track Solo and Duo)", "audio"}}
	if len(got) != len(want) {
		t.Fatalf("ParseDevices = %+v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("[%d] = %+v, esperaba %+v", i, got[i], want[i])
		}
	}
	old := ParseDevices(dshowOld)
	if len(old) != 2 || old[0] != (Device{"USB2.0 HD UVC WebCam", "video"}) || old[1] != (Device{"Micrófono (Realtek(R) Audio)", "audio"}) {
		t.Fatalf("formato antiguo: %+v", old)
	}
}

func TestParseEncoders(t *testing.T) {
	out := `Encoders:
 V..... = Video
 ------
 V....D libx264              libx264 H.264
 V....D h264_nvenc           NVIDIA NVENC H.264 encoder (codec h264)
 A....D aac                  AAC (Advanced Audio Coding)
 A....D libmp3lame           libmp3lame MP3`
	e := ParseEncoders(out)
	if !e["libx264"] || !e["h264_nvenc"] || !e["aac"] || !e["libmp3lame"] || e["libfdk_aac"] || e["Video"] {
		t.Fatalf("ParseEncoders = %v", e)
	}
}

func TestProgressParser(t *testing.T) {
	in := "frame=150\nfps=30.00\nstream_0_0_q=23.0\nbitrate=2510.3kbits/s\ntotal_size=1572864\nout_time_us=5000000\n" +
		"out_time=00:00:05.000000\ndup_frames=0\ndrop_frames=2\nspeed=1.01x\nprogress=continue\n" +
		"[tcp @ 0001] Connection to tcp://host:1935 failed: Connection refused\n" +
		"bitrate=N/A\nout_time_us=N/A\nprogress=end\n"
	var p ProgressParser
	var got []Progress
	var logs []string
	sc := bufio.NewScanner(strings.NewReader(in))
	sc.Split(SplitLines)
	for sc.Scan() {
		pr, done, isLog := p.Feed(sc.Text())
		if done {
			got = append(got, pr)
		}
		if isLog {
			logs = append(logs, sc.Text())
		}
	}
	if len(got) != 2 {
		t.Fatalf("bloques = %d", len(got))
	}
	g := got[0]
	if g.Frame != 150 || g.FPS != 30 || g.BitrateKbps != 2510.3 || g.TotalBytes != 1572864 || g.OutTimeSec != 5 || g.DropFrames != 2 ||
		g.Speed != 1.01 || g.End {
		t.Errorf("primer bloque: %+v", g)
	}
	if !got[1].End || math.Abs(got[1].BitrateKbps-2516.58) > 0.01 {
		t.Errorf("segundo bloque: %+v", got[1])
	}
	if len(logs) != 1 || Hint(logs[0]) == "" {
		t.Errorf("mensajes = %q", logs)
	}
}

func TestLevelParser(t *testing.T) {
	in := "frame:0    pts:0       pts_time:0\r\nlavfi.astats.1.Peak_level=-31.5\nlavfi.astats.1.RMS_level=-46.2\n" +
		"lavfi.astats.2.Peak_level=-inf\nlavfi.astats.2.RMS_level=-inf\nframe:1    pts:4410    pts_time:0.1\n" +
		"lavfi.astats.1.Peak_level=0.000000\nlavfi.astats.1.RMS_level=-3.01\n"
	var p LevelParser
	var got [][]Level
	sc := bufio.NewScanner(strings.NewReader(in))
	sc.Split(SplitLines)
	for sc.Scan() {
		if l, ok := p.Feed(sc.Text()); ok {
			got = append(got, l)
		}
	}
	if l, ok := p.Flush(); ok {
		got = append(got, l)
	}
	if len(got) != 2 || len(got[0]) != 2 || len(got[1]) != 1 {
		t.Fatalf("bloques: %+v", got)
	}
	if got[0][0] != (Level{-31.5, -46.2}) || got[0][1] != (Level{Silence, Silence}) || got[1][0] != (Level{0, -3.01}) {
		t.Fatalf("niveles: %+v", got)
	}
}

func TestHint(t *testing.T) {
	cases := map[string]string{
		"[http @ 0x1] HTTP error 401 Unauthorized":                          "401",
		"[tcp @ 0x2] Connection to tcp://x:8010 failed: Connection refused": "rechazada",
		"[dshow @ 0x3] Could not find audio only device with name [Mic]":    "micrófono",
		"[aost#0:0 @ 0x4] Unknown encoder 'libfdk_aac'":                     "AAC+",
		"av_interleaved_write_frame(): I/O error":                           "cortó",
	}
	for msg, want := range cases {
		if h := Hint(msg); !strings.Contains(h, want) {
			t.Errorf("Hint(%q) = %q, debía contener %q", msg, h, want)
		}
	}
	if Hint("Guessed Channel Layout for Input Stream") != "" {
		t.Error("un aviso inofensivo no lleva explicación")
	}
}

func TestEnhance(t *testing.T) {
	r := radio()
	off := after(RadioArgs(r), "-filter_complex")
	r.Enhance = true
	on := after(RadioArgs(r), "-filter_complex")
	for _, f := range []string{"highpass@mejora", "bass@mejora", "treble@mejora", "acompressor@mejora", "alimiter@mejora"} {
		if !strings.Contains(off, f) || !strings.Contains(on, f) {
			t.Fatalf("la cadena MEJORA debe estar siempre (para cambiarla al aire): falta %s", f)
		}
	}
	// Apagada = neutra; encendida = compresor y ganancia. Va antes del medidor.
	if !strings.Contains(off, "ratio=1:") || !strings.Contains(off, "makeup=1,") || !strings.Contains(on, "ratio=3:") ||
		!strings.Contains(on, "makeup=2.5,") || strings.Index(on, "alimiter@mejora") > strings.Index(on, "asplit") {
		t.Fatalf("cadena MEJORA:\napagada: %s\nencendida: %s", off, on)
	}
	cmds := EnhanceCommands(true)
	if len(cmds) != 6 || cmds[0] != "cacompressor@mejora -1 makeup 2.5\n" {
		t.Fatalf("comandos: %q", cmds)
	}
	for _, c := range append(cmds, EnhanceCommands(false)...) {
		if !strings.HasPrefix(c, "c") || !strings.HasSuffix(c, "\n") || strings.Count(c, " ") != 3 {
			t.Fatalf("comando mal formado: %q", c)
		}
	}
	if EnhanceCommands(false)[0] != "cacompressor@mejora -1 makeup 1\n" {
		t.Fatalf("apagar: %q", EnhanceCommands(false))
	}
	if a := TVArgs(tv()); !strings.Contains(after(a, "-filter_complex"), "acompressor@mejora") {
		t.Fatal("la TV también lleva MEJORA")
	}
}

func TestEQ(t *testing.T) {
	r := radio()
	fc := after(RadioArgs(r), "-filter_complex")
	// Apagado: las 10 bandas están (para cambiarlas al aire) pero en 0 dB, y hay limitador de seguridad.
	for i, f := range config.EQBands {
		if !strings.Contains(fc, fmt.Sprintf("equalizer@eq%d=f=%d:t=o:w=1:g=0", i, f)) {
			t.Fatalf("falta la banda %d Hz en 0 dB: %s", f, fc)
		}
	}
	if !strings.Contains(fc, "alimiter@seguro") || strings.Index(fc, "equalizer@eq9") > strings.Index(fc, "acompressor@mejora") {
		t.Fatalf("orden: ecualizador → MEJORA → limitador: %s", fc)
	}
	r.EQ = config.EQ{Enabled: true, Gains: [10]float64{6, 5, 3, 0, -1.5, -1, 0, 2, 4, 5}}
	if fc := after(RadioArgs(r), "-filter_complex"); !strings.Contains(fc, "equalizer@eq4=f=500:t=o:w=1:g=-1.5") {
		t.Fatalf("ganancias: %s", fc)
	}
	r.EQ.Enabled = false
	if fc := after(RadioArgs(r), "-filter_complex"); strings.Contains(fc, "g=6") {
		t.Fatal("ecualizador apagado = todas las bandas en 0")
	}
	// Al aire solo se mandan las bandas que cambian.
	from := [10]float64{0, 0, 0, 0, 0, 0, 0, 0, 0, 0}
	to := [10]float64{3, 0, 0, 0, 0, 0, 0, 0, 0, -2.5}
	if got := EQCommands(from, to); len(got) != 2 || got[0] != "cequalizer@eq0 -1 g 3\n" || got[1] != "cequalizer@eq9 -1 g -2.5\n" {
		t.Fatalf("EQCommands = %q", got)
	}
	if len(EQCommands(to, to)) != 0 {
		t.Fatal("sin cambios no hay comandos")
	}
}

func TestGainCommand(t *testing.T) {
	if got := GainCommand(-4.5); len(got) != 1 || got[0] != "cvolume@ganancia -1 volume -4.5dB\n" {
		t.Fatalf("GainCommand = %q", got)
	}
	if !strings.Contains(after(RadioArgs(radio()), "-filter_complex"), "volume@ganancia=volume=0.0dB") {
		t.Fatal("la ganancia debe estar siempre en el filtro (para cambiarla al aire)")
	}
}

func TestTVArgsRTSP(t *testing.T) {
	v := tv()
	v.Video = config.Source{Kind: config.SourceRTSP, Name: "rtsp://admin:1234@192.168.1.64:554/stream1"}
	v.Audio = config.Source{Kind: config.SourceLoopback, Addr: "tcp://127.0.0.1:5000"}
	a := TVArgs(v)
	s := join(a)
	if after(a, "-rtsp_transport") != "tcp" || after(a, "-timeout") != "15000000" ||
		after(a, "-i") != "rtsp://admin:1234@192.168.1.64:554/stream1" {
		t.Errorf("entrada RTSP: %s", s)
	}
	// RTSP es un directo: no lleva -re (solo la segunda entrada, el bus, no lo usa tampoco).
	if strings.Contains(s, "-re -rtsp") || strings.Contains(s, "-reconnect") {
		t.Errorf("RTSP no debe llevar -re ni opciones http: %s", s)
	}
	if !strings.Contains(s, "[1:a]aresample") {
		t.Errorf("modo other: el audio debe salir de la segunda entrada: %s", s)
	}
	// La clave de la cámara no aparece en el registro.
	if r := Redact(s, config.URLPassword(v.Video.Name)); strings.Contains(r, "1234") {
		t.Errorf("la clave de la cámara quedó en el registro: %s", r)
	}
}

func TestTVArgsAudioModes(t *testing.T) {
	v := tv()
	v.Video = config.Source{Kind: config.SourceRTSP, Name: "rtsp://camara/stream1"}
	v.Audio = config.Source{Kind: config.SourceLoopback, Addr: "tcp://127.0.0.1:5000"}

	// Solo el audio del video: una sola entrada.
	v.AudioMode = config.AudioModeVideo
	s := join(TVArgs(v))
	if strings.Count(s, " -i ") != 1 || strings.Contains(s, "tcp://127.0.0.1:5000") || !strings.Contains(s, "[0:a]aresample") {
		t.Errorf("modo video: %s", s)
	}

	// Los dos mezclados, cada uno con su volumen, sin que amix baje el nivel.
	v.AudioMode, v.VideoMixDB, v.OtherMixDB = config.AudioModeBoth, -3, -6.5
	a := TVArgs(v)
	s = join(a)
	f := after(a, "-filter_complex")
	if strings.Count(s, " -i ") != 2 || !strings.Contains(s, "-i tcp://127.0.0.1:5000") {
		t.Errorf("modo both: faltan las dos entradas: %s", s)
	}
	for _, want := range []string{"[0:a]aresample=44100:async=1", "volume=-3dB[avid]", "[1:a]aresample=44100:async=1",
		"volume=-6.5dB[aotro]", "[avid][aotro]amix=inputs=2:duration=longest:dropout_transition=0:normalize=0[amix]", "[amix]aresample"} {
		if !strings.Contains(f, want) {
			t.Errorf("modo both: falta %q en %s", want, f)
		}
	}

	// Con video que no viene de la red, el modo se ignora (no hay audio de video).
	v.Video = config.Source{Kind: config.SourceScreen}
	s = join(TVArgs(v))
	if strings.Contains(s, "amix") || !strings.Contains(s, "[1:a]aresample") {
		t.Errorf("pantalla: debe usar solo el otro audio: %s", s)
	}
}

func TestProbe(t *testing.T) {
	a := ProbeArgs(config.Source{Kind: config.SourceURL, Name: "https://x/v.mp4"})
	if strings.Contains(join(a), " -re ") || after(a, "-i") != "https://x/v.mp4" || after(a, "-loglevel") != "info" {
		t.Errorf("probe http: %s", join(a))
	}
	a = ProbeArgs(config.Source{Kind: config.SourceRTSP, Name: "rtsp://c/s"})
	if after(a, "-rtsp_transport") != "tcp" || after(a, "-i") != "rtsp://c/s" {
		t.Errorf("probe rtsp: %s", join(a))
	}
	withAudio := `Input #0, rtsp, from 'rtsp://c/s':
  Duration: N/A, start: 0.000000, bitrate: N/A
  Stream #0:0: Video: h264 (Main), yuv420p(progressive), 1920x1080, 25 fps
  Stream #0:1: Audio: pcm_alaw, 8000 Hz, mono, s16, 64 kb/s
At least one output file must be specified`
	noAudio := `Input #0, mov,mp4,m4a,3gp,3g2,mj2, from 'https://x/fondo.mp4':
  Stream #0:0[0x1](und): Video: h264 (High) (avc1 / 0x31637661), yuv420p, 1280x720, 30 fps (default)
At least one output file must be specified`
	hlsAudio := `  Stream #0:1(und): Audio: aac (LC) ([15][0][0][0] / 0x000F), 44100 Hz, stereo, fltp`
	if !HasAudioStream(withAudio) || HasAudioStream(noAudio) || !HasAudioStream(hlsAudio) {
		t.Error("HasAudioStream no distingue un video con y sin audio")
	}
	if !HasVideoStream(noAudio) || HasVideoStream("Connection refused") {
		t.Error("HasVideoStream")
	}
}

func TestRTSPHints(t *testing.T) {
	h := Hint("[rtsp @ 000001] method DESCRIBE failed: 401 Unauthorized")
	if !strings.Contains(h, "cámara RTSP rechazó") || !IsAuthHint(h) {
		t.Errorf("401 de la cámara: %q", h)
	}
	if h := Hint("method DESCRIBE failed: 404 Not Found"); !strings.Contains(h, "no tiene esa ruta") {
		t.Errorf("404 de la cámara: %q", h)
	}
	if h := Hint("Server returned 401 Unauthorized"); IsAuthHint(h) == false || strings.Contains(h, "cámara") {
		t.Errorf("el 401 del servidor no debe confundirse con la cámara: %q", h)
	}
}
