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

func TestFMSpotsAndAirplays(t *testing.T) {
	ctx := context.Background()
	d := testutil.DB(t)
	spots, plays := sqlite.NewFMSpotRepo(d), sqlite.NewAirplayRepo(d)

	end := time.Now().Add(48 * time.Hour)
	s := &domain.FMSpot{Advertiser: "Ferretería El Sol", Name: "Verano 20 s", FileURL: "/uploads/fm/a.mp3",
		SHA256: "abc", DurationMs: 20000, EndsAt: &end, PlaysPerDay: 6, HourFrom: 7, HourTo: 13, Active: true}
	if err := spots.Create(ctx, s); err != nil {
		t.Fatal(err)
	}
	got, err := spots.Get(ctx, s.ID)
	if err != nil || got.HourTo != 13 || got.EndsAt == nil || got.SHA256 != "abc" {
		t.Fatalf("Get = %+v, %v", got, err)
	}
	if err := spots.Create(ctx, &domain.FMSpot{Advertiser: "X", Name: "Mala", FileURL: "/x", PlaysPerDay: 1,
		HourFrom: 10, HourTo: 9}); err == nil {
		t.Fatal("la base debe rechazar una franja con inicio >= fin")
	}

	aired := time.Date(2026, 11, 8, 15, 0, 0, 0, time.UTC)
	batch := []domain.Airplay{
		{SpotID: s.ID, AiredAt: aired, DurationMs: 20000, Complete: true, PlayUUID: "u1"},
		{SpotID: s.ID, AiredAt: aired.Add(time.Hour), DurationMs: 9000, PlayUUID: "u2"},
		{SpotID: 999, AiredAt: aired, PlayUUID: "u3"}, // cuña inexistente: se ignora
	}
	if n, err := plays.Record(ctx, batch); err != nil || n != 2 {
		t.Fatalf("Record = %d, %v (esperaba 2)", n, err)
	}
	if n, err := plays.Record(ctx, batch); err != nil || n != 0 {
		t.Fatalf("reenviar el mismo lote debe insertar 0, insertó %d (%v)", n, err)
	}
	list, err := plays.List(ctx, domain.AirplayFilter{From: aired.Add(-time.Hour), To: aired.Add(2 * time.Hour),
		Advertiser: "ferretería el sol"})
	if err != nil || len(list) != 2 || !list[0].Complete || list[0].Advertiser != "Ferretería El Sol" {
		t.Fatalf("List = %+v, %v", list, err)
	}
	counts, err := plays.CountSince(ctx, aired.Add(-time.Minute))
	if err != nil || counts[s.ID] != 2 {
		t.Fatalf("CountSince = %v, %v", counts, err)
	}

	// borrar la cuña conserva el comprobante
	if err := spots.Delete(ctx, s.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := spots.Get(ctx, s.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("Get tras borrar = %v", err)
	}
	list, _ = plays.List(ctx, domain.AirplayFilter{From: aired.Add(-time.Hour), To: aired.Add(2 * time.Hour)})
	if len(list) != 2 || list[0].SpotID != 0 || list[0].SpotName != "Verano 20 s" {
		t.Fatalf("las emisiones deben sobrevivir al borrado de la cuña: %+v", list)
	}
}

func TestStudioState(t *testing.T) {
	ctx := context.Background()
	repo := sqlite.NewStudioRepo(testutil.DB(t))
	st, err := repo.Get(ctx)
	if err != nil || st.HasToken() || st.LastSeen != nil {
		t.Fatalf("estado inicial = %+v, %v", st, err)
	}
	now := time.Now()
	if err := repo.SetToken(ctx, "hash", now); err != nil {
		t.Fatal(err)
	}
	if err := repo.Heartbeat(ctx, now, "1.0.0", domain.StudioAuto, `{"connected":true}`); err != nil {
		t.Fatal(err)
	}
	if err := repo.SetNowPlaying(ctx, domain.NowPlaying{Artist: "Grupo 5", Title: "Motor y motivo", Kind: "music"}, now); err != nil {
		t.Fatal(err)
	}
	st, _ = repo.Get(ctx)
	if st.TokenHash != "hash" || st.Version != "1.0.0" || st.Mode != domain.StudioAuto || st.NowPlaying == nil ||
		st.NowPlaying.Display() != "Grupo 5 - Motor y motivo" || !st.Online(now, time.Minute) {
		t.Fatalf("estado = %+v", st)
	}
	if err := repo.SetToken(ctx, "", now); err != nil {
		t.Fatal(err)
	}
	if st, _ = repo.Get(ctx); st.HasToken() || st.TokenCreatedAt != nil {
		t.Fatal("revocar debe borrar el token")
	}
}
