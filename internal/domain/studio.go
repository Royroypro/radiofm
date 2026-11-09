package domain

import (
	"context"
	"fmt"
	"time"
)

// Integración con "Club25 Estudio", el programa de automatización de radio de
// la PC de la emisora (carpeta estudio/ de este repositorio). La PC descarga
// las cuñas FM, informa lo que suena y reporta cada emisión de cuña (as-run)
// para el comprobante del anunciante. Ver docs/api.md → "API del estudio".

// FMSpot es una cuña para la emisión FM (no confundir con AdAudioSpot, que es
// la cuña que suena en la web antes de conectar el reproductor).
type FMSpot struct {
	ID          int64
	Advertiser  string // anunciante
	Name        string // nombre de la cuña
	FileURL     string // ruta pública del archivo (/uploads/fm/...)
	SHA256      string // hash hex del archivo
	DurationMs  int64  // 0 = desconocida
	StartsAt    *time.Time
	EndsAt      *time.Time
	PlaysPerDay int // veces que debe sonar por día (1–96)
	HourFrom    int // franja horaria [HourFrom, HourTo) en hora local de la radio
	HourTo      int
	Active      bool
	FrequencyID int64 // 0 = todas las frecuencias
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// AppliesTo indica si la cuña se emite en la frecuencia fid.
func (s FMSpot) AppliesTo(fid int64) bool { return s.FrequencyID == 0 || s.FrequencyID == fid }

// IsLive indica si la cuña está vigente en el instante t.
func (s FMSpot) IsLive(t time.Time) bool {
	if !s.Active {
		return false
	}
	if s.StartsAt != nil && t.Before(*s.StartsAt) {
		return false
	}
	if s.EndsAt != nil && !t.Before(*s.EndsAt) {
		return false
	}
	return true
}

// Expired indica si la cuña ya no volverá a estar vigente después de t.
func (s FMSpot) Expired(t time.Time) bool { return s.EndsAt != nil && !t.Before(*s.EndsAt) }

// InHours indica si la hora local h (0–23) cae dentro de la franja de la cuña.
func (s FMSpot) InHours(h int) bool { return h >= s.HourFrom && h < s.HourTo }

// HoursLabel devuelve la franja para la interfaz ("Todo el día" o "07:00–13:00").
func (s FMSpot) HoursLabel() string {
	if s.HourFrom == 0 && s.HourTo == 24 {
		return "Todo el día"
	}
	return fmt.Sprintf("%02d:00–%02d:00", s.HourFrom, s.HourTo)
}

// DurationLabel devuelve la duración para la interfaz ("20 s" o "—" si es desconocida).
func (s FMSpot) DurationLabel() string { return msLabel(s.DurationMs) }

func msLabel(ms int64) string {
	if ms <= 0 {
		return "—"
	}
	return fmt.Sprintf("%.0f s", float64(ms)/1000)
}

// FMSpotRepository persiste las cuñas FM.
type FMSpotRepository interface {
	Create(ctx context.Context, s *FMSpot) error
	Get(ctx context.Context, id int64) (*FMSpot, error)
	List(ctx context.Context, onlyActive bool) ([]FMSpot, error)
	Update(ctx context.Context, s *FMSpot) error
	Delete(ctx context.Context, id int64) error
}

// Airplay es una emisión real de una cuña reportada por la PC.
// PlayUUID lo genera la PC y hace que reenviar el mismo lote no duplique.
type Airplay struct {
	ID          int64
	SpotID      int64 // 0 si la cuña se borró después
	Advertiser  string
	SpotName    string
	AiredAt     time.Time
	DurationMs  int64
	Complete    bool
	PlayUUID    string
	FrequencyID int64 // dónde sonó (0 = desconocido)
	CreatedAt   time.Time
}

// DurationLabel devuelve lo que sonó para la interfaz ("19 s").
func (a Airplay) DurationLabel() string { return msLabel(a.DurationMs) }

// AirplayFilter filtra el comprobante. Los campos vacíos no filtran.
type AirplayFilter struct {
	From, To    time.Time // rango [From, To)
	Advertiser  string
	SpotID      int64
	FrequencyID int64
}

// AirplayRepository persiste las emisiones.
type AirplayRepository interface {
	// Record guarda las emisiones cuyo PlayUUID no exista ya y devuelve cuántas eran nuevas.
	// Advertiser y SpotName se copian de la cuña dentro de la misma operación.
	Record(ctx context.Context, plays []Airplay) (int, error)
	List(ctx context.Context, f AirplayFilter) ([]Airplay, error)
	// CountSince cuenta las emisiones por cuña desde since (para el panel).
	CountSince(ctx context.Context, since time.Time) (map[int64]int, error)
}

// StudioMode es el modo de trabajo de la PC.
type StudioMode string

const (
	StudioAuto     StudioMode = "auto"     // automático
	StudioAssisted StudioMode = "assisted" // asistido: avanza el operador
	StudioLive     StudioMode = "live"     // en vivo: mic + botonera
)

// Label devuelve el nombre del modo para la interfaz.
func (m StudioMode) Label() string {
	switch m {
	case StudioAuto:
		return "Automático"
	case StudioAssisted:
		return "Asistido"
	case StudioLive:
		return "En vivo"
	}
	return "—"
}

// Valid indica si el modo es conocido.
func (m StudioMode) Valid() bool { return m == StudioAuto || m == StudioAssisted || m == StudioLive }

// NowPlaying es lo que está sonando en la PC del estudio.
type NowPlaying struct {
	Artist string     `json:"artist"`
	Title  string     `json:"title"`
	Kind   string     `json:"kind"` // music, spot, jingle, voice, live...
	EndsAt *time.Time `json:"ends_at,omitempty"`
}

// Display devuelve "Artista - Título" (o solo el título).
func (n NowPlaying) Display() string {
	if n.Artist != "" && n.Title != "" {
		return n.Artist + " - " + n.Title
	}
	return n.Artist + n.Title
}

// StudioState es el estado conocido de la PC del estudio de una frecuencia.
type StudioState struct {
	FrequencyID    int64
	TokenHash      string // SHA-256 hex del token vigente; "" = sin token
	TokenCreatedAt *time.Time
	LastSeen       *time.Time
	Version        string
	Mode           StudioMode
	EncoderJSON    string
	NowPlaying     *NowPlaying
	NowPlayingAt   *time.Time
}

// HasToken indica si hay un token generado.
func (s StudioState) HasToken() bool { return s.TokenHash != "" }

// Online indica si la PC dio señales de vida en los últimos maxAge.
func (s StudioState) Online(now time.Time, maxAge time.Duration) bool {
	return s.LastSeen != nil && now.Sub(*s.LastSeen) <= maxAge
}

// StudioRepository persiste el estado de la PC del estudio de cada frecuencia.
// Get de una frecuencia sin PC devuelve un estado vacío (no ErrNotFound).
type StudioRepository interface {
	Get(ctx context.Context, frequencyID int64) (*StudioState, error)
	List(ctx context.Context) (map[int64]*StudioState, error)
	// FindToken devuelve la frecuencia del token con ese hash (ErrNotFound si no hay).
	FindToken(ctx context.Context, hash string) (int64, error)
	SetToken(ctx context.Context, frequencyID int64, hash string, at time.Time) error // hash "" revoca
	Heartbeat(ctx context.Context, frequencyID int64, at time.Time, version string, mode StudioMode, encoderJSON string) error
	SetNowPlaying(ctx context.Context, frequencyID int64, np NowPlaying, at time.Time) error
}
