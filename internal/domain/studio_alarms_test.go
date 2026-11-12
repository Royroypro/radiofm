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
