package rotation

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

// Rules son las separaciones mínimas.
type Rules struct {
	ArtistSeparation time.Duration // no repetir un intérprete antes de esto
	TitleSeparation  time.Duration // no repetir una canción antes de esto
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

// Pick elige la siguiente pista entre cands para sonar en at.
//
// Orden de preferencia: la menos reciente (según history y LastPlayedAt; las
// nunca usadas primero). Niveles:
//  1. todas las reglas: separación de título, de artista y distinto intérprete que el slot anterior;
//  2. sin separación de artista (mantiene título e intérprete anterior);
//  3. sin separación de título ni de artista (mantiene intérprete anterior);
//  4. cualquiera disponible (la menos reciente).
//
// prevArtist es el intérprete del elemento anterior de la playlist. Determinista.
func Pick(cands []domain.Track, history []Use, prevArtist string, at time.Time, r Rules) Result {
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
	levels := []struct {
		note string
		ok   func(cand) bool
	}{
		{"", func(c cand) bool { return titleOK(c) && artistOK(c) && !sameAsPrev(c) }},
		{"separación de artista", func(c cand) bool { return titleOK(c) && !sameAsPrev(c) }},
		{"separación de título y artista", func(c cand) bool { return !sameAsPrev(c) }},
		{"todas las reglas", func(cand) bool { return true }},
	}
	for _, lv := range levels {
		for _, c := range pool {
			if lv.ok(c) {
				return Result{Track: c.t, Relaxed: lv.note, OK: true}
			}
		}
	}
	return Result{}
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
