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
