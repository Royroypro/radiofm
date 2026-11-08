package library

import (
	"testing"
	"time"
)

// series construye una serie momentánea: silencio, cuerpo a level LUFS y un fundido final lineal.
func series(silenceStart, body, fade, silenceEnd time.Duration, level float64) ([]float64, time.Duration) {
	var out []float64
	add := func(d time.Duration, f func(k, n int) float64) {
		n := int(d / frameStep)
		for k := 0; k < n; k++ {
			out = append(out, f(k, n))
		}
	}
	add(silenceStart, func(int, int) float64 { return -120 })
	add(body, func(int, int) float64 { return level })
	add(fade, func(k, n int) float64 { return level - 40*float64(k)/float64(n) })
	add(silenceEnd, func(int, int) float64 { return -120 })
	return out, silenceStart + body + fade + silenceEnd
}

func TestDeriveFadeOut(t *testing.T) {
	m, d := series(time.Second, 3*time.Minute, 8*time.Second, 2*time.Second, -12)
	a := Derive(-12, m, d, DefaultAnalysisRules())
	if a.Err != "" {
		t.Fatal(a.Err)
	}
	if a.GainDB != -4 {
		t.Fatalf("ganancia = %v, quería −4 (−16 − (−12))", a.GainDB)
	}
	// cue-in ≈ 1 s (menos la ventana de 400 ms)
	if a.CueIn < 600*time.Millisecond || a.CueIn > time.Second {
		t.Fatalf("cue-in = %v", a.CueIn)
	}
	// el fundido baja 40 LU en 8 s; −50 LUFS se cruza a ~7,6 s del inicio del fundido
	bodyEnd := time.Second + 3*time.Minute
	if a.CueOut < bodyEnd+7*time.Second || a.CueOut > bodyEnd+8*time.Second {
		t.Fatalf("cue-out = %v, esperaba entre %v y %v", a.CueOut, bodyEnd+7*time.Second, bodyEnd+8*time.Second)
	}
	// caída de 10 LU = 2 s después de empezar el fundido
	if a.SegueAt < bodyEnd+1500*time.Millisecond || a.SegueAt > bodyEnd+2500*time.Millisecond {
		t.Fatalf("segue = %v, esperaba ~%v", a.SegueAt, bodyEnd+2*time.Second)
	}
}

func TestDeriveDryEndAndBoostCap(t *testing.T) {
	m, d := series(0, 2*time.Minute, 0, 0, -30) // pista floja que termina en seco
	a := Derive(-30, m, d, DefaultAnalysisRules())
	if a.GainDB != 6 {
		t.Fatalf("la subida debe limitarse a +6 dB, dio %v", a.GainDB)
	}
	if a.CueIn != 0 || a.CueOut != 0 {
		t.Fatalf("sin silencios no hay cues: in=%v out=%v", a.CueIn, a.CueOut)
	}
	if a.SegueAt != d-time.Second {
		t.Fatalf("final seco: segue = %v, quería %v", a.SegueAt, d-time.Second)
	}
}

func TestEnvelope(t *testing.T) {
	m, d := series(time.Second, time.Minute, 8*time.Second, time.Second, -12)
	env := Derive(-12, m, d, DefaultAnalysisRules()).Envelope
	if len(env) != 200 || env[0] != '0' || env[len(env)-1] != '0' {
		t.Fatalf("envolvente = %q", env)
	}
	// cuerpo a −12 LUFS → (48/60)·35 = 28 → "s"
	if env[100] != 's' {
		t.Fatalf("nivel del cuerpo = %q, quería 's'", env[100])
	}
}

func TestDeriveSilence(t *testing.T) {
	m, d := series(10*time.Second, 0, 0, 0, -120)
	if a := Derive(-120, m, d, DefaultAnalysisRules()); a.Err == "" {
		t.Fatal("una pista en silencio debe marcarse con error")
	}
}
