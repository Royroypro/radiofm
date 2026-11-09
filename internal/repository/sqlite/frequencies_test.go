package sqlite_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"radioclub25/internal/domain"
	"radioclub25/internal/repository/sqlite"
	"radioclub25/internal/testutil"
)

func TestFrequencyRepo(t *testing.T) {
	ctx := context.Background()
	repo := sqlite.NewFrequencyRepo(testutil.DB(t))

	b := &domain.Frequency{City: "Barranca", Frequency: "103.2", Band: "FM", SortOrder: 10, Active: true, Source: domain.SourceLocal, Mount: "/radio-barranca"}
	h := &domain.Frequency{City: "Huacho", Frequency: "93.3", Band: "FM", SortOrder: 0, Active: true, Source: domain.SourceLocal, Mount: "/radio"}
	x := &domain.Frequency{City: "Huaral", Frequency: "105.4", Band: "FM", SortOrder: 20, Source: domain.SourceExternal, StreamURL: "https://ejemplo.pe/huaral"}
	for _, f := range []*domain.Frequency{b, h, x} {
		if err := repo.Create(ctx, f); err != nil {
			t.Fatal(err)
		}
	}
	all, err := repo.List(ctx, false)
	if err != nil || len(all) != 3 || all[0].City != "Huacho" || all[2].City != "Huaral" {
		t.Fatalf("List = %+v, %v", all, err)
	}
	active, _ := repo.List(ctx, true)
	if len(active) != 2 || active[0].Label() != "93.3 FM" {
		t.Fatalf("activas = %+v", active)
	}
	x.Active, x.Band, x.Frequency = true, "AM", "1240"
	if err := repo.Update(ctx, x); err != nil {
		t.Fatal(err)
	}
	if got, _ := repo.Get(ctx, x.ID); got.Label() != "1240 AM" || !got.Active || got.ListenURL() != "https://ejemplo.pe/huaral" {
		t.Fatalf("Get = %+v", got)
	}
	if err := repo.Create(ctx, &domain.Frequency{City: "Lima", Frequency: "99", Band: "XX", Source: domain.SourceLocal, Mount: "/radio-lima"}); err == nil {
		t.Fatal("la base debe rechazar una banda que no sea FM/AM")
	}
	dup := &domain.Frequency{City: "Otra", Frequency: "99.9", Band: "FM", Source: domain.SourceLocal, Mount: "/radio"}
	if err := repo.Create(ctx, dup); err == nil || !strings.Contains(err.Error(), "montaje") {
		t.Fatalf("dos frecuencias locales no pueden compartir montaje: %v", err)
	}
	if err := repo.Delete(ctx, x.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Get(ctx, x.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("tras borrar = %v", err)
	}
}
