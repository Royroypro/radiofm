package domain

import (
	"context"
	"time"
)

// Spot es una cuña para la emisión. Normalmente viene del panel web
// (RemoteID > 0, ver service/sync); también se pueden crear cuñas locales
// (RemoteID = 0) para cuando no hay conexión.
type Spot struct {
	ID          int64         `json:"id"`
	RemoteID    int64         `json:"remote_id"` // id en el panel web (0 = local)
	Advertiser  string        `json:"advertiser"`
	Name        string        `json:"name"`
	Path        string        `json:"path"` // archivo local (descargado o elegido)
	SHA256      string        `json:"sha256"`
	Duration    time.Duration `json:"duration"`
	StartsAt    *time.Time    `json:"starts_at"`
	EndsAt      *time.Time    `json:"ends_at"`
	PlaysPerDay int           `json:"plays_per_day"`
	HourFrom    int           `json:"hour_from"` // franja [HourFrom, HourTo) en hora de la radio
	HourTo      int           `json:"hour_to"`
	Active      bool          `json:"active"`
	UpdatedAt   time.Time     `json:"updated_at"`

	// Tráfico (campañas del panel web, 1.2): una cuña es una versión de una
	// campaña; las veces por día son de la campaña y sus versiones rotan.
	CampaignID int64  `json:"campaign_id"` // 0 = la cuña es su propia campaña
	Weight     int    `json:"weight"`      // peso en la rotación de versiones
	DaysMask   int    `json:"days_mask"`   // bit 0 = domingo … bit 6 = sábado (0 = todos)
	Position   string `json:"position"`    // any | first | last
	Priority   int    `json:"priority"`    // 1 alta, 2 normal, 3 baja (0 = normal)
	Sector     string `json:"sector"`      // rubro: dos del mismo rubro no van en la misma tanda
}

// CampaignKey identifica la campaña de la cuña (las locales son su propia campaña).
func (s Spot) CampaignKey() int64 {
	if s.CampaignID > 0 {
		return s.CampaignID
	}
	return -s.ID // negativo: no choca con IDs de campaña
}

// OnDay indica si la cuña puede sonar ese día de la semana.
func (s Spot) OnDay(d time.Weekday) bool { return s.DaysMask == 0 || s.DaysMask&(1<<int(d)) != 0 }

// Prio devuelve la prioridad (1 alta … 3 baja; sin dato = 2).
func (s Spot) Prio() int {
	if s.Priority < 1 || s.Priority > 3 {
		return 2
	}
	return s.Priority
}

// LiveAt indica si la cuña puede sonar en t (loc = zona de la radio para la franja).
func (s Spot) LiveAt(t time.Time, loc *time.Location) bool {
	if !s.Active || s.Path == "" {
		return false
	}
	if s.StartsAt != nil && t.Before(*s.StartsAt) {
		return false
	}
	if s.EndsAt != nil && !t.Before(*s.EndsAt) {
		return false
	}
	local := t.In(loc)
	if !s.OnDay(local.Weekday()) {
		return false
	}
	h := local.Hour()
	from, to := s.HourFrom, s.HourTo
	if to == 0 {
		to = 24
	}
	return h >= from && h < to
}

// SpotRepository persiste las cuñas locales.
type SpotRepository interface {
	List(ctx context.Context) ([]Spot, error)
	Get(ctx context.Context, id int64) (*Spot, error)
	ByRemoteID(ctx context.Context, remoteID int64) (*Spot, error)
	Save(ctx context.Context, s *Spot) error // crea si ID = 0
	Delete(ctx context.Context, id int64) error
}

// TandaSpot es una cuña planificada en una tanda (un LogItem de tipo SlotSpots).
type TandaSpot struct {
	LogItemID int64 `json:"log_item_id"`
	Position  int   `json:"position"`
	SpotID    int64 `json:"spot_id"`
}

// TandaRepository guarda el contenido planificado de las tandas.
type TandaRepository interface {
	ForItems(ctx context.Context, logItemIDs []int64) (map[int64][]int64, error) // tanda → cuñas en orden
	Set(ctx context.Context, logItemID int64, spotIDs []int64) error
}
