package service_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"radioclub25/internal/domain"
	"radioclub25/internal/repository/sqlite"
	"radioclub25/internal/service/audit"
	"radioclub25/internal/service/studio"
	"radioclub25/internal/testutil"
)

// hashFiles simula el almacenamiento: el hash es fijo y recuerda lo borrado.
type hashFiles struct{ deleted []string }

func (f *hashFiles) Delete(u string) error           { f.deleted = append(f.deleted, u); return nil }
func (f *hashFiles) SHA256(u string) (string, error) { return "sha-" + u, nil }

func newStudio(t *testing.T) (*studio.Service, *hashFiles) {
	d := testutil.DB(t)
	files := &hashFiles{}
	au := audit.New(sqlite.NewAuditRepo(d), testutil.Logger())
	return studio.New(sqlite.NewFMSpotRepo(d), sqlite.NewAirplayRepo(d), sqlite.NewStudioRepo(d), files, au,
		testutil.Lima(t)), files
}

func TestStudioToken(t *testing.T) {
	ctx := context.Background()
	svc, _ := newStudio(t)
	actor := domain.Actor{UserID: 1, Email: "a@b.pe", Role: domain.RoleAdmin}

	if err := svc.Authenticate(ctx, "c25s_cualquiera"); !errors.Is(err, domain.ErrUnauthorized) {
		t.Fatalf("sin token generado debe rechazar, dio %v", err)
	}
	tok, err := svc.GenerateToken(ctx, actor)
	if err != nil || !strings.HasPrefix(tok, studio.TokenPrefix) || len(tok) != len(studio.TokenPrefix)+64 {
		t.Fatalf("GenerateToken = %q, %v", tok, err)
	}
	if err := svc.Authenticate(ctx, tok); err != nil {
		t.Fatalf("token válido rechazado: %v", err)
	}
	if err := svc.Authenticate(ctx, tok+"x"); !errors.Is(err, domain.ErrUnauthorized) {
		t.Fatal("token alterado aceptado")
	}
	tok2, _ := svc.GenerateToken(ctx, actor)
	if svc.Authenticate(ctx, tok) == nil || svc.Authenticate(ctx, tok2) != nil {
		t.Fatal("generar un token nuevo debe invalidar el anterior")
	}
	if err := svc.RevokeToken(ctx, actor); err != nil || svc.Authenticate(ctx, tok2) == nil {
		t.Fatal("revocar debe invalidar el token")
	}
}

func TestStudioSpotsAndAirplays(t *testing.T) {
	ctx := context.Background()
	svc, files := newStudio(t)
	actor := domain.Actor{UserID: 1, Email: "a@b.pe", Role: domain.RoleEditor}

	bad := &domain.FMSpot{Advertiser: "A", Name: "B", FileURL: "/uploads/fm/a.mp3", PlaysPerDay: 1, HourFrom: 12, HourTo: 12}
	if err := svc.CreateSpot(ctx, actor, bad); !isValidation(err) {
		t.Fatalf("franja vacía debe ser error de validación, dio %v", err)
	}
	past := time.Now().Add(-time.Hour)
	sp := &domain.FMSpot{Advertiser: " Ferretería ", Name: "Verano", FileURL: "/uploads/fm/a.mp3", PlaysPerDay: 4,
		HourTo: 24, Active: true}
	expired := &domain.FMSpot{Advertiser: "Viejo", Name: "Navidad", FileURL: "/uploads/fm/b.mp3", PlaysPerDay: 1,
		HourTo: 24, Active: true, EndsAt: &past}
	for _, s := range []*domain.FMSpot{sp, expired} {
		if err := svc.CreateSpot(ctx, actor, s); err != nil {
			t.Fatal(err)
		}
	}
	if sp.Advertiser != "Ferretería" || sp.SHA256 != "sha-/uploads/fm/a.mp3" {
		t.Fatalf("debe limpiar el anunciante y calcular el hash: %+v", sp)
	}
	live, err := svc.SpotsForStudio(ctx)
	if err != nil || len(live) != 1 || live[0].ID != sp.ID {
		t.Fatalf("SpotsForStudio debe excluir la vencida: %+v, %v", live, err)
	}

	// reemplazar el archivo recalcula el hash y borra el anterior
	sp.FileURL = "/uploads/fm/c.mp3"
	if err := svc.UpdateSpot(ctx, actor, sp); err != nil || sp.SHA256 != "sha-/uploads/fm/c.mp3" ||
		len(files.deleted) != 1 || files.deleted[0] != "/uploads/fm/a.mp3" {
		t.Fatalf("UpdateSpot: %+v, %v, borrados %v", sp, err, files.deleted)
	}

	now := time.Now().UTC().Truncate(time.Second)
	batch := []studio.AirplayInput{
		{SpotID: sp.ID, AiredAt: now.Add(-time.Minute), DurationMs: 20000, Complete: true, PlayUUID: "0b6c1d7e-aaaa-4bbb-8ccc-111111111111"},
		{SpotID: sp.ID, AiredAt: now.Add(time.Hour), PlayUUID: "futuro-0001"},    // en el futuro: rechazada
		{SpotID: sp.ID, AiredAt: now, PlayUUID: "con espacios"},                  // uuid inválido: rechazada
		{SpotID: 999, AiredAt: now.Add(-time.Minute), PlayUUID: "sin-cuna-0001"}, // cuña inexistente: ignorada
	}
	res, err := svc.RecordAirplays(ctx, batch)
	if err != nil || res.Inserted != 1 || res.Ignored != 1 || len(res.Rejected) != 2 {
		t.Fatalf("RecordAirplays = %+v, %v", res, err)
	}
	if res, _ = svc.RecordAirplays(ctx, batch); res.Inserted != 0 || res.Ignored != 2 {
		t.Fatalf("reenviar no debe duplicar: %+v", res)
	}
	counts, _ := svc.TodayCounts(ctx)
	if counts[sp.ID] != 1 {
		t.Fatalf("TodayCounts = %v", counts)
	}
	if _, err := svc.RecordAirplays(ctx, make([]studio.AirplayInput, studio.MaxAirplayBatch+1)); !isValidation(err) {
		t.Fatal("un lote demasiado grande debe ser error de validación")
	}
}

func TestStudioHeartbeatAndNowPlaying(t *testing.T) {
	ctx := context.Background()
	svc, _ := newStudio(t)
	if err := svc.Heartbeat(ctx, studio.HeartbeatInput{Version: "0.1.0", Mode: "otro"}); !isValidation(err) {
		t.Fatal("modo desconocido debe ser error de validación")
	}
	if err := svc.Heartbeat(ctx, studio.HeartbeatInput{Version: "0.1.0", Mode: domain.StudioAuto,
		Encoder: []byte(`{"connected":true,"kbps":128}`)}); err != nil {
		t.Fatal(err)
	}
	if svc.CurrentNowPlaying(ctx, time.Minute) != nil {
		t.Fatal("sin datos no debe haber 'lo que suena'")
	}
	if err := svc.SetNowPlaying(ctx, domain.NowPlaying{Artist: "Agua Marina", Title: "Paloma ajena"}); err != nil {
		t.Fatal(err)
	}
	if np := svc.CurrentNowPlaying(ctx, time.Minute); np == nil || np.Display() != "Agua Marina - Paloma ajena" {
		t.Fatalf("CurrentNowPlaying = %+v", np)
	}
	st, _ := svc.State(ctx)
	if st.Version != "0.1.0" || !strings.Contains(st.EncoderJSON, "128") {
		t.Fatalf("estado = %+v", st)
	}
}

func isValidation(err error) bool {
	var ve *domain.ValidationError
	return errors.As(err, &ve)
}
