package timeannounce

import (
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func fakeFiles(present ...string) Files {
	set := map[string]bool{}
	for _, p := range present {
		set[filepath.Join(`C:\loc`, p)] = true
	}
	return Files{Dir: `C:\loc`, HourPrefix: "HRS", MinutePrefix: "MIN", TempPrefix: "TMP",
		Exists: func(p string) bool { return set[p] }}
}

func TestTime(t *testing.T) {
	f := fakeFiles("HRS15.mp3", "MIN07.wav", "TMP-3.mp3", "TMP22.mp3")
	got, err := f.Time(time.Date(2026, 1, 1, 15, 7, 59, 0, time.UTC))
	if err != nil || len(got) != 2 || !strings.HasSuffix(got[0], "HRS15.mp3") || !strings.HasSuffix(got[1], "MIN07.wav") {
		t.Fatalf("Time = %v, %v", got, err)
	}
	if _, err := f.Time(time.Date(2026, 1, 1, 16, 0, 0, 0, time.UTC)); err == nil || !strings.Contains(err.Error(), "HRS16") {
		t.Fatalf("debe avisar qué falta: %v", err)
	}
	if got, err := f.Temperature(21.6); err != nil || !strings.HasSuffix(got[0], "TMP22.mp3") {
		t.Fatalf("Temperature(21.6) = %v, %v", got, err)
	}
	if got, err := f.Temperature(-2.7); err != nil || !strings.HasSuffix(got[0], "TMP-3.mp3") {
		t.Fatalf("Temperature(-2.7) = %v, %v", got, err)
	}
	if _, err := f.Temperature(50); err == nil {
		t.Fatal("fuera de rango debe dar error")
	}
	if n := len(f.Check()); n != 24+60+56-4 {
		t.Fatalf("faltantes = %d", n)
	}
}
