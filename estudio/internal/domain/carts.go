package domain

import "context"

// Cart es un botón de la botonera: dispara un audio corto al instante
// (jingle, efecto, cortina). Páginas de 4 × 6 = 24 botones.
type Cart struct {
	Page       int    `json:"page"` // 0…9
	Slot       int    `json:"slot"` // 0…23 (F1–F12 = 0–11)
	Label      string `json:"label"`
	Color      string `json:"color"`
	Path       string `json:"path"`         // archivo de audio (máx. 30 s en memoria)
	StopOnPush bool   `json:"stop_on_push"` // pulsar de nuevo lo detiene (si no, vuelve a empezar)
}

// CartsPerPage es el número de botones por página.
const CartsPerPage = 24

// CartRepository persiste la botonera.
type CartRepository interface {
	Page(ctx context.Context, page int) ([]Cart, error)
	Save(ctx context.Context, c Cart) error // Path vacío = borrar el botón
}
