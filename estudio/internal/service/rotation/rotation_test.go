package rotation

import (
	"testing"
	"time"

	"radioclub25/estudio/internal/domain"
)

var t0 = time.Date(2026, 11, 8, 10, 0, 0, 0, time.UTC)

func track(id int64, artist, title string) domain.Track {
	return domain.Track{ID: id, Artist: artist, Title: title, Active: true, Duration: 3 * time.Minute}
}

func ago(d time.Duration) time.Time { return t0.Add(-d) }

var rules = Rules{ArtistSeparation: 45 * time.Minute, TitleSeparation: 3 * time.Hour}

func TestPicksLeastRecent(t *testing.T) {
	cands := []domain.Track{track(1, "A", "a1"), track(2, "B", "b1"), track(3, "C", "c1")}
	hist := []Use{{TrackID: 1, Artist: "A", Title: "a1", At: ago(5 * time.Hour)}, {TrackID: 3, Artist: "C", Title: "c1", At: ago(4 * time.Hour)}}
	r := Pick(cands, hist, "", t0, rules)
	if !r.OK || r.Track.ID != 2 || r.Relaxed != "" {
		t.Fatalf("debe elegir la nunca usada (2): %+v", r)
	}
	hist = append(hist, Use{TrackID: 2, Artist: "B", Title: "b1", At: ago(4*time.Hour + time.Minute)})
	if r := Pick(cands, hist, "", t0, rules); r.Track.ID != 1 {
		t.Fatalf("debe elegir la menos reciente (1): %+v", r)
	}
}

func TestArtistSeparationAndFeat(t *testing.T) {
	cands := []domain.Track{track(1, "Grupo 5 feat. Daniela Darcourt", "x"), track(2, "Agua Marina", "y")}
	hist := []Use{{TrackID: 9, Artist: "Daniela Darcourt", Title: "z", At: ago(20 * time.Minute)},
		{TrackID: 2, Artist: "Agua Marina", Title: "y", At: ago(4 * time.Hour)}}
	r := Pick(cands, hist, "", t0, rules)
	if r.Track.ID != 2 {
		t.Fatalf("Darcourt sonó hace 20 min: debe evitarse aunque venga como feat: %+v", r)
	}
}

func TestPrevArtistAndAccents(t *testing.T) {
	cands := []domain.Track{track(1, "Armonía 10", "a"), track(2, "Corazón Serrano", "b")}
	r := Pick(cands, nil, "ARMONIA 10", t0, Rules{})
	if r.Track.ID != 2 {
		t.Fatalf("no debe repetir el intérprete del slot anterior (sin importar tildes): %+v", r)
	}
}

func TestRelaxationOrder(t *testing.T) {
	cands := []domain.Track{track(1, "A", "a1"), track(2, "A", "a2")}
	// A sonó hace 10 min (con a1): separación de artista imposible; a2 respeta la de título.
	hist := []Use{{TrackID: 1, Artist: "A", Title: "a1", At: ago(10 * time.Minute)}}
	r := Pick(cands, hist, "", t0, rules)
	if r.Track.ID != 2 || r.Relaxed != "separación de artista" {
		t.Fatalf("debe relajar primero la separación de artista: %+v", r)
	}
	// solo queda la misma canción que acaba de sonar y es el intérprete anterior: se relaja todo
	r = Pick(cands[:1], hist, "A", t0, rules)
	if !r.OK || r.Relaxed != "todas las reglas" {
		t.Fatalf("sin alternativa debe relajar todo: %+v", r)
	}
}

func TestUnavailableAndFuture(t *testing.T) {
	inactive := track(1, "A", "a")
	inactive.Active = false
	expired := track(2, "B", "b")
	end := ago(time.Hour)
	expired.ValidTo = &end
	ok := track(3, "C", "c")
	if r := Pick([]domain.Track{inactive, expired}, nil, "", t0, rules); r.OK {
		t.Fatal("inactivas o vencidas no se eligen")
	}
	// un uso planificado en el futuro no cuenta como reciente
	hist := []Use{{TrackID: 3, Artist: "C", Title: "c", At: t0.Add(time.Hour)}}
	if r := Pick([]domain.Track{ok}, hist, "", t0, rules); r.Relaxed != "" {
		t.Fatalf("un uso futuro no debe relajar reglas: %+v", r)
	}
}

func TestArtistsSplit(t *testing.T) {
	got := Artists("Grupo 5 ft. Ezio Oliva & Los Ardiles")
	if len(got) != 3 || got[0] != " grupo 5 " && got[0] != "grupo 5" {
		t.Fatalf("Artists = %q", got)
	}
}

func attrs(id int64, artist string, voice string, energy int, bpm float64, genre, year string) domain.Track {
	t := track(id, artist, "t"+artist)
	t.Voice, t.Energy, t.BPM, t.Genre, t.Year = voice, energy, bpm, genre, year
	return t
}

// Reglas de flujo: voz, salto de energía, salto de tempo, rachas de género y década.
func TestFlowRules(t *testing.T) {
	prev := attrs(100, "Previo", "female", 2, 90, "Cumbia", "1995")
	r := Rules{NoSameVoice: true, MaxEnergyJump: 1, MaxTempoJump: 20, MaxGenreRun: 2, MaxEraRun: 3}
	// la menos reciente (1) rompe la regla de voz; 2 sí la cumple
	cands := []domain.Track{attrs(1, "A", "female", 2, 92, "Salsa", "2001"), attrs(2, "B", "male", 2, 92, "Salsa", "2001")}
	if got := PickFlow(cands, nil, []domain.Track{prev}, t0, r); got.Track.ID != 2 || got.Relaxed != "" {
		t.Fatalf("voz: %+v", got)
	}
	// energía 2 → 5 es un salto de 3 (máximo 1)
	cands = []domain.Track{attrs(1, "A", "male", 5, 92, "Salsa", "2001"), attrs(2, "B", "male", 3, 92, "Salsa", "2001")}
	if got := PickFlow(cands, nil, []domain.Track{prev}, t0, r); got.Track.ID != 2 {
		t.Fatalf("energía: %+v", got)
	}
	// tempo 90 → 140 es +55 % (máximo 20 %)
	cands = []domain.Track{attrs(1, "A", "male", 2, 140, "Salsa", "2001"), attrs(2, "B", "male", 2, 100, "Salsa", "2001")}
	if got := PickFlow(cands, nil, []domain.Track{prev}, t0, r); got.Track.ID != 2 {
		t.Fatalf("tempo: %+v", got)
	}
	// ya van 2 cumbias seguidas: la tercera no
	two := []domain.Track{attrs(101, "X", "", 0, 0, "Cumbia", ""), prev}
	cands = []domain.Track{attrs(1, "A", "male", 2, 92, "cumbia ", "2001"), attrs(2, "B", "male", 2, 92, "Salsa", "2001")}
	if got := PickFlow(cands, nil, two, t0, r); got.Track.ID != 2 {
		t.Fatalf("racha de género: %+v", got)
	}
	// sin atributos cargados, las reglas de flujo no estorban
	plain := []domain.Track{track(1, "A", "a"), track(2, "B", "b")}
	if got := PickFlow(plain, nil, []domain.Track{prev}, t0, r); got.Track.ID != 1 || got.Relaxed != "" {
		t.Fatalf("sin atributos: %+v", got)
	}
}

// Si ninguna cumple, se relaja primero la regla de flujo menos importante,
// antes que las separaciones de artista y título.
func TestFlowRelaxationOrder(t *testing.T) {
	prev := attrs(100, "Previo", "female", 1, 90, "Cumbia", "1995")
	r := Rules{ArtistSeparation: 45 * time.Minute, NoSameVoice: true, MaxEnergyJump: 1, MaxEraRun: 1}
	// la única candidata es femenina, energía 5 y de la misma década: hay que relajar
	// década → energía → voz (de la menos a la más importante)
	cands := []domain.Track{attrs(1, "A", "female", 5, 0, "", "1998")}
	got := PickFlow(cands, nil, []domain.Track{prev}, t0, r)
	if !got.OK || got.Relaxed != "voz, energía, racha de década" {
		t.Fatalf("relajación = %q", got.Relaxed)
	}
	// si basta con relajar la década, se relaja solo esa
	cands = []domain.Track{attrs(1, "A", "male", 1, 0, "", "1998")}
	if got = PickFlow(cands, nil, []domain.Track{prev}, t0, r); got.Relaxed != "racha de década" {
		t.Fatalf("relajación = %q", got.Relaxed)
	}
}
