package rotation

import (
	"math"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"

	"radioclub25/estudio/internal/domain"
)

// Rules son las separaciones mínimas y las reglas de flujo. Un valor cero
// desactiva la regla. Las de flujo solo miran pistas con el atributo cargado.
type Rules struct {
	ArtistSeparation time.Duration // no repetir un intérprete antes de esto
	TitleSeparation  time.Duration // no repetir una canción antes de esto

	// Flujo (atributos de la pista; ver domain.Track):
	NoSameVoice   bool    // no dos voces femeninas (o masculinas) seguidas
	MaxEnergyJump int     // salto máximo de energía entre canciones seguidas (1–4)
	MaxTempoJump  float64 // salto máximo de tempo entre canciones seguidas, en %
	MaxGenreRun   int     // máximo de canciones seguidas del mismo género
	MaxEraRun     int     // máximo de canciones seguidas de la misma década
}

// Use es un uso pasado o planificado de una pista (historial + lo ya programado).
// Créelo con NewUse para precalcular las claves normalizadas (si no, Pick las calcula cada vez).
type Use struct {
	TrackID int64
	Artist  string
	Title   string
	At      time.Time

	artists []string // claves normalizadas (caché)
	title   string
	cached  bool
}

// NewUse crea un uso con las claves normalizadas ya calculadas.
func NewUse(trackID int64, artist, title string, at time.Time) Use {
	return Use{TrackID: trackID, Artist: artist, Title: title, At: at,
		artists: Artists(artist), title: titleKey(artist, title), cached: true}
}

func (u *Use) keys() ([]string, string) {
	if !u.cached {
		return Artists(u.Artist), titleKey(u.Artist, u.Title)
	}
	return u.artists, u.title
}

// Result es la pista elegida y qué reglas hubo que relajar.
type Result struct {
	Track   domain.Track
	Relaxed string // "" = se cumplieron todas; si no, la regla relajada
	OK      bool
}

// Pick es PickFlow conociendo solo el intérprete anterior.
func Pick(cands []domain.Track, history []Use, prevArtist string, at time.Time, r Rules) Result {
	var recent []domain.Track
	if prevArtist != "" {
		recent = []domain.Track{{Artist: prevArtist}}
	}
	return PickFlow(cands, history, recent, at, r)
}

// PickFlow elige la siguiente pista entre cands para sonar en at.
//
// Orden de preferencia: la menos reciente (según history y LastPlayedAt; las
// nunca usadas primero). Las reglas, de la más a la menos importante:
//
//	distinto intérprete que la anterior · separación de título · separación de artista ·
//	voz · salto de energía · salto de tempo · racha de género · racha de década
//
// Si ninguna candidata las cumple todas, se relajan de a una empezando por la
// menos importante (las desactivadas no cuentan); al final, cualquiera
// disponible. Result.Relaxed dice qué se relajó.
//
// recent son las pistas anteriores de la playlist (la última es la inmediata
// anterior). Determinista.
func PickFlow(cands []domain.Track, history []Use, recent []domain.Track, at time.Time, r Rules) Result {
	prevArtist := ""
	var last *domain.Track
	if len(recent) > 0 {
		last = &recent[len(recent)-1]
		prevArtist = last.Artist
	}
	lastTrack := map[int64]time.Time{}
	lastTitle := map[string]time.Time{}
	lastArtist := map[string]time.Time{}
	for i := range history {
		u := &history[i]
		if u.At.After(at) {
			continue // planificado más tarde: no cuenta como pasado
		}
		if u.At.After(lastTrack[u.TrackID]) {
			lastTrack[u.TrackID] = u.At
		}
		artists, tk := u.keys()
		if tk != "" && u.At.After(lastTitle[tk]) {
			lastTitle[tk] = u.At
		}
		for _, a := range artists {
			if u.At.After(lastArtist[a]) {
				lastArtist[a] = u.At
			}
		}
	}
	type cand struct {
		t       domain.Track
		last    time.Time
		artists []string
		title   string
	}
	var pool []cand
	for _, t := range cands {
		if !t.Available(at) {
			continue
		}
		last := lastTrack[t.ID]
		if t.LastPlayedAt != nil && t.LastPlayedAt.After(last) && !t.LastPlayedAt.After(at) {
			last = *t.LastPlayedAt
		}
		pool = append(pool, cand{t, last, Artists(t.Artist), titleKey(t.Artist, t.Title)})
	}
	if len(pool) == 0 {
		return Result{}
	}
	sort.SliceStable(pool, func(i, j int) bool {
		if !pool[i].last.Equal(pool[j].last) {
			return pool[i].last.Before(pool[j].last)
		}
		return pool[i].t.ID < pool[j].t.ID
	})
	prev := Artists(prevArtist)
	sameAsPrev := func(c cand) bool { return overlap(c.artists, prev) }
	titleOK := func(c cand) bool {
		if r.TitleSeparation <= 0 {
			return true
		}
		last, ok := lastTitle[c.title]
		if lt := lastTrack[c.t.ID]; lt.After(last) {
			last, ok = lt, true
		}
		return !ok || at.Sub(last) >= r.TitleSeparation
	}
	artistOK := func(c cand) bool {
		if r.ArtistSeparation <= 0 {
			return true
		}
		for _, a := range c.artists {
			if last, ok := lastArtist[a]; ok && at.Sub(last) < r.ArtistSeparation {
				return false
			}
		}
		return true
	}
	type rule struct {
		name string
		ok   func(cand) bool
	}
	rules := []rule{
		{"intérprete anterior", func(c cand) bool { return !sameAsPrev(c) }},
		{"separación de título", titleOK},
		{"separación de artista", artistOK},
	}
	if last != nil {
		if r.NoSameVoice {
			rules = append(rules, rule{"voz", func(c cand) bool { return !sameVoice(c.t.Voice, last.Voice) }})
		}
		if r.MaxEnergyJump > 0 {
			rules = append(rules, rule{"energía", func(c cand) bool {
				return c.t.Energy == 0 || last.Energy == 0 || abs(c.t.Energy-last.Energy) <= r.MaxEnergyJump
			}})
		}
		if r.MaxTempoJump > 0 {
			rules = append(rules, rule{"tempo", func(c cand) bool {
				return c.t.BPM <= 0 || last.BPM <= 0 || math.Abs(c.t.BPM-last.BPM)/last.BPM*100 <= r.MaxTempoJump
			}})
		}
		if r.MaxGenreRun > 0 {
			rules = append(rules, rule{"racha de género", func(c cand) bool {
				return run(recent, c.t, func(t domain.Track) string { return strings.TrimSpace(normalize(t.Genre)) }) < r.MaxGenreRun
			}})
		}
		if r.MaxEraRun > 0 {
			rules = append(rules, rule{"racha de década", func(c cand) bool { return run(recent, c.t, decade) < r.MaxEraRun }})
		}
	}
	// nivel k: se exigen las primeras len(rules)−k reglas
	for k := 0; k <= len(rules); k++ {
		active := rules[:len(rules)-k]
		for _, c := range pool {
			ok := true
			for _, ru := range active {
				if !ru.ok(c) {
					ok = false
					break
				}
			}
			if ok {
				var dropped []string
				for _, ru := range rules[len(rules)-k:] {
					dropped = append(dropped, ru.name)
				}
				return Result{Track: c.t, Relaxed: relaxedNote(dropped, len(rules)), OK: true}
			}
		}
	}
	return Result{}
}

// relaxedNote describe las reglas relajadas: "" si ninguna, "todas las reglas"
// si no quedó ninguna, o sus nombres de la más a la menos importante.
func relaxedNote(dropped []string, total int) string {
	switch {
	case len(dropped) == 0:
		return ""
	case len(dropped) == total:
		return "todas las reglas"
	}
	return strings.Join(dropped, ", ")
}

// sameVoice: dos voces femeninas o dos masculinas seguidas.
func sameVoice(a, b string) bool { return a == b && (a == "female" || a == "male") }

// run cuenta cuántas pistas seguidas al final de recent tienen la misma clave que t
// (clave vacía = sin dato: no cuenta).
func run(recent []domain.Track, t domain.Track, key func(domain.Track) string) int {
	k := key(t)
	if k == "" {
		return 0
	}
	n := 0
	for i := len(recent) - 1; i >= 0 && key(recent[i]) == k; i-- {
		n++
	}
	return n
}

// decade devuelve la década del año de la pista ("1990") o "" si no hay año.
func decade(t domain.Track) string {
	y := strings.TrimSpace(t.Year)
	if len(y) < 4 {
		return ""
	}
	if _, err := strconv.Atoi(y[:4]); err != nil {
		return ""
	}
	return y[:3] + "0"
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// Artists separa los intérpretes de un campo de artista ("A feat. B & C" → [a b c]),
// normalizados (minúsculas, sin tildes ni espacios sobrantes).
func Artists(s string) []string {
	s = normalize(s)
	if s == "" {
		return nil
	}
	for _, sep := range []string{" featuring ", " feat. ", " feat ", " ft. ", " ft ", " con ", " y ", " & ", " x ", " vs ", ","} {
		s = strings.ReplaceAll(s, sep, "|")
	}
	var out []string
	for _, p := range strings.Split(s, "|") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func titleKey(artist, title string) string {
	t := normalize(title)
	if t == "" {
		return ""
	}
	return normalize(artist) + "|" + t
}

func overlap(a, b []string) bool {
	for _, x := range a {
		for _, y := range b {
			if x == y {
				return true
			}
		}
	}
	return false
}

var stripMarks = transform.Chain(norm.NFD, runes.Remove(runes.In(unicode.Mn)), norm.NFC)

// normalize pasa a minúsculas, quita tildes y colapsa espacios.
func normalize(s string) string {
	out, _, err := transform.String(stripMarks, strings.ToLower(s))
	if err != nil {
		out = strings.ToLower(s)
	}
	return " " + strings.Join(strings.Fields(out), " ") + " "
}
