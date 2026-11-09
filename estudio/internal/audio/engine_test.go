package audio

import (
	"io"
	"math"
	"sync"
	"testing"
	"time"
)

// waitReady espera a que la goroutine de llenado haya cargado algo (los tests renderizan más rápido que tiempo real).
func waitReady(t *testing.T, e *Engine, slot int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if it := e.slots[slot]; it != nil && it.ready(BlockFrames*Channels*10) {
			return
		}
		applyPending(e)
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("el slot %d no se llenó a tiempo", slot)
}

// applyPending procesa los comandos en cola sin producir audio.
func applyPending(e *Engine) { e.renderBlock(make([]float32, 0)) }

func db(v float32) float64 { return GainToDB(float64(v)) }

func collect(e *Engine) []Event {
	var out []Event
	for {
		select {
		case ev := <-e.events:
			out = append(out, ev)
		default:
			return out
		}
	}
}

func TestRingBufferSPSC(t *testing.T) {
	rb := NewRingBuffer(1000)
	if rb.Cap() != 1024 {
		t.Fatalf("capacidad = %d, quería 1024", rb.Cap())
	}
	const total = 200000
	var wg sync.WaitGroup
	wg.Add(1)
	go func() { // productor
		defer wg.Done()
		buf := make([]float32, 37)
		next := 0
		for next < total {
			n := 0
			for i := range buf {
				if next+i >= total {
					break
				}
				buf[i] = float32(next + i)
				n++
			}
			w := rb.Write(buf[:n])
			next += w
		}
	}()
	got := make([]float32, 53)
	expect := 0
	for expect < total {
		n := rb.Read(got)
		for i := 0; i < n; i++ {
			if got[i] != float32(expect) {
				t.Fatalf("muestra %d = %v", expect, got[i])
			}
			expect++
		}
	}
	wg.Wait()
	if rb.Len() != 0 {
		t.Fatalf("debería quedar vacío, quedan %d", rb.Len())
	}
}

func TestPlayLengthAndEvents(t *testing.T) {
	e := NewEngine(Config{LimiterCeilingDB: 0})
	src := NewMemorySource(Sine(440, 0.5, 2))
	if err := e.Load(DeckA, src, LoadOptions{Tag: 7, Length: time.Second, SegueAt: 800 * time.Millisecond}); err != nil {
		t.Fatal(err)
	}
	applyPending(e)
	waitReady(t, e, DeckA)
	_ = e.Play(DeckA, 0)
	RenderOffline(e, 1500*time.Millisecond, nil)
	evs := collect(e)
	var types []EventType
	for _, ev := range evs {
		if ev.Tag != 7 {
			t.Fatalf("tag = %d", ev.Tag)
		}
		types = append(types, ev.Type)
	}
	if len(types) != 3 || types[0] != EventStarted || types[1] != EventSegue || types[2] != EventEnded {
		t.Fatalf("eventos = %v, quería [Started Segue Ended]", types)
	}
	if pos := e.Slot(DeckA).Position; pos != time.Second {
		t.Fatalf("debe parar en Length (1 s), paró en %v", pos)
	}
}

func TestCrossfadeLevels(t *testing.T) {
	e := NewEngine(Config{LimiterCeilingDB: 0})
	_ = e.Load(DeckA, NewMemorySource(Sine(440, 0.5, 3)), LoadOptions{})
	_ = e.Load(DeckB, NewMemorySource(Sine(440, 0.5, 3)), LoadOptions{GainDB: -6})
	applyPending(e)
	waitReady(t, e, DeckA)
	waitReady(t, e, DeckB)
	_ = e.Play(DeckA, 0)
	RenderOffline(e, 500*time.Millisecond, nil)
	if p := e.Status().Levels.PeakL; math.Abs(db(p)-db(0.5)) > 0.2 {
		t.Fatalf("deck A solo: pico %.2f dB, quería %.2f", db(p), db(0.5))
	}
	// A se funde a 0 en 1 s mientras B entra con −6 dB de ganancia de pista
	_ = e.Fade(DeckA, 0, time.Second, true)
	_ = e.Play(DeckB, 0)
	RenderOffline(e, 1200*time.Millisecond, nil)
	if e.Slot(DeckA).Playing {
		t.Fatal("A debe detenerse al terminar el fundido")
	}
	want := 0.5 * float64(DBToGain(-6))
	if p := e.Status().Levels.PeakL; math.Abs(db(p)-GainToDB(want)) > 0.3 {
		t.Fatalf("tras el fundido: pico %.2f dB, quería %.2f (solo B a −6 dB)", db(p), GainToDB(want))
	}
}

func TestDuckingWithMic(t *testing.T) {
	cfg := DefaultConfig()
	cfg.LimiterCeilingDB = 0
	e := NewEngine(cfg)
	_ = e.Load(DeckA, NewMemorySource(Sine(440, 0.4, 3)), LoadOptions{})
	applyPending(e)
	waitReady(t, e, DeckA)
	_ = e.Play(DeckA, 0)
	RenderOffline(e, 300*time.Millisecond, nil)
	if g := e.Status().DuckGain; g < 0.99 {
		t.Fatalf("sin mic no debe haber ducking: %v", g)
	}
	// "voz" en el micrófono: el buffer de captura recibe un seno a −20 dBFS
	e.SetMic(true)
	voice := Sine(200, 0.1, 1)
	RenderOffline(e, time.Second, func([]float32) { e.MicInput().Write(voice[:BlockFrames*Channels]) })
	if g := db(e.Status().DuckGain); math.Abs(g-(-12)) > 0.5 {
		t.Fatalf("con voz la música debe bajar 12 dB, bajó %.2f dB", g)
	}
	// silencio en el mic: tras la relajación vuelve a 0 dB
	RenderOffline(e, 3*time.Second, nil)
	if g := db(e.Status().DuckGain); g < -0.5 {
		t.Fatalf("sin voz debe volver a 0 dB, está en %.2f dB", g)
	}
}

func TestLimiterCeiling(t *testing.T) {
	l := NewLimiter(-1, 1500*time.Microsecond, 50*time.Millisecond)
	buf := Sine(1000, 1.6, 0.5) // +4 dBFS
	l.Process(buf)
	ceiling := DBToGain(-1) + 1e-4
	if p := peakOf(buf); p > ceiling {
		t.Fatalf("pico %.3f supera el techo %.3f", p, ceiling)
	}
	if l.Reduction > 0.6 {
		t.Fatalf("debería reducir ~5 dB, reducción %.2f", l.Reduction)
	}
}

func TestDropoutCounted(t *testing.T) {
	e := NewEngine(Config{})
	src := &slowSource{}
	_ = e.Load(DeckA, src, LoadOptions{})
	applyPending(e)
	_ = e.Play(DeckA, 0)
	RenderOffline(e, 100*time.Millisecond, nil)
	if e.Status().Dropouts == 0 {
		t.Fatal("una fuente que no entrega datos debe contar dropouts")
	}
	if !e.Slot(DeckA).Playing {
		t.Fatal("un decodificador lento no debe terminar la pista")
	}
	_ = e.Stop(DeckA)
	applyPending(e)
	if evs := collect(e); len(evs) < 2 || evs[len(evs)-1].Type != EventEnded {
		t.Fatalf("Stop debe emitir Ended: %v", evs)
	}
}

func TestEncoderTapDropsWhenFull(t *testing.T) {
	e := NewEngine(Config{EncoderBuffer: 50 * time.Millisecond})
	RenderOffline(e, 300*time.Millisecond, nil) // nadie vacía el buffer
	if e.Status().EncDropped == 0 {
		t.Fatal("con el buffer del codificador lleno se deben descartar muestras")
	}
}

func TestSilenceEvent(t *testing.T) {
	e := NewEngine(Config{SilenceThreshold: -50, SilenceAfter: time.Second})
	RenderOffline(e, 1100*time.Millisecond, nil)
	evs := collect(e)
	if len(evs) != 1 || evs[0].Type != EventSilence {
		t.Fatalf("debe avisar silencio una vez: %v", evs)
	}
}

func TestPCMSourceOddChunks(t *testing.T) {
	want := []float32{0.25, -0.5, 1, -1, 0.125}
	var raw []byte
	for _, v := range want {
		b := math.Float32bits(v)
		raw = append(raw, byte(b), byte(b>>8), byte(b>>16), byte(b>>24))
	}
	src := NewPCMSource(nopCloser{&chunkReader{data: raw, size: 3}})
	got, err := ReadAll(src, 100)
	if err != nil || len(got) != len(want) {
		t.Fatalf("ReadAll = %v, %v", got, err)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("muestra %d = %v, quería %v", i, got[i], want[i])
		}
	}
}

// slowSource nunca entrega datos (simula un decodificador atascado).
type slowSource struct{}

func (*slowSource) Read(p []float32) (int, error) { time.Sleep(time.Millisecond); return 0, nil }
func (*slowSource) Close() error                  { return nil }

// chunkReader entrega los bytes de a size para probar muestras partidas entre lecturas.
type chunkReader struct {
	data []byte
	size int
}

func (c *chunkReader) Read(p []byte) (int, error) {
	if len(c.data) == 0 {
		return 0, errEOF
	}
	n := min(len(p), c.size, len(c.data))
	copy(p, c.data[:n])
	c.data = c.data[n:]
	return n, nil
}

type nopCloser struct{ *chunkReader }

func (nopCloser) Close() error { return nil }

var errEOF = io.EOF

// PlayAfter espera antes de sonar (la tanda entra cuando terminó de fundirse la música).
func TestPlayAfterDelay(t *testing.T) {
	e := NewEngine(Config{LimiterCeilingDB: 0})
	_ = e.Load(DeckA, NewMemorySource(Sine(440, 0.5, 2)), LoadOptions{Tag: 1})
	applyPending(e)
	waitReady(t, e, DeckA)
	_ = e.PlayAfter(DeckA, 0, 300*time.Millisecond)
	RenderOffline(e, 250*time.Millisecond, nil)
	if pos := e.Slot(DeckA).Position; pos != 0 {
		t.Fatalf("antes del retardo no debe avanzar: posición %v", pos)
	}
	RenderOffline(e, 300*time.Millisecond, nil)
	if pos := e.Slot(DeckA).Position; pos < 200*time.Millisecond || pos > 300*time.Millisecond {
		t.Fatalf("tras el retardo debe sonar: posición %v", pos)
	}
}

// Curva equal-power: a mitad de un fundido la ganancia es sin(π/4) ≈ 0,707
// (−3 dB), no 0,5 (−6 dB): dos pistas cruzándose suman la misma potencia.
func TestFadeShapeEqualPower(t *testing.T) {
	if g := fadeShape(0.5); math.Abs(float64(g)-math.Sqrt2/2) > 1e-4 {
		t.Fatalf("mitad del fundido = %v", g)
	}
	a, b := fadeShape(0.3), fadeShape(0.7)
	if p := a*a + b*b; math.Abs(float64(p)-1) > 1e-4 {
		t.Fatalf("potencia sumada = %v, quería 1", p)
	}
	if fadeShape(0) != 0 || fadeShape(1) != 1 {
		t.Fatal("los extremos no cambian")
	}
}
