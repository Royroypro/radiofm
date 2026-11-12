package domain

import (
	"context"
	"encoding/json"
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
	StatusJSON     string // estado completo de la PC (StudioReport en JSON)
	NowPlaying     *NowPlaying
	NowPlayingAt   *time.Time
}

// StudioReport es el estado que envía la PC en cada heartbeat (ver Club25
// Estudio, app.StudioStatus). Todos los campos son opcionales.
type StudioReport struct {
	Queue      []QueueItem `json:"queue"` // lo que suena y lo que viene (hasta 15)
	Running    bool        `json:"running"`
	Waiting    bool        `json:"waiting"`
	Now        string      `json:"now"`
	NextBreak  *time.Time  `json:"next_break"`
	LeftToday  int         `json:"left_today_s"`
	SilenceFor int         `json:"silence_for_s"`
	AudioError string      `json:"audio_error"`
	Warnings   []string    `json:"warnings"`
	Web        struct {
		Enabled   bool   `json:"enabled"`
		Connected bool   `json:"connected"`
		Error     string `json:"error"`
	} `json:"web"`
	TV struct {
		Enabled   bool   `json:"enabled"`
		Connected bool   `json:"connected"`
		Error     string `json:"error"`
	} `json:"tv"`
	Logger struct {
		Enabled   bool   `json:"enabled"`
		Recording bool   `json:"recording"`
		Error     string `json:"error"`
	} `json:"logger"`
	RDS struct {
		Enabled   bool   `json:"enabled"`
		Connected bool   `json:"connected"`
		Error     string `json:"error"`
	} `json:"rds"`
	Update struct {
		Latest    string `json:"latest"`
		Available bool   `json:"available"`
	} `json:"update"`
	BackupAt *time.Time `json:"backup_at"`
}

// QueueItem es un elemento de la playlist de la PC (para el control remoto).
type QueueItem struct {
	ID      int64     `json:"id"`
	At      time.Time `json:"at"`
	Title   string    `json:"title"`
	Artist  string    `json:"artist"`
	Kind    string    `json:"kind"`
	Hard    bool      `json:"hard"`
	Playing bool      `json:"playing"`
}

// Report devuelve el estado completo de la PC (vacío si nunca lo envió).
func (s StudioState) Report() StudioReport {
	var r StudioReport
	if s.StatusJSON != "" {
		_ = json.Unmarshal([]byte(s.StatusJSON), &r)
	}
	return r
}

// AlarmLevel es la gravedad de una alarma.
type AlarmLevel string

const (
	AlarmCritical AlarmLevel = "critical" // el aire está en riesgo
	AlarmWarning  AlarmLevel = "warning"  // algo no anda, pero el aire sigue
)

// Alarm es un problema de la PC del estudio para el panel.
type Alarm struct {
	Level AlarmLevel
	Text  string
}

// Alarms arma las alarmas de la PC a partir de su última señal. online es el
// máximo sin señal para considerarla conectada. hasPC = la frecuencia tiene token.
func (s StudioState) Alarms(now time.Time, online time.Duration) []Alarm {
	if !s.HasToken() {
		return nil
	}
	var out []Alarm
	add := func(l AlarmLevel, t string) { out = append(out, Alarm{l, t}) }
	if s.LastSeen == nil {
		return []Alarm{{AlarmWarning, "La PC todavía no se conectó"}}
	}
	if !s.Online(now, online) {
		add(AlarmCritical, "Sin señal de la PC desde "+s.LastSeen.Local().Format("02/01 15:04")+
			" (apagada, sin internet o el programa cerrado)")
		return out
	}
	r := s.Report()
	if s.StatusJSON == "" || s.StatusJSON == "{}" {
		return out // PC con versión anterior: solo se sabe si está conectada
	}
	if !r.Running && s.Mode != StudioLive {
		add(AlarmCritical, "La automatización está detenida")
	}
	if r.SilenceFor >= 20 {
		add(AlarmCritical, "Silencio al aire hace "+humanSecs(r.SilenceFor))
	}
	if r.AudioError != "" {
		add(AlarmCritical, "Audio: "+r.AudioError)
	}
	if r.Running && r.LeftToday < 30*60 {
		add(AlarmWarning, "Queda poca playlist hoy ("+humanSecs(r.LeftToday)+"): revise la parrilla")
	}
	if r.Web.Enabled && !r.Web.Connected {
		add(AlarmWarning, "La radio no llega a la web"+suffix(r.Web.Error))
	}
	if r.TV.Enabled && !r.TV.Connected {
		add(AlarmWarning, "La TV no llega a la web"+suffix(r.TV.Error))
	}
	if r.Logger.Enabled && !r.Logger.Recording {
		add(AlarmWarning, "El grabador de la emisión no está grabando"+suffix(r.Logger.Error))
	}
	if r.RDS.Enabled && !r.RDS.Connected {
		add(AlarmWarning, "RDS sin conexión"+suffix(r.RDS.Error))
	}
	if r.BackupAt == nil || now.Sub(*r.BackupAt) > 48*time.Hour {
		add(AlarmWarning, "Sin respaldo de la base del Estudio en el servidor hace más de 2 días")
	}
	return out
}

func suffix(s string) string {
	if s == "" {
		return ""
	}
	return ": " + s
}

func humanSecs(n int) string {
	switch {
	case n < 120:
		return fmt.Sprintf("%d s", n)
	case n < 2*3600:
		return fmt.Sprintf("%d min", n/60)
	}
	return fmt.Sprintf("%d h %02d min", n/3600, n%3600/60)
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
	Heartbeat(ctx context.Context, frequencyID int64, at time.Time, version string, mode StudioMode, encoderJSON, statusJSON string) error
	SetNowPlaying(ctx context.Context, frequencyID int64, np NowPlaying, at time.Time) error

	// Control remoto (órdenes del panel a la PC).
	AddCommand(ctx context.Context, c *StudioCommand) error
	// TakeCommands devuelve las órdenes sin tomar creadas desde since y las marca tomadas.
	TakeCommands(ctx context.Context, frequencyID int64, since, at time.Time) ([]StudioCommand, error)
	FinishCommand(ctx context.Context, frequencyID, id int64, ok bool, result string, at time.Time) error
	RecentCommands(ctx context.Context, frequencyID int64, limit int) ([]StudioCommand, error)
}

// Órdenes de control remoto que entiende la PC del estudio.
const (
	CmdNext   = "next"   // pasar a lo siguiente
	CmdStart  = "start"  // iniciar la automatización
	CmdStop   = "stop"   // detenerla
	CmdMode   = "mode"   // Arg: auto | assisted | live
	CmdMove   = "move"   // Arg: "<id del elemento> <-1|1>"
	CmdRemove = "remove" // Arg: id del elemento
	CmdTV     = "tv"     // Arg: on | off
)

// StudioCommand es una orden del panel a la PC del estudio.
type StudioCommand struct {
	ID          int64      `json:"id"`
	FrequencyID int64      `json:"-"`
	Kind        string     `json:"kind"`
	Arg         string     `json:"arg"`
	CreatedAt   time.Time  `json:"created_at"`
	CreatedBy   string     `json:"-"`
	TakenAt     *time.Time `json:"-"`
	DoneAt      *time.Time `json:"-"`
	OK          bool       `json:"-"`
	Result      string     `json:"-"`
}

// Label describe la orden para el panel.
func (c StudioCommand) Label() string {
	switch c.Kind {
	case CmdNext:
		return "Siguiente"
	case CmdStart:
		return "Iniciar"
	case CmdStop:
		return "Detener"
	case CmdMode:
		return "Modo " + StudioMode(c.Arg).Label()
	case CmdMove:
		return "Mover elemento"
	case CmdRemove:
		return "Quitar elemento"
	case CmdTV:
		if c.Arg == "on" {
			return "TV al aire"
		}
		return "Apagar TV"
	}
	return c.Kind
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
