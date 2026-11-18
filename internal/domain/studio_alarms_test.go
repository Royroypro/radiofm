package domain

import (
	"strings"
	"testing"
	"time"
)

func TestStudioAlarms(t *testing.T) {
	now := time.Date(2026, 11, 11, 15, 0, 0, 0, time.UTC)
	seen := now.Add(-30 * time.Second)
	backup := now.Add(-3 * time.Hour)
	base := StudioState{TokenHash: "x", LastSeen: &seen, Mode: StudioAuto}

	// sin token: la frecuencia no tiene PC, no hay alarmas
	if al := (StudioState{}).Alarms(now, 3*time.Minute); len(al) != 0 {
		t.Fatalf("sin PC no debe haber alarmas: %v", al)
	}
	// todo bien
	ok := base
	ok.StatusJSON = `{"running":true,"left_today_s":30000,"web":{"enabled":true,"connected":true},"backup_at":"` +
		backup.Format(time.RFC3339) + `"}`
	if al := ok.Alarms(now, 3*time.Minute); len(al) != 0 {
		t.Fatalf("todo en orden no debe dar alarmas: %v", al)
	}
	// sin señal
	old := now.Add(-10 * time.Minute)
	off := base
	off.LastSeen = &old
	if al := off.Alarms(now, 3*time.Minute); len(al) != 1 || al[0].Level != AlarmCritical || !strings.Contains(al[0].Text, "Sin señal") {
		t.Fatalf("PC sin señal: %v", al)
	}
	// silencio, detenida, web caída y sin respaldo
	bad := base
	bad.StatusJSON = `{"running":false,"silence_for_s":95,"web":{"enabled":true,"connected":false,"error":"rechazado"}}`
	al := bad.Alarms(now, 3*time.Minute)
	text := ""
	crit := 0
	for _, a := range al {
		text += a.Text + " | "
		if a.Level == AlarmCritical {
			crit++
		}
	}
	for _, want := range []string{"detenida", "Silencio al aire hace 95 s", "no llega a la web: rechazado", "respaldo"} {
		if !strings.Contains(text, want) {
			t.Fatalf("falta %q en %s", want, text)
		}
	}
	if crit != 2 {
		t.Fatalf("detenida y silencio son críticas: %s", text)
	}
	// PC con versión anterior (sin estado): solo se sabe si está conectada
	legacy := base
	legacy.StatusJSON = "{}"
	if al := legacy.Alarms(now, 3*time.Minute); len(al) != 0 {
		t.Fatalf("sin estado no se inventan alarmas: %v", al)
	}
}

func TestStudioAlarmsWithEmisor(t *testing.T) {
	now := time.Date(2026, 11, 18, 12, 0, 0, 0, time.UTC)
	ago := func(d time.Duration) *time.Time { x := now.Add(-d); return &x }
	// Solo Club25 Emisor da señal: no hay «sin señal» ni alarmas de automatización.
	only := StudioState{TokenHash: "h", EmisorLastSeen: ago(time.Minute), EmisorVersion: "1.5.0",
		EmisorStatusJSON: `{"radio":{"state":"en_vivo","bitrate_kbps":128},"tv":{"state":"detenido"}}`}
	if al := only.Alarms(now, 3*time.Minute); len(al) != 0 {
		t.Fatalf("solo emisor al aire: %+v", al)
	}
	if src := only.SignalSources(now, 3*time.Minute); len(src) != 1 || src[0] != "Club25 Emisor" {
		t.Fatalf("fuentes: %v", src)
	}
	if r := only.EmisorReport(); !r.Radio.Live() || r.Radio.Label() != "al aire" || r.TV.Label() != "detenida" {
		t.Fatalf("reporte: %+v", r)
	}
	// El emisor no llega al servidor: aviso con el motivo.
	bad := only
	bad.EmisorStatusJSON = `{"radio":{"state":"reconectando","message":"El servidor rechazó la clave (401)."}}`
	if al := bad.Alarms(now, 3*time.Minute); len(al) != 1 || al[0].Level != AlarmWarning || !strings.Contains(al[0].Text, "Club25 Emisor: la radio") {
		t.Fatalf("emisor reconectando: %+v", al)
	}
	// Ninguno da señal: «sin señal» con la última de las dos.
	off := StudioState{TokenHash: "h", LastSeen: ago(2 * time.Hour), EmisorLastSeen: ago(time.Hour)}
	al := off.Alarms(now, 3*time.Minute)
	if len(al) != 1 || al[0].Level != AlarmCritical || !strings.Contains(al[0].Text, "Sin señal") || !off.LastSignal().Equal(*off.EmisorLastSeen) {
		t.Fatalf("sin señal: %+v", al)
	}
	// Ambos dan señal.
	both := StudioState{TokenHash: "h", LastSeen: ago(time.Minute), EmisorLastSeen: ago(time.Minute)}
	if src := both.SignalSources(now, 3*time.Minute); len(src) != 2 {
		t.Fatalf("ambos: %v", src)
	}
}

func TestSignalNeedsCurrentToken(t *testing.T) {
	now := time.Date(2026, 11, 18, 12, 0, 0, 0, time.UTC)
	ago := func(d time.Duration) *time.Time { x := now.Add(-d); return &x }
	st := StudioState{TokenHash: "h", TokenCreatedAt: ago(time.Hour), LastSeen: ago(time.Minute), EmisorLastSeen: ago(time.Minute)}
	if len(st.SignalSources(now, 3*time.Minute)) != 2 {
		t.Fatal("con el token vigente, los dos están conectados")
	}
	// Token nuevo generado hace 30 s: las señales de hace 1 min eran del token anterior.
	st.TokenCreatedAt = ago(30 * time.Second)
	if st.Online(now, 3*time.Minute) || st.EmisorOnline(now, 3*time.Minute) {
		t.Fatal("una señal anterior al token vigente no cuenta como conectada")
	}
	// Token revocado: nadie está conectado.
	st.TokenHash, st.TokenCreatedAt = "", nil
	if len(st.SignalSources(now, 3*time.Minute)) != 0 {
		t.Fatal("sin token no hay programas conectados")
	}
}
