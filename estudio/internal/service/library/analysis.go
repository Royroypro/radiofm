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
	SilenceRelLU float64       // 42: también es silencio lo que está esto por debajo del integrado (pistas muy fuertes)
	SegueDropLU  float64       // 8: el punto de mezcla es donde el final cae esto por debajo del integrado
	SegueWindow  time.Duration // 15 s: buscar el punto de mezcla solo en los últimos segundos
	MinSegueTail time.Duration // 1 s: el punto de mezcla queda al menos esto antes del cue-out
}

// DefaultAnalysisRules son los valores del plan.
func DefaultAnalysisRules() AnalysisRules {
	return AnalysisRules{TargetLUFS: -16, MaxBoostDB: 6, SilenceLUFS: -50, SilenceRelLU: 42, SegueDropLU: 8,
		SegueWindow: 15 * time.Second, MinSegueTail: time.Second}
}

// frameStep es el paso de la serie momentánea de ebur128.
const frameStep = 100 * time.Millisecond

// Derive calcula ganancia, cue-in, cue-out, punto de mezcla y tipo de final a
// partir de la medición. Basado en autocue de Moonbase59 (MIT,
// https://github.com/Moonbase59/autocue): umbrales relativos a la sonoridad de
// la pista y punto de entrada de la siguiente buscado hacia atrás desde el final.
//
//   - momentary[i] es la sonoridad (ventana de 400 ms) que termina en (i+1)·100 ms.
//   - umbral de silencio: el mayor entre SilenceLUFS e integrado − SilenceRelLU.
//   - cue-in: inicio de la primera ventana por encima del umbral de silencio.
//   - cue-out: fin de la última ventana por encima del umbral.
//   - segue: en los últimos SegueWindow, el último instante en que la sonoridad
//     estaba a menos de SegueDropLU del integrado (empieza la caída/fundido final).
//   - final: «fundido» si entre el segue y el cue-out hay al menos 2 s de caída;
//     si no, «seco» (la siguiente entra a 1 s del final sin pisar el golpe).
//
// Función pura: se prueba sin ffmpeg.
func Derive(integrated float64, momentary []float64, duration time.Duration, r AnalysisRules) domain.Analysis {
	momentary = cleanLoudness(momentary)
	a := domain.Analysis{Loudness: integrated}
	if integrated <= -70 || math.IsNaN(integrated) || math.IsInf(integrated, 0) {
		a.Err = "pista en silencio"
		return a
	}
	a.GainDB = math.Min(r.TargetLUFS-integrated, r.MaxBoostDB)
	a.GainDB = math.Round(a.GainDB*10) / 10

	silence := math.Max(r.SilenceLUFS, integrated-r.SilenceRelLU)
	first, last := -1, -1
	for i, m := range momentary {
		if m > silence {
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
	a.Ending = EndingOf(segue, end, r)
	if segue == 0 || segue > end-r.MinSegueTail {
		segue = end - r.MinSegueTail // final seco: mezclar casi al final
	}
	if segue < a.CueIn {
		segue = 0
	}
	a.SegueAt = segue
	if a.Ending == domain.EndingFade {
		a.FadeOut = end - segue
	}
	a.Envelope = Envelope(momentary, 200)
	return a
}

// minFadeTail es lo mínimo que debe durar la caída final para considerarla fundido.
const minFadeTail = 2 * time.Second

// EndingOf decide si la pista termina en fundido o seco según dónde empieza la
// caída final (segue, 0 = no se encontró) y dónde termina el audio.
func EndingOf(segue, end time.Duration, r AnalysisRules) domain.Ending {
	if segue > 0 && end-segue >= minFadeTail && segue <= end-r.MinSegueTail {
		return domain.EndingFade
	}
	return domain.EndingCold
}

// cleanLoudness cambia los valores que no son números (ffmpeg escribe "nan" en
// algunos tramos de ciertos archivos) por silencio, para que ningún cálculo
// posterior reciba NaN. Devuelve una copia si hay algo que cambiar.
func cleanLoudness(m []float64) []float64 {
	for i, v := range m {
		if math.IsNaN(v) {
			out := append([]float64(nil), m...)
			for j := i; j < len(out); j++ {
				if math.IsNaN(out[j]) {
					out[j] = -120
				}
			}
			return out
		}
	}
	return m
}

// Envelope reduce la sonoridad momentánea a n puntos (el máximo de cada tramo) y
// los codifica como un carácter base 36 por punto: −60 LUFS o menos = "0", 0 LUFS = "z".
// Es la "forma de onda" que dibuja la pantalla Al aire. Función pura.
func Envelope(momentary []float64, n int) string {
	if len(momentary) == 0 || n <= 0 {
		return ""
	}
	const digits = "0123456789abcdefghijklmnopqrstuvwxyz"
	out := make([]byte, n)
	for i := 0; i < n; i++ {
		from := i * len(momentary) / n
		to := max((i+1)*len(momentary)/n, from+1)
		peak := -120.0
		for _, v := range momentary[from:min(to, len(momentary))] {
			if !math.IsNaN(v) {
				peak = math.Max(peak, v)
			}
		}
		level := (peak + 60) / 60 // 0…1
		level = math.Max(0, math.Min(1, level))
		out[i] = digits[int(math.Round(level*35))]
	}
	return string(out)
}
