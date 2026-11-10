package scheduler

import (
	"fmt"
	"testing"
	"time"

	"radioclub25/estudio/internal/domain"
)

var lima = time.FixedZone("PET", -5*3600)
var day = time.Date(2026, 11, 9, 0, 0, 0, 0, lima)

// songs crea n pistas de 3:30 (punto de mezcla a 3:27 con crossfade de 3 s) con artistas distintos.
func songs(cat int64, prefix string, n int) []domain.Track {
	var out []domain.Track
	for i := 1; i <= n; i++ {
		out = append(out, domain.Track{ID: cat*1000 + int64(i), Artist: fmt.Sprintf("%s artista %d", prefix, i),
			Title: fmt.Sprintf("%s tema %d", prefix, i), CategoryID: cat, Active: true, Duration: 3*time.Minute + 30*time.Second})
	}
	return out
}

func basePlan(clock domain.Clock) Plan {
	music := songs(1, "cumbia", 40)
	salsa := songs(2, "salsa", 40)
	filler := songs(9, "relleno", 20)
	byID := map[int64]domain.Track{}
	for _, t := range append(append(music, salsa...), filler...) {
		byID[t.ID] = t
	}
	return Plan{
		Day:        day,
		ClockFor:   func(int) int64 { return clock.ID },
		Clocks:     map[int64]domain.Clock{clock.ID: clock},
		ByCategory: map[int64][]domain.Track{1: music, 2: salsa, 9: filler},
		ByID:       byID,
		Filler:     9,
	}
}

func hourItems(items []domain.LogItem, h int) []domain.LogItem {
	var out []domain.LogItem
	for _, it := range items {
		if it.Hour == h {
			out = append(out, it)
		}
	}
	return out
}

func TestGenerateHardEventsAndFill(t *testing.T) {
	clock := domain.Clock{ID: 1, Name: "Musical", Slots: []domain.ClockSlot{
		{Kind: domain.SlotTime, Hard: true, Minute: 0},
		{Kind: domain.SlotCategory, CategoryID: 1},
		{Kind: domain.SlotCategory, CategoryID: 2},
		{Kind: domain.SlotSpots, Hard: true, Minute: 30},
		{Kind: domain.SlotCategory, CategoryID: 1},
	}}
	items := Generate(basePlan(clock), DefaultConfig())
	h10 := hourItems(items, 10)
	if len(h10) < 10 {
		t.Fatalf("la hora 10 tiene %d elementos", len(h10))
	}
	// la hora locutada abre la hora en punto
	if h10[0].Kind != domain.SlotTime || !h10[0].Hard || !h10[0].At.Equal(day.Add(10*time.Hour)) {
		t.Fatalf("primer elemento de las 10: %+v", h10[0])
	}
	// la tanda está exactamente a las 10:30 y antes se rellenó
	var tanda *domain.LogItem
	fillBefore := 0
	for i := range h10 {
		if h10[i].Kind == domain.SlotSpots {
			tanda = &h10[i]
			break
		}
		if h10[i].Note != "" && h10[i].Kind == domain.SlotCategory {
			fillBefore++
		}
	}
	if tanda == nil || !tanda.At.Equal(day.Add(10*time.Hour+30*time.Minute)) {
		t.Fatalf("tanda: %+v", tanda)
	}
	if fillBefore == 0 {
		t.Fatal("antes de la tanda debe haber relleno (2 canciones no llenan 30 min)")
	}
	// el último elemento de la hora termina cerca del final (dentro de la tolerancia + una canción)
	last := h10[len(h10)-1]
	endsAt := last.At.Add(last.Length)
	hourEnd := day.Add(11 * time.Hour)
	if endsAt.Before(hourEnd.Add(-30*time.Second)) || endsAt.After(hourEnd.Add(4*time.Minute)) {
		t.Fatalf("la hora termina a las %s", endsAt.Format("15:04:05"))
	}
	// el orden de horas previstas es creciente
	for i := 1; i < len(items); i++ {
		if items[i].At.Before(items[i-1].At) {
			t.Fatalf("horas no crecientes en %d: %s < %s", i, items[i].At, items[i-1].At)
		}
	}
}

func TestGenerateNoRepeatArtistBackToBack(t *testing.T) {
	clock := domain.Clock{ID: 1, Slots: []domain.ClockSlot{{Kind: domain.SlotCategory, CategoryID: 1}}}
	p := basePlan(clock)
	p.Filler = 0 // rellena con la categoría del reloj
	items := Generate(p, DefaultConfig())
	seen := map[int64]time.Time{}
	for i, it := range items {
		if i > 0 && it.Artist == items[i-1].Artist {
			t.Fatalf("artista repetido seguido en %d: %s", i, it.Artist)
		}
		if last, ok := seen[it.TrackID]; ok && it.At.Sub(last) < 2*time.Hour && it.Note == "" {
			t.Fatalf("tema %d repetido a los %s sin relajar reglas", it.TrackID, it.At.Sub(last))
		}
		seen[it.TrackID] = it.At
	}
	if n := len(hourItems(items, 0)); n < 16 || n > 19 {
		t.Fatalf("una hora de canciones de ~3:27 debe tener ~17 elementos, tiene %d", n)
	}
}

func TestGenerateKeepsLocked(t *testing.T) {
	clock := domain.Clock{ID: 1, Slots: []domain.ClockSlot{{Kind: domain.SlotCategory, CategoryID: 1}}}
	p := basePlan(clock)
	fixed := p.ByID[2005]
	p.Locked = []domain.LogItem{{Hour: 15, Seq: 400, Kind: domain.SlotTrack, TrackID: fixed.ID, Title: fixed.Title,
		Length: 3 * time.Minute, Locked: true}}
	items := Generate(p, DefaultConfig())
	h15 := hourItems(items, 15)
	if len(h15) == 0 || !h15[0].Locked || h15[0].TrackID != fixed.ID {
		t.Fatalf("el elemento bloqueado debe conservarse al inicio de su hora: %+v", h15[0])
	}
}

func TestGenerateWithoutClockUsesFiller(t *testing.T) {
	p := basePlan(domain.Clock{ID: 1})
	p.ClockFor = func(int) int64 { return 0 }
	items := Generate(p, DefaultConfig())
	if len(items) == 0 || items[0].TrackID/1000 != 9 {
		t.Fatalf("sin reloj debe sonar la categoría de relleno: %d elementos", len(items))
	}
}

// Con pistas muy cortas el día tiene miles de elementos: la generación debe seguir siendo rápida.
func TestGenerateManyShortTracksIsFast(t *testing.T) {
	clock := domain.Clock{ID: 1, Slots: []domain.ClockSlot{{Kind: domain.SlotCategory, CategoryID: 1}}}
	p := basePlan(clock)
	p.Filler = 0
	short := songs(1, "corto", 5)
	for i := range short {
		short[i].Duration = 20 * time.Second
	}
	p.ByCategory[1] = short
	start := time.Now()
	items := Generate(p, DefaultConfig())
	if el := time.Since(start); el > 2*time.Second {
		t.Fatalf("generar %d elementos tardó %s", len(items), el)
	}
	if len(items) < 1500 {
		t.Fatalf("elementos = %d", len(items))
	}
}

func TestRetime(t *testing.T) {
	items := []domain.LogItem{
		{Hour: 0, Length: time.Minute},
		{Hour: 0, Length: time.Minute},
		{Hour: 1, Length: time.Minute},
		{Hour: 1, Hard: true, At: day.Add(time.Hour + 30*time.Minute), Length: time.Minute},
		{Hour: 1, Length: time.Minute},
	}
	Retime(items, day)
	want := []time.Duration{0, time.Minute, time.Hour, time.Hour + 30*time.Minute, time.Hour + 31*time.Minute}
	for i, w := range want {
		if !items[i].At.Equal(day.Add(w)) {
			t.Fatalf("elemento %d a las %s, quería %s", i, items[i].At.Format("15:04"), day.Add(w).Format("15:04"))
		}
	}
}

func TestGenerateSatellite(t *testing.T) {
	clock := domain.Clock{ID: 1, Name: "Satélite", Slots: []domain.ClockSlot{
		{Kind: domain.SlotSatellite, URL: "https://red.pe/stream", Title: "Red Nacional"},
		{Kind: domain.SlotSpots, Hard: true, Minute: 20},
		{Kind: domain.SlotSpots, Hard: true, Minute: 50},
	}}
	items := hourItems(Generate(basePlan(clock), DefaultConfig()), 3)
	type want struct {
		kind  domain.SlotKind
		at    int // minuto
		lenMn int
	}
	exp := []want{{domain.SlotSatellite, 0, 20}, {domain.SlotSpots, 20, 2}, {domain.SlotSatellite, 22, 28},
		{domain.SlotSpots, 50, 2}, {domain.SlotSatellite, 52, 8}}
	if len(items) != len(exp) {
		t.Fatalf("elementos = %d, quería %d: %+v", len(items), len(exp), items)
	}
	h := day.Add(3 * time.Hour)
	for i, w := range exp {
		it := items[i]
		if it.Kind != w.kind || it.At.Sub(h) != time.Duration(w.at)*time.Minute || it.Length != time.Duration(w.lenMn)*time.Minute {
			t.Fatalf("elemento %d = %s %v +%v, quería %s min %d +%d min", i, it.Kind, it.At.Sub(h), it.Length, w.kind, w.at, w.lenMn)
		}
		if it.Kind == domain.SlotSatellite && (it.URL == "" || it.Title != "Red Nacional") {
			t.Fatalf("satélite sin URL o nombre: %+v", it)
		}
	}
}
