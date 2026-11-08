package onair

import (
	"context"
	"fmt"
	"sync"
	"time"

	"radioclub25/estudio/internal/audio"
	"radioclub25/estudio/internal/domain"
)

// MaxCartLength es lo máximo que se carga en memoria por botón.
const MaxCartLength = 30 * time.Second

// cartDeck maneja la botonera: decodifica cada audio una vez (en memoria) y lo
// dispara en los slots de carts del motor. Varios botones pueden sonar a la vez.
type cartDeck struct {
	d     Deps
	repo  domain.CartRepository
	mu    sync.Mutex
	cache map[string][]float32 // ruta → muestras
	byKey map[int]int          // página*24+slot → slot del motor
}

func newCartDeck(d Deps) *cartDeck {
	return &cartDeck{d: d, cache: map[string][]float32{}, byKey: map[int]int{}}
}

// SetCartRepository conecta la botonera con su persistencia.
func (s *Service) SetCartRepository(r domain.CartRepository) { s.carts.repo = r }

// Carts devuelve los botones de una página.
func (s *Service) Carts(ctx context.Context, page int) ([]domain.Cart, error) {
	if s.carts.repo == nil {
		return nil, nil
	}
	return s.carts.repo.Page(ctx, page)
}

// SaveCart guarda un botón (Path vacío lo borra) y precarga su audio.
func (s *Service) SaveCart(ctx context.Context, c domain.Cart) error {
	if c.Page < 0 || c.Page > 9 || c.Slot < 0 || c.Slot >= domain.CartsPerPage {
		return domain.Invalid("slot", "Botón fuera de rango")
	}
	if c.Path != "" {
		if _, err := s.carts.load(ctx, c.Path); err != nil {
			return domain.Invalid("path", "No se pudo leer el audio: "+err.Error())
		}
	}
	return s.carts.repo.Save(ctx, c)
}

func (c *cartDeck) load(ctx context.Context, path string) ([]float32, error) {
	c.mu.Lock()
	if b, ok := c.cache[path]; ok {
		c.mu.Unlock()
		return b, nil
	}
	c.mu.Unlock()
	src, err := c.d.Decode(ctx, path, 0)
	if err != nil {
		return nil, err
	}
	samples, err := audio.ReadAll(src, audio.DurationToFrames(MaxCartLength))
	if err != nil {
		return nil, err
	}
	c.mu.Lock()
	c.cache[path] = samples
	c.mu.Unlock()
	return samples, nil
}

// trigger dispara (o detiene) un botón.
func (c *cartDeck) trigger(ctx context.Context, page, slot int) error {
	if c.repo == nil {
		return fmt.Errorf("botonera no configurada")
	}
	list, err := c.repo.Page(ctx, page)
	if err != nil {
		return err
	}
	var cart *domain.Cart
	for i := range list {
		if list[i].Slot == slot {
			cart = &list[i]
		}
	}
	if cart == nil {
		return nil // botón vacío
	}
	key := page*domain.CartsPerPage + slot
	engSlot := audio.CartSlot(slot)
	if c.d.Engine.Slot(engSlot).Playing && c.byKey[key] == engSlot {
		if cart.StopOnPush {
			return c.d.Engine.Stop(engSlot)
		}
	}
	samples, err := c.load(ctx, cart.Path)
	if err != nil {
		return err
	}
	if err := c.d.Engine.Load(engSlot, audio.NewMemorySource(samples), audio.LoadOptions{Tag: uint64(key + 1),
		BufferAhead: MaxCartLength + time.Second}); err != nil {
		return err
	}
	c.byKey[key] = engSlot
	return c.d.Engine.Play(engSlot, 0)
}

func (c *cartDeck) stopAll() {
	for i := 0; i < audio.NumCarts; i++ {
		_ = c.d.Engine.Stop(audio.CartSlot(i))
	}
}

func (c *cartDeck) event(audio.Event) {}
