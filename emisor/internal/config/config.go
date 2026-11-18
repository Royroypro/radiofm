// Package config define los ajustes del emisor, sus valores por defecto y cómo
// se guardan en disco.
//
// Orden de carga (cada capa pisa a la anterior):
//  1. defaults.json embebido: los datos de ingesta públicos que muestra el panel
//     (Admin → Streams y emisión): host, puertos, montaje, usuario y ruta. Sin claves.
//  2. emisor.defaults.json junto al .exe (opcional): para empaquetar el emisor de
//     otra frecuencia u otro servidor sin recompilar.
//  3. %APPDATA%\Club25 Emisor\emisor.json: lo que guardó la persona.
//
// Las claves (fuente de Icecast, clave de la TV, token del panel) nunca están en
// el código: se escriben en la interfaz, se traen del panel o se pegan, y se
// guardan cifradas (paquete secret). Este paquete no abre dispositivos ni red.
package config

import (
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"radioclub25/emisor/internal/secret"
)

//go:embed defaults.json
var defaultsJSON []byte

// Límites pedidos por la radio.
const (
	MaxAudioKbps = 320
	MaxVideoKbps = 6000
)

// Tipos de fuente.
const (
	SourceDShow     = "dshow"     // dispositivo de entrada de Windows (micrófono, línea, cámara, capturadora)
	SourceLoopback  = "loopback"  // lo que suena en un adaptador de reproducción (altavoces); Name "" = el predeterminado
	SourceScreen    = "screen"    // captura de pantalla (solo video)
	SourceURL       = "url"       // video (con o sin audio) desde una dirección http:// o https:// (solo video)
	SourceFromVideo = "fromvideo" // el audio que trae la fuente de video (URL); solo para el audio de la TV
	SourceTest      = "test"      // señal de prueba: tono o barras de color
	SourceNone      = "none"      // sin audio: silencio (solo para el audio de la TV)
)

// Códecs de audio de la radio.
const (
	CodecMP3     = "mp3"
	CodecAAC     = "aac"     // AAC-LC
	CodecHEAAC   = "heaac"   // AAC+ (HE-AAC v1); necesita libfdk_aac
	CodecHEAACv2 = "heaacv2" // AAC+ v2 (HE-AAC v2, estéreo paramétrico); necesita libfdk_aac
	CodecOpus    = "opus"    // Opus en Ogg
	CodecVorbis  = "vorbis"  // Vorbis en Ogg
)

// Protocolos de la TV.
const (
	ProtoRTMP = "rtmp"
	ProtoSRT  = "srt"
)

// Source es una entrada de audio o video.
type Source struct {
	Kind string `json:"kind"` // SourceDShow, SourceScreen, SourceTest o SourceNone
	Name string `json:"name"` // dispositivo DirectShow, adaptador de reproducción o URL, según Kind
	// Addr es la dirección local donde el emisor sirve la captura de altavoces
	// (la llena internal/ui en cada intento; no se guarda).
	Addr string `json:"-"`
}

// Radio es la salida de solo audio hacia Icecast (como BUTT).
type Radio struct {
	Enabled     bool    `json:"enabled"`
	Host        string  `json:"host"`
	Port        int     `json:"port"`
	Mount       string  `json:"mount"`
	User        string  `json:"user"`
	Password    string  `json:"password"` // en memoria: texto plano; en disco: cifrado
	Codec       string  `json:"codec"`
	BitrateKbps int     `json:"bitrate_kbps"`
	SampleRate  int     `json:"sample_rate"`
	Channels    int     `json:"channels"`
	Input       Source  `json:"input"`
	GainDB      float64 `json:"gain_db"`
	Enhance     bool    `json:"enhance"` // MEJORA: procesador de sonido (se cambia al aire, sin reiniciar)
	EQ          EQ      `json:"eq"`      // ecualizador de 10 bandas (se cambia al aire)
	Titles      Titles  `json:"titles"`  // nombre de la canción: manual, Jazler o archivo
	StreamName  string  `json:"stream_name"`
	Genre       string  `json:"genre"`
	Description string  `json:"description"`
	Website     string  `json:"website"`
}

// TV es la salida de video (H.264 + AAC) hacia MediaMTX por RTMP o SRT.
type TV struct {
	Enabled     bool    `json:"enabled"`
	Protocol    string  `json:"protocol"`
	Host        string  `json:"host"`
	RTMPPort    int     `json:"rtmp_port"`
	SRTPort     int     `json:"srt_port"`
	Path        string  `json:"path"`
	User        string  `json:"user"`
	Password    string  `json:"password"`
	Video       Source  `json:"video"`
	Audio       Source  `json:"audio"`
	Width       int     `json:"width"`
	Height      int     `json:"height"`
	FPS         int     `json:"fps"`
	VideoKbps   int     `json:"video_kbps"`
	AudioKbps   int     `json:"audio_kbps"`
	Encoder     string  `json:"encoder"` // libx264, h264_nvenc, h264_qsv, h264_amf
	KeyframeSec int     `json:"keyframe_sec"`
	GainDB      float64 `json:"gain_db"`
	Enhance     bool    `json:"enhance"` // MEJORA para el audio de la TV
	EQ          EQ      `json:"eq"`      // ecualizador del audio de la TV
}

// Panel son los datos para traer la configuración desde la web (API del estudio).
type Panel struct {
	URL   string `json:"url"`
	Token string `json:"token"`
}

// Startup es cómo arranca y se cierra el programa.
type Startup struct {
	WithWindows   bool `json:"with_windows"`   // abrirse solo al iniciar sesión en Windows (oculto junto al reloj)
	AutoBroadcast bool `json:"auto_broadcast"` // al abrirse, empezar a emitir las salidas marcadas con «Emitir»
	Background    bool `json:"background"`     // la X oculta la ventana y la emisión sigue (ícono junto al reloj)
}

// Config es todo lo que se guarda.
type Config struct {
	Radio   Radio   `json:"radio"`
	TV      TV      `json:"tv"`
	Panel   Panel   `json:"panel"`
	Startup Startup `json:"startup"`
	// EQProfiles son los perfiles de ecualización guardados por la persona (los de fábrica están en EQPresets).
	EQProfiles []EQProfile `json:"eq_profiles"`
	FFmpegPath string      `json:"ffmpeg_path"` // vacío: junto al .exe o en el PATH
}

// Defaults devuelve los valores embebidos.
func Defaults() Config {
	var c Config
	if err := json.Unmarshal(defaultsJSON, &c); err != nil {
		panic("config: defaults.json inválido: " + err.Error()) // error de compilación, no del usuario
	}
	return c
}

// Normalize corrige detalles de forma que no son errores (espacios, barra del montaje).
func (c *Config) Normalize() {
	r := &c.Radio
	r.Host = cleanHost(r.Host)
	r.Mount = strings.TrimSpace(r.Mount)
	if r.Mount != "" && !strings.HasPrefix(r.Mount, "/") {
		r.Mount = "/" + r.Mount
	}
	r.User = strings.TrimSpace(r.User)
	r.Codec = strings.ToLower(strings.TrimSpace(r.Codec))
	t := &c.TV
	t.Host = cleanHost(t.Host)
	t.Path = strings.Trim(strings.TrimSpace(t.Path), "/")
	t.User = strings.TrimSpace(t.User)
	t.Protocol = strings.ToLower(strings.TrimSpace(t.Protocol))
	if t.Video.Kind == SourceURL {
		t.Video.Name = strings.TrimSpace(t.Video.Name)
	}
	c.Panel.URL = strings.TrimRight(strings.TrimSpace(c.Panel.URL), "/")
}

// cleanHost acepta que peguen "https://radio.com/" en el campo de servidor.
func cleanHost(h string) string {
	h = strings.TrimSpace(h)
	for _, p := range []string{"http://", "https://", "icecast://", "rtmp://", "srt://"} {
		h = strings.TrimPrefix(h, p)
	}
	if i := strings.IndexAny(h, "/?"); i >= 0 {
		h = h[:i]
	}
	if i := strings.LastIndex(h, "@"); i >= 0 { // usuario:clave@host
		h = h[i+1:]
	}
	if host, _, err := net.SplitHostPort(h); err == nil { // el puerto va en su propio campo
		h = host
	}
	return h
}

// FieldError es un error de validación con el campo de la interfaz al que pertenece.
type FieldError struct {
	Field   string
	Message string
}

func (e *FieldError) Error() string { return e.Message }

func invalid(field, format string, a ...any) error {
	return &FieldError{Field: field, Message: fmt.Sprintf(format, a...)}
}

// ValidateRadio revisa lo necesario para emitir la radio.
func (c Config) ValidateRadio() error {
	r := c.Radio
	switch {
	case r.Host == "":
		return invalid("radio.host", "Falta el servidor de la radio.")
	case r.Port < 1 || r.Port > 65535:
		return invalid("radio.port", "El puerto de Icecast no es válido.")
	case r.Mount == "" || r.Mount == "/":
		return invalid("radio.mount", "Falta el punto de montaje (por ejemplo /radio).")
	case r.User == "":
		return invalid("radio.user", "Falta el usuario de Icecast (normalmente «source»).")
	case r.Password == "":
		return invalid("radio.password", "Falta la contraseña de fuente de Icecast (Admin → Streams y emisión).")
	}
	if err := validateAudioSource("radio.input", r.Input, false); err != nil {
		return err
	}
	if r.Channels != 1 && r.Channels != 2 {
		return invalid("radio.channels", "Los canales deben ser 1 (mono) o 2 (estéreo).")
	}
	if r.SampleRate != 44100 && r.SampleRate != 48000 {
		return invalid("radio.sample_rate", "La frecuencia de muestreo debe ser 44100 o 48000 Hz.")
	}
	min, max, ok := AudioBitrateRange(r.Codec, r.Channels)
	if !ok {
		return invalid("radio.codec", "Formato de audio desconocido: %q.", r.Codec)
	}
	if r.BitrateKbps < min || r.BitrateKbps > max {
		return invalid("radio.bitrate_kbps", "Para %s el bitrate debe estar entre %d y %d kbps.", CodecLabel(r.Codec), min, max)
	}
	if r.Codec == CodecHEAACv2 && r.Channels != 2 {
		return invalid("radio.channels", "AAC+ v2 solo funciona en estéreo.")
	}
	return ValidateEQ("radio.eq", r.EQ)
}

// ValidateTV revisa lo necesario para emitir la TV.
func (c Config) ValidateTV() error {
	t := c.TV
	switch {
	case t.Host == "":
		return invalid("tv.host", "Falta el servidor de la TV.")
	case t.Protocol != ProtoRTMP && t.Protocol != ProtoSRT:
		return invalid("tv.protocol", "El protocolo debe ser RTMP o SRT.")
	case t.Protocol == ProtoRTMP && (t.RTMPPort < 1 || t.RTMPPort > 65535):
		return invalid("tv.rtmp_port", "El puerto RTMP no es válido.")
	case t.Protocol == ProtoSRT && (t.SRTPort < 1 || t.SRTPort > 65535):
		return invalid("tv.srt_port", "El puerto SRT no es válido.")
	case t.Path == "":
		return invalid("tv.path", "Falta la ruta de la TV (normalmente «tv»).")
	case t.User == "" || t.Password == "":
		return invalid("tv.password", "Falta el usuario o la clave de la TV (Admin → Streams y emisión).")
	case t.Protocol == ProtoSRT && (strings.Contains(t.User, ":") || strings.Contains(t.Password, ":")):
		return invalid("tv.password", "Con SRT el usuario y la clave no pueden llevar «:».")
	}
	switch t.Video.Kind {
	case SourceScreen, SourceTest:
	case SourceDShow:
		if t.Video.Name == "" {
			return invalid("tv.video", "Elija la cámara o capturadora.")
		}
	case SourceURL:
		if !ValidVideoURL(t.Video.Name) {
			return invalid("tv.video_url", "Escriba la dirección del video: debe empezar con http:// o https://.")
		}
	default:
		return invalid("tv.video", "Elija una fuente de video.")
	}
	if t.Audio.Kind == SourceFromVideo {
		if t.Video.Kind != SourceURL {
			return invalid("tv.audio", "«Audio del video» solo sirve cuando el video viene de una dirección http/https.")
		}
	} else if err := validateAudioSource("tv.audio", t.Audio, true); err != nil {
		return err
	}
	switch {
	case t.Width < 160 || t.Width > 3840 || t.Width%2 != 0 || t.Height < 120 || t.Height > 2160 || t.Height%2 != 0:
		return invalid("tv.resolution", "La resolución no es válida (ancho y alto pares).")
	case t.FPS < 10 || t.FPS > 60:
		return invalid("tv.fps", "Los cuadros por segundo deben estar entre 10 y 60.")
	case t.VideoKbps < 300 || t.VideoKbps > MaxVideoKbps:
		return invalid("tv.video_kbps", "El bitrate de video debe estar entre 300 y %d kbps.", MaxVideoKbps)
	case t.AudioKbps < 32 || t.AudioKbps > MaxAudioKbps:
		return invalid("tv.audio_kbps", "El bitrate de audio debe estar entre 32 y %d kbps.", MaxAudioKbps)
	case t.KeyframeSec < 1 || t.KeyframeSec > 10:
		return invalid("tv.keyframe_sec", "El intervalo de fotogramas clave debe estar entre 1 y 10 s.")
	}
	if !knownEncoder(t.Encoder) {
		return invalid("tv.encoder", "Codificador de video desconocido: %q.", t.Encoder)
	}
	return ValidateEQ("tv.eq", t.EQ)
}

func validateAudioSource(field string, s Source, allowNone bool) error {
	switch s.Kind {
	case SourceTest:
		return nil
	case SourceNone:
		if allowNone {
			return nil
		}
	case SourceDShow:
		if s.Name != "" {
			return nil
		}
		return invalid(field, "Elija el micrófono o la entrada de línea.")
	case SourceLoopback:
		return nil // Name vacío = altavoces predeterminados de Windows
	}
	return invalid(field, "Elija una entrada de audio.")
}

// ValidVideoURL acepta direcciones http:// o https:// con servidor.
func ValidVideoURL(raw string) bool {
	u, err := url.Parse(strings.TrimSpace(raw))
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}

// Encoders son los codificadores H.264 que ofrece la interfaz.
var Encoders = []struct{ ID, Label string }{
	{"libx264", "x264 (procesador)"},
	{"h264_nvenc", "NVIDIA NVENC"},
	{"h264_qsv", "Intel Quick Sync"},
	{"h264_amf", "AMD AMF"},
}

func knownEncoder(id string) bool {
	for _, e := range Encoders {
		if e.ID == id {
			return true
		}
	}
	return false
}

// Codecs son los formatos de audio de la radio, en el orden de la interfaz.
var Codecs = []string{CodecMP3, CodecAAC, CodecHEAAC, CodecHEAACv2, CodecOpus, CodecVorbis}

// CodecLabel es el nombre que ve la persona.
func CodecLabel(codec string) string {
	switch codec {
	case CodecMP3:
		return "MP3"
	case CodecAAC:
		return "AAC (AAC-LC)"
	case CodecHEAAC:
		return "AAC+ (HE-AAC v1)"
	case CodecHEAACv2:
		return "AAC+ v2 (HE-AAC v2)"
	case CodecOpus:
		return "Opus (Ogg)"
	case CodecVorbis:
		return "Ogg Vorbis"
	}
	return codec
}

// AudioBitrateRange devuelve el bitrate mínimo y máximo razonable (kbps) de cada
// códec. El máximo general es 320 kbps; AAC+ rinde solo en bitrates bajos (por
// encima de ~64 kbps se usa AAC-LC) y libfdk_aac no acepta más.
func AudioBitrateRange(codec string, channels int) (min, max int, ok bool) {
	switch codec {
	case CodecMP3:
		return 32, MaxAudioKbps, true
	case CodecAAC:
		return 32, MaxAudioKbps, true
	case CodecHEAAC:
		if channels == 1 {
			return 16, 48, true
		}
		return 24, 64, true
	case CodecHEAACv2:
		return 16, 48, true
	case CodecOpus:
		return 16, MaxAudioKbps, true
	case CodecVorbis:
		return 48, MaxAudioKbps, true
	}
	return 0, 0, false
}

// ---------- archivo ----------

// Dir devuelve la carpeta de datos del usuario (%APPDATA%\Club25 Emisor).
func Dir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "Club25 Emisor"), nil
}

// Load lee la configuración: defaults embebidos, luego el archivo junto al
// programa (exeDefaults, puede no existir) y luego el del usuario (path, puede no existir).
func Load(exeDefaults, path string) (Config, error) {
	c := Defaults()
	for _, p := range []string{exeDefaults, path} {
		if p == "" {
			continue
		}
		b, err := os.ReadFile(p)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return c, err
		}
		if err := json.Unmarshal(b, &c); err != nil {
			return c, fmt.Errorf("%s: %w", filepath.Base(p), err)
		}
	}
	var err error
	if c.Radio.Password, err = secret.Open(c.Radio.Password); err != nil {
		c.Radio.Password = ""
	}
	if c.TV.Password, err = secret.Open(c.TV.Password); err != nil {
		c.TV.Password = ""
	}
	if c.Panel.Token, err = secret.Open(c.Panel.Token); err != nil {
		c.Panel.Token = ""
	}
	c.Normalize()
	return c, nil
}

// Save escribe la configuración con las claves cifradas (escritura atómica).
func Save(path string, c Config) error {
	c.Normalize()
	var err error
	if c.Radio.Password, err = secret.Seal(c.Radio.Password); err != nil {
		return err
	}
	if c.TV.Password, err = secret.Seal(c.TV.Password); err != nil {
		return err
	}
	if c.Panel.Token, err = secret.Seal(c.Panel.Token); err != nil {
		return err
	}
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
