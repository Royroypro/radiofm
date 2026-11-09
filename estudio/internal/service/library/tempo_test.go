package library

import (
	"math"
	"math/rand"
	"testing"
)

// clicks genera golpes cortos a bpm durante secs segundos, con algo de ruido.
func clicks(bpm float64, secs int, rate int) []float32 {
	out := make([]float32, secs*rate)
	rng := rand.New(rand.NewSource(1))
	for i := range out {
		out[i] = float32(rng.NormFloat64() * 0.005)
	}
	period := 60 / bpm * float64(rate)
	for k := 0.0; int(k*period) < len(out); k++ {
		start := int(k * period)
		for j := 0; j < 300 && start+j < len(out); j++ { // golpe de ~27 ms con caída
			out[start+j] += float32(0.8 * math.Exp(-float64(j)/60) * math.Sin(float64(j)*0.9))
		}
	}
	return out
}

func TestEstimateBPM(t *testing.T) {
	for _, want := range []float64{72, 95, 120, 128, 150} {
		got := EstimateBPM(clicks(want, 40, TempoRate), TempoRate)
		// se acepta el tempo o su doble/mitad cercana al centro (ambigüedad clásica)
		if math.Abs(got-want) > 2 && math.Abs(got-want*2) > 3 && math.Abs(got-want/2) > 2 {
			t.Errorf("BPM %v: estimado %v", want, got)
		}
	}
	if got := EstimateBPM(make([]float32, 3*TempoRate), TempoRate); got != 0 {
		t.Errorf("señal muy corta: %v", got)
	}
}

func TestEnergyFrom(t *testing.T) {
	cases := []struct {
		bpm, lufs float64
		want      int
	}{{70, -12, 2}, {100, -12, 3}, {120, -12, 4}, {140, -10, 5}, {70, -20, 1}, {0, 0, 3}}
	for _, c := range cases {
		if got := EnergyFrom(c.bpm, c.lufs); got != c.want {
			t.Errorf("EnergyFrom(%v, %v) = %d, quería %d", c.bpm, c.lufs, got, c.want)
		}
	}
}
