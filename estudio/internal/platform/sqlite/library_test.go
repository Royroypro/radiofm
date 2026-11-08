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
	if pend, _ := tracks.PendingAnalysis(ctx, 10); len(pend) != 0 {
		t.Fatal("no debe quedar pendiente")
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
