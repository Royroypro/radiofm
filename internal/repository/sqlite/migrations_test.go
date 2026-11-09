package sqlite_test

import (
	"context"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"radioclub25/internal/platform/db"
	"radioclub25/internal/repository/sqlite"
)

// upTo devuelve las migraciones embebidas anteriores a la versión dada.
func upTo(t *testing.T, before string) fs.FS {
	t.Helper()
	all := sqlite.Migrations()
	names, _ := fs.Glob(all, "*.sql")
	out := fstest.MapFS{}
	for _, n := range names {
		if n < before {
			b, err := fs.ReadFile(all, n)
			if err != nil {
				t.Fatal(err)
			}
			out[n] = &fstest.MapFile{Data: b}
		}
	}
	return out
}

// 0004_stations sobre una base en producción: frecuencias de 0003 y la PC ya
// configurada en studio_state.
func TestMigration0004Stations(t *testing.T) {
	ctx := context.Background()
	d, err := db.Open(filepath.Join(t.TempDir(), "old.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	if err := db.Migrate(ctx, d, upTo(t, "0004")); err != nil {
		t.Fatal(err)
	}
	if _, err := d.ExecContext(ctx, `INSERT INTO frequencies (city, frequency, band, sort_order, active, created_at) VALUES
		('Huacho', '93.3', 'FM', 0, 1, '2026-11-08T00:00:00Z'),
		('Barranca', '103.2', 'FM', 10, 1, '2026-11-08T00:00:00Z'),
		('San José', '99.9', 'FM', 20, 1, '2026-11-08T00:00:00Z');
		UPDATE studio_state SET token_hash = 'hash-pc', version = '1.0.2' WHERE id = 1;`); err != nil {
		t.Fatal(err)
	}
	if err := db.Migrate(ctx, d, sqlite.Migrations()); err != nil {
		t.Fatal(err)
	}
	list, err := sqlite.NewFrequencyRepo(d).List(ctx, false)
	if err != nil || len(list) != 3 {
		t.Fatalf("frecuencias = %+v, %v", list, err)
	}
	var mounts []string
	for _, f := range list {
		mounts = append(mounts, string(f.Source)+":"+f.Mount)
	}
	if got := strings.Join(mounts, " "); got != "local:/radio local:/radio-barranca local:/radio-3" {
		t.Fatalf("montajes = %s", got)
	}
	fid, err := sqlite.NewStudioRepo(d).FindToken(ctx, "hash-pc")
	if err != nil || fid != list[0].ID {
		t.Fatalf("la PC configurada debe quedar en la principal: %d, %v", fid, err)
	}
	if st, _ := sqlite.NewStudioRepo(d).Get(ctx, fid); st.Version != "1.0.2" {
		t.Fatalf("estado migrado = %+v", st)
	}
}
