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
	needs := []Need{{Versions: []domain.Spot{spot(1, "Ferretería", 6, 6, 24)}, Count: 6}, {Versions: []domain.Spot{spot(2, "Farmacia", 3, 7, 13)}, Count: 3}}
	plan, missing := Plan(dayTandas(), needs, lima, Limits{})
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
		{Versions: []domain.Spot{spot(1, "Ferretería El Sol", 3, 0, 24)}, Count: 3},
		{Versions: []domain.Spot{spot(2, "ferreteria  el sol", 3, 0, 24)}, Count: 3}, // el mismo anunciante escrito distinto
		{Versions: []domain.Spot{spot(3, "Ferretería El Sol", 2, 0, 24)}, Count: 2},
	}
	plan, missing := Plan(tandas, needs, lima, Limits{})
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
	if missing[-1].Count+missing[-2].Count+missing[-3].Count != 5 {
		t.Fatalf("deberían faltar 5 emisiones del anunciante repetido: %v", missing)
	}
}

func TestPlanRespectsValidityAndRemaining(t *testing.T) {
	end := day.Add(12 * time.Hour)
	s := spot(1, "Promo", 10, 0, 24)
	s.EndsAt = &end
	plan, missing := Plan(dayTandas(), []Need{{Versions: []domain.Spot{s}, Count: 10}}, lima, Limits{})
	n := 0
	for _, tnd := range dayTandas() {
		for range plan[tnd.ID] {
			if !tnd.At.Before(end) {
				t.Fatal("no debe planificarse después del fin de vigencia")
			}
			n++
		}
	}
	if n != 6 || missing[-1].Count != 4 || missing[-1].Reason != ReasonFewBreaks { // tandas 6:30 … 11:30
		t.Fatalf("planificadas %d, faltan %v", n, missing)
	}
	if plan, _ := Plan(dayTandas(), []Need{{Versions: []domain.Spot{s}, Count: 0}}, lima, Limits{}); len(plan) != 0 {
		t.Fatal("una cuña que ya cumplió sus veces no se planifica")
	}
}

// Campaña con dos versiones de pesos 2 y 1: las veces por día son de la campaña
// y las versiones rotan A, B, A…, continuando desde lo emitido hoy.
func TestPlanCampaignVersionsRotate(t *testing.T) {
	a, b := spot(10, "Banco", 6, 0, 24), spot(11, "Banco", 6, 0, 24)
	a.CampaignID, b.CampaignID, a.Weight, b.Weight = 7, 7, 2, 1
	plan, missing := Plan(dayTandas(), []Need{{Versions: []domain.Spot{a, b}, Count: 6}}, lima, Limits{})
	if len(missing) != 0 {
		t.Fatalf("faltan %v", missing)
	}
	var seq []int64
	for _, tnd := range dayTandas() {
		seq = append(seq, plan[tnd.ID]...)
	}
	if len(seq) != 6 {
		t.Fatalf("la campaña debe sonar 6 veces en total (no 6 por versión): %v", seq)
	}
	na := 0
	for _, id := range seq {
		if id == 10 {
			na++
		}
	}
	if na != 4 {
		t.Fatalf("con pesos 2:1, la versión A debe sonar 4 de 6: %v", seq)
	}
	// ya sonaron 2 hoy: la rotación continúa (no empieza siempre por A)
	p2, _ := Plan(dayTandas(), []Need{{Versions: []domain.Spot{a, b}, Count: 1, Aired: 1}}, lima, Limits{})
	for _, ids := range p2 {
		if ids[0] != 11 {
			t.Fatalf("tras una emisión de A sigue B: %v", ids)
		}
	}
}

// Competidores (mismo rubro) nunca en la misma tanda.
func TestPlanSectorSeparation(t *testing.T) {
	tandas := dayTandas()[:4]
	b1, b2 := spot(1, "Banco Andino", 4, 0, 24), spot(2, "Caja Huacho", 4, 0, 24)
	b1.Sector, b2.Sector = "banca", "Banca "
	plan, missing := Plan(tandas, []Need{{Versions: []domain.Spot{b1}, Count: 2}, {Versions: []domain.Spot{b2}, Count: 2}}, lima, Limits{})
	if len(missing) != 0 {
		t.Fatalf("en 4 tandas entran 2+2 sin cruzarse: %v", missing)
	}
	for _, tnd := range tandas {
		if len(plan[tnd.ID]) > 1 {
			t.Fatalf("dos bancos en la tanda %d: %v", tnd.ID, plan[tnd.ID])
		}
	}
}

// Topes por tanda y prioridad: si no hay espacio, cede la de prioridad baja.
func TestPlanLimitsAndPriority(t *testing.T) {
	tandas := dayTandas()[:2]
	hi, lo := spot(1, "Alta", 2, 0, 24), spot(2, "Baja", 2, 0, 24)
	hi.Priority, lo.Priority = 1, 3
	other := spot(3, "Normal", 2, 0, 24)
	plan, missing := Plan(tandas, []Need{
		{Versions: []domain.Spot{lo}, Count: 2}, {Versions: []domain.Spot{other}, Count: 2}, {Versions: []domain.Spot{hi}, Count: 2},
	}, lima, Limits{MaxSpots: 2})
	for _, tnd := range tandas {
		if len(plan[tnd.ID]) > 2 {
			t.Fatalf("tope de 2 cuñas superado: %v", plan[tnd.ID])
		}
	}
	if missing[-2].Count != 2 || missing[-2].Reason != ReasonFull || missing[-1].Count != 0 || missing[-3].Count != 0 {
		t.Fatalf("debe ceder la de prioridad baja: %+v", missing)
	}
	// tope por duración: 40 s con cuñas de 20 s = 2 por tanda
	_, missing = Plan(tandas, []Need{
		{Versions: []domain.Spot{spot(4, "A", 2, 0, 24)}, Count: 2}, {Versions: []domain.Spot{spot(5, "B", 2, 0, 24)}, Count: 2},
		{Versions: []domain.Spot{spot(6, "C", 2, 0, 24)}, Count: 2},
	}, lima, Limits{MaxLength: 40 * time.Second})
	total := 0
	for _, m := range missing {
		total += m.Count
	}
	if total != 2 {
		t.Fatalf("con 40 s por tanda caben 4 de 6: faltan %+v", missing)
	}
}

// Posición pedida: la «primera» abre la tanda y la «última» la cierra; días de la semana.
func TestPlanPositionAndDays(t *testing.T) {
	tandas := dayTandas()[:1] // lunes 9/11/2026
	first, last, mid := spot(1, "Primero", 1, 0, 24), spot(2, "Último", 1, 0, 24), spot(3, "Medio", 1, 0, 24)
	first.Position, last.Position = "first", "last"
	weekend := spot(4, "Finde", 1, 0, 24)
	weekend.DaysMask = 0b1000001
	plan, missing := Plan(tandas, []Need{
		{Versions: []domain.Spot{last}, Count: 1}, {Versions: []domain.Spot{mid}, Count: 1},
		{Versions: []domain.Spot{first}, Count: 1}, {Versions: []domain.Spot{weekend}, Count: 1},
	}, lima, Limits{})
	got := plan[tandas[0].ID]
	if len(got) != 3 || got[0] != 1 || got[2] != 2 {
		t.Fatalf("orden en la tanda = %v, quería [1 3 2]", got)
	}
	if missing[-4].Count != 1 || missing[-4].Reason != ReasonFewBreaks {
		t.Fatalf("la de fin de semana no suena un lunes: %+v", missing)
	}
}
