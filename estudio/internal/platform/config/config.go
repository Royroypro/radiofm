package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Settings es la configuración local de Club25 Estudio (settings.json en la carpeta de datos).
// Los secretos (token del panel, clave de Icecast) NO van aquí: ver platform/secrets.
type Settings struct {
	Audio     AudioSettings     `json:"audio"`
	Icecast   IcecastSettings   `json:"icecast"`
	Server    ServerSettings    `json:"server"`
	Station   StationSettings   `json:"station"`
	Rotation  RotationSettings  `json:"rotation"`
	Locutions LocutionSettings  `json:"locutions"`
	Safety    SafetySettings    `json:"safety"`
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
}

// Crossfade devuelve la duración del crossfade.
func (a AudioSettings) Crossfade() time.Duration {
	return time.Duration(a.CrossfadeMs) * time.Millisecond
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
}

// LocutionSettings: archivos de hora y temperatura locutadas.
type LocutionSettings struct {
	Dir          string `json:"dir"`
	HourPrefix   string `json:"hour_prefix"`   // HRS → HRS00.mp3 … HRS23.mp3
	MinutePrefix string `json:"minute_prefix"` // MIN → MIN00.mp3 … MIN59.mp3
	TempPrefix   string `json:"temp_prefix"`   // TMP → TMP-10.mp3 … TMP45.mp3
	TimeOff      bool   `json:"time_off"`      // interruptor «Hora locutada» de Al aire apagado
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
//   - IngestHost: el audio va directo al Nodo A (Icecast 8010), sin pasar por el VPS.
//   - OldServerURL: el dominio anterior de la web; las instalaciones que lo tengan
//     guardado pasan solas a ServerURL al abrir (los tokens siguen valiendo: es el
//     mismo servidor).
const (
	ServerURL    = "https://radioclub25peru.com"
	IngestHost   = "radioclub25.sehuacho.com"
	OldServerURL = "https://radioclub25.sehuacho.com"
)

// Defaults devuelve la configuración inicial del plan.
func Defaults() Settings {
	return Settings{
		Audio: AudioSettings{CrossfadeMs: 3000, DuckDepthDB: -12, DuckThresholdDB: -35, DuckAttackMs: 50,
			DuckReleaseMs: 500, HardFadeMs: 2000, BreakFadeMs: 1000, ProcPreset: "natural", ProcScope: "both"},
		Icecast: IcecastSettings{Host: IngestHost, Port: 8010, Mount: "/radio", User: "source",
			Format: "mp3", BitrateKbps: 128, Source: "bus", MetadataMusic: true},
		Server:    ServerSettings{URL: ServerURL},
		Station:   StationSettings{Name: "Radio Club 25", City: "Huacho", Timezone: "America/Lima", Latitude: -11.1067, Longitude: -77.6050},
		Rotation:  RotationSettings{ArtistSeparationMin: 45, TitleSeparationHours: 3},
		Locutions: LocutionSettings{HourPrefix: "HRS", MinutePrefix: "MIN", TempPrefix: "TMP"},
		Safety:    SafetySettings{SilenceSeconds: 15, StartPlaying: true},
		Schedule:  ScheduleSettings{DaysAhead: 1, FillToleranceMs: 30000},
		UI:        InterfaceSettings{Theme: "dark", CustomBase: "dark"},
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
	case c.Rotation.ArtistSeparationMin < 0 || c.Rotation.TitleSeparationHours < 0:
		return errors.New("las separaciones no pueden ser negativas")
	case c.Safety.SilenceSeconds < 0:
		return errors.New("el tiempo de silencio no puede ser negativo")
	case c.Audio.ProcPreset != "" && c.Audio.ProcPreset != "natural" && c.Audio.ProcPreset != "fm" &&
		c.Audio.ProcPreset != "voice" && c.Audio.ProcPreset != "off":
		return errors.New("preset del procesador desconocido")
	case c.Audio.ProcScope != "" && c.Audio.ProcScope != "both" && c.Audio.ProcScope != "stream":
		return errors.New("el procesador se aplica a both o stream")
	case c.Audio.BreakFadeMs < 0 || c.Audio.BreakFadeMs > 5000:
		return errors.New("el fundido de tanda debe estar entre 0 y 5 s")
	case c.UI.Theme != "" && c.UI.Theme != "dark" && c.UI.Theme != "light" && c.UI.Theme != "club25" && c.UI.Theme != "custom":
		return errors.New("tema desconocido")
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
