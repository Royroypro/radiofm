package library

import (
	"math"
	"time"

	"radioclub25/estudio/internal/domain"
)

// AnalysisRules son los umbrales para deducir cues y ganancia. Ver docs/library.md.
type AnalysisRules struct {
	TargetLUFS   float64       // −16: sonoridad objetivo de toda la biblioteca
	MaxBoostDB   float64       // +6: nunca subir más que esto (evita levantar ruido)
	SilenceLUFS  float64       // −50: por debajo es silencio (cue-in / cue-out)
	SegueDropLU  float64       // 10: el punto de mezcla es donde el final cae esto por debajo del integrado
	SegueWindow  time.Duration // 15 s: buscar el punto de mezcla solo en los últimos segundos
	MinSegueTail time.Duration // 1 s: el punto de mezcla queda al menos esto antes del cue-out
}

// DefaultAnalysisRules son los valores del plan.
func DefaultAnalysisRules() AnalysisRules {
	return AnalysisRules{TargetLUFS: -16, MaxBoostDB: 6, SilenceLUFS: -50, SegueDropLU: 10,
		SegueWindow: 15 * time.Second, MinSegueTail: time.Second}
}

// frameStep es el paso de la serie momentánea de ebur128.
const frameStep = 100 * time.Millisecond

// Derive calcula ganancia, cue-in, cue-out y punto de mezcla a partir de la medición.
//
//   - momentary[i] es la sonoridad (ventana de 400 ms) que termina en (i+1)·100 ms.
//   - cue-in: inicio de la primera ventana por encima del umbral de silencio.
//   - cue-out: fin de la última ventana por encima del umbral.
//   - segue: en los últimos SegueWindow, el último instante en que la sonoridad
//     estaba a menos de SegueDropLU del integrado (empieza la caída/fundido final).
//
// Función pura: se prueba sin ffmpeg.
func Derive(integrated float64, momentary []float64, duration time.Duration, r AnalysisRules) domain.Analysis {
	a := domain.Analysis{Loudness: integrated}
	if integrated <= -70 || math.IsNaN(integrated) {
		a.Err = "pista en silencio"
		return a
	}
	a.GainDB = math.Min(r.TargetLUFS-integrated, r.MaxBoostDB)
	a.GainDB = math.Round(a.GainDB*10) / 10

	first, last := -1, -1
	for i, m := range momentary {
		if m > r.SilenceLUFS {
			if first < 0 {
				first = i
			}
			last = i
		}
	}
	if first < 0 {
		a.Err = "no se encontró audio por encima del umbral"
		return a
	}
	window := 4 * frameStep
	at := func(i int) time.Duration { return time.Duration(i+1) * frameStep }
	a.CueIn = max(at(first)-window, 0)
	a.CueOut = at(last)
	if duration > 0 && a.CueOut > duration {
		a.CueOut = duration
	}
	if a.CueOut >= duration-150*time.Millisecond {
		a.CueOut = 0 // la pista suena hasta el final: sin cue-out
	}
	end := duration
	if a.CueOut > 0 {
		end = a.CueOut
	}

	// punto de mezcla
	from := end - r.SegueWindow
	threshold := integrated - r.SegueDropLU
	segue := time.Duration(0)
	for i := last; i >= 0; i-- {
		t := at(i)
		if t < from {
			break
		}
		if momentary[i] >= threshold {
			segue = t
			break
		}
	}
	if segue == 0 || segue > end-r.MinSegueTail {
		segue = end - r.MinSegueTail // final seco: mezclar casi al final
	}
	if segue < a.CueIn {
		segue = 0
	}
	a.SegueAt = segue
	return a
}
