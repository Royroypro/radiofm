package onair

import (
	"context"
	"fmt"
	"time"

	"radioclub25/estudio/internal/domain"
	"radioclub25/estudio/internal/service/rotation"
)

// Segment es una unidad de reproducción: un archivo (o un silencio). Un
// elemento de la playlist se expande en uno o varios segmentos:
//
//	canción/pista fija → 1 segmento (con crossfade en su punto de mezcla)
//	tanda              → cortina + cuñas + cortina (sin solape; las cuñas sin audio se saltan)
//	hora locutada      → HRShh + MINmm (nada si está apagada o falta la locución)
//	temperatura        → TMPxx (se omite si no hay dato)
//	silencio           → espera de N segundos sin audio
//	cambio de modo     → ningún audio (se aplica el modo)
//	enlace a cadena    → el stream externo (URL) durante N segundos
//	satélite           → el stream externo hasta el evento siguiente (con respaldo si se cae)
type Segment struct {
	ItemID   int64           `json:"item_id"` // elemento de la playlist (0 = emergencia)
	Index    int             `json:"index"`   // posición dentro del elemento
	Last     bool            `json:"last"`    // último segmento del elemento
	Kind     domain.PlayKind `json:"kind"`
	Path     string          `json:"path"`
	TrackID  int64           `json:"track_id"`
	SpotID   int64           `json:"spot_id"`
	RemoteID int64           `json:"remote_id"` // cuña del panel web
	Artist   string          `json:"artist"`
	Title    string          `json:"title"`
	GainDB   float64         `json:"gain_db"`
	CueIn    time.Duration   `json:"cue_in"`
	Length   time.Duration   `json:"length"` // lo que suena (0 = hasta el final)
	Segue    time.Duration   `json:"segue"`  // desde el inicio: arrancar lo siguiente aquí (0 = al terminar)
	Intro    time.Duration   `json:"intro"`
	Silence  time.Duration   `json:"silence"`  // segmento sin audio
	Mode     string          `json:"mode"`     // cambio de modo
	Music    bool            `json:"music"`    // es una canción (se informa como "lo que suena")
	Envelope string          `json:"envelope"` // forma de onda (domain.Track.Envelope)
	Ending   domain.Ending   `json:"ending"`   // cómo termina (decide la transición)
	FadeOut  time.Duration   `json:"fade_out"` // duración de su fundido final propio
	Break    bool            `json:"break"`    // es parte de una tanda (cortina o cuña)
	Duration time.Duration   `json:"duration"` // duración total del archivo (para ubicar cues en la forma de onda)
	// SatEnd: satélite. El stream suena hasta esta hora (la del evento siguiente);
	// la duración se calcula al arrancar. Si la señal se cae antes, suena música de
	// respaldo y se vuelve a intentar.
	SatEnd time.Time `json:"sat_end"`
}

// Key identifica el segmento (para saber si lo precargado sigue siendo lo siguiente).
func (s Segment) Key() string { return fmt.Sprintf("%d/%d/%s", s.ItemID, s.Index, s.Path) }

// Display devuelve "Artista - Título".
func (s Segment) Display() string {
	if s.Artist != "" && s.Title != "" {
		return s.Artist + " - " + s.Title
	}
	return s.Artist + s.Title
}

// expander arma los segmentos de un elemento de la playlist.
type expander struct {
	d         Deps
	crossfade time.Duration
	kinds     map[int64]domain.CategoryKind
	timeOn    bool // interruptor «Hora locutada»
}

// exists dice si el archivo de audio está disponible.
func (x *expander) exists(path string) bool {
	if path == "" {
		return false
	}
	return x.d.Exists == nil || x.d.Exists(path)
}

func (x *expander) trackSegment(t domain.Track) Segment {
	kind := domain.PlayMusic
	switch x.kinds[t.CategoryID] {
	case domain.KindJingle:
		kind = domain.PlayJingle
	case domain.KindVoice:
		kind = domain.PlayVoice
	case domain.KindSpot:
		kind = domain.PlaySpot
	}
	return Segment{Kind: kind, Path: t.Path, TrackID: t.ID, Artist: t.Artist, Title: t.Title, GainDB: t.GainDB,
		CueIn: t.CueIn, Length: t.PlayLength(), Segue: t.Segue(x.crossfade), Intro: t.Intro,
		Music: kind == domain.PlayMusic, Envelope: t.Envelope, Duration: t.Duration, Ending: t.Ending, FadeOut: t.FadeOut}
}

// expand devuelve los segmentos de it. Puede devolver cero (p. ej. temperatura sin dato).
func (x *expander) expand(ctx context.Context, it domain.LogItem) ([]Segment, []string) {
	var segs []Segment
	var warn []string
	switch it.Kind {
	case domain.SlotCategory, domain.SlotTrack:
		t, err := x.d.Tracks.Get(ctx, it.TrackID)
		if err != nil || t.Missing || !t.Active {
			warn = append(warn, fmt.Sprintf("«%s» no está disponible (borrada, inactiva o sin archivo)", it.Display()))
			break
		}
		segs = append(segs, x.trackSegment(*t))
	case domain.SlotSpots:
		spots, err := x.d.Spots.TandaSpots(ctx, it.ID)
		if err != nil {
			warn = append(warn, "tanda: "+err.Error())
		}
		// Las cuñas sin audio (no descargadas o borradas del disco) se saltan; si
		// no queda ninguna, la tanda entera se salta (sin cortinas sueltas).
		ok := spots[:0:0]
		for _, s := range spots {
			if x.exists(s.Path) {
				ok = append(ok, s)
			} else {
				warn = append(warn, fmt.Sprintf("cuña «%s» de %s sin audio: se salta", s.Name, s.Advertiser))
			}
		}
		spots = ok
		if len(spots) == 0 {
			break
		}
		cortina := x.cortina(ctx)
		if cortina != nil {
			segs = append(segs, *cortina)
		}
		for _, s := range spots {
			segs = append(segs, Segment{Kind: domain.PlaySpot, Path: s.Path, SpotID: s.ID, RemoteID: s.RemoteID,
				Artist: s.Advertiser, Title: s.Name, Length: s.Duration})
		}
		if cortina != nil {
			segs = append(segs, *cortina)
		}
		for i := range segs {
			segs[i].Break = true
		}
	case domain.SlotTime:
		if !x.timeOn {
			break // interruptor «Hora locutada» apagado en Al aire: se salta sin aviso
		}
		files, err := x.d.Voice().Time(it.At.In(x.d.Location()))
		if err != nil {
			warn = append(warn, "hora locutada sin audio, se salta: "+err.Error())
			break
		}
		for _, f := range files {
			segs = append(segs, Segment{Kind: domain.PlayVoice, Path: f, Title: "Hora locutada"})
		}
	case domain.SlotTemp:
		c, err := x.d.Temperature(ctx)
		if err != nil {
			warn = append(warn, "temperatura: "+err.Error())
			break
		}
		files, err := x.d.Voice().Temperature(c)
		if err != nil {
			warn = append(warn, "temperatura: "+err.Error())
			break
		}
		for _, f := range files {
			segs = append(segs, Segment{Kind: domain.PlayVoice, Path: f, Title: fmt.Sprintf("Temperatura %.0f °C", c)})
		}
	case domain.SlotSilence:
		segs = append(segs, Segment{Silence: time.Duration(it.Seconds) * time.Second, Title: "Silencio"})
	case domain.SlotRelay:
		// Enlace a cadena: el stream suena hasta Seconds. Si no responde o se corta
		// antes, el segmento termina y la playlist sigue (fallback, idea de Liquidsoap).
		if it.URL == "" || it.Seconds <= 0 {
			warn = append(warn, "enlace a cadena sin URL o duración: se salta")
			break
		}
		segs = append(segs, Segment{Kind: domain.PlayRelay, Path: it.URL, Title: it.Title,
			Length: time.Duration(it.Seconds) * time.Second})
	case domain.SlotSatellite:
		if it.URL == "" || it.Length <= 0 {
			warn = append(warn, "satélite sin URL: se salta")
			break
		}
		segs = append(segs, Segment{Kind: domain.PlayRelay, Path: it.URL, Title: it.Title,
			Length: it.Length, SatEnd: it.At.Add(it.Length)})
	case domain.SlotMode:
		segs = append(segs, Segment{Mode: it.Mode, Title: "Cambio de modo"})
	}
	for i := range segs {
		segs[i].ItemID, segs[i].Index, segs[i].Last = it.ID, i, i == len(segs)-1
	}
	return segs, warn
}

// cortina elige la cortina de tanda menos reciente.
func (x *expander) cortina(ctx context.Context) *Segment {
	id := x.d.Spots.CortinaCategory(ctx)
	if id == 0 {
		return nil
	}
	list, err := x.d.Tracks.ByCategory(ctx, id)
	if err != nil {
		return nil
	}
	r := rotation.Pick(list, nil, "", time.Now(), rotation.Rules{})
	if !r.OK {
		return nil
	}
	s := x.trackSegment(r.Track)
	s.Kind, s.Music, s.Segue = domain.PlayJingle, false, 0
	return &s
}
