package domain

import (
	"context"
	"time"
)

// AdType es el tipo de anuncio.
type AdType string

const (
	// AdBanner es una imagen con enlace que se muestra en una posición de la web.
	AdBanner AdType = "banner"
	// AdPrerollVideo es un video corto que se reproduce antes de la TV en vivo.
	AdPrerollVideo AdType = "preroll_video"
	// AdAudioSpot es una cuña de audio que suena antes de conectar la radio.
	AdAudioSpot AdType = "audio_spot"
)

// Valid indica si el tipo es conocido.
func (t AdType) Valid() bool { return t == AdBanner || t == AdPrerollVideo || t == AdAudioSpot }

// Label devuelve el nombre del tipo para la interfaz.
func (t AdType) Label() string {
	switch t {
	case AdBanner:
		return "Banner"
	case AdPrerollVideo:
		return "Pre-roll de video"
	case AdAudioSpot:
		return "Cuña de audio"
	}
	return string(t)
}

// AdPosition es la ubicación de un banner en la web pública.
type AdPosition string

const (
	PosHeader  AdPosition = "header"  // bajo la cabecera
	PosSidebar AdPosition = "sidebar" // columna lateral junto a los reproductores
	PosBetween AdPosition = "between" // entre secciones
	PosFooter  AdPosition = "footer"  // sobre el pie de página
)

// AdPositions lista las posiciones válidas en el orden de la página.
var AdPositions = []AdPosition{PosHeader, PosSidebar, PosBetween, PosFooter}

// Valid indica si la posición es conocida.
func (p AdPosition) Valid() bool {
	for _, x := range AdPositions {
		if x == p {
			return true
		}
	}
	return false
}

// Label devuelve el nombre de la posición para la interfaz.
func (p AdPosition) Label() string {
	switch p {
	case PosHeader:
		return "Cabecera"
	case PosSidebar:
		return "Lateral"
	case PosBetween:
		return "Entre secciones"
	case PosFooter:
		return "Pie de página"
	}
	return string(p)
}

// Ad es un anuncio publicitario.
type Ad struct {
	ID           int64
	Name         string
	Type         AdType
	Position     AdPosition // solo para banners
	FileURL      string     // ruta pública del archivo (/uploads/...)
	LinkURL      string     // destino al hacer clic (opcional)
	StartsAt     *time.Time // nil = sin fecha de inicio
	EndsAt       *time.Time // nil = sin fecha de fin
	Weight       int        // probabilidad relativa frente a otros anuncios del mismo hueco (1-100)
	SkipAfterSec int        // segundos tras los que se puede saltar (pre-roll/cuña). 0 = no se puede saltar
	Active       bool
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// IsLive indica si el anuncio debe mostrarse en el instante t.
func (a Ad) IsLive(t time.Time) bool {
	if !a.Active {
		return false
	}
	if a.StartsAt != nil && t.Before(*a.StartsAt) {
		return false
	}
	if a.EndsAt != nil && !t.Before(*a.EndsAt) {
		return false
	}
	return true
}

// AdFilter filtra la lista de anuncios. Los campos vacíos no filtran.
type AdFilter struct {
	Type       AdType
	Position   AdPosition
	OnlyActive bool
}

// AdRepository persiste anuncios.
type AdRepository interface {
	Create(ctx context.Context, a *Ad) error
	Get(ctx context.Context, id int64) (*Ad, error)
	List(ctx context.Context, f AdFilter) ([]Ad, error)
	Update(ctx context.Context, a *Ad) error
	Delete(ctx context.Context, id int64) error
	CountActive(ctx context.Context) (int, error)
}

// AdEventType es el tipo de evento medido sobre un anuncio.
type AdEventType string

const (
	EventImpression AdEventType = "impression" // el anuncio se vio / empezó a reproducirse
	EventClick      AdEventType = "click"      // clic hacia el enlace del anunciante
	EventComplete   AdEventType = "complete"   // pre-roll/cuña reproducido hasta el final
	EventSkip       AdEventType = "skip"       // pre-roll/cuña saltado por el usuario
)

// Valid indica si el tipo de evento es conocido.
func (t AdEventType) Valid() bool {
	return t == EventImpression || t == EventClick || t == EventComplete || t == EventSkip
}

// AdEvent es un evento de medición. IPHash es un hash de la IP (nunca la IP en claro).
type AdEvent struct {
	AdID      int64
	Type      AdEventType
	IPHash    string
	CreatedAt time.Time
}

// AdStats son los totales de un anuncio en un rango de fechas.
type AdStats struct {
	AdID        int64
	Name        string
	Type        AdType
	Impressions int64
	Clicks      int64
	Completes   int64
	Skips       int64
}

// CTR es el porcentaje de clics sobre impresiones.
func (s AdStats) CTR() float64 {
	if s.Impressions == 0 {
		return 0
	}
	return float64(s.Clicks) * 100 / float64(s.Impressions)
}

// CompletionRate es el porcentaje de reproducciones completas sobre impresiones.
func (s AdStats) CompletionRate() float64 {
	if s.Impressions == 0 {
		return 0
	}
	return float64(s.Completes) * 100 / float64(s.Impressions)
}

// AdEventRepository persiste eventos y calcula estadísticas.
type AdEventRepository interface {
	Record(ctx context.Context, e AdEvent) error
	Stats(ctx context.Context, from, to time.Time) ([]AdStats, error)
}
