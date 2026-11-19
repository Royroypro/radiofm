package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// valid devuelve los defaults con lo que el usuario debe completar.
func valid() Config {
	c := Defaults()
	c.Radio.Password = "fuente"
	c.Radio.Input = Source{Kind: SourceTest}
	c.TV.Password = "clave"
	c.TV.Video = Source{Kind: SourceTest}
	c.TV.Audio = Source{Kind: SourceNone}
	return c
}

func TestDefaultsMatchPanel(t *testing.T) {
	c := Defaults()
	// Los mismos datos que muestra Admin → Streams y emisión (docs/streaming.md).
	if c.Radio.Host != "radioclub25peru.com" || c.Radio.Port != 8010 || c.Radio.Mount != "/radio" || c.Radio.User != "source" {
		t.Fatalf("radio por defecto inesperada: %+v", c.Radio)
	}
	if c.TV.RTMPPort != 1935 || c.TV.SRTPort != 8890 || c.TV.Path != "tv" || c.TV.User != "emisora" {
		t.Fatalf("tv por defecto inesperada: %+v", c.TV)
	}
	// Arranque: nada automático de fábrica, pero cerrar la ventana no corta la emisión.
	if c.Startup.WithWindows || c.Startup.AutoBroadcast || !c.Startup.Background {
		t.Fatalf("arranque por defecto inesperado: %+v", c.Startup)
	}
	if c.Radio.Password != "" || c.TV.Password != "" || c.Panel.Token != "" {
		t.Fatal("los defaults embebidos no deben traer claves")
	}
}

func TestNormalize(t *testing.T) {
	c := Defaults()
	c.Radio.Host = " https://radio.example/ "
	c.Radio.Mount = "radio"
	c.TV.Host = "rtmp://tv.example:1935/tv"
	c.TV.Path = "/tv/"
	c.Panel.URL = "https://radio.example/"
	c.Normalize()
	if c.Radio.Host != "radio.example" || c.Radio.Mount != "/radio" || c.TV.Host != "tv.example" || c.TV.Path != "tv" ||
		c.Panel.URL != "https://radio.example" {
		t.Fatalf("Normalize: %+v / %+v / %q", c.Radio, c.TV, c.Panel.URL)
	}
}

func fieldOf(err error) string {
	var fe *FieldError
	if errors.As(err, &fe) {
		return fe.Field
	}
	return ""
}

func TestValidateRadio(t *testing.T) {
	if err := valid().ValidateRadio(); err != nil {
		t.Fatalf("config válida rechazada: %v", err)
	}
	cases := []struct {
		field string
		mod   func(*Config)
	}{
		{"radio.password", func(c *Config) { c.Radio.Password = "" }},
		{"radio.host", func(c *Config) { c.Radio.Host = "" }},
		{"radio.mount", func(c *Config) { c.Radio.Mount = "/" }},
		{"radio.bitrate_kbps", func(c *Config) { c.Radio.BitrateKbps = 321 }},
		{"radio.bitrate_kbps", func(c *Config) { c.Radio.Codec = CodecHEAAC; c.Radio.BitrateKbps = 128 }},
		{"radio.codec", func(c *Config) { c.Radio.Codec = "wma" }},
		{"radio.input", func(c *Config) { c.Radio.Input = Source{Kind: SourceDShow} }},
		{"radio.input", func(c *Config) { c.Radio.Input = Source{Kind: SourceNone} }},
		{"radio.channels", func(c *Config) { c.Radio.Codec = CodecHEAACv2; c.Radio.BitrateKbps = 32; c.Radio.Channels = 1 }},
	}
	for _, tc := range cases {
		c := valid()
		tc.mod(&c)
		if got := fieldOf(c.ValidateRadio()); got != tc.field {
			t.Errorf("esperaba error en %s, obtuve %q", tc.field, got)
		}
	}
	// 320 kbps es el máximo y debe aceptarse en MP3, AAC y Opus.
	for _, codec := range []string{CodecMP3, CodecAAC, CodecOpus, CodecVorbis} {
		c := valid()
		c.Radio.Codec, c.Radio.BitrateKbps = codec, 320
		if err := c.ValidateRadio(); err != nil {
			t.Errorf("%s a 320 kbps: %v", codec, err)
		}
	}
}

func TestValidateTV(t *testing.T) {
	if err := valid().ValidateTV(); err != nil {
		t.Fatalf("config válida rechazada: %v", err)
	}
	cases := []struct {
		field string
		mod   func(*Config)
	}{
		{"tv.password", func(c *Config) { c.TV.Password = "" }},
		{"tv.protocol", func(c *Config) { c.TV.Protocol = "hls" }},
		{"tv.video_kbps", func(c *Config) { c.TV.VideoKbps = 6001 }},
		{"tv.audio_kbps", func(c *Config) { c.TV.AudioKbps = 400 }},
		{"tv.video", func(c *Config) { c.TV.Video = Source{Kind: SourceDShow} }},
		{"tv.resolution", func(c *Config) { c.TV.Width = 1279 }},
		{"tv.encoder", func(c *Config) { c.TV.Encoder = "x265" }},
		{"tv.password", func(c *Config) { c.TV.Protocol = ProtoSRT; c.TV.Password = "a:b" }},
		{"tv.video_url", func(c *Config) { c.TV.Video = Source{Kind: SourceURL, Name: "rtmp://x/y"} }},
		{"tv.video_url", func(c *Config) { c.TV.Video = Source{Kind: SourceURL, Name: "https://"} }},
		{"tv.video_url", func(c *Config) { c.TV.Video = Source{Kind: SourceRTSP, Name: "http://camara/stream"} }},
		{"tv.video_url", func(c *Config) { c.TV.Video = Source{Kind: SourceRTSP, Name: "rtsp://"} }},
		{"tv.audio_mode", func(c *Config) { c.TV.AudioMode = "mitad" }},
		{"tv.audio_mode", func(c *Config) { c.TV.OtherMixDB = 13 }},
		{"tv.audio_mode", func(c *Config) { c.TV.VideoMixDB = -31 }},
		{"tv.audio", func(c *Config) { c.TV.Audio = Source{Kind: SourceRadio}; c.Radio.Input = Source{Kind: SourceDShow} }},
		{"tv.audio", func(c *Config) { c.TV.Audio = Source{Kind: SourceIcecast}; c.Radio.Mount = "" }},
	}
	for _, tc := range cases {
		c := valid()
		tc.mod(&c)
		if got := fieldOf(c.ValidateTV()); got != tc.field {
			t.Errorf("esperaba error en %s, obtuve %q", tc.field, got)
		}
	}
	// Video desde https con su propio audio, y radio con los altavoces de la PC.
	c := valid()
	c.TV.Video = Source{Kind: SourceURL, Name: " https://cdn.example/live/index.m3u8 "}
	c.TV.Audio = Source{Kind: SourceRadio}
	c.TV.AudioMode = AudioModeVideo
	c.Radio.Input = Source{Kind: SourceDShow, Name: "Micrófono (USB)"}
	c.Normalize()
	if err := c.ValidateTV(); err != nil || c.TV.Video.Name != "https://cdn.example/live/index.m3u8" {
		t.Fatalf("video por URL: %v (%q)", err, c.TV.Video.Name)
	}
	c.TV.Audio = Source{Kind: SourceLoopback}
	c.Radio.Input = Source{Kind: SourceLoopback, Name: "Altavoces (Realtek(R) Audio)"}
	if err := c.ValidateTV(); err != nil {
		t.Fatalf("audio de altavoces en la TV: %v", err)
	}
	if err := c.ValidateRadio(); err != nil {
		t.Fatalf("audio de altavoces en la radio: %v", err)
	}
	c = valid()
	c.TV.VideoKbps = MaxVideoKbps
	c.TV.Video = Source{Kind: SourceScreen}
	if err := c.ValidateTV(); err != nil {
		t.Fatalf("6000 kbps con pantalla debe ser válido: %v", err)
	}
}

func TestSaveLoadEncryptsSecrets(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sub", "emisor.json")
	c := valid()
	c.Radio.Password, c.TV.Password, c.Panel.Token = "s3creto-radio", "s3creto-tv", "tok-123"
	c.Radio.BitrateKbps = 192
	if err := Save(path, c); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(path)
	for _, s := range []string{"s3creto-radio", "s3creto-tv", "tok-123"} {
		if strings.Contains(string(raw), s) {
			t.Fatalf("la clave %q quedó en texto plano en el archivo", s)
		}
	}
	got, err := Load("", path)
	if err != nil {
		t.Fatal(err)
	}
	if got.Radio.Password != "s3creto-radio" || got.TV.Password != "s3creto-tv" || got.Panel.Token != "tok-123" || got.Radio.BitrateKbps != 192 {
		t.Fatalf("Load no devolvió lo guardado: %+v", got)
	}
}

func TestLoadLayers(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "emisor.defaults.json")
	user := filepath.Join(dir, "emisor.json")
	os.WriteFile(exe, []byte(`{"radio":{"host":"otra.example","mount":"radio-barranca"}}`), 0o600)
	os.WriteFile(user, []byte(`{"radio":{"bitrate_kbps":96}}`), 0o600)
	c, err := Load(exe, user)
	if err != nil {
		t.Fatal(err)
	}
	if c.Radio.Host != "otra.example" || c.Radio.Mount != "/radio-barranca" || c.Radio.BitrateKbps != 96 || c.Radio.Port != 8010 {
		t.Fatalf("capas mal aplicadas: %+v", c.Radio)
	}
	if _, err := Load(filepath.Join(dir, "no-existe.json"), ""); err != nil {
		t.Fatalf("un archivo que no existe no es error: %v", err)
	}
	os.WriteFile(user, []byte(`{roto`), 0o600)
	if _, err := Load("", user); err == nil {
		t.Fatal("JSON roto debe dar error")
	}
}

func TestRTSPAndTVAudioMode(t *testing.T) {
	for raw, ok := range map[string]bool{
		"rtsp://192.168.1.64:554/Streaming/Channels/101": true,
		"rtsp://admin:cl@ve@camara.local/stream1":        true,
		"rtsps://camara.example/live":                    true,
		" rtsp://10.0.0.5/h264 ":                         true,
		"rtsp://":                                        false,
		"https://camara/stream":                          false,
		"camara:554":                                     false,
	} {
		if got := ValidRTSPURL(raw); got != ok {
			t.Errorf("ValidRTSPURL(%q) = %v", raw, got)
		}
	}

	// Cámara RTSP con la radio como respaldo y mezcla.
	c := valid()
	c.TV.Video = Source{Kind: SourceRTSP, Name: " rtsp://admin:1234@192.168.1.64/stream1 "}
	c.TV.Audio = Source{Kind: SourceRadio}
	c.TV.AudioMode, c.TV.VideoMixDB, c.TV.OtherMixDB = AudioModeBoth, 0, -6
	c.Normalize()
	if err := c.ValidateTV(); err != nil {
		t.Fatalf("cámara RTSP: %v", err)
	}
	if c.TV.Video.Name != "rtsp://admin:1234@192.168.1.64/stream1" {
		t.Errorf("no se recortaron los espacios: %q", c.TV.Video.Name)
	}
	if got := c.TV.EffectiveAudioMode(); got != AudioModeBoth {
		t.Errorf("modo %q, esperaba both", got)
	}
	if URLPassword(c.TV.Video.Name) != "1234" || URLPassword("rtsp://camara/x") != "" {
		t.Error("URLPassword no devuelve la clave de la dirección")
	}

	// Con cámara USB, pantalla o barras no hay audio de video: siempre el otro.
	c.TV.Video = Source{Kind: SourceScreen}
	if got := c.TV.EffectiveAudioMode(); got != AudioModeOther {
		t.Errorf("con pantalla el modo debe ser other, es %q", got)
	}

	// Radio desde el servidor: la dirección de oyente de Icecast.
	c.TV.Audio = Source{Kind: SourceIcecast}
	if err := c.ValidateTV(); err != nil {
		t.Fatalf("radio desde el servidor: %v", err)
	}
	if got := c.Radio.ListenURL(); got != "http://radioclub25peru.com:8010/radio" {
		t.Errorf("ListenURL = %q", got)
	}
}

// Una configuración anterior a 1.7.0 con «el audio que trae el video» pasa a
// modo «solo el audio del video» con la radio de respaldo.
func TestMigrateFromVideo(t *testing.T) {
	dir := t.TempDir()
	user := filepath.Join(dir, "emisor.json")
	os.WriteFile(user, []byte(`{"tv":{"video":{"kind":"url","name":"https://x/v.mp4"},"audio":{"kind":"fromvideo","name":""}}}`), 0o600)
	c, err := Load("", user)
	if err != nil {
		t.Fatal(err)
	}
	if c.TV.AudioMode != AudioModeVideo || c.TV.Audio.Kind != SourceRadio {
		t.Fatalf("migración: modo %q, audio %+v", c.TV.AudioMode, c.TV.Audio)
	}
	// Sin modo guardado (configuración vieja con otro audio) queda «other».
	os.WriteFile(user, []byte(`{"tv":{"audio":{"kind":"none"}}}`), 0o600)
	c, _ = Load("", user)
	if c.TV.AudioMode != AudioModeOther {
		t.Fatalf("modo por defecto %q", c.TV.AudioMode)
	}
}
