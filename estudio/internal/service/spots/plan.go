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

// Need es lo que falta emitir hoy de una campaña (una cuña local es su propia campaña).
type Need struct {
	Versions []domain.Spot // versiones activas con audio (las reglas son las de Versions[0])
	Count    int           // veces por día − las que ya sonaron hoy
	Aired    int           // emisiones de hoy (continúa la rotación de versiones)
}

// Limits son los topes de una tanda (0 = sin tope).
type Limits struct {
	MaxSpots  int           // cuñas por tanda
	MaxLength time.Duration // duración de las cuñas de una tanda (sin cortinas)
}

// Missing es lo que no entró de una campaña y por qué.
type Missing struct {
	Advertiser string `json:"advertiser"`
	Name       string `json:"name"`
	Count      int    `json:"count"`
	Reason     string `json:"reason"`
}

// Motivos de lo que no entra.
const (
	ReasonFewBreaks = "no hay tandas suficientes en su franja y días"
	ReasonFull      = "tandas llenas o con un competidor del mismo rubro"
)

// unknownLength se usa para las cuñas sin duración medida al calcular el tope de la tanda.
const unknownLength = 30 * time.Second

// Plan reparte las campañas entre las tandas. Función pura. Reglas de tráfico
// (ideas de los sistemas de tráfico de radio; ver docs/spots-and-events.md):
//
//   - Cada campaña va solo a tandas dentro de su franja, días y vigencia.
//   - Orden: primero la prioridad alta; a igual prioridad, la que tiene menos
//     tandas posibles (la más difícil). Si no hay espacio, cede la de prioridad baja.
//   - Reparto equitativo: n emisiones en m tandas válidas van a las posiciones
//     (k+½)·m/n; si la ideal no sirve, la válida libre más cercana.
//   - En una tanda: nunca el mismo anunciante dos veces ni dos del mismo rubro
//     (competidores); respeta el máximo de cuñas y de duración (Limits).
//   - Posición pedida: «primera» y «última» se reservan una por tanda (si no hay
//     otra opción se comparte). Al final cada tanda se ordena: primeras, el
//     resto en orden de prioridad, últimas.
//   - Versiones: rotan según su peso, continuando desde lo ya emitido hoy.
//
// Devuelve, por tanda, los IDs de las versiones en orden, y por campaña
// (domain.Spot.CampaignKey) lo que no entró con el motivo.
func Plan(tandas []Tanda, needs []Need, loc *time.Location, lim Limits) (map[int64][]int64, map[int64]Missing) {
	missing := map[int64]Missing{}
	sort.SliceStable(tandas, func(i, j int) bool { return tandas[i].At.Before(tandas[j].At) })

	type state struct {
		advertisers, sectors map[string]bool
		count                int
		length               time.Duration
		first, last          bool
		items                []placed
	}
	st := make([]state, len(tandas))
	for i := range st {
		st[i] = state{advertisers: map[string]bool{}, sectors: map[string]bool{}}
	}

	type job struct {
		need  Need
		rules domain.Spot
		valid []int // índices de tandas válidas
	}
	var jobs []job
	for _, n := range needs {
		if n.Count <= 0 || len(n.Versions) == 0 {
			continue
		}
		r := n.Versions[0]
		var valid []int
		for i, t := range tandas {
			if r.LiveAt(t.At, loc) {
				valid = append(valid, i)
			}
		}
		jobs = append(jobs, job{n, r, valid})
	}
	sort.SliceStable(jobs, func(i, j int) bool {
		a, b := jobs[i], jobs[j]
		if a.rules.Prio() != b.rules.Prio() {
			return a.rules.Prio() < b.rules.Prio()
		}
		if len(a.valid) != len(b.valid) {
			return len(a.valid) < len(b.valid)
		}
		return a.rules.CampaignKey() < b.rules.CampaignKey()
	})

	for order, j := range jobs {
		r := j.rules
		adv, sector := normalizeAdv(r.Advertiser), normalizeAdv(r.Sector)
		versions := rotation(j.need.Versions)
		count := min(j.need.Count, len(j.valid))
		used := map[int]bool{}
		placedN := 0
		for k := 0; k < count; k++ {
			v := versions[(j.need.Aired+placedN)%len(versions)]
			length := v.Duration
			if length <= 0 {
				length = unknownLength
			}
			ok := func(strictPosition bool) func(int) bool {
				return func(p int) bool {
					s := &st[j.valid[p]]
					switch {
					case used[p], s.advertisers[adv], sector != "" && s.sectors[sector],
						lim.MaxSpots > 0 && s.count >= lim.MaxSpots,
						lim.MaxLength > 0 && s.length+length > lim.MaxLength:
						return false
					case strictPosition && r.Position == "first" && s.first,
						strictPosition && r.Position == "last" && s.last:
						return false
					}
					return true
				}
			}
			ideal := int(float64(len(j.valid)) * (float64(k) + 0.5) / float64(count))
			pos := nearestFree(j.valid, ideal, ok(true))
			if pos < 0 && r.Position != "any" && r.Position != "" {
				pos = nearestFree(j.valid, ideal, ok(false))
			}
			if pos < 0 {
				continue
			}
			used[pos] = true
			s := &st[j.valid[pos]]
			s.advertisers[adv] = true
			if sector != "" {
				s.sectors[sector] = true
			}
			s.count++
			s.length += length
			s.first = s.first || r.Position == "first"
			s.last = s.last || r.Position == "last"
			s.items = append(s.items, placed{id: v.ID, position: r.Position, order: order})
			placedN++
		}
		if miss := j.need.Count - placedN; miss > 0 {
			reason := ReasonFull
			if len(j.valid) < j.need.Count {
				reason = ReasonFewBreaks
			}
			missing[r.CampaignKey()] = Missing{Advertiser: r.Advertiser, Name: campaignName(r), Count: miss, Reason: reason}
		}
	}

	out := map[int64][]int64{}
	for i, s := range st {
		if len(s.items) == 0 {
			continue
		}
		sort.SliceStable(s.items, func(a, b int) bool {
			ra, rb := posRank(s.items[a].position), posRank(s.items[b].position)
			if ra != rb {
				return ra < rb
			}
			return s.items[a].order < s.items[b].order
		})
		for _, it := range s.items {
			out[tandas[i].ID] = append(out[tandas[i].ID], it.id)
		}
	}
	return out, missing
}

// placed es una cuña puesta en una tanda.
type placed struct {
	id       int64
	position string
	order    int // orden de la campaña en el reparto (prioridad)
}

func posRank(p string) int {
	switch p {
	case "first":
		return 0
	case "last":
		return 2
	}
	return 1
}

func campaignName(s domain.Spot) string { return s.Name }

// rotation arma el ciclo de versiones según sus pesos con round-robin ponderado
// suave (el de nginx): pesos 2 y 1 dan A, B, A — sin dos iguales seguidas si se
// puede. Orden estable por ID.
func rotation(vs []domain.Spot) []domain.Spot {
	if len(vs) <= 1 {
		return vs
	}
	sorted := append([]domain.Spot(nil), vs...)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].ID < sorted[j].ID })
	total := 0
	weights := make([]int, len(sorted))
	for i, v := range sorted {
		weights[i] = max(v.Weight, 1)
		total += weights[i]
	}
	current := make([]int, len(sorted))
	out := make([]domain.Spot, 0, total)
	for n := 0; n < total; n++ {
		best := 0
		for i := range sorted {
			current[i] += weights[i]
			if current[i] > current[best] {
				best = i
			}
		}
		current[best] -= total
		out = append(out, sorted[best])
	}
	return out
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

// normalizeAdv compara anunciantes y rubros sin mayúsculas, tildes ni espacios ("Ferretería El Sol" = "ferreteria elsol").
func normalizeAdv(s string) string {
	out, _, err := transform.String(stripMarks, strings.ToLower(s))
	if err != nil {
		out = strings.ToLower(s)
	}
	return strings.Join(strings.Fields(out), "")
}
