package spots_test

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"path/filepath"
	"testing"
	"time"

	"radioclub25/estudio/internal/domain"
	"radioclub25/estudio/internal/platform/ffmpeg"
	"radioclub25/estudio/internal/platform/sqlite"
	"radioclub25/estudio/internal/service/scheduler"
	"radioclub25/estudio/internal/service/spots"
)

var lima = time.FixedZone("PET", -5*3600)

type probe20 struct{}

func (probe20) Probe(context.Context, string) (ffmpeg.Info, error) {
	return ffmpeg.Info{Duration: 20 * time.Second}, nil
}

func TestReplanTomorrow(t *testing.T) {
	ctx := context.Background()
	db, err := sqlite.Open(ctx, filepath.Join(t.TempDir(), "e.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	cats, tracks, plays := sqlite.NewCategoryRepo(db), sqlite.NewTrackRepo(db), sqlite.NewPlayRepo(db)
	for _, c := range domain.DefaultCategories {
		c := c
		_ = cats.Create(ctx, &c)
	}
	list, _ := cats.List(ctx)
	for _, c := range list {
		if c.Kind == domain.KindMusic || c.Name == "Cortinas" {
			for i := 0; i < 12; i++ {
				_, _ = tracks.Upsert(ctx, &domain.Track{Path: fmt.Sprintf(`C:\m\%s%d.mp3`, c.Name, i), Artist: fmt.Sprintf("%s %d", c.Name, i),
					Title: "t", CategoryID: c.ID, Active: true, Duration: 3 * time.Minute})
			}
		}
	}
	cfg := func() (scheduler.Config, *time.Location) { return scheduler.DefaultConfig(), lima }
	sched := scheduler.New(sqlite.NewClockRepo(db), sqlite.NewGridRepo(db), sqlite.NewLogRepo(db), tracks, cats, plays, cfg)
	if err := sched.EnsureDefaults(ctx); err != nil {
		t.Fatal(err)
	}
	tomorrow := time.Now().In(lima).AddDate(0, 0, 1).Format(domain.DayLayout)
	if _, err := sched.Generate(ctx, tomorrow); err != nil {
		t.Fatal(err)
	}
	svc := spots.New(sqlite.NewSpotRepo(db), sqlite.NewTandaRepo(db), plays, cats, tracks, sched, probe20{},
		func() *time.Location { return lima }, slog.New(slog.NewTextHandler(io.Discard, nil)))

	sp := &domain.Spot{Advertiser: "Ferretería", Name: "Verano", Path: `C:\cunas\a.mp3`, PlaysPerDay: 4, HourFrom: 6, HourTo: 22, Active: true}
	if err := svc.SaveLocal(ctx, sp); err != nil || sp.Duration != 20*time.Second {
		t.Fatalf("SaveLocal = %v (duración %v)", err, sp.Duration)
	}
	missing, err := svc.Replan(ctx, tomorrow)
	if err != nil || len(missing) != 0 {
		t.Fatalf("Replan = %v, faltan %v", err, missing)
	}
	items, _ := sched.Day(ctx, tomorrow)
	views, err := svc.DayTandas(ctx, items)
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, v := range views {
		n += len(v.Spots)
		if h := v.At.In(lima).Hour(); len(v.Spots) > 0 && (h < 6 || h >= 22) {
			t.Fatalf("cuña fuera de franja a las %s", v.At)
		}
	}
	if len(views) != 24 || n != 4 {
		t.Fatalf("tandas=%d cuñas planificadas=%d", len(views), n)
	}
	// la tanda con cuña ocupa 20 s + 2 cortinas de 3 min (promedio de la categoría)
	for _, it := range items {
		if it.Kind == domain.SlotSpots && it.Title == "Tanda · 1 cuñas" {
			if it.Length != 20*time.Second+6*time.Minute {
				t.Fatalf("duración de tanda = %v", it.Length)
			}
			got, _ := svc.TandaSpots(ctx, it.ID)
			if len(got) != 1 || got[0].ID != sp.ID {
				t.Fatalf("TandaSpots = %+v", got)
			}
			break
		}
	}
}
