package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	// base de zonas horarias dentro del ejecutable: Windows no la trae y sin ella
	// LoadLocation("America/Lima") falla en las PCs sin Go instalado
	_ "time/tzdata"
)

// Settings es la configuración local de Club25 Estudio (settings.json en la carpeta de datos).
// Los secretos (token del panel, clave de Icecast) NO van aquí: ver platform/secrets.
type Settings struct {
	Audio     AudioSettings     `json:"audio"`
	Icecast   IcecastSettings   `json:"icecast"`
	Server    ServerSettings    `json:"server"`
	Station   StationSettings   `json:"station"`
	Rotation  RotationSettings  `json:"rotation"`
	TV        TVSettings        `json:"tv"`
	Locutions LocutionSettings  `json:"locutions"`
	Safety    SafetySettings    `json:"safety"`
	Logger    LoggerSettings    `json:"logger"`
	RDS       RDSSettings       `json:"rds"`
	Schedule  ScheduleSettings  `json:"schedule"`
	UI        InterfaceSettings `json:"ui"`
}

// AudioSettings: dispositivos y mezcla.
type AudioSettings struct {
	OutputDevice    string  `json:"output_device"`  // "" = predeterminado
	PreviewDevice   string  `json:"preview_device"` // "" = sin pre-escucha
	MicDevice       string  `json:"mic_device"`     // "" = sin micrófono
	MicGainDB       float64 `json:"mic_gain_db"`
	CrossfadeMs     int     `json:"crossfade_ms"`
	DuckDepthDB     float64 `json:"duck_depth_db"`
	DuckThresholdDB float64 `json:"duck_threshold_db"`
	DuckAttackMs    int     `json:"duck_attack_ms"`
	DuckReleaseMs   int     `json:"duck_release_ms"`
	HardFadeMs      int     `json:"hard_fade_ms"`  // fundido al entrar un evento duro
	BreakFadeMs     int     `json:"break_fade_ms"` // fundido de la música al entrar una tanda
	ProcPreset      string  `json:"proc_preset"`   // procesador: natural | fm | voice | off
	ProcScope       string  `json:"proc_scope"`    // a qué se aplica: both (FM y stream) | stream
	// EQ: ecualizador de 10 bandas (dB, ±12) en 31, 62, 125, 250, 500 Hz, 1, 2, 4, 8 y 16 kHz.
	EQ []float64 `json:"eq"`
	// StereoWidth: ampliación estéreo en % (100 = normal, 0 = mono, hasta 200).
	StereoWidth int `json:"stereo_width"`
}

// Crossfade devuelve la duración del crossfade.
func (a AudioSettings) Crossfade() time.Duration {
	return time.Duration(a.CrossfadeMs) * time.Millisecond
}

// TVSettings: modo TV (video para la web). La clave va en secrets (TVPassword).
type TVSettings struct {
	Enabled     bool   `json:"enabled"`
	URL         string `json:"url"`        // rtmp://servidor:1935/tv
	User        string `json:"user"`       // usuario de publicación (panel → Ajustes → Streams)
	Resolution  string `json:"resolution"` // 480p | 720p | 1080p
	FPS         int    `json:"fps"`        // 25 | 30
	BitrateKbps int    `json:"bitrate_kbps"`
	Codec       string `json:"codec"`       // x264 | nvenc | qsv | amf
	Logo        string `json:"logo"`        // PNG (con transparencia); vacío = nombre de la radio
	LogoPos     string `json:"logo_pos"`    // tl | tr | bl | br
	Background  string `json:"background"`  // imagen de fondo; vacío = degradado
	Plate       string `json:"plate"`       // imagen de la escena «placa»
	Accent      string `json:"accent"`      // #rrggbb
	Accent2     string `json:"accent2"`     // #rrggbb
	Panel       string `json:"panel"`       // #rrggbb (fondo de recuadros y del degradado)
	ShowLogo    bool   `json:"show_logo"`   // capas encendidas al arrancar
	ShowNow     bool   `json:"show_now"`    // cintillo «ahora suena»
	ShowClock   bool   `json:"show_clock"`  // reloj
	ShowTicker  bool   `json:"show_ticker"` // texto que corre abajo
	Ticker      string `json:"ticker"`
}

// Size devuelve el ancho y alto de la resolución elegida.
func (t TVSettings) Size() (int, int) {
	switch t.Resolution {
	case "480p":
		return 854, 480
	case "1080p":
		return 1920, 1080
	}
	return 1280, 720
}

// IcecastSettings: emisión a la web.
type IcecastSettings struct {
	Enabled       bool   `json:"enabled"`
	Host          string `json:"host"`
	Port          int    `json:"port"`
	Mount         string `json:"mount"`
	User          string `json:"user"`
	Format        string `json:"format"` // mp3 | aac
	BitrateKbps   int    `json:"bitrate_kbps"`
	Source        string `json:"source"` // "bus" (mezcla de la PC) | "capture" (entrada de línea)
	CaptureDevice string `json:"capture_device"`
	MetadataMusic bool   `json:"metadata_music_only"` // no enviar cuñas/jingles como título
}

// ServerSettings: panel web.
type ServerSettings struct {
	URL string `json:"url"` // https://radioclub25peru.com
}

// StationSettings: datos de la radio (se completan con /api/studio/config).
type StationSettings struct {
	Name      string  `json:"name"`
	City      string  `json:"city"`
	Timezone  string  `json:"timezone"`
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
}

// RotationSettings: separaciones de la rotación musical.
type RotationSettings struct {
	ArtistSeparationMin  int `json:"artist_separation_min"`
	TitleSeparationHours int `json:"title_separation_hours"`
	// Reglas de flujo (0 / false = desactivada); ver service/rotation.
	NoSameVoice     bool `json:"no_same_voice"`      // no dos voces femeninas (o masculinas) seguidas
	MaxEnergyJump   int  `json:"max_energy_jump"`    // salto máximo de energía (1–4)
	MaxTempoJumpPct int  `json:"max_tempo_jump_pct"` // salto máximo de tempo en %
	MaxGenreRun     int  `json:"max_genre_run"`      // máximo seguidas del mismo género
	MaxEraRun       int  `json:"max_era_run"`        // máximo seguidas de la misma década
}

// LocutionSettings: archivos de hora y temperatura locutadas.
type LocutionSettings struct {
	Dir          string `json:"dir"`
	HourPrefix   string `json:"hour_prefix"`   // HRS → HRS00.mp3 … HRS23.mp3
	MinutePrefix string `json:"minute_prefix"` // MIN → MIN00.mp3 … MIN59.mp3
	TempPrefix   string `json:"temp_prefix"`   // TMP → TMP-10.mp3 … TMP45.mp3
	TimeOff      bool   `json:"time_off"`      // interruptor «Hora locutada» de Al aire apagado
}

// LoggerSettings: grabador legal de la emisión (service/airlog).
type LoggerSettings struct {
	Enabled    bool                 `json:"enabled"`
	Dir        string               `json:"dir"`      // "" = <datos>/grabaciones
	MaxMB      int                  `json:"max_mb"`   // tamaño máximo total (0 = sin límite)
	MaxDays    int                  `json:"max_days"` // días máximos (0 = sin límite)
	Kbps       int                  `json:"kbps"`     // MP3 mono
	Recordings []ScheduledRecording `json:"recordings"`
}

// ScheduledRecording es una grabación programada (recorte de la emisión al terminar la franja).
type ScheduledRecording struct {
	Name    string `json:"name"`
	Days    int    `json:"days"`  // bit 0 = domingo … bit 6 = sábado (0 = todos)
	Start   string `json:"start"` // "HH:MM"
	Minutes int    `json:"minutes"`
}

// RDSSettings: envío del nombre de la radio y lo que suena al codificador RDS del transmisor (service/rds).
type RDSSettings struct {
	Enabled    bool   `json:"enabled"`
	Transport  string `json:"transport"`   // tcp | udp | serial
	Address    string `json:"address"`     // host:puerto (tcp/udp) o COM3 (serial)
	Baud       int    `json:"baud"`        // serie
	PS         string `json:"ps"`          // nombre de la radio (8 caracteres)
	PSTemplate string `json:"ps_template"` // comando para el PS; {ps}
	RTTemplate string `json:"rt_template"` // comando para el RadioText; {text}
	RTFormat   string `json:"rt_format"`   // texto: {artist}, {title}, {station}
	SkipSpots  bool   `json:"skip_spots"`  // en cuñas/jingles muestra el nombre de la radio
	MinSeconds int    `json:"min_seconds"` // mínimo entre envíos
}

// SafetySettings: seguridad operativa.
type SafetySettings struct {
	SilenceSeconds int  `json:"silence_seconds"` // 0 = desactivado
	AutoStart      bool `json:"auto_start"`      // arrancar con Windows
	StartPlaying   bool `json:"start_playing"`   // empezar a sonar al abrir (modo automático)
}

// ScheduleSettings: generación de la playlist.
type ScheduleSettings struct {
	DaysAhead       int `json:"days_ahead"`        // 1 = hoy y mañana
	FillToleranceMs int `json:"fill_tolerance_ms"` // margen para cuadrar la hora
	BreakMaxSpots   int `json:"break_max_spots"`   // máximo de cuñas por tanda (0 = sin tope)
	BreakMaxSeconds int `json:"break_max_seconds"` // máximo de segundos de cuñas por tanda (0 = sin tope)
}

// InterfaceSettings: preferencias de la interfaz.
type InterfaceSettings struct {
	CartPage int    `json:"cart_page"`
	Theme    string `json:"theme"` // dark | light | club25 | custom
	// Colores del tema personalizado (#RRGGBB; vacío = el del tema base).
	CustomBase    string `json:"custom_base"` // dark | light | club25
	CustomAccent  string `json:"custom_accent"`
	CustomBg      string `json:"custom_bg"`
	CustomPanel   string `json:"custom_panel"`
	CustomSidebar string `json:"custom_sidebar"`
	CustomText    string `json:"custom_text"`
}

// Dominios del servidor.
//   - ServerURL: la web y la API del estudio (radioclub25peru.com, detrás del VPS con
//     HAProxy y nodos de reserva; ver scripts/ha/README.md en la raíz del repositorio).
//   - IngestHost: el audio (Icecast 8010). Entra por el mismo dominio: el VPS lo pasa
//     por TCP al nodo activo (scripts/ha/vps-radioclub25-ingesta.sh).
//   - OldServerURL / OldIngestHost: el subdominio anterior, que se desactiva; las
//     instalaciones que lo tengan guardado pasan solas al dominio nuevo al abrir (los
//     tokens y claves siguen valiendo: es el mismo servidor).
const (
	ServerURL     = "https://radioclub25peru.com"
	IngestHost    = "radioclub25peru.com"
	OldServerURL  = "https://radioclub25.sehuacho.com"
	OldIngestHost = "radioclub25.sehuacho.com"
)

// Defaults devuelve la configuración inicial del plan.
func Defaults() Settings {
	return Settings{
		Audio: AudioSettings{CrossfadeMs: 3000, DuckDepthDB: -12, DuckThresholdDB: -35, DuckAttackMs: 50,
			DuckReleaseMs: 500, HardFadeMs: 2000, BreakFadeMs: 1000, ProcPreset: "natural", ProcScope: "both",
			StereoWidth: 100},
		Icecast: IcecastSettings{Host: IngestHost, Port: 8010, Mount: "/radio", User: "source",
			Format: "mp3", BitrateKbps: 128, Source: "bus", MetadataMusic: true},
		Server:    ServerSettings{URL: ServerURL},
		TV: TVSettings{URL: "rtmp://" + IngestHost + ":1935/tv", Resolution: "720p", FPS: 30, BitrateKbps: 2500,
			Codec: "x264", LogoPos: "tr", Accent: "#e5262a", Accent2: "#f7a21b", Panel: "#1b2a6e",
			ShowLogo: true, ShowNow: true, ShowClock: true},
		Station:   StationSettings{Name: "Radio Club 25", City: "Huacho", Timezone: "America/Lima", Latitude: -11.1067, Longitude: -77.6050},
		Rotation:  RotationSettings{ArtistSeparationMin: 45, TitleSeparationHours: 3, MaxEnergyJump: 2, MaxGenreRun: 3},
		Locutions: LocutionSettings{HourPrefix: "HRS", MinutePrefix: "MIN", TempPrefix: "TMP"},
		Safety:    SafetySettings{SilenceSeconds: 15, StartPlaying: true},
		Logger:    LoggerSettings{Enabled: true, MaxMB: 1024, MaxDays: 30, Kbps: 64},
		RDS: RDSSettings{Transport: "tcp", Baud: 9600, PS: "CLUB 25", PSTemplate: "PS={ps}\r\n",
			RTTemplate: "RT={text}\r\n", RTFormat: "{artist} - {title}", SkipSpots: true, MinSeconds: 5},
		Schedule: ScheduleSettings{DaysAhead: 1, FillToleranceMs: 30000},
		UI:       InterfaceSettings{Theme: "dark", CustomBase: "dark"},
	}
}

// DataDir devuelve la carpeta de datos: CLUB25_DATA_DIR, o %ProgramData%\Club25Estudio
// si se puede escribir ahí, o %LOCALAPPDATA%\Club25Estudio.
func DataDir() string {
	if d := os.Getenv("CLUB25_DATA_DIR"); d != "" {
		return d
	}
	for _, base := range []string{os.Getenv("ProgramData"), os.Getenv("LOCALAPPDATA")} {
		if base == "" {
			continue
		}
		d := filepath.Join(base, "Club25Estudio")
		if writable(d) {
			return d
		}
	}
	d, _ := os.UserConfigDir()
	return filepath.Join(d, "Club25Estudio")
}

func writable(dir string) bool {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return false
	}
	f, err := os.CreateTemp(dir, ".w*")
	if err != nil {
		return false
	}
	name := f.Name()
	f.Close()
	_ = os.Remove(name)
	return true
}

// Store lee y guarda settings.json.
type Store struct {
	path string
}

// NewStore crea el almacén en dir.
func NewStore(dir string) *Store { return &Store{path: filepath.Join(dir, "settings.json")} }

// Path devuelve la ruta del archivo.
func (s *Store) Path() string { return s.path }

// Load lee la configuración; si no existe devuelve los valores por defecto.
// Los campos que falten en el archivo toman el valor por defecto.
func (s *Store) Load() (Settings, error) {
	cfg := Defaults()
	b, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return cfg, nil
	}
	if err != nil {
		return cfg, err
	}
	if err := json.Unmarshal(b, &cfg); err != nil {
		return Defaults(), fmt.Errorf("config: %s inválido: %w", s.path, err)
	}
	cfg.migrate()
	return cfg, nil
}

// migrate actualiza valores de versiones anteriores.
func (c *Settings) migrate() {
	// 1.0.7: la web pasó a radioclub25peru.com (el dominio anterior sigue
	// funcionando, pero el nuevo tiene nodos de reserva).
	if strings.TrimRight(strings.TrimSpace(c.Server.URL), "/") == OldServerURL {
		c.Server.URL = ServerURL
	}
	// 1.0.8: el audio también entra por radioclub25peru.com (el subdominio se apaga).
	if strings.EqualFold(strings.TrimSpace(c.Icecast.Host), OldIngestHost) {
		c.Icecast.Host = IngestHost
	}
}

// Save guarda la configuración de forma atómica (archivo temporal + renombrar).
func (s *Store) Save(cfg Settings) error {
	if err := cfg.Validate(); err != nil {
		return err
	}
	b, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

// Validate comprueba rangos básicos.
func (c Settings) Validate() error {
	switch {
	case c.Audio.CrossfadeMs < 0 || c.Audio.CrossfadeMs > 15000:
		return errors.New("el crossfade debe estar entre 0 y 15 s")
	case c.Audio.DuckDepthDB > 0 || c.Audio.DuckDepthDB < -40:
		return errors.New("el ducking debe estar entre −40 y 0 dB")
	case c.Icecast.Port < 0 || c.Icecast.Port > 65535:
		return errors.New("puerto de Icecast inválido")
	case c.Icecast.Source != "bus" && c.Icecast.Source != "capture":
		return errors.New("la fuente de Icecast debe ser bus o capture")
	case c.Icecast.Format != "mp3" && c.Icecast.Format != "aac":
		return errors.New("el formato debe ser mp3 o aac")
	case c.Rotation.MaxEnergyJump < 0 || c.Rotation.MaxEnergyJump > 4 || c.Rotation.MaxTempoJumpPct < 0 ||
		c.Rotation.MaxGenreRun < 0 || c.Rotation.MaxEraRun < 0:
		return errors.New("reglas de flujo fuera de rango")
	case c.Rotation.ArtistSeparationMin < 0 || c.Rotation.TitleSeparationHours < 0:
		return errors.New("las separaciones no pueden ser negativas")
	case c.Safety.SilenceSeconds < 0:
		return errors.New("el tiempo de silencio no puede ser negativo")
	case c.Audio.ProcPreset != "" && c.Audio.ProcPreset != "natural" && c.Audio.ProcPreset != "fm" &&
		c.Audio.ProcPreset != "voice" && c.Audio.ProcPreset != "off":
		return errors.New("preset del procesador desconocido")
	case c.Audio.ProcScope != "" && c.Audio.ProcScope != "both" && c.Audio.ProcScope != "stream":
		return errors.New("el procesador se aplica a both o stream")
	case c.Logger.MaxMB < 0 || c.Logger.MaxDays < 0 || c.Logger.Kbps < 0 || c.Logger.Kbps > 320:
		return errors.New("límites del grabador inválidos")
	case c.RDS.Transport != "" && c.RDS.Transport != "tcp" && c.RDS.Transport != "udp" && c.RDS.Transport != "serial":
		return errors.New("el RDS se envía por tcp, udp o serial")
	case c.Audio.BreakFadeMs < 0 || c.Audio.BreakFadeMs > 5000:
		return errors.New("el fundido de tanda debe estar entre 0 y 5 s")
	case c.UI.Theme != "" && c.UI.Theme != "dark" && c.UI.Theme != "light" && c.UI.Theme != "club25" && c.UI.Theme != "custom":
		return errors.New("tema desconocido")
	}
	if t := c.TV; t.Enabled {
		switch {
		case !strings.HasPrefix(t.URL, "rtmp://") && !strings.HasPrefix(t.URL, "rtmps://") && !strings.HasPrefix(t.URL, "srt://"):
			return errors.New("TV: la dirección debe empezar con rtmp://, rtmps:// o srt://")
		case t.FPS != 25 && t.FPS != 30:
			return errors.New("TV: los cuadros por segundo deben ser 25 o 30")
		case t.BitrateKbps < 300 || t.BitrateKbps > 12000:
			return errors.New("TV: la tasa de video debe estar entre 300 y 12000 kbps")
		case t.Resolution != "480p" && t.Resolution != "720p" && t.Resolution != "1080p":
			return errors.New("TV: resolución desconocida")
		}
	}
	if len(c.Audio.EQ) > 10 {
		return errors.New("el ecualizador tiene 10 bandas")
	}
	for _, g := range c.Audio.EQ {
		if g < -12 || g > 12 {
			return errors.New("cada banda del ecualizador va de −12 a +12 dB")
		}
	}
	if c.Audio.StereoWidth < 0 || c.Audio.StereoWidth > 200 {
		return errors.New("la ampliación estéreo va de 0 % a 200 %")
	}
	if _, err := time.LoadLocation(c.Station.Timezone); err != nil {
		return fmt.Errorf("zona horaria inválida: %w", err)
	}
	return nil
}

// Location devuelve la zona horaria de la radio (America/Lima por defecto).
func (c Settings) Location() *time.Location {
	if loc, err := time.LoadLocation(c.Station.Timezone); err == nil {
		return loc
	}
	return time.FixedZone("PET", -5*3600)
}
