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
	h := t.In(loc).Hour()
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
