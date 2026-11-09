package domain

import (
	"context"
	"fmt"
	"strings"
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

	// Una cuña es una VERSIÓN (un audio) de una campaña. Las reglas de arriba y
	// las de abajo son copia de las de su campaña (FMCampaign.Apply); solo Name,
	// el archivo, la duración, Weight y Active son propios de la versión.
	CampaignID int64
	Weight     int          // peso en la rotación de versiones (1–10)
	DaysMask   int          // días de la semana (bit 0 = domingo … bit 6 = sábado; 127 = todos)
	Position   SpotPosition // posición pedida en la tanda
	Priority   int          // 1 alta, 2 normal, 3 baja
	Sector     string       // rubro del anunciante (competencia)
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
	ByCampaign(ctx context.Context, campaignID int64) ([]FMSpot, error)
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

// ---------- Tráfico: anunciantes y campañas ----------

// SpotPosition es la posición pedida de una cuña dentro de la tanda.
type SpotPosition string

const (
	PositionAny   SpotPosition = "any"
	PositionFirst SpotPosition = "first"
	PositionLast  SpotPosition = "last"
)

// Label devuelve la posición para la interfaz.
func (p SpotPosition) Label() string {
	switch p {
	case PositionFirst:
		return "Primera"
	case PositionLast:
		return "Última"
	}
	return "Cualquiera"
}

// AllDays es la máscara de todos los días.
const AllDays = 127

// DaysLabel describe una máscara de días ("Todos", "Lun–Vie", "Lun, Mié, Vie").
func DaysLabel(mask int) string {
	switch mask & AllDays {
	case AllDays, 0:
		return "Todos"
	case 0b0111110:
		return "Lun–Vie"
	case 0b1000001:
		return "Sáb y Dom"
	}
	short := [7]string{"Dom", "Lun", "Mar", "Mié", "Jue", "Vie", "Sáb"}
	var out []string
	for _, d := range []int{1, 2, 3, 4, 5, 6, 0} { // lunes primero
		if mask&(1<<d) != 0 {
			out = append(out, short[d])
		}
	}
	return strings.Join(out, ", ")
}

// PriorityLabel devuelve la prioridad para la interfaz.
func PriorityLabel(p int) string {
	switch p {
	case 1:
		return "Alta"
	case 3:
		return "Baja"
	}
	return "Normal"
}

// FMAdvertiser es un anunciante. Sector (rubro) agrupa a los competidores:
// dos anunciantes del mismo rubro no suenan en la misma tanda.
type FMAdvertiser struct {
	ID        int64
	Name      string
	Sector    string
	CreatedAt time.Time
}

// FMCampaign es una campaña: las reglas de emisión de un anunciante. Sus
// versiones (audios) rotan entre sí; las veces por día son de la campaña.
type FMCampaign struct {
	ID           int64
	AdvertiserID int64
	Advertiser   string // nombre (se completa al leer)
	Sector       string // rubro del anunciante (se completa al leer)
	Name         string
	StartsAt     *time.Time
	EndsAt       *time.Time
	PlaysPerDay  int
	HourFrom     int
	HourTo       int
	DaysMask     int
	Position     SpotPosition
	Priority     int
	FrequencyID  int64
	Active       bool
	CreatedAt    time.Time
	UpdatedAt    time.Time
	Versions     []FMSpot // se completa al leer una campaña
}

// HoursLabel devuelve la franja ("Todo el día" o "07:00–13:00").
func (c FMCampaign) HoursLabel() string {
	return FMSpot{HourFrom: c.HourFrom, HourTo: c.HourTo}.HoursLabel()
}

// DaysLabel devuelve los días de emisión.
func (c FMCampaign) DaysLabel() string { return DaysLabel(c.DaysMask) }

// PriorityLabel devuelve la prioridad.
func (c FMCampaign) PriorityLabel() string { return PriorityLabel(c.Priority) }

// IsLive indica si la campaña está vigente en t (fechas y activa; no mira franja ni día).
func (c FMCampaign) IsLive(t time.Time) bool {
	return FMSpot{Active: c.Active, StartsAt: c.StartsAt, EndsAt: c.EndsAt}.IsLive(t)
}

// PlaysOn indica si la campaña debe sonar el día d (fecha local de la radio):
// activa, vigente ese día y el día de la semana está en la máscara.
func (c FMCampaign) PlaysOn(d time.Time) bool {
	if !c.Active || c.DaysMask&(1<<int(d.Weekday())) == 0 {
		return false
	}
	start := time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, d.Location())
	end := start.AddDate(0, 0, 1)
	if c.EndsAt != nil && !c.EndsAt.After(start) {
		return false
	}
	if c.StartsAt != nil && !c.StartsAt.Before(end) {
		return false
	}
	return true
}

// Apply copia las reglas de la campaña a una versión.
func (c FMCampaign) Apply(v *FMSpot) {
	v.CampaignID, v.Advertiser, v.Sector = c.ID, c.Advertiser, c.Sector
	v.StartsAt, v.EndsAt, v.PlaysPerDay, v.HourFrom, v.HourTo = c.StartsAt, c.EndsAt, c.PlaysPerDay, c.HourFrom, c.HourTo
	v.DaysMask, v.Position, v.Priority, v.FrequencyID = c.DaysMask, c.Position, c.Priority, c.FrequencyID
	if !c.Active {
		v.Active = false // una campaña pausada pausa sus versiones
	}
}

// FMAdvertiserRepository persiste los anunciantes.
type FMAdvertiserRepository interface {
	List(ctx context.Context) ([]FMAdvertiser, error)
	Get(ctx context.Context, id int64) (*FMAdvertiser, error)
	ByName(ctx context.Context, name string) (*FMAdvertiser, error)
	Save(ctx context.Context, a *FMAdvertiser) error // crea si ID = 0
	Delete(ctx context.Context, id int64) error      // ErrConflict si tiene campañas
}

// FMCampaignRepository persiste las campañas.
type FMCampaignRepository interface {
	List(ctx context.Context) ([]FMCampaign, error) // sin versiones
	Get(ctx context.Context, id int64) (*FMCampaign, error)
	Save(ctx context.Context, c *FMCampaign) error // crea si ID = 0
	Delete(ctx context.Context, id int64) error
	// SyncVersions copia las reglas de la campaña a todas sus versiones.
	SyncVersions(ctx context.Context, c FMCampaign) error
}
