// Tests de los servicios con repositorios SQLite reales (base temporal).
package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"radioclub25/internal/domain"
	"radioclub25/internal/repository/sqlite"
	"radioclub25/internal/service/ads"
	"radioclub25/internal/service/audit"
	"radioclub25/internal/service/auth"
	"radioclub25/internal/service/billing"
	"radioclub25/internal/service/gate"
	"radioclub25/internal/service/license"
	"radioclub25/internal/service/media"
	"radioclub25/internal/service/settings"
	"radioclub25/internal/testutil"
)

type noFiles struct{}

func (noFiles) Delete(string) error { return nil }

// fixedLicense permite fijar el estado de licencia en los tests.
type fixedLicense struct{ st domain.LicenseState }

func (f *fixedLicense) Current() domain.LicenseState { return f.st }

func TestAuthLoginAndPermissions(t *testing.T) {
	ctx := context.Background()
	d := testutil.DB(t)
	au := audit.New(sqlite.NewAuditRepo(d), testutil.Logger())
	svc := auth.New(sqlite.NewUserRepo(d), sqlite.NewSessionRepo(d), au, time.Hour)

	super, created, err := svc.EnsureSuperAdmin(ctx, "Super@Test.pe", "supersecreta")
	if err != nil || !created {
		t.Fatalf("EnsureSuperAdmin: %v", err)
	}
	superActor := domain.Actor{UserID: super.ID, Email: super.Email, Role: domain.RoleSuperAdmin}
	admin, err := svc.CreateUser(ctx, superActor, auth.UserInput{Email: "admin@test.pe", Name: "Admin", Role: domain.RoleAdmin, Active: true, Password: "adminpass"})
	if err != nil {
		t.Fatal(err)
	}
	adminActor := domain.Actor{UserID: admin.ID, Email: admin.Email, Role: domain.RoleAdmin}

	// Nadie crea superadmins desde el panel
	if _, err := svc.CreateUser(ctx, superActor, auth.UserInput{Email: "x@test.pe", Name: "X", Role: domain.RoleSuperAdmin, Password: "12345678"}); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("crear superadmin debe dar ErrForbidden, dio %v", err)
	}
	// Un admin no puede tocar al superadmin
	if _, err := svc.GetUser(ctx, adminActor, super.ID); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("admin → superadmin debe dar ErrForbidden, dio %v", err)
	}
	// Un admin no ve al superadmin en la lista
	list, _ := svc.ListUsers(ctx, adminActor)
	for _, u := range list {
		if u.Role == domain.RoleSuperAdmin {
			t.Fatal("el admin no debe ver superadmins")
		}
	}

	token, _, err := svc.Login(ctx, "ADMIN@test.pe", "adminpass", "1.1.1.1", "test")
	if err != nil || token == "" {
		t.Fatalf("Login: %v", err)
	}
	if u, err := svc.Authenticate(ctx, token); err != nil || u.ID != admin.ID {
		t.Fatalf("Authenticate: %v", err)
	}
	if _, _, err := svc.Login(ctx, "admin@test.pe", "mala", "1.1.1.1", "test"); !errors.Is(err, domain.ErrInvalidCredentials) {
		t.Fatalf("contraseña mala debe dar ErrInvalidCredentials, dio %v", err)
	}
	for i := 0; i < 10; i++ {
		_, _, _ = svc.Login(ctx, "admin@test.pe", "mala", "2.2.2.2", "test")
	}
	if _, _, err := svc.Login(ctx, "admin@test.pe", "adminpass", "2.2.2.2", "test"); !errors.Is(err, domain.ErrTooManyAttempts) {
		t.Fatalf("tras 10 fallos debe limitar, dio %v", err)
	}
	if err := svc.Logout(ctx, token); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Authenticate(ctx, token); !errors.Is(err, domain.ErrUnauthorized) {
		t.Fatal("la sesión debe quedar cerrada")
	}
}

func TestAdsSelectionLimitsAndLicense(t *testing.T) {
	ctx := context.Background()
	d := testutil.DB(t)
	au := audit.New(sqlite.NewAuditRepo(d), testutil.Logger())
	lic := &fixedLicense{st: domain.LicenseState{Level: domain.LicenseActive, Limits: domain.PlanLimits{MaxActiveAds: 2, StatsEnabled: true}}}
	svc := ads.New(sqlite.NewAdRepo(d), sqlite.NewAdEventRepo(d), lic, noFiles{}, au, "sal")
	actor := domain.SystemActor

	heavy := &domain.Ad{Name: "Pesado", Type: domain.AdBanner, Position: domain.PosHeader, FileURL: "/u/a.jpg", Weight: 9, Active: true}
	light := &domain.Ad{Name: "Ligero", Type: domain.AdBanner, Position: domain.PosHeader, FileURL: "/u/b.jpg", Weight: 1, Active: true}
	for _, a := range []*domain.Ad{heavy, light} {
		if err := svc.Create(ctx, actor, a); err != nil {
			t.Fatal(err)
		}
	}
	// límite del plan: 2 activos
	third := &domain.Ad{Name: "Tercero", Type: domain.AdAudioSpot, FileURL: "/u/c.mp3", Weight: 1, Active: true}
	if err := svc.Create(ctx, actor, third); !errors.Is(err, domain.ErrPlanLimit) {
		t.Fatalf("el tercer anuncio activo debe dar ErrPlanLimit, dio %v", err)
	}
	// validación
	if err := svc.Create(ctx, actor, &domain.Ad{Name: "X", Type: domain.AdBanner, FileURL: "/u/x.jpg", Weight: 1}); err == nil {
		t.Fatal("un banner sin posición debe fallar")
	}

	// selección ponderada: el pesado sale ~90 %
	counts := map[int64]int{}
	for i := 0; i < 2000; i++ {
		b, err := svc.PickBanners(ctx)
		if err != nil {
			t.Fatal(err)
		}
		counts[b[domain.PosHeader].ID]++
	}
	if share := float64(counts[heavy.ID]) / 2000; share < 0.85 || share > 0.95 {
		t.Fatalf("el anuncio de peso 9 salió el %.0f %% de las veces (se esperaba ~90 %%)", share*100)
	}

	// licencia sin publicidad
	lic.st.Level = domain.LicenseNoAds
	if b, _ := svc.PickBanners(ctx); len(b) != 0 {
		t.Fatal("con no_ads no debe haber banners")
	}

	// estadísticas deshabilitadas en el plan
	lic.st.Limits.StatsEnabled = false
	if _, err := svc.Stats(ctx, time.Now().Add(-time.Hour), time.Now()); !errors.Is(err, domain.ErrPlanLimit) {
		t.Fatalf("sin estadísticas en el plan debe dar ErrPlanLimit, dio %v", err)
	}
}

func TestLicenseRefreshCutsStreams(t *testing.T) {
	ctx := context.Background()
	d := testutil.DB(t)
	lima := testutil.Lima(t)
	au := audit.New(sqlite.NewAuditRepo(d), testutil.Logger())
	radio, tv := &testutil.FakeRadio{Online: true}, &testutil.FakeTV{Online: true}
	mediaSvc := media.New(radio, tv, testutil.Logger())
	billingRepo := sqlite.NewBillingRepo(d)
	lic := license.New(sqlite.NewLicenseRepo(d), billingRepo, mediaSvc, au, testutil.Logger(), lima)
	bill := billing.New(billingRepo, au, lima)
	bill.OnChange(func(ctx context.Context) { _, _ = lic.Refresh(ctx) })

	if st, _ := lic.Refresh(ctx); st.Level != domain.LicenseActive || st.Enforced {
		t.Fatalf("sin contrato: %+v", st)
	}
	plan := &domain.Plan{Name: "P", Price: 100, Period: domain.PeriodMonthly, Active: true, TVEnabled: true}
	if err := bill.SavePlan(ctx, domain.SystemActor, plan); err != nil {
		t.Fatal(err)
	}
	start := time.Now().In(lima).AddDate(0, -2, 0)
	c := &domain.Contract{PlanID: plan.ID, StartDate: time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, time.UTC),
		Amount: 100, Status: domain.ContractActive}
	if err := bill.SaveContract(ctx, domain.SystemActor, c); err != nil {
		t.Fatal(err)
	}
	if lic.Current().Level != domain.LicensePanelOnly {
		t.Fatalf("contrato de hace 2 meses sin pagos debe bloquear, nivel %s", lic.Current().Level)
	}
	// bloqueo total manual → corta streams
	if err := lic.Block(ctx, domain.SystemActor, domain.LicenseFull, "prueba"); err != nil {
		t.Fatal(err)
	}
	if radio.Killed != 1 || tv.Kicked != 1 {
		t.Fatalf("el bloqueo total debe cortar radio y TV (radio=%d tv=%d)", radio.Killed, tv.Kicked)
	}
	// desbloquear y pagar el periodo → activo
	if err := lic.Unblock(ctx, domain.SystemActor); err != nil {
		t.Fatal(err)
	}
	ps, pe, err := bill.NextPeriod(ctx, c)
	if err != nil || !ps.Equal(c.StartDate) {
		t.Fatalf("NextPeriod = %v %v %v", ps, pe, err)
	}
	pay := &domain.Payment{ContractID: c.ID, PeriodStart: ps, PeriodEnd: time.Now().AddDate(0, 1, 0).Truncate(24 * time.Hour),
		Amount: 100, Status: domain.PaymentPaid, Method: "yape"}
	if err := bill.SavePayment(ctx, domain.SystemActor, pay); err != nil {
		t.Fatal(err)
	}
	if lic.Current().Level != domain.LicenseActive {
		t.Fatalf("tras pagar debe quedar activo, nivel %s (%s)", lic.Current().Level, lic.Current().Reason)
	}
}

func TestGate(t *testing.T) {
	ctx := context.Background()
	d := testutil.DB(t)
	au := audit.New(sqlite.NewAuditRepo(d), testutil.Logger())
	set := settings.New(sqlite.NewSettingsRepo(d), au)
	if err := set.EnsureDefaults(ctx); err != nil {
		t.Fatal(err)
	}
	s, _ := set.Get(ctx)
	if len(s.TVPublishPass) < 8 {
		t.Fatal("EnsureDefaults debe generar una clave de TV")
	}
	lic := &fixedLicense{st: domain.LicenseState{Level: domain.LicenseActive, Limits: domain.UnlimitedLimits()}}
	g := gate.New(lic, set, "tv", "fuente")

	pub := gate.TVRequest{Action: "publish", Path: "tv", User: s.TVPublishUser, Password: s.TVPublishPass}
	if !g.AuthorizeTV(ctx, pub) {
		t.Fatal("publicar con la clave correcta debe permitirse")
	}
	bad := pub
	bad.Password = "otra"
	if g.AuthorizeTV(ctx, bad) {
		t.Fatal("clave incorrecta debe rechazarse")
	}
	if g.AuthorizeTV(ctx, gate.TVRequest{Action: "read", Path: "otro"}) {
		t.Fatal("otro path debe rechazarse")
	}
	if g.AuthorizeTV(ctx, gate.TVRequest{Action: "api", Path: "tv"}) {
		t.Fatal("la API de MediaMTX no se autoriza por aquí")
	}
	if !g.RadioSourceAllowed(ctx, "fuente") || g.RadioSourceAllowed(ctx, "mala") {
		t.Fatal("contraseña de fuente de radio mal validada")
	}
	lic.st.Level = domain.LicenseFull
	if g.AuthorizeTV(ctx, pub) || g.RadioAllowed(ctx) || g.RadioSourceAllowed(ctx, "fuente") {
		t.Fatal("con bloqueo total todo debe rechazarse")
	}
	lic.st.Level = domain.LicenseActive
	lic.st.Limits.TVEnabled = false
	if g.AuthorizeTV(ctx, gate.TVRequest{Action: "read", Path: "tv"}) {
		t.Fatal("si el plan no incluye TV, no se puede ver")
	}
}

func TestSettingsValidation(t *testing.T) {
	ctx := context.Background()
	d := testutil.DB(t)
	set := settings.New(sqlite.NewSettingsRepo(d), audit.New(sqlite.NewAuditRepo(d), testutil.Logger()))
	_ = set.EnsureDefaults(ctx)
	s, _ := set.Get(ctx)

	bad := s
	bad.ColorRed = "red; background:url(x)"
	if err := set.Update(ctx, domain.SystemActor, bad); err == nil {
		t.Fatal("un color que no es #RRGGBB debe rechazarse")
	}
	bad = s
	bad.Facebook = "javascript:alert(1)"
	if err := set.Update(ctx, domain.SystemActor, bad); err == nil {
		t.Fatal("un enlace que no es http(s) debe rechazarse")
	}
	ok := s
	ok.SiteName = "Nueva Radio"
	if err := set.Update(ctx, domain.SystemActor, ok); err != nil {
		t.Fatal(err)
	}
	if got, _ := set.Get(ctx); got.SiteName != "Nueva Radio" {
		t.Fatal("la caché no se invalidó al guardar")
	}
}
