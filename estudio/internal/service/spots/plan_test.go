package spots

import (
	"testing"
	"time"

	"radioclub25/estudio/internal/domain"
)

var lima = time.FixedZone("PET", -5*3600)
var day = time.Date(2026, 11, 9, 0, 0, 0, 0, lima)

// tandas a las :30 de cada hora de 6 a 23 (18 tandas).
func dayTandas() []Tanda {
	var out []Tanda
	for h := 6; h < 24; h++ {
		out = append(out, Tanda{ID: int64(100 + h), At: day.Add(time.Duration(h)*time.Hour + 30*time.Minute)})
	}
	return out
}

func spot(id int64, adv string, perDay, from, to int) domain.Spot {
	return domain.Spot{ID: id, Advertiser: adv, Name: adv, Path: "x.mp3", PlaysPerDay: perDay, HourFrom: from,
		HourTo: to, Active: true, Duration: 20 * time.Second}
}

func TestPlanEvenAndInRange(t *testing.T) {
	needs := []Need{{Spot: spot(1, "Ferretería", 6, 6, 24), Count: 6}, {Spot: spot(2, "Farmacia", 3, 7, 13), Count: 3}}
	plan, missing := Plan(dayTandas(), needs, lima)
	if len(missing) != 0 {
		t.Fatalf("todas deben caber: %v", missing)
	}
	var hours1, hours2 []int
	for _, tnd := range dayTandas() {
		for _, id := range plan[tnd.ID] {
			if id == 1 {
				hours1 = append(hours1, tnd.At.Hour())
			} else {
				hours2 = append(hours2, tnd.At.Hour())
			}
		}
	}
	if len(hours1) != 6 || len(hours2) != 3 {
		t.Fatalf("emisiones: ferretería %v, farmacia %v", hours1, hours2)
	}
	// 6 de 18 tandas: separadas ~3 h
	for i := 1; i < len(hours1); i++ {
		if d := hours1[i] - hours1[i-1]; d < 2 || d > 4 {
			t.Fatalf("reparto no equitativo: %v", hours1)
		}
	}
	for _, h := range hours2 {
		if h < 7 || h >= 13 {
			t.Fatalf("la farmacia salió fuera de su franja: %v", hours2)
		}
	}
}

func TestPlanNoSameAdvertiserInTanda(t *testing.T) {
	tandas := dayTandas()[:3]
	needs := []Need{
		{Spot: spot(1, "Ferretería El Sol", 3, 0, 24), Count: 3},
		{Spot: spot(2, "ferreteria  el sol", 3, 0, 24), Count: 3}, // el mismo anunciante escrito distinto
		{Spot: spot(3, "Ferretería El Sol", 2, 0, 24), Count: 2},
	}
	plan, missing := Plan(tandas, needs, lima)
	for _, tnd := range tandas {
		seen := map[int64]bool{}
		for _, id := range plan[tnd.ID] {
			if seen[id] {
				t.Fatalf("cuña %d dos veces en la tanda %d", id, tnd.ID)
			}
			seen[id] = true
		}
		if (seen[1] && seen[3]) || (seen[1] && seen[2]) || (seen[2] && seen[3]) {
			t.Fatalf("dos cuñas del mismo anunciante en la tanda %d: %v", tnd.ID, plan[tnd.ID])
		}
	}
	// las tres son del mismo anunciante: necesitan 8 emisiones y solo hay 3 tandas
	if missing[1]+missing[2]+missing[3] != 5 {
		t.Fatalf("deberían faltar 5 emisiones del anunciante repetido: %v", missing)
	}
}

func TestPlanRespectsValidityAndRemaining(t *testing.T) {
	end := day.Add(12 * time.Hour)
	s := spot(1, "Promo", 10, 0, 24)
	s.EndsAt = &end
	plan, missing := Plan(dayTandas(), []Need{{Spot: s, Count: 10}}, lima)
	n := 0
	for _, tnd := range dayTandas() {
		for range plan[tnd.ID] {
			if !tnd.At.Before(end) {
				t.Fatal("no debe planificarse después del fin de vigencia")
			}
			n++
		}
	}
	if n != 6 || missing[1] != 4 { // tandas 6:30 … 11:30
		t.Fatalf("planificadas %d, faltan %v", n, missing)
	}
	if plan, _ := Plan(dayTandas(), []Need{{Spot: s, Count: 0}}, lima); len(plan) != 0 {
		t.Fatal("una cuña que ya cumplió sus veces no se planifica")
	}
}
