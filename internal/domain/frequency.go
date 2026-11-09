package domain

import (
	"context"
	"time"
)

// Frequency es una frecuencia de la radio en una ciudad ("93.3 FM" en Huacho).
// Cada frecuencia es una emisora: tiene su propio audio en la web, su propia
// PC del estudio y puede tener programación y cuñas propias. La primera
// activa (menor SortOrder) es la principal.
type Frequency struct {
	ID        int64
	City      string // Huacho
	Frequency string // 93.3
	Band      string // FM | AM
	SortOrder int    // orden en la web (menor primero)
	Active    bool
	Source    StreamSource // de dónde sale el audio para la web
	Mount     string       // montaje en nuestro Icecast (/radio, /radio-barranca) si Source = local
	StreamURL string       // URL del servidor externo si Source = external
	CreatedAt time.Time
}

// StreamSource indica de dónde sale el audio de una frecuencia.
type StreamSource string

const (
	SourceLocal    StreamSource = "local"    // montaje propio en nuestro Icecast
	SourceExternal StreamSource = "external" // otro servidor de streaming
)

// Label devuelve "93.3 FM".
func (f Frequency) Label() string {
	if f.Band == "" {
		return f.Frequency + " FM"
	}
	return f.Frequency + " " + f.Band
}

// IsExternal indica si el audio sale de otro servidor.
func (f Frequency) IsExternal() bool { return f.Source == SourceExternal }

// ListenURL es la dirección que usa el reproductor de la web: el montaje
// (servido por nginx en el mismo dominio) o la URL externa.
func (f Frequency) ListenURL() string {
	if f.IsExternal() {
		return f.StreamURL
	}
	return f.Mount
}

// FrequencyUsage cuenta lo que está asignado solo a una frecuencia.
type FrequencyUsage struct {
	Programs int
	Spots    int
}

// FrequencyRepository persiste las frecuencias.
type FrequencyRepository interface {
	List(ctx context.Context, onlyActive bool) ([]Frequency, error) // por SortOrder
	Get(ctx context.Context, id int64) (*Frequency, error)
	Create(ctx context.Context, f *Frequency) error
	Update(ctx context.Context, f *Frequency) error
	Delete(ctx context.Context, id int64) error
	Usage(ctx context.Context, id int64) (FrequencyUsage, error)
}
