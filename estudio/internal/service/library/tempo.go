package library

import (
	"math"
)

// Estimación del tempo (BPM) por autocorrelación de la envolvente de ataques.
// Método clásico (ver p. ej. Ellis, "Beat Tracking by Dynamic Programming",
// 2007, y el prior de tempo de librosa): energía por ventanas cortas → aumento
// positivo del logaritmo de la energía (ataques) → autocorrelación → el retardo
// con más correlación, ponderado hacia 120 BPM para elegir entre el doble y la
// mitad. Escrito desde cero; función pura, se prueba sin ffmpeg.

// TempoRate es la frecuencia de muestreo con que se decodifica para estimar el tempo.
const TempoRate = 11025

// tempoHop es el paso entre ventanas de energía (≈23 ms a 11 025 Hz).
const tempoHop = 256

// EstimateBPM estima el tempo de una señal mono. Devuelve 0 si no hay pulso claro.
func EstimateBPM(samples []float32, rate int) float64 {
	if rate <= 0 || len(samples) < rate*8 { // menos de 8 s: no hay con qué
		return 0
	}
	// 1. energía por ventana (2 pasos de ancho) y envolvente de ataques
	n := len(samples)/tempoHop - 1
	onset := make([]float64, n)
	prev := 0.0
	for i := 0; i < n; i++ {
		var e float64
		for _, v := range samples[i*tempoHop : i*tempoHop+2*tempoHop] {
			e += float64(v) * float64(v)
		}
		le := math.Log(1e-10 + e)
		if i > 0 && le > prev {
			onset[i] = le - prev
		}
		prev = le
	}
	// quitar la media (que la autocorrelación no favorezca retardos cortos)
	var mean float64
	for _, v := range onset {
		mean += v
	}
	mean /= float64(n)
	for i := range onset {
		onset[i] -= mean
	}
	// 2. autocorrelación hasta 4 períodos del tempo más lento
	fps := float64(rate) / tempoHop
	maxLag := min(int(math.Ceil(fps*60/60))*4+2, n/2)
	if maxLag < int(fps) {
		return 0
	}
	ac := make([]float64, maxLag+1)
	for lag := 1; lag <= maxLag; lag++ {
		var s float64
		for i := lag; i < n; i++ {
			s += onset[i] * onset[i-lag]
		}
		ac[lag] = s / float64(n-lag)
	}
	at := func(x float64) float64 { // interpolación lineal (el retardo de un tempo no es entero)
		i := int(x)
		if i < 1 || i+1 > maxLag {
			return 0
		}
		f := x - float64(i)
		return ac[i]*(1-f) + ac[i+1]*f
	}
	// 3. puntaje de cada tempo de 60 a 200 BPM (paso 0,1): suma de los primeros
	// armónicos del período (un pulso real también correlaciona a 2 y 3 períodos)
	// por un prior log-normal centrado en 120 BPM.
	best, bestScore := 0.0, 0.0
	for bpm := 60.0; bpm <= 200; bpm += 0.1 {
		lag := 60 * fps / bpm
		var s float64
		for k := 1.0; k <= 3; k++ {
			s += at(lag*k) / k
		}
		w := math.Exp(-0.5 * math.Pow(math.Log2(bpm/120), 2))
		if score := s * w; score > bestScore {
			best, bestScore = bpm, score
		}
	}
	if best == 0 || bestScore <= 0 {
		return 0
	}
	return math.Round(best*10) / 10
}

// EnergyFrom estima la energía (1 calma … 5 muy movida) a partir del tempo y la
// sonoridad. Es una primera aproximación para ordenar el flujo; el operador la
// corrige en la Biblioteca (lo que pone a mano no se recalcula).
func EnergyFrom(bpm, loudness float64) int {
	e := 3
	switch {
	case bpm <= 0:
		e = 3
	case bpm < 85:
		e = 2
	case bpm < 105:
		e = 3
	case bpm < 125:
		e = 4
	default:
		e = 5
	}
	if loudness != 0 && loudness < -16 {
		e-- // grabación suave (baladas, acústicos)
	}
	return min(max(e, 1), 5)
}
