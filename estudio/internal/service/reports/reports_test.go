package reports_test

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"radioclub25/estudio/internal/domain"
	"radioclub25/estudio/internal/platform/sqlite"
	"radioclub25/estudio/internal/service/reports"
)

var lima = time.FixedZone("PET", -5*3600)

func TestReports(t *testing.T) {
	ctx := context.Background()
	db, err := sqlite.Open(ctx, filepath.Join(t.TempDir(), "e.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	plays, spots := sqlite.NewPlayRepo(db), sqlite.NewSpotRepo(db)
	sp := &domain.Spot{Advertiser: "Ferretería El Sol", Name: "Verano", Path: "a.mp3", PlaysPerDay: 1, HourTo: 24, Active: true}
	_ = spots.Save(ctx, sp)
	base := time.Date(2026, 11, 8, 15, 0, 0, 0, lima)
	for i, p := range []domain.Play{
		{Kind: domain.PlayMusic, Artist: "Grupo 5", Title: "Motor", StartedAt: base, Duration: 3 * time.Minute, Complete: true, UUID: "a1"},
		{Kind: domain.PlaySpot, SpotID: sp.ID, Artist: "Ferretería El Sol", Title: "Verano", StartedAt: base.Add(3 * time.Minute), Duration: 20 * time.Second, Complete: true, UUID: "a2"},
		{Kind: domain.PlaySpot, SpotID: 999, Artist: "Otra", Title: "X", StartedAt: base.Add(4 * time.Minute), Duration: 10 * time.Second, UUID: "a3"},
		{Kind: domain.PlayMusic, Artist: "Fuera", Title: "de rango", StartedAt: base.AddDate(0, 0, 2), UUID: "a4"},
	} {
		p := p
		if err := plays.Add(ctx, &p); err != nil {
			t.Fatal(i, err)
		}
	}
	svc := reports.New(plays, spots, func() *time.Location { return lima })
	asrun, err := svc.AsRun(ctx, "2026-11-08", "2026-11-08")
	if err != nil || len(asrun) != 3 {
		t.Fatalf("AsRun = %d, %v", len(asrun), err)
	}
	proof, _ := svc.SpotProof(ctx, "2026-11-08", "2026-11-09", "ferretería el sol")
	if len(proof) != 1 || proof[0].Spot != "Verano" || !proof[0].Complete {
		t.Fatalf("SpotProof = %+v", proof)
	}
	var buf bytes.Buffer
	if err := svc.WriteProofCSV(&buf, proof); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "08/11/2026;15:03:00;Ferretería El Sol;Verano;20.0;Sí;a2") {
		t.Fatalf("CSV:\n%s", buf.String())
	}
	if _, err := svc.AsRun(ctx, "x", "2026-11-08"); err == nil {
		t.Fatal("fecha inválida debe dar error")
	}
}

func TestMusicReport(t *testing.T) {
	ctx := context.Background()
	db, err := sqlite.Open(ctx, filepath.Join(t.TempDir(), "e.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	plays, tracks := sqlite.NewPlayRepo(db), sqlite.NewTrackRepo(db)
	tr := &domain.Track{Path: "motor.mp3", Artist: "Grupo 5", Title: "Motor y motivo", Composer: "Elmer Yaipén",
		Album: "Éxitos", Year: "2010", Duration: 4 * time.Minute, Active: true}
	if _, err := tracks.Upsert(ctx, tr); err != nil {
		t.Fatal(err)
	}
	base := time.Date(2026, 11, 8, 10, 0, 0, 0, lima)
	for i, p := range []domain.Play{
		{TrackID: tr.ID, Kind: domain.PlayMusic, Artist: "Grupo 5", Title: "Motor y motivo", StartedAt: base, Duration: 3 * time.Minute, UUID: "m1"},
		{TrackID: tr.ID, Kind: domain.PlayMusic, Artist: "Grupo 5", Title: "Motor y motivo", StartedAt: base.Add(5 * time.Hour), Duration: 2 * time.Minute, UUID: "m2"},
		{Kind: domain.PlayMusic, Artist: "Agua Marina", Title: "Paloma ajena", StartedAt: base.Add(time.Hour), Duration: time.Minute, UUID: "m3"},
		{Kind: domain.PlayJingle, Title: "ID", StartedAt: base.Add(time.Minute), UUID: "m4"},
	} {
		p := p
		if err := plays.Add(ctx, &p); err != nil {
			t.Fatal(i, err)
		}
	}
	svc := reports.New(plays, sqlite.NewSpotRepo(db), func() *time.Location { return lima }).WithTracks(tracks)
	rows, err := svc.Music(ctx, "2026-11-08", "2026-11-08")
	if err != nil || len(rows) != 2 {
		t.Fatalf("informe = %+v, %v", rows, err)
	}
	if r := rows[0]; r.Title != "Motor y motivo" || r.Plays != 2 || r.Aired != 5*time.Minute || r.Composer != "Elmer Yaipén" || r.Year != "2010" {
		t.Fatalf("fila = %+v", r)
	}
	var buf bytes.Buffer
	_ = svc.WriteMusicCSV(&buf, rows, "2026-11-08", "2026-11-08")
	if !strings.Contains(buf.String(), "Motor y motivo;Grupo 5;Elmer Yaipén;Éxitos;2010;2;240;300") {
		t.Fatalf("CSV:\n%s", buf.String())
	}
}
