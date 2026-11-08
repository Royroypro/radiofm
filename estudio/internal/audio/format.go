package audio

import (
	"math"
	"time"
)

// Formato interno del motor: float32 entrelazado (L, R, L, R...), estéreo, 44 100 Hz.
const (
	SampleRate = 44100
	Channels   = 2
	// BlockFrames es el tamaño de bloque del reloj de software (10 ms).
	BlockFrames = SampleRate / 100
	// maxBlockFrames es el mayor bloque que Render procesa de una vez; los
	// bloques más grandes se parten. Así los buffers internos se reservan una
	// sola vez y el callback no asigna memoria.
	maxBlockFrames = 4096
)

// DBToGain convierte decibelios a ganancia lineal (0 dB = 1.0).
func DBToGain(db float64) float32 { return float32(math.Pow(10, db/20)) }

// GainToDB convierte ganancia lineal a decibelios (0 → -inf).
func GainToDB(g float64) float64 {
	if g <= 0 {
		return math.Inf(-1)
	}
	return 20 * math.Log10(g)
}

// DurationToFrames convierte una duración a frames (1 frame = una muestra por canal).
func DurationToFrames(d time.Duration) int64 { return int64(d) * SampleRate / int64(time.Second) }

// FramesToDuration convierte frames a duración.
func FramesToDuration(frames int64) time.Duration {
	return time.Duration(frames) * time.Second / SampleRate
}
