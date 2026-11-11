package audio

import (
	"math"
	"time"
)

// Procesador de audio del bus master: AGC lento → ecualizador de 10 bandas →
// ampliación estéreo → compresor de 3 bandas. El
// limitador de picos (Limiter) va después. Es lo que hace que la radio suene
// pareja como una FM comercial: canciones grabadas a distinto nivel quedan
// iguales y la voz del locutor no queda tapada ni salta.
//
// Algoritmos de referencia (reescritos, no copiados): acompressor/mcompand de
// FFmpeg (LGPL) y los compresores de LSP plugins (LGPL). Cruces de bandas
// Linkwitz-Riley de 4.º orden (dos Butterworth de 2.º orden en cascada); la
// banda media se obtiene restando las otras dos a la señal, así que con los
// compresores sin actuar la salida es idéntica a la entrada.
//
// Reglas del hilo de audio: Process no bloquea ni reserva memoria.

// ProcPreset es un ajuste predefinido del procesador.
type ProcPreset string

const (
	ProcOff     ProcPreset = "off"     // sin procesar (solo el limitador)
	ProcNatural ProcPreset = "natural" // suave: iguala niveles sin que se note
	ProcFM      ProcPreset = "fm"      // FM comercial: más densidad y presencia
	ProcVoice   ProcPreset = "voice"   // hablado: la voz siempre adelante
)

// ProcPresets son los presets en el orden de la interfaz.
var ProcPresets = []ProcPreset{ProcNatural, ProcFM, ProcVoice, ProcOff}

// BandSettings es el compresor de una banda.
type BandSettings struct {
	ThresholdDB float64
	Ratio       float64
	Attack      time.Duration
	Release     time.Duration
	MakeupDB    float64
}

// ProcSettings son los parámetros del procesador.
type ProcSettings struct {
	AGCTargetDB float64       // nivel RMS objetivo (dBFS)
	AGCMaxDB    float64       // corrección máxima del AGC (±dB)
	AGCGateDB   float64       // por debajo (silencio, fundidos) el AGC no se mueve
	AGCTime     time.Duration // constante de tiempo del AGC (lenta: segundos)
	Low         BandSettings  // < 150 Hz
	Mid         BandSettings
	High        BandSettings // > 4 kHz

	// Bypass: sin AGC ni compresores (preset «Sin procesar» con ecualizador o
	// ampliación activos).
	Bypass bool
	// EQ es la ganancia de cada banda del ecualizador (dB, ±12) en EQFreqs.
	EQ [EQBands]float64
	// Wide es la ampliación estéreo: 0 = normal, −1 = mono, +1 = el doble de ancho
	// (el cero es neutro: los ajustes sin ampliación no cambian el estéreo).
	Wide float64
}

// EQBands es la cantidad de bandas del ecualizador gráfico.
const EQBands = 10

// EQFreqs son las frecuencias centrales de las bandas (octavas, como un ecualizador gráfico de consola).
var EQFreqs = [EQBands]float64{31, 62, 125, 250, 500, 1000, 2000, 4000, 8000, 16000}

// eqQ es el Q de cada banda: ~1 octava de ancho, las vecinas se suman sin huecos.
const eqQ = 1.41

// soundNeutral dice si el ecualizador está plano y el estéreo normal (no hace falta procesarlos).
func (s ProcSettings) soundNeutral() bool {
	return s.EQ == [EQBands]float64{} && s.Wide == 0
}

// WithSound agrega a s el ecualizador (dB por banda; se ignoran las que falten)
// y la ampliación estéreo (porcentaje: 100 = normal).
func (s ProcSettings) WithSound(eq []float64, widthPct int) ProcSettings {
	for i := 0; i < EQBands && i < len(eq); i++ {
		s.EQ[i] = min(max(eq[i], -12), 12)
	}
	s.Wide = float64(min(max(widthPct, 0), 200))/100 - 1
	return s
}

// SoundOnly es un procesador sin dinámica: solo ecualizador y ampliación.
// ok=false si los dos están en neutro (no hace falta procesador).
func SoundOnly(eq []float64, widthPct int) (ProcSettings, bool) {
	s := ProcSettings{Bypass: true}.WithSound(eq, widthPct)
	return s, !s.soundNeutral()
}

// peaking calcula un filtro de campana (Audio EQ Cookbook, R. Bristow-Johnson).
func peaking(freq, gainDB, q float64) biquad {
	A := math.Pow(10, gainDB/40)
	w := 2 * math.Pi * freq / SampleRate
	cosw := math.Cos(w)
	alpha := math.Sin(w) / (2 * q)
	a0 := 1 + alpha/A
	return biquad{b0: float32((1 + alpha*A) / a0), b1: float32(-2 * cosw / a0), b2: float32((1 - alpha*A) / a0),
		a1: float32(-2 * cosw / a0), a2: float32((1 - alpha/A) / a0)}
}

// PresetSettings devuelve los parámetros de un preset (ok=false si es ProcOff o desconocido).
func PresetSettings(p ProcPreset) (ProcSettings, bool) {
	ms := time.Millisecond
	switch p {
	case ProcNatural:
		b := BandSettings{ThresholdDB: -20, Ratio: 2, Attack: 20 * ms, Release: 400 * ms, MakeupDB: 2}
		return ProcSettings{AGCTargetDB: -18, AGCMaxDB: 4, AGCGateDB: -45, AGCTime: 3 * time.Second,
			Low: b, Mid: b, High: b}, true
	case ProcFM:
		return ProcSettings{AGCTargetDB: -16, AGCMaxDB: 6, AGCGateDB: -45, AGCTime: 2 * time.Second,
			Low:  BandSettings{ThresholdDB: -24, Ratio: 3, Attack: 30 * ms, Release: 300 * ms, MakeupDB: 5},
			Mid:  BandSettings{ThresholdDB: -24, Ratio: 3, Attack: 10 * ms, Release: 200 * ms, MakeupDB: 5},
			High: BandSettings{ThresholdDB: -26, Ratio: 3, Attack: 5 * ms, Release: 150 * ms, MakeupDB: 6}}, true
	case ProcVoice:
		return ProcSettings{AGCTargetDB: -17, AGCMaxDB: 8, AGCGateDB: -45, AGCTime: 1500 * ms,
			Low:  BandSettings{ThresholdDB: -26, Ratio: 4, Attack: 30 * ms, Release: 300 * ms, MakeupDB: 2},
			Mid:  BandSettings{ThresholdDB: -22, Ratio: 2.5, Attack: 10 * ms, Release: 200 * ms, MakeupDB: 5},
			High: BandSettings{ThresholdDB: -24, Ratio: 2.5, Attack: 5 * ms, Release: 150 * ms, MakeupDB: 4}}, true
	}
	return ProcSettings{}, false
}

// biquad es un filtro de 2.º orden (forma directa transpuesta II), estéreo.
type biquad struct {
	b0, b1, b2, a1, a2 float32
	z1, z2             [Channels]float32
}

func (f *biquad) process(ch int, x float32) float32 {
	y := f.b0*x + f.z1[ch]
	f.z1[ch] = f.b1*x - f.a1*y + f.z2[ch]
	f.z2[ch] = f.b2*x - f.a2*y
	return y
}

// butter2 calcula un Butterworth de 2.º orden pasa-bajos o pasa-altos (fórmulas del
// "Audio EQ Cookbook" de R. Bristow-Johnson, Q = 1/√2).
func butter2(freq float64, high bool) biquad {
	w := 2 * math.Pi * freq / SampleRate
	cosw := math.Cos(w)
	alpha := math.Sin(w) / (2 * (1 / math.Sqrt2)) // α = sin(ω)/(2Q), Q = 1/√2
	a0 := 1 + alpha
	var b0, b1, b2 float64
	if high {
		b0, b1, b2 = (1+cosw)/2, -(1 + cosw), (1+cosw)/2
	} else {
		b0, b1, b2 = (1-cosw)/2, 1-cosw, (1-cosw)/2
	}
	return biquad{b0: float32(b0 / a0), b1: float32(b1 / a0), b2: float32(b2 / a0),
		a1: float32(-2 * cosw / a0), a2: float32((1 - alpha) / a0)}
}

// lr4 es un filtro Linkwitz-Riley de 4.º orden: dos Butterworth iguales en cascada.
type lr4 struct{ a, b biquad }

func newLR4(freq float64, high bool) lr4 { return lr4{a: butter2(freq, high), b: butter2(freq, high)} }

func (f *lr4) process(ch int, x float32) float32 { return f.b.process(ch, f.a.process(ch, x)) }

// bandComp es el compresor de una banda (detector de pico enlazado en estéreo).
type bandComp struct {
	threshold, slope float32 // slope = 1 − 1/ratio
	attack, release  float32
	makeup           float32
	env              float32 // envolvente del detector (lineal)
	gain             float32 // ganancia aplicada (sin makeup)
}

func newBandComp(b BandSettings) bandComp {
	ratio := math.Max(b.Ratio, 1)
	return bandComp{threshold: DBToGain(b.ThresholdDB), slope: float32(1 - 1/ratio),
		attack: timeCoef(b.Attack), release: timeCoef(b.Release), makeup: DBToGain(b.MakeupDB), gain: 1}
}

// step actualiza el detector con el pico del frame y devuelve la ganancia a aplicar.
func (c *bandComp) step(peak float32) float32 {
	if peak > c.env {
		c.env += (peak - c.env) * c.attack
	} else {
		c.env += (peak - c.env) * c.release
	}
	g := float32(1)
	if c.env > c.threshold && c.slope > 0 {
		// reducción en dB = (nivel − umbral)·(1 − 1/ratio)
		over := float64(c.env / c.threshold)
		g = float32(math.Pow(over, -float64(c.slope)))
	}
	c.gain = g
	return g * c.makeup
}

// Processor es la cadena AGC → compresor de 3 bandas.
type Processor struct {
	s ProcSettings

	agcTarget, agcMax, agcMin, agcGate float32
	agcCoef                            float32
	agcMS                              float32 // media cuadrática lenta (el nivel que se corrige)
	gateMS, gateCoef                   float32 // media rápida (300 ms): decide si hay señal
	agcGain                            float32

	lowLP, highHP  lr4
	low, mid, high bandComp

	eq    [EQBands]biquad
	eqN   int // bandas activas (las de 0 dB no se calculan)
	width float32
	sound bool // hay ecualizador o ampliación que aplicar

	// Reduction es la reducción máxima de los compresores en el último bloque
	// (0…1, 1 = sin reducción) y AGC la ganancia del AGC: para el medidor de Al aire.
	Reduction float32
	AGC       float32
}

// NewProcessor crea el procesador con los parámetros dados. Reserva memoria:
// crearlo fuera del hilo de audio y pasarlo al motor con SetProcessor.
func NewProcessor(s ProcSettings) *Processor {
	p := &Processor{s: s, agcGain: 1, Reduction: 1, AGC: 1,
		lowLP: newLR4(150, false), highHP: newLR4(4000, true),
		low: newBandComp(s.Low), mid: newBandComp(s.Mid), high: newBandComp(s.High)}
	p.agcTarget = DBToGain(s.AGCTargetDB)
	p.agcMax, p.agcMin = DBToGain(s.AGCMaxDB), DBToGain(-s.AGCMaxDB)
	p.agcGate = DBToGain(s.AGCGateDB)
	p.agcCoef = timeCoef(s.AGCTime)
	p.agcMS = p.agcTarget * p.agcTarget
	p.gateCoef = timeCoef(300 * time.Millisecond)
	for i, g := range s.EQ {
		if g != 0 && EQFreqs[i] < SampleRate/2 {
			p.eq[p.eqN] = peaking(EQFreqs[i], g, eqQ)
			p.eqN++
		}
	}
	p.width = float32(1 + min(max(s.Wide, -1), 1))
	p.sound = p.eqN > 0 || p.width != 1
	return p
}

// Process procesa buf (entrelazado) en el lugar.
func (p *Processor) Process(buf []float32) {
	red := float32(1)
	if p.s.Bypass {
		p.processSound(buf)
		p.Reduction, p.AGC = 1, 1
		return
	}
	for i := 0; i+1 < len(buf); i += 2 {
		l, r := buf[i], buf[i+1]

		// AGC: media cuadrática lenta. La ganancia solo se corrige mientras hay señal
		// según un detector rápido: en un fundido hacia el silencio o en una pausa se
		// congela (no "bombea" subiendo el ruido de fondo).
		m := (l*l + r*r) / 2
		p.agcMS += (m - p.agcMS) * p.agcCoef
		p.gateMS += (m - p.gateMS) * p.gateCoef
		if rms := float32(math.Sqrt(float64(p.agcMS))); p.gateMS > p.agcGate*p.agcGate && rms > 0 {
			want := min(max(p.agcTarget/rms, p.agcMin), p.agcMax)
			p.agcGain += (want - p.agcGain) * p.agcCoef
		}
		l, r = l*p.agcGain, r*p.agcGain
		if p.sound {
			l, r = p.soundSample(l, r)
		}

		// bandas: graves y agudos por Linkwitz-Riley, medios = resto (reconstrucción exacta)
		lowL, lowR := p.lowLP.process(0, l), p.lowLP.process(1, r)
		highL, highR := p.highHP.process(0, l), p.highHP.process(1, r)
		midL, midR := l-lowL-highL, r-lowR-highR

		gl := p.low.step(max(abs32(lowL), abs32(lowR)))
		gm := p.mid.step(max(abs32(midL), abs32(midR)))
		gh := p.high.step(max(abs32(highL), abs32(highR)))
		buf[i] = lowL*gl + midL*gm + highL*gh
		buf[i+1] = lowR*gl + midR*gm + highR*gh

		red = min(red, p.low.gain, p.mid.gain, p.high.gain)
	}
	p.Reduction, p.AGC = red, p.agcGain
}

// processSound aplica solo el ecualizador y la ampliación (sin dinámica).
func (p *Processor) processSound(buf []float32) {
	if !p.sound {
		return
	}
	for i := 0; i+1 < len(buf); i += 2 {
		buf[i], buf[i+1] = p.soundSample(buf[i], buf[i+1])
	}
}

// soundSample: ecualizador (bandas en cascada) y ampliación estéreo por
// medio/lados (M = (L+R)/2 queda igual; los lados S = (L−R)/2 se multiplican).
func (p *Processor) soundSample(l, r float32) (float32, float32) {
	for k := 0; k < p.eqN; k++ {
		l, r = p.eq[k].process(0, l), p.eq[k].process(1, r)
	}
	if p.width != 1 {
		m, s := (l+r)/2, (l-r)/2*p.width
		l, r = m+s, m-s
	}
	return l, r
}
