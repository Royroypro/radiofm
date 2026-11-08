package onair

import (
	"time"

	"radioclub25/estudio/internal/audio"
	"radioclub25/estudio/internal/domain"
)

// Warning es un aviso reciente del aire.
type Warning struct {
	At      time.Time `json:"at"`
	Message string    `json:"message"`
}

// DeckView es lo que muestra la interfaz de un deck.
type DeckView struct {
	Playing   bool          `json:"playing"`
	Current   bool          `json:"current"` // es el que suena "al frente" (el último arrancado)
	Segment   *Segment      `json:"segment"`
	Position  time.Duration `json:"position"`
	Remaining time.Duration `json:"remaining"`
	IntroLeft time.Duration `json:"intro_left"` // tiempo de intro que queda (para hablar encima)
}

// Snapshot es la foto del aire para la interfaz y la sincronización.
type Snapshot struct {
	Running   bool                     `json:"running"`
	Mode      domain.StudioMode        `json:"mode"`
	Waiting   bool                     `json:"waiting"` // asistido: esperando al operador
	StopAfter bool                     `json:"stop_after"`
	Current   *domain.LogItem          `json:"current"`         // elemento de la playlist en curso
	NextBreak *time.Time               `json:"next_break"`      // hora prevista de la próxima tanda
	Remaining time.Duration            `json:"remaining_today"` // lo que queda programado hoy
	Decks     [audio.NumDecks]DeckView `json:"decks"`
	ItemID    int64                    `json:"item_id"`
	Next      []domain.LogItem         `json:"next"`
	Silence   time.Duration            `json:"silence_left"`
	Engine    audio.Status             `json:"engine"`
	CartsOn   []int                    `json:"carts_on"`
	Warnings  []Warning                `json:"warnings"`
	UpdatedAt time.Time                `json:"updated_at"`
}

// publish actualiza la foto (la llama Run tras cada evento, orden o tick).
func (s *Service) publish() {
	st := s.d.Engine.Status()
	var decks [audio.NumDecks]DeckView
	for slot := 0; slot < audio.NumDecks; slot++ {
		var a *active
		if s.current != nil && s.current.slot == slot {
			a = s.current
		}
		for _, f := range s.fading {
			if f.slot == slot && a == nil {
				a = f
			}
		}
		ds := st.Decks[slot]
		v := DeckView{Playing: ds.Playing, Position: ds.Position}
		if a != nil && uint64(a.playID) == ds.Tag {
			seg := a.seg
			v.Segment = &seg
			v.Current = s.current == a
			length := seg.Length
			if seg.Segue > 0 {
				length = seg.Segue
			}
			if length > 0 {
				v.Remaining = max(length-ds.Position, 0)
			}
			if seg.Intro > 0 {
				v.IntroLeft = max(seg.Intro-ds.Position, 0)
			}
		}
		decks[slot] = v
	}
	var next []domain.LogItem
	var nextBreak *time.Time
	var remaining time.Duration
	var current *domain.LogItem
	now := s.d.Now()
	today := now.In(s.d.Location()).Format(domain.DayLayout)
	for i, it := range s.upcoming {
		if it.ID == s.itemID {
			c := s.upcoming[i]
			current = &c
		}
	}
	for _, it := range s.pending() {
		if len(next) < 40 {
			next = append(next, it)
		}
		if nextBreak == nil && it.Kind == domain.SlotSpots && it.At.After(now) {
			at := it.At
			nextBreak = &at
		}
		if it.Day == today {
			remaining += it.Length
		}
	}
	silence := time.Duration(0)
	if !s.silenceAt.IsZero() {
		silence = max(s.silenceAt.Sub(s.d.Now()), 0)
	}
	s.mu.Lock()
	s.snap.Running, s.snap.Mode, s.snap.Waiting = s.running, s.mode, s.waiting
	s.snap.StopAfter, s.snap.Current, s.snap.NextBreak, s.snap.Remaining = s.stopAfter, current, nextBreak, remaining
	s.snap.Decks, s.snap.ItemID, s.snap.Next, s.snap.Silence = decks, s.itemID, next, silence
	s.snap.Engine, s.snap.CartsOn, s.snap.UpdatedAt = st, st.CartsOn, s.d.Now()
	s.mu.Unlock()
}

// Snapshot devuelve una copia de la foto del aire.
func (s *Service) Snapshot() Snapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	c := s.snap
	c.Warnings = append([]Warning(nil), s.snap.Warnings...)
	if c.Next == nil {
		c.Next = []domain.LogItem{}
	}
	if c.CartsOn == nil {
		c.CartsOn = []int{}
	}
	return c
}
