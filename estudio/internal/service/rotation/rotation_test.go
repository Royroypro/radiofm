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
