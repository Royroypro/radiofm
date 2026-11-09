package audio

import (
	"math"
	"math/rand"
	"testing"
	"time"
)

// tone llena n frames estéreo con la suma de senos de las frecuencias dadas a amplitud a cada uno.
func tone(n int, a float64, freqs ...float64) []float32 {
	out := make([]float32, n*Channels)
	for i := 0; i < n; i++ {
		var v float64
		for _, f := range freqs {
			v += a * math.Sin(2*math.Pi*f*float64(i)/SampleRate)
		}
		out[i*2], out[i*2+1] = float32(v), float32(v)
	}
	return out
}

func rmsDB(buf []float32) float64 {
	var s float64
	for _, v := range buf {
		s += float64(v) * float64(v)
	}
	return GainToDB(math.Sqrt(s / float64(len(buf))))
}

// Con los compresores sin actuar (ratio 1) y el AGC sin margen, la salida es
// idéntica a la entrada: las tres bandas se reconstruyen exactamente.
func TestProcessorTransparent(t *testing.T) {
	flat := BandSettings{ThresholdDB: -20, Ratio: 1}
	p := NewProcessor(ProcSettings{AGCTargetDB: -18, AGCMaxDB: 0, AGCGateDB: -60, AGCTime: time.Second,
		Low: flat, Mid: flat, High: flat})
	in := tone(SampleRate, 0.2, 80, 1000, 9000)
	out := append([]float32(nil), in...)
	p.Process(out)
	for i := range in {
		if math.Abs(float64(out[i]-in[i])) > 1e-4 {
			t.Fatalf("muestra %d: %v ≠ %v", i, out[i], in[i])
		}
	}
}

// El AGC acerca el nivel de una canción floja y una fuerte.
func TestProcessorAGCEvensLevels(t *testing.T) {
	s, _ := PresetSettings(ProcNatural)
	quiet, loud := tone(10*SampleRate, 0.03, 440), tone(10*SampleRate, 0.3, 440) // −20 dB de diferencia
	before := rmsDB(loud) - rmsDB(quiet)
	pq, pl := NewProcessor(s), NewProcessor(s)
	pq.Process(quiet)
	pl.Process(loud)
	tail := 2 * SampleRate * Channels // últimos 2 s, con el AGC ya asentado
	after := rmsDB(loud[len(loud)-tail:]) - rmsDB(quiet[len(quiet)-tail:])
	if after > before-6 {
		t.Fatalf("diferencia de nivel: antes %.1f dB, después %.1f dB (el AGC debía acercarlas)", before, after)
	}
}

// En silencio el AGC no sube la ganancia (no levanta el ruido de fondo).
func TestProcessorAGCGate(t *testing.T) {
	s, _ := PresetSettings(ProcFM)
	p := NewProcessor(s)
	silence := make([]float32, 5*SampleRate*Channels)
	p.Process(silence)
	if p.AGC > 1.01 {
		t.Fatalf("ganancia del AGC en silencio = %.2f", p.AGC)
	}
}

// Con el preset FM y ruido fuerte, la salida del motor nunca supera el techo del limitador.
func TestProcessorThenLimiterCeiling(t *testing.T) {
	e := NewEngine(Config{LimiterCeilingDB: -1})
	s, _ := PresetSettings(ProcFM)
	_ = e.SetProcessor(NewProcessor(s), ProcBoth)
	rng := rand.New(rand.NewSource(1))
	noise := make([]float32, 3*SampleRate*Channels)
	for i := range noise {
		noise[i] = float32(rng.Float64()*1.6 - 0.8)
	}
	_ = e.Load(DeckA, NewMemorySource(noise), LoadOptions{})
	applyPending(e)
	waitReady(t, e, DeckA)
	_ = e.Play(DeckA, 0)
	ceiling := DBToGain(-1) + 1e-3
	out := make([]float32, BlockFrames*Channels)
	for b := 0; b < 200; b++ {
		e.Render(out)
		for i, v := range out {
			if abs32(v) > ceiling {
				t.Fatalf("bloque %d muestra %d: %v supera el techo", b, i, v)
			}
		}
	}
	if st := e.Status(); st.ProcGain >= 1 {
		t.Fatalf("con ruido fuerte los compresores deben actuar: %+v", st.ProcGain)
	}
}

// «Solo stream»: la salida a la tarjeta (FM) no se procesa; la copia al codificador sí.
func TestProcessorStreamOnly(t *testing.T) {
	e := NewEngine(Config{LimiterCeilingDB: 0})
	s, _ := PresetSettings(ProcFM)
	_ = e.SetProcessor(NewProcessor(s), ProcStream)
	_ = e.Load(DeckA, NewMemorySource(Sine(440, 0.1, 2)), LoadOptions{})
	applyPending(e)
	waitReady(t, e, DeckA)
	_ = e.Play(DeckA, 0)
	out := make([]float32, BlockFrames*Channels)
	var fm, stream []float32
	for b := 0; b < 100; b++ {
		e.Render(out)
		fm = append(fm, out...)
	}
	buf := make([]float32, len(fm))
	stream = buf[:e.encRing.Read(buf)]
	if d := rmsDB(fm[len(fm)/2:]) - GainToDB(0.1/math.Sqrt2); math.Abs(d) > 0.5 {
		t.Fatalf("la FM no debe procesarse: nivel %.2f dB respecto del original", d)
	}
	if len(stream) == 0 || math.Abs(rmsDB(stream[len(stream)/2:])-rmsDB(fm[len(fm)/2:])) < 1 {
		t.Fatal("el stream debe salir procesado (nivel distinto de la FM)")
	}
}
