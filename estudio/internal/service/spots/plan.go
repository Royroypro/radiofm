package spots

import (
	"sort"
	"strings"
	"time"
	"unicode"

	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"

	"radioclub25/estudio/internal/domain"
)

// Tanda es una tanda futura del día con su hora prevista.
type Tanda struct {
	ID int64     // LogItem.ID
	At time.Time // hora prevista
}

// Need es cuántas veces debe sonar todavía hoy una cuña.
type Need struct {
	Spot  domain.Spot
	Count int // plays_per_day − las que ya sonaron hoy
}

// Plan reparte las cuñas entre las tandas. Función pura.
//
//   - Cada cuña va solo a tandas dentro de su franja y vigencia.
//   - Reparto equitativo: si quedan n emisiones y hay m tandas válidas, se eligen
//     tandas espaciadas (posiciones (k+½)·m/n).
//   - Nunca dos cuñas del mismo anunciante en la misma tanda, ni la misma cuña dos veces.
//     Si la tanda ideal choca, se usa la tanda válida libre más cercana.
//   - Se planifican primero las cuñas con menos tandas posibles (las más difíciles).
//
// Devuelve, por tanda, los IDs de cuña en orden de reparto (y las que no cupieron).
func Plan(tandas []Tanda, needs []Need, loc *time.Location) (map[int64][]int64, map[int64]int) {
	out := map[int64][]int64{}
	missing := map[int64]int{}
	sort.SliceStable(tandas, func(i, j int) bool { return tandas[i].At.Before(tandas[j].At) })
	type job struct {
		need  Need
		valid []int // índices de tandas válidas
	}
	var jobs []job
	for _, n := range needs {
		if n.Count <= 0 {
			continue
		}
		var valid []int
		for i, t := range tandas {
			if n.Spot.LiveAt(t.At, loc) {
				valid = append(valid, i)
			}
		}
		jobs = append(jobs, job{n, valid})
	}
	sort.SliceStable(jobs, func(i, j int) bool {
		if len(jobs[i].valid) != len(jobs[j].valid) {
			return len(jobs[i].valid) < len(jobs[j].valid)
		}
		return jobs[i].need.Spot.ID < jobs[j].need.Spot.ID
	})
	advertisers := map[int64]map[string]bool{} // tanda → anunciantes ya puestos
	for _, j := range jobs {
		s := j.need.Spot
		count := min(j.need.Count, len(j.valid))
		missing[s.ID] = j.need.Count - count
		used := map[int]bool{}
		placed := 0
		for k := 0; k < count; k++ {
			ideal := int(float64(len(j.valid)) * (float64(k) + 0.5) / float64(count))
			pos := nearestFree(j.valid, ideal, func(p int) bool {
				t := tandas[j.valid[p]]
				return !used[p] && !advertisers[t.ID][normalizeAdv(s.Advertiser)]
			})
			if pos < 0 {
				continue
			}
			used[pos] = true
			t := tandas[j.valid[pos]]
			out[t.ID] = append(out[t.ID], s.ID)
			if advertisers[t.ID] == nil {
				advertisers[t.ID] = map[string]bool{}
			}
			advertisers[t.ID][normalizeAdv(s.Advertiser)] = true
			placed++
		}
		missing[s.ID] = j.need.Count - placed
		if missing[s.ID] == 0 {
			delete(missing, s.ID)
		}
	}
	return out, missing
}

// nearestFree busca desde ideal hacia ambos lados la primera posición que cumpla ok.
func nearestFree(valid []int, ideal int, ok func(int) bool) int {
	for d := 0; d < len(valid); d++ {
		for _, p := range []int{ideal + d, ideal - d} {
			if p >= 0 && p < len(valid) && ok(p) {
				return p
			}
		}
	}
	return -1
}

var stripMarks = transform.Chain(norm.NFD, runes.Remove(runes.In(unicode.Mn)), norm.NFC)

// normalizeAdv compara anunciantes sin mayúsculas, tildes ni espacios ("Ferretería El Sol" = "ferreteria elsol").
func normalizeAdv(s string) string {
	out, _, err := transform.String(stripMarks, strings.ToLower(s))
	if err != nil {
		out = strings.ToLower(s)
	}
	return strings.Join(strings.Fields(out), "")
}
