package scheduler_test

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"radioclub25/estudio/internal/domain"
	"radioclub25/estudio/internal/platform/sqlite"
	"radioclub25/estudio/internal/service/scheduler"
)

var lima = time.FixedZone("PET", -5*3600)

func setup(t *testing.T) (*scheduler.Service, domain.TrackRepository, domain.CategoryRepository) {
	t.Helper()
	ctx := context.Background()
	db, err := sqlite.Open(ctx, filepath.Join(t.TempDir(), "e.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	cats, tracks := sqlite.NewCategoryRepo(db), sqlite.NewTrackRepo(db)
	for _, c := range domain.DefaultCategories {
		c := c
		if err := cats.Create(ctx, &c); err != nil {
			t.Fatal(err)
		}
	}
	list, _ := cats.List(ctx)
	for _, c := range list {
		if c.Kind != domain.KindMusic && c.Name != "IDs" && c.Name != "Jingles" {
			continue
		}
		for i := 0; i < 15; i++ {
			tr := &domain.Track{Path: fmt.Sprintf(`C:\m\%s-%d.mp3`, c.Name, i), Artist: fmt.Sprintf("%s %d", c.Name, i),
				Title: fmt.Sprintf("tema %d", i), CategoryID: c.ID, Active: true, Duration: 3 * time.Minute}
			if _, err := tracks.Upsert(ctx, tr); err != nil {
				t.Fatal(err)
			}
		}
	}
	svc := scheduler.New(sqlite.NewClockRepo(db), sqlite.NewGridRepo(db), sqlite.NewLogRepo(db), tracks, cats,
		sqlite.NewPlayRepo(db), func() (scheduler.Config, *time.Location) { return scheduler.DefaultConfig(), lima })
	return svc, tracks, cats
}

func TestDefaultsAndGenerateDay(t *testing.T) {
	ctx := context.Background()
	svc, tracks, _ := setup(t)
	if err := svc.EnsureDefaults(ctx); err != nil {
		t.Fatal(err)
	}
	clocks, _ := svc.Clocks(ctx)
	grid, _ := svc.Grid(ctx)
	if len(clocks) != 1 || len(grid) != 7*24 {
		t.Fatalf("relojes=%d celdas=%d", len(clocks), len(grid))
	}
	if err := svc.EnsureDays(ctx, 1); err != nil {
		t.Fatal(err)
	}
	today := svc.DayOf(time.Now())
	items, err := svc.Day(ctx, today)
	if err != nil || len(items) < 24*10 {
		t.Fatalf("playlist de hoy: %d elementos, %v", len(items), err)
	}
	if items[0].Kind != domain.SlotTime || !items[0].Hard {
		t.Fatalf("el día debe empezar con la hora locutada: %+v", items[0])
	}

	// editar: bloquear, mover, insertar y regenerar conserva lo bloqueado
	target := items[5]
	if err := svc.ToggleLock(ctx, target.ID); err != nil {
		t.Fatal(err)
	}
	all, _, _ := tracks.List(ctx, domain.TrackFilter{Limit: 1})
	if err := svc.InsertTrack(ctx, items[2].ID, all[0].ID); err != nil {
		t.Fatal(err)
	}
	items, _ = svc.Day(ctx, today)
	if items[3].TrackID != all[0].ID || !items[3].Locked {
		t.Fatalf("la pista insertada debe ir después del elemento elegido: %+v", items[3])
	}
	if !items[3].At.Equal(items[2].At.Add(items[2].Length)) {
		t.Fatal("las horas deben recalcularse tras insertar")
	}
	if _, err := svc.Generate(ctx, today); err != nil {
		t.Fatal(err)
	}
	items, _ = svc.Day(ctx, today)
	locked := 0
	for _, it := range items {
		if it.Locked {
			locked++
		}
	}
	if locked != 2 {
		t.Fatalf("regenerar debe conservar los 2 bloqueados, quedan %d", locked)
	}
	// lo que ya sonó no se puede mover
	_ = svc.SetStatus(ctx, items[0].ID, domain.LogPlayed)
	if err := svc.Move(ctx, items[0].ID, 1); err == nil {
		t.Fatal("no se debe mover un elemento que ya sonó")
	}
	// arrastrar y soltar: el 8.º elemento pasa a estar antes del 3.º, y luego después del 10.º
	items, _ = svc.Day(ctx, today)
	moved, before := items[7].ID, items[2].ID
	if err := svc.MoveTo(ctx, moved, before, false); err != nil {
		t.Fatal(err)
	}
	items, _ = svc.Day(ctx, today)
	if items[2].ID != moved || items[3].ID != before {
		t.Fatalf("debe quedar antes del elegido: %d %d", items[2].ID, items[3].ID)
	}
	if !items[3].At.Equal(items[2].At.Add(items[2].Length)) {
		t.Fatal("las horas deben recalcularse tras mover")
	}
	after := items[9].ID
	if err := svc.MoveTo(ctx, moved, after, true); err != nil {
		t.Fatal(err)
	}
	items, _ = svc.Day(ctx, today)
	if items[8].ID != after || items[9].ID != moved { // al sacarlo de arriba, los de en medio suben uno
		t.Fatalf("debe quedar después del elegido: %d %d", items[8].ID, items[9].ID)
	}
	if err := svc.MoveTo(ctx, moved, items[0].ID, false); err == nil {
		t.Fatal("no se puede poner antes de lo que ya sonó")
	}
	up, _ := svc.Upcoming(ctx, time.Now(), 5)
	if len(up) != 5 || up[0].ID == items[0].ID {
		t.Fatalf("Upcoming = %+v", up)
	}
}

func TestClockValidation(t *testing.T) {
	ctx := context.Background()
	svc, _, _ := setup(t)
	bad := &domain.Clock{Name: "X", Slots: []domain.ClockSlot{
		{Kind: domain.SlotSpots, Hard: true, Minute: 30}, {Kind: domain.SlotTime, Hard: true, Minute: 0}}}
	if err := svc.SaveClock(ctx, bad); err == nil {
		t.Fatal("eventos duros fuera de orden deben rechazarse")
	}
	if err := svc.SaveClock(ctx, &domain.Clock{Name: "Y", Slots: []domain.ClockSlot{{Kind: domain.SlotCategory}}}); err == nil {
		t.Fatal("slot de categoría sin categoría debe rechazarse")
	}
	ok := &domain.Clock{Name: "Noche", Slots: []domain.ClockSlot{{Kind: domain.SlotMode, Mode: "assisted"}}}
	if err := svc.SaveClock(ctx, ok); err != nil {
		t.Fatal(err)
	}
	if err := svc.SetGrid(ctx, []domain.GridCell{{Weekday: 1, Hour: 22, ClockID: ok.ID}}); err != nil {
		t.Fatal(err)
	}
	if err := svc.DeleteClock(ctx, ok.ID); err == nil {
		t.Fatal("no se puede borrar un reloj usado en la parrilla")
	}
}
