package service_test

import (
	"context"
	"testing"
	"time"

	"radioclub25/internal/domain"
	"radioclub25/internal/service/studio"
	"radioclub25/internal/testutil"
)

// Campaña con dos versiones: las reglas se copian a las versiones, el rubro
// del anunciante también, y el cumplimiento cuenta lo programado y lo emitido.
func TestCampaignsAndCompliance(t *testing.T) {
	ctx := context.Background()
	svc, _ := newStudio(t)
	actor := domain.Actor{UserID: 1, Email: "a@b.pe", Role: domain.RoleAdmin}
	lima := testutil.Lima(t)
	// martes 10/11/2026 15:00 en Lima
	now := time.Date(2026, 11, 10, 15, 0, 0, 0, lima)
	svc.WithClock(func() time.Time { return now })

	c := &domain.FMCampaign{Name: "Verano", PlaysPerDay: 8, HourFrom: 7, HourTo: 23,
		DaysMask: 0b0111110, Position: domain.PositionFirst, Priority: 1, Active: true}
	if err := svc.SaveCampaign(ctx, actor, c, "Banco Andino"); err != nil {
		t.Fatal(err)
	}
	if c.Advertiser != "Banco Andino" || c.AdvertiserID == 0 {
		t.Fatalf("anunciante creado al vuelo = %+v", c)
	}
	for _, name := range []string{"20 s", "30 s"} {
		if err := svc.SaveVersion(ctx, actor, c.ID, &domain.FMSpot{Name: name, FileURL: "/uploads/fm/" + name + ".mp3",
			DurationMs: 20000, Weight: 1}); err != nil {
			t.Fatal(err)
		}
	}
	// el rubro se pone después: debe llegar a las versiones
	adv, _ := svc.Advertiser(ctx, c.AdvertiserID)
	adv.Sector = " Banca "
	if err := svc.SaveAdvertiser(ctx, actor, adv); err != nil {
		t.Fatal(err)
	}
	got, err := svc.Campaign(ctx, c.ID)
	if err != nil || len(got.Versions) != 2 {
		t.Fatalf("campaña = %+v, %v", got, err)
	}
	for _, v := range got.Versions {
		if v.CampaignID != c.ID || v.PlaysPerDay != 8 || v.DaysMask != 0b0111110 || v.Position != domain.PositionFirst ||
			v.Priority != 1 || v.Sector != "banca" || v.Advertiser != "Banco Andino" || !v.Active {
			t.Fatalf("versión sin las reglas de la campaña: %+v", v)
		}
	}
	// pausar la campaña pausa sus versiones
	got.Active = false
	if err := svc.SaveCampaign(ctx, actor, got, ""); err != nil {
		t.Fatal(err)
	}
	if live, _ := svc.SpotsForStudio(ctx, 1); len(live) != 0 {
		t.Fatalf("campaña pausada: la PC no debe recibir sus versiones: %+v", live)
	}
	got.Active = true
	_ = svc.SaveCampaign(ctx, actor, got, "")

	// emisiones: hoy 3 (a las 15:00 de una franja 7–23 debía llevar 8·8/16 = 4)
	v := got.Versions[0]
	var batch []studio.AirplayInput
	for i := 0; i < 3; i++ {
		batch = append(batch, studio.AirplayInput{SpotID: v.ID, AiredAt: now.Add(-time.Duration(i+1) * time.Hour),
			DurationMs: 20000, Complete: true, PlayUUID: "uuid-hoy-000" + string(rune('a'+i))})
	}
	if _, err := svc.RecordAirplays(ctx, 1, batch); err != nil {
		t.Fatal(err)
	}
	today := time.Date(2026, 11, 10, 0, 0, 0, 0, lima)
	comp, err := svc.Compliance(ctx, today, today.AddDate(0, 0, 1))
	if err != nil || len(comp) != 1 {
		t.Fatalf("cumplimiento = %+v, %v", comp, err)
	}
	if cc := comp[0]; cc.Scheduled != 4 || cc.Aired != 3 || cc.Missing() != 1 {
		t.Fatalf("hoy: programadas %d, emitidas %d, faltan %d (quería 4, 3, 1)", cc.Scheduled, cc.Aired, cc.Missing())
	}
	// semana completa del lun 2 al dom 8: 5 días hábiles × 8 = 40, nada emitido
	week := time.Date(2026, 11, 2, 0, 0, 0, 0, lima)
	if comp, _ = svc.Compliance(ctx, week, week.AddDate(0, 0, 7)); comp[0].Scheduled != 40 || comp[0].Missing() != 40 {
		t.Fatalf("semana: %+v", comp[0])
	}
	if short, _ := svc.TodayShortfall(ctx); len(short) != 1 || short[0].Missing() != 1 {
		t.Fatalf("atrasadas hoy = %+v", short)
	}
	// un anunciante con campañas no se puede borrar
	if err := svc.DeleteAdvertiser(ctx, actor, c.AdvertiserID); err == nil {
		t.Fatal("no debe borrarse un anunciante con campañas")
	}
	if err := svc.DeleteCampaign(ctx, actor, c.ID); err != nil {
		t.Fatal(err)
	}
	if err := svc.DeleteAdvertiser(ctx, actor, c.AdvertiserID); err != nil {
		t.Fatal(err)
	}
}

func TestCampaignValidation(t *testing.T) {
	ctx := context.Background()
	svc, _ := newStudio(t)
	actor := domain.Actor{UserID: 1}
	bad := []*domain.FMCampaign{
		{Name: "", PlaysPerDay: 1, HourTo: 24},
		{Name: "x", PlaysPerDay: 0, HourTo: 24},
		{Name: "x", PlaysPerDay: 1, HourFrom: 10, HourTo: 9},
		{Name: "x", PlaysPerDay: 1, HourTo: 24, Position: "middle"},
		{Name: "x", PlaysPerDay: 1, HourTo: 24, Priority: 9},
	}
	for i, c := range bad {
		if err := svc.SaveCampaign(ctx, actor, c, "Anunciante"); !isValidation(err) {
			t.Errorf("caso %d: se esperaba error de validación, dio %v", i, err)
		}
	}
	if err := svc.SaveCampaign(ctx, actor, &domain.FMCampaign{Name: "x", PlaysPerDay: 1, HourTo: 24}, ""); !isValidation(err) {
		t.Error("sin anunciante debe dar error de validación")
	}
}
