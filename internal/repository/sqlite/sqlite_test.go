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

func TestUsersAndSessions(t *testing.T) {
	ctx := context.Background()
	d := testutil.DB(t)
	users, sessions := sqlite.NewUserRepo(d), sqlite.NewSessionRepo(d)

	u := &domain.User{Email: "a@b.pe", Name: "A", PasswordHash: "x", Role: domain.RoleAdmin, Active: true}
	if err := users.Create(ctx, u); err != nil {
		t.Fatal(err)
	}
	if err := users.Create(ctx, &domain.User{Email: "A@B.PE", PasswordHash: "x", Role: domain.RoleEditor}); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("email duplicado (sin distinguir mayúsculas) debe dar ErrConflict, dio %v", err)
	}
	got, err := users.GetByEmail(ctx, "a@b.pe")
	if err != nil || got.ID != u.ID || got.Role != domain.RoleAdmin {
		t.Fatalf("GetByEmail = %+v, %v", got, err)
	}
	if _, err := users.GetByID(ctx, 999); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("GetByID inexistente debe dar ErrNotFound, dio %v", err)
	}

	now := time.Now()
	s := &domain.Session{TokenHash: "h", UserID: u.ID, CreatedAt: now, ExpiresAt: now.Add(-time.Minute)}
	if err := sessions.Create(ctx, s); err != nil {
		t.Fatal(err)
	}
	if n, err := sessions.DeleteExpired(ctx, now); err != nil || n != 1 {
		t.Fatalf("DeleteExpired = %d, %v", n, err)
	}
}

func TestAdsAndStats(t *testing.T) {
	ctx := context.Background()
	d := testutil.DB(t)
	ads, events := sqlite.NewAdRepo(d), sqlite.NewAdEventRepo(d)

	end := time.Now().Add(24 * time.Hour)
	a := &domain.Ad{Name: "Banner", Type: domain.AdBanner, Position: domain.PosHeader, FileURL: "/uploads/ads/x.jpg",
		Weight: 2, Active: true, EndsAt: &end}
	if err := ads.Create(ctx, a); err != nil {
		t.Fatal(err)
	}
	b := &domain.Ad{Name: "Spot", Type: domain.AdAudioSpot, FileURL: "/uploads/ads/x.mp3", Weight: 1}
	if err := ads.Create(ctx, b); err != nil {
		t.Fatal(err)
	}
	list, err := ads.List(ctx, domain.AdFilter{Type: domain.AdBanner, OnlyActive: true})
	if err != nil || len(list) != 1 || list[0].EndsAt == nil {
		t.Fatalf("List = %+v, %v", list, err)
	}
	if n, _ := ads.CountActive(ctx); n != 1 {
		t.Fatalf("CountActive = %d", n)
	}

	now := time.Now()
	for _, ev := range []domain.AdEventType{domain.EventImpression, domain.EventImpression, domain.EventClick} {
		if err := events.Record(ctx, domain.AdEvent{AdID: a.ID, Type: ev, CreatedAt: now}); err != nil {
			t.Fatal(err)
		}
	}
	// evento fuera de rango
	_ = events.Record(ctx, domain.AdEvent{AdID: a.ID, Type: domain.EventImpression, CreatedAt: now.AddDate(0, -2, 0)})

	stats, err := events.Stats(ctx, now.Add(-time.Hour), now.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if len(stats) != 2 {
		t.Fatalf("Stats debe incluir todos los anuncios, devolvió %d", len(stats))
	}
	if s := stats[0]; s.AdID != a.ID || s.Impressions != 2 || s.Clicks != 1 || s.CTR() != 50 {
		t.Fatalf("estadística inesperada: %+v", s)
	}
	if err := ads.Delete(ctx, a.ID); err != nil {
		t.Fatal(err)
	}
}

func TestBillingAndLicense(t *testing.T) {
	ctx := context.Background()
	d := testutil.DB(t)
	repo, lic := sqlite.NewBillingRepo(d), sqlite.NewLicenseRepo(d)

	plan := &domain.Plan{Name: "Básico", Price: 35000, Currency: "PEN", Period: domain.PeriodMonthly, Active: true}
	if err := repo.CreatePlan(ctx, plan); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.CurrentContract(ctx); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("sin contratos debe dar ErrNotFound, dio %v", err)
	}
	c := &domain.Contract{PlanID: plan.ID, StartDate: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), Amount: 35000,
		Currency: "PEN", Status: domain.ContractActive}
	if err := repo.CreateContract(ctx, c); err != nil {
		t.Fatal(err)
	}
	cur, err := repo.CurrentContract(ctx)
	if err != nil || cur.ID != c.ID || cur.PlanName != "Básico" {
		t.Fatalf("CurrentContract = %+v, %v", cur, err)
	}
	if err := repo.DeletePlan(ctx, plan.ID); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("borrar un plan en uso debe dar ErrConflict, dio %v", err)
	}
	p := &domain.Payment{ContractID: c.ID, PeriodStart: c.StartDate, PeriodEnd: c.StartDate.AddDate(0, 1, -1),
		Amount: 35000, Currency: "PEN", Status: domain.PaymentPaid, Method: "yape"}
	if err := repo.CreatePayment(ctx, p); err != nil {
		t.Fatal(err)
	}
	pays, err := repo.ListPayments(ctx, c.ID)
	if err != nil || len(pays) != 1 || !pays[0].PeriodEnd.Equal(p.PeriodEnd) {
		t.Fatalf("ListPayments = %+v, %v", pays, err)
	}

	pol, err := lic.GetPolicy(ctx)
	if err != nil || pol.GraceDays != 5 {
		t.Fatalf("política por defecto = %+v, %v", pol, err)
	}
	pol.ManualLevel, pol.ManualReason = domain.LicenseFull, "prueba"
	if err := lic.SavePolicy(ctx, pol); err != nil {
		t.Fatal(err)
	}
	if got, _ := lic.GetPolicy(ctx); got.ManualLevel != domain.LicenseFull {
		t.Fatalf("no se guardó el bloqueo manual: %+v", got)
	}
}
