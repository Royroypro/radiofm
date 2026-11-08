package domain

import (
	"context"
	"time"
)

// Frequency es una frecuencia de la radio en una ciudad ("93.3 FM" en Huacho).
// La radio puede tener varias (repetidoras). La primera activa es la principal.
type Frequency struct {
	ID        int64
	City      string // Huacho
	Frequency string // 93.3
	Band      string // FM | AM
	SortOrder int    // orden en la web (menor primero)
	Active    bool
	CreatedAt time.Time
}

// Label devuelve "93.3 FM".
func (f Frequency) Label() string {
	if f.Band == "" {
		return f.Frequency + " FM"
	}
	return f.Frequency + " " + f.Band
}

// FrequencyRepository persiste las frecuencias.
type FrequencyRepository interface {
	List(ctx context.Context, onlyActive bool) ([]Frequency, error) // por SortOrder
	Get(ctx context.Context, id int64) (*Frequency, error)
	Create(ctx context.Context, f *Frequency) error
	Update(ctx context.Context, f *Frequency) error
	Delete(ctx context.Context, id int64) error
}
