package sqlite_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"radioclub25/internal/domain"
	"radioclub25/internal/repository/sqlite"
	"radioclub25/internal/testutil"
)

func TestEmisorReleases(t *testing.T) {
	ctx := context.Background()
	repo := sqlite.NewEmisorReleaseRepo(testutil.DB(t))
	a := &domain.EmisorRelease{Version: "1.4.0", FileName: "club25-emisor.exe", SHA256: "aa", Size: 10, CreatedAt: time.Now()}
	b := &domain.EmisorRelease{Version: "1.5.0", FileName: "club25-emisor.exe", SHA256: "bb", Size: 20, Notes: "MEJORA",
		CreatedAt: time.Now(), CreatedBy: "super@x"}
	for _, r := range []*domain.EmisorRelease{a, b} {
		if err := repo.Create(ctx, r); err != nil {
			t.Fatal(err)
		}
	}
	if err := repo.Create(ctx, &domain.EmisorRelease{Version: "1.5.0", CreatedAt: time.Now()}); err == nil {
		t.Fatal("una versión repetida debe dar error")
	}
	list, err := repo.List(ctx)
	if err != nil || len(list) != 2 || list[0].Version != "1.5.0" || list[0].Notes != "MEJORA" || list[0].Size != 20 {
		t.Fatalf("List = %+v, %v", list, err)
	}
	got, err := repo.Get(ctx, a.ID)
	if err != nil || got.SHA256 != "aa" {
		t.Fatalf("Get = %+v, %v", got, err)
	}
	if err := repo.Delete(ctx, a.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Get(ctx, a.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("tras borrar: %v", err)
	}
}
