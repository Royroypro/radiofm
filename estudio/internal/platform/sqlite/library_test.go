package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"radioclub25/estudio/internal/domain"
)

func testDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := Open(context.Background(), filepath.Join(t.TempDir(), "estudio.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestOpenTwiceKeepsData(t *testing.T) {
	path := filepath.Join(t.TempDir(), "e.db")
	db, err := Open(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	_ = NewCategoryRepo(db).Create(context.Background(), &domain.Category{Name: "Salsa", Kind: domain.KindMusic})
	db.Close()
	db, err = Open(context.Background(), path) // migraciones ya aplicadas: no debe fallar
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	list, _ := NewCategoryRepo(db).List(context.Background())
	if len(list) != 1 {
		t.Fatalf("categorías = %d", len(list))
	}
}

func TestTracksAndCategories(t *testing.T) {
	ctx := context.Background()
	db := testDB(t)
	cats, tracks := NewCategoryRepo(db), NewTrackRepo(db)
	c := &domain.Category{Name: "Cumbia", Kind: domain.KindMusic, Color: "#00FF00"}
	if err := cats.Create(ctx, c); err != nil {
		t.Fatal(err)
	}
	if err := cats.Create(ctx, &domain.Category{Name: "Cumbia"}); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("nombre duplicado debe dar ErrConflict: %v", err)
	}
	tr := &domain.Track{Path: `C:\Musica\Grupo 5 - Motor.mp3`, Title: "Motor y motivo", Artist: "Grupo 5",
		CategoryID: c.ID, Duration: 3*time.Minute + 20*time.Second, Active: true}
	isNew, err := tracks.Upsert(ctx, tr)
	if err != nil || !isNew || tr.ID == 0 {
		t.Fatalf("Upsert = %v, %v", isNew, err)
	}
	// misma ruta con otra capitalización: actualiza, no duplica
	again := &domain.Track{Path: `c:\musica\GRUPO 5 - Motor.mp3`, Title: "Motor y Motivo", Artist: "Grupo 5",
		Duration: tr.Duration}
	if isNew, err := tracks.Upsert(ctx, again); err != nil || isNew || again.ID != tr.ID {
		t.Fatalf("re-Upsert = %v, %v (id %d)", isNew, err, again.ID)
	}
	if err := tracks.SetAnalysis(ctx, tr.ID, domain.Analysis{Loudness: -11.5, GainDB: -4.5, CueIn: 120 * time.Millisecond,
		CueOut: 3*time.Minute + 18*time.Second, SegueAt: 3*time.Minute + 12*time.Second}); err != nil {
		t.Fatal(err)
	}
	got, _ := tracks.Get(ctx, tr.ID)
	if !got.Analyzed || got.GainDB != -4.5 || got.CueIn != 120*time.Millisecond || got.Title != "Motor y Motivo" || got.CategoryID != c.ID {
		t.Fatalf("pista = %+v", got)
	}
	// analizada pero sin tempo: sigue pendiente solo para estimarlo
	if pend, _ := tracks.PendingAnalysis(ctx, 10); len(pend) != 1 || pend[0].TempoDone {
		t.Fatal("debe quedar pendiente el tempo")
	}
	if err := tracks.SetTempo(ctx, tr.ID, 124.5, 4); err != nil {
		t.Fatal(err)
	}
	if pend, _ := tracks.PendingAnalysis(ctx, 10); len(pend) != 0 {
		t.Fatal("no debe quedar pendiente")
	}
	// la energía puesta a mano no la pisa una nueva estimación
	got, _ = tracks.Get(ctx, tr.ID)
	got.Energy = 2
	_ = tracks.Update(ctx, got)
	_ = tracks.SetTempo(ctx, tr.ID, 130, 5)
	if got, _ = tracks.Get(ctx, tr.ID); got.BPM != 124.5 || got.Energy != 2 {
		t.Fatalf("tempo/energía = %v / %d", got.BPM, got.Energy)
	}
	if n, err := tracks.SetAttributes(ctx, []int64{tr.ID}, domain.TrackAttributes{Voice: "male", Language: "es"}); err != nil || n != 1 {
		t.Fatalf("edición masiva = %d, %v", n, err)
	}
	if got, _ = tracks.Get(ctx, tr.ID); got.Voice != "male" || got.Language != "es" || got.Energy != 2 {
		t.Fatalf("atributos = %+v", got)
	}
	list, total, err := tracks.List(ctx, domain.TrackFilter{Query: "motor", CategoryID: c.ID})
	if err != nil || total != 1 || len(list) != 1 {
		t.Fatalf("List = %d/%d, %v", len(list), total, err)
	}
	if _, total, _ := tracks.List(ctx, domain.TrackFilter{Query: "100%"}); total != 0 {
		t.Fatal("el % debe escaparse en la búsqueda")
	}
	if err := cats.Delete(ctx, c.ID); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("borrar categoría con pistas debe dar ErrConflict: %v", err)
	}

	plays := NewPlayRepo(db)
	p := &domain.Play{TrackID: tr.ID, Kind: domain.PlayMusic, Artist: "Grupo 5", Title: "Motor", StartedAt: time.Now(), UUID: "u-1"}
	if err := plays.Add(ctx, p); err != nil {
		t.Fatal(err)
	}
	if err := plays.Finish(ctx, p.ID, 3*time.Minute, true); err != nil {
		t.Fatal(err)
	}
	got, _ = tracks.Get(ctx, tr.ID)
	if got.PlayCount != 1 || got.LastPlayedAt == nil {
		t.Fatalf("la emisión debe actualizar la pista: %+v", got)
	}
	hist, _ := plays.Since(ctx, time.Now().Add(-time.Hour))
	if len(hist) != 1 || !hist[0].Complete || hist[0].Duration != 3*time.Minute {
		t.Fatalf("historial = %+v", hist)
	}
}

// Reanalizar no pisa las marcas que el operador ajustó a mano.
func TestSetAnalysisKeepsManualMarks(t *testing.T) {
	ctx := context.Background()
	r := NewTrackRepo(testDB(t))
	tr := &domain.Track{Path: "a.mp3", Title: "A", Duration: 3 * time.Minute, Active: true}
	if _, err := r.Upsert(ctx, tr); err != nil {
		t.Fatal(err)
	}
	auto := domain.Analysis{Loudness: -12, GainDB: -4, CueIn: time.Second, SegueAt: 170 * time.Second,
		Ending: domain.EndingFade, FadeOut: 8 * time.Second, Envelope: "abc"}
	if err := r.SetAnalysis(ctx, tr.ID, auto); err != nil {
		t.Fatal(err)
	}
	got, _ := r.Get(ctx, tr.ID)
	if got.CueIn != time.Second || got.Ending != domain.EndingFade || got.FadeOut != 8*time.Second {
		t.Fatalf("análisis automático = %+v", got)
	}
	got.CueIn, got.SegueAt, got.MarksManual = 2*time.Second, 160*time.Second, true
	if err := r.Update(ctx, got); err != nil {
		t.Fatal(err)
	}
	auto.GainDB, auto.Envelope = -3, "xyz"
	_ = r.SetAnalysis(ctx, tr.ID, auto)
	got, _ = r.Get(ctx, tr.ID)
	if got.CueIn != 2*time.Second || got.SegueAt != 160*time.Second || got.GainDB != -3 || got.Envelope != "xyz" {
		t.Fatalf("marcas manuales tras reanalizar = in %v segue %v gain %v env %q", got.CueIn, got.SegueAt, got.GainDB, got.Envelope)
	}
}
