package domain

import (
	"testing"
	"time"
)

func date(s string) time.Time {
	t, err := time.Parse(DateLayout, s)
	if err != nil {
		panic(err)
	}
	return t
}

func TestEvaluateLicense(t *testing.T) {
	lima, _ := time.LoadLocation("America/Lima")
	// 10 de marzo de 2026, 23:30 en Lima (ya es 11 de marzo en UTC: no debe confundirse)
	now := time.Date(2026, 3, 10, 23, 30, 0, 0, lima)
	contract := &Contract{ID: 1, StartDate: date("2026-01-01"), Status: ContractActive}
	pol := LicensePolicy{GraceDays: 5, AutoBlockLevel: LicenseNoAds}
	ptr := func(s string) *time.Time { d := date(s); return &d }

	cases := []struct {
		name      string
		pol       LicensePolicy
		contract  *Contract
		paidUntil *time.Time
		want      LicenseLevel
	}{
		{"sin contrato no se controla", pol, nil, nil, LicenseActive},
		{"pagado hasta hoy (último día)", pol, contract, ptr("2026-03-10"), LicenseActive},
		{"pagado a futuro", pol, contract, ptr("2026-04-30"), LicenseActive},
		{"vencido ayer: gracia", pol, contract, ptr("2026-03-09"), LicenseOverdue},
		{"último día de gracia", pol, contract, ptr("2026-03-05"), LicenseOverdue},
		{"gracia vencida: bloqueo automático", pol, contract, ptr("2026-03-04"), LicenseNoAds},
		{"sin pagos y contrato antiguo: bloqueo", pol, contract, nil, LicenseNoAds},
		{"sin pagos, contrato empieza hoy: gracia", pol, &Contract{StartDate: date("2026-03-10")}, nil, LicenseOverdue},
		{"bloqueo manual manda", LicensePolicy{GraceDays: 5, AutoBlockLevel: LicenseNoAds, ManualLevel: LicenseFull, ManualReason: "x"},
			contract, ptr("2026-12-31"), LicenseFull},
		{"nivel automático inválido cae a panel_only", LicensePolicy{GraceDays: 0, AutoBlockLevel: LicenseOverdue}, contract, ptr("2026-03-01"), LicensePanelOnly},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := EvaluateLicense(now, c.pol, c.contract, c.paidUntil, UnlimitedLimits())
			if got.Level != c.want {
				t.Fatalf("nivel = %s, se esperaba %s (%s)", got.Level, c.want, got.Reason)
			}
		})
	}
}

func TestLicenseLevelEffects(t *testing.T) {
	if LicenseOverdue.AdminReadOnly() || LicenseOverdue.AdsDisabled() {
		t.Error("la mora no debe restringir")
	}
	if !LicensePanelOnly.AdminReadOnly() || LicensePanelOnly.AdsDisabled() || LicensePanelOnly.PublicBlocked() {
		t.Error("panel_only: solo lectura en panel")
	}
	if !LicenseNoAds.AdsDisabled() || LicenseNoAds.PublicBlocked() {
		t.Error("no_ads: sin publicidad, web pública sigue")
	}
	if !LicenseFull.PublicBlocked() || !LicenseFull.AdsDisabled() || !LicenseFull.AdminReadOnly() {
		t.Error("full: todo bloqueado")
	}
}

func TestProgramIsOnAir(t *testing.T) {
	lima, _ := time.LoadLocation("America/Lima")
	mon := func(h, m int) time.Time { return time.Date(2026, 3, 9, h, m, 0, 0, lima) } // lunes
	tue := func(h, m int) time.Time { return time.Date(2026, 3, 10, h, m, 0, 0, lima) }
	morning := Program{Weekday: time.Monday, StartTime: "06:00", EndTime: "09:00", Active: true}
	night := Program{Weekday: time.Monday, StartTime: "22:00", EndTime: "02:00", Active: true}

	cases := []struct {
		name string
		p    Program
		at   time.Time
		want bool
	}{
		{"dentro", morning, mon(7, 30), true},
		{"inicio incluido", morning, mon(6, 0), true},
		{"fin excluido", morning, mon(9, 0), false},
		{"otro día", morning, tue(7, 0), false},
		{"inactivo", Program{Weekday: time.Monday, StartTime: "06:00", EndTime: "09:00"}, mon(7, 0), false},
		{"cruza medianoche: antes", night, mon(23, 0), true},
		{"cruza medianoche: después", night, tue(1, 0), true},
		{"cruza medianoche: terminó", night, tue(2, 30), false},
	}
	for _, c := range cases {
		if got := c.p.IsOnAir(c.at); got != c.want {
			t.Errorf("%s: IsOnAir = %v, se esperaba %v", c.name, got, c.want)
		}
	}
}

func TestSiteSettingsRoundTrip(t *testing.T) {
	s := DefaultSiteSettings()
	s.SiteName, s.TVEnabled, s.PrerollIntervalMin, s.Phone = "Otra Radio", false, 15, "+51 1 234"
	got := SiteSettingsFromMap(s.ToMap())
	if got != s {
		t.Fatalf("la conversión ida y vuelta cambió los ajustes:\n%+v\n%+v", got, s)
	}
	if len(s.ToMap()) != len(SettingKeys()) {
		t.Fatalf("ToMap tiene %d claves y settingKeys %d: actualice ambas", len(s.ToMap()), len(SettingKeys()))
	}
	for _, k := range SettingKeys() {
		if _, ok := s.ToMap()[k]; !ok {
			t.Errorf("la clave %q no está en ToMap", k)
		}
	}
}

func TestMoneyString(t *testing.T) {
	cases := map[Money]string{0: "0.00", 5: "0.05", 35000: "350.00", 123456789: "1,234,567.89", -1050: "-10.50"}
	for m, want := range cases {
		if got := m.String(); got != want {
			t.Errorf("Money(%d) = %q, se esperaba %q", int64(m), got, want)
		}
	}
}

func TestAdIsLive(t *testing.T) {
	now := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	before, after := now.Add(-time.Hour), now.Add(time.Hour)
	cases := []struct {
		ad   Ad
		want bool
	}{
		{Ad{Active: true}, true},
		{Ad{Active: false}, false},
		{Ad{Active: true, StartsAt: &after}, false},
		{Ad{Active: true, EndsAt: &before}, false},
		{Ad{Active: true, StartsAt: &before, EndsAt: &after}, true},
	}
	for i, c := range cases {
		if got := c.ad.IsLive(now); got != c.want {
			t.Errorf("caso %d: IsLive = %v", i, got)
		}
	}
}
