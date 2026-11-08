package library_test

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"radioclub25/estudio/internal/domain"
	"radioclub25/estudio/internal/platform/ffmpeg"
	"radioclub25/estudio/internal/platform/sqlite"
	"radioclub25/estudio/internal/service/library"
)

// fakeMedia simula ffprobe/ffmpeg.
type fakeMedia struct{}

func (fakeMedia) Probe(_ context.Context, path string) (ffmpeg.Info, error) {
	if filepath.Base(path) == "roto.mp3" {
		return ffmpeg.Info{}, io.ErrUnexpectedEOF
	}
	info := ffmpeg.Info{Duration: 3 * time.Minute}
	if filepath.Base(path) == "etiquetado.mp3" {
		info.Artist, info.Title = "Agua Marina", "Paloma ajena"
	}
	return info, nil
}

func (fakeMedia) Analyze(context.Context, string) (ffmpeg.Loudness, error) {
	m := make([]float64, 1800) // 3 min a −14 LUFS
	for i := range m {
		m[i] = -14
	}
	return ffmpeg.Loudness{Integrated: -14, Momentary: m}, nil
}

func newService(t *testing.T) (*library.Service, domain.TrackRepository) {
	t.Helper()
	db, err := sqlite.Open(context.Background(), filepath.Join(t.TempDir(), "e.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	tracks := sqlite.NewTrackRepo(db)
	svc := library.New(tracks, sqlite.NewCategoryRepo(db), fakeMedia{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err := svc.EnsureDefaultCategories(context.Background()); err != nil {
		t.Fatal(err)
	}
	return svc, tracks
}

func touch(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestImportAndAnalyze(t *testing.T) {
	ctx := context.Background()
	svc, tracks := newService(t)
	cats, _ := svc.Categories(ctx)
	if len(cats) != len(domain.DefaultCategories) {
		t.Fatalf("categorías iniciales = %d", len(cats))
	}
	salsa := cats[2].ID

	dir := t.TempDir()
	touch(t, filepath.Join(dir, "Grupo 5 - Motor y motivo.mp3"))
	touch(t, filepath.Join(dir, "etiquetado.mp3"))
	touch(t, filepath.Join(dir, "roto.mp3"))
	touch(t, filepath.Join(dir, "portada.jpg"))
	touch(t, filepath.Join(dir, "sub", "otra.flac"))

	var progress int
	res, err := svc.ImportFolder(ctx, dir, salsa, false, func(library.ImportProgress) { progress++ })
	if err != nil || res.Added != 2 || res.Failed != 1 || progress != 3 {
		t.Fatalf("importar sin subcarpetas = %+v, %v (progreso %d)", res, err, progress)
	}
	res, _ = svc.ImportFolder(ctx, dir, salsa, true, nil)
	if res.Added != 1 || res.Updated != 2 {
		t.Fatalf("importar con subcarpetas = %+v", res)
	}
	list, total, _ := svc.Tracks(ctx, domain.TrackFilter{Query: "motor"})
	if total != 1 || list[0].Artist != "Grupo 5" || list[0].Title != "Motor y motivo" || list[0].CategoryID != salsa {
		t.Fatalf("título desde el nombre de archivo: %+v", list)
	}
	list, _, _ = svc.Tracks(ctx, domain.TrackFilter{Query: "paloma"})
	if len(list) != 1 || list[0].Artist != "Agua Marina" {
		t.Fatalf("etiquetas: %+v", list)
	}

	// analizador
	actx, cancel := context.WithCancel(ctx)
	go svc.RunAnalyzer(actx)
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if p, _ := tracks.PendingAnalysis(ctx, 10); len(p) == 0 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	cancel()
	got, _ := svc.Track(ctx, list[0].ID)
	if !got.Analyzed || got.GainDB != -2 || got.Loudness != -14 {
		t.Fatalf("análisis = %+v", got)
	}

	// archivo borrado → missing
	_ = os.Remove(filepath.Join(dir, "etiquetado.mp3"))
	if n, _ := svc.CheckMissing(ctx); n != 1 {
		t.Fatalf("faltantes = %d", n)
	}
	got, _ = svc.Track(ctx, list[0].ID)
	if !got.Missing || got.Available(time.Now()) {
		t.Fatal("una pista sin archivo no debe estar disponible")
	}
}

func TestUpdateTrackValidation(t *testing.T) {
	ctx := context.Background()
	svc, _ := newService(t)
	cats, _ := svc.Categories(ctx)
	dir := t.TempDir()
	touch(t, filepath.Join(dir, "a.mp3"))
	_, _ = svc.ImportFolder(ctx, dir, cats[0].ID, false, nil)
	list, _, _ := svc.Tracks(ctx, domain.TrackFilter{})
	tr := list[0]
	tr.CueIn, tr.CueOut = 10*time.Second, 5*time.Second
	if err := svc.UpdateTrack(ctx, &tr); err == nil {
		t.Fatal("cue-out antes del cue-in debe ser inválido")
	}
	tr.CueOut = 2*time.Minute + 50*time.Second
	tr.Intro = 12 * time.Second
	if err := svc.UpdateTrack(ctx, &tr); err != nil {
		t.Fatal(err)
	}
	got, _ := svc.Track(ctx, tr.ID)
	if got.PlayLength() != 2*time.Minute+40*time.Second || got.Intro != 12*time.Second {
		t.Fatalf("pista = %+v", got)
	}
	if err := svc.DeleteCategory(ctx, cats[0].ID); err == nil {
		t.Fatal("no se puede borrar una categoría con pistas")
	}
}
