package events

import (
	"testing"
	"time"

	"radioclub25/estudio/internal/domain"
)

var t0 = time.Date(2026, 11, 9, 10, 0, 0, 0, time.UTC)

func at(min, sec int) time.Time {
	return t0.Add(time.Duration(min)*time.Minute + time.Duration(sec)*time.Second)
}

func soft(id int64, start time.Time, l time.Duration) domain.LogItem {
	return domain.LogItem{ID: id, At: start, Length: l, Kind: domain.SlotCategory}
}

func hard(id int64, start time.Time) domain.LogItem {
	return domain.LogItem{ID: id, At: start, Length: 5 * time.Second, Hard: true, Kind: domain.SlotTime}
}

func TestSoftNext(t *testing.T) {
	up := []domain.LogItem{soft(1, at(0, 0), 3*time.Minute), soft(2, at(3, 0), 3*time.Minute)}
	d := Next(at(0, 2), true, up, DefaultRules())
	if d.Action != Start || d.Item.ID != 1 || len(d.Skip) != 0 {
		t.Fatalf("debe arrancar el 1: %+v", d)
	}
	if d := Next(at(0, 2), false, up, DefaultRules()); d.Action != None {
		t.Fatalf("en un tick sin pedir siguiente no se hace nada: %+v", d)
	}
}

func TestHardCutSkipsSoftBefore(t *testing.T) {
	up := []domain.LogItem{soft(1, at(28, 0), 3*time.Minute), soft(2, at(31, 0), 3*time.Minute), hard(3, at(30, 0)), soft(4, at(30, 5), time.Minute)}
	// 10:29:59.9 → faltan 100 ms: se dispara (lead 150 ms)
	d := Next(at(29, 59).Add(900*time.Millisecond), false, up, DefaultRules())
	if d.Action != HardCut || d.Item.ID != 3 || len(d.Skip) != 2 {
		t.Fatalf("debe cortar para el evento duro y saltar los blandos previos: %+v", d)
	}
	if d := Next(at(29, 50), false, up, DefaultRules()); d.Action != None {
		t.Fatalf("antes de la hora no se corta: %+v", d)
	}
}

func TestHardNotAdvanced(t *testing.T) {
	up := []domain.LogItem{hard(3, at(30, 0)), soft(4, at(30, 5), time.Minute)}
	if d := Next(at(29, 0), true, up, DefaultRules()); d.Action != None {
		t.Fatalf("el evento duro no se adelanta ni se arranca lo de después: %+v", d)
	}
}

func TestRecoveryAndMissedHard(t *testing.T) {
	// la PC estuvo apagada de 10:00 a 10:40
	up := []domain.LogItem{soft(1, at(0, 0), 3*time.Minute), hard(2, at(30, 0)), soft(3, at(30, 5), 3*time.Minute),
		soft(4, at(38, 0), 3*time.Minute), soft(5, at(41, 0), 3*time.Minute)}
	d := Next(at(40, 0), true, up, DefaultRules())
	if d.Action != Start || d.Item.ID != 4 {
		t.Fatalf("debe retomar en el elemento que corresponde a la hora: %+v", d)
	}
	if len(d.Skip) != 3 || d.Skip[1] != 2 {
		t.Fatalf("debe saltar los atrasados y el duro perdido: %v", d.Skip)
	}
}

func TestOptionalLate(t *testing.T) {
	opt := soft(1, at(58, 0), 3*time.Minute)
	opt.Optional = true
	up := []domain.LogItem{opt, soft(2, at(61, 0), 3*time.Minute)}
	if d := Next(at(58, 10), true, up, DefaultRules()); d.Item.ID != 1 {
		t.Fatalf("un opcional a tiempo suena: %+v", d)
	}
	if d := Next(at(59, 0), true, up, DefaultRules()); d.Item.ID != 2 || len(d.Skip) != 1 {
		t.Fatalf("un opcional tardío se salta: %+v", d)
	}
}
