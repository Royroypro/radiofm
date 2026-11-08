package audio

import (
	"math"
	"time"
)

// --- Ducking ---------------------------------------------------------------

// Ducker baja la música cuando habla el micrófono (o a mano). La ganancia
// sigue al objetivo con constantes de tiempo de ataque y relajación.
// Idea del "talkover ducking" de Mixxx (GPL-2.0-or-later), reescrito.
type Ducker struct {
	depth            float32 // ganancia con ducking (p. ej. −12 dB → 0.25)
	threshold        float32 // nivel de pico del mic que activa el ducking
	attack, release  float32 // coeficientes por frame
	gain             float32 // ganancia actual
	holdFrames, held int64   // mantener el ducking un rato tras la última voz
}

// NewDucker crea un ducker. depthDB es negativo (−12), thresholdDB el umbral del mic (−35).
func NewDucker(depthDB, thresholdDB float64, attack, release time.Duration) *Ducker {
	d := &Ducker{gain: 1, holdFrames: DurationToFrames(300 * time.Millisecond)}
	d.Configure(depthDB, thresholdDB, attack, release)
	return d
}

// Configure cambia los parámetros (llamar solo desde el hilo de audio o antes de arrancar).
func (d *Ducker) Configure(depthDB, thresholdDB float64, attack, release time.Duration) {
	d.depth, d.threshold = DBToGain(depthDB), DBToGain(thresholdDB)
	d.attack, d.release = timeCoef(attack), timeCoef(release)
}

// timeCoef convierte una constante de tiempo a coeficiente de suavizado por frame.
func timeCoef(t time.Duration) float32 {
	if t <= 0 {
		return 1
	}
	return float32(1 - math.Exp(-1/(t.Seconds()*SampleRate)))
}

// Process calcula la ganancia de la música para un bloque de frames.
// active fuerza el ducking (botón); micPeak es el pico del mic en el bloque.
func (d *Ducker) Process(frames int, active bool, micPeak float32) float32 {
	if micPeak >= d.threshold {
		d.held = d.holdFrames
	}
	on := active || d.held > 0
	if d.held > 0 {
		d.held -= int64(frames)
	}
	target, coef := float32(1), d.release
	if on {
		target, coef = d.depth, d.attack
	}
	// aplicar el suavizado de frames pasos de una vez: g += (t−g)·(1−(1−c)^n)
	k := 1 - float32(math.Pow(float64(1-coef), float64(frames)))
	d.gain += (target - d.gain) * k
	return d.gain
}

// --- Limitador ---------------------------------------------------------------

// Limiter es un limitador de picos con anticipación (look-ahead): retrasa la
// señal lookahead frames y baja la ganancia antes de que llegue el pico, así
// la salida nunca supera el techo. Referencia: af_alimiter de FFmpeg (LGPL).
type Limiter struct {
	ceiling float32
	release float32
	delay   []float32 // línea de retardo (muestras entrelazadas)
	want    []float32 // ganancia deseada por frame en la ventana
	pos     int
	gain    float32
	// Reduction es la reducción máxima del último bloque (para la interfaz).
	Reduction float32
}

// NewLimiter crea un limitador con techo en dBFS (p. ej. −1).
func NewLimiter(ceilingDB float64, lookahead, release time.Duration) *Limiter {
	n := int(DurationToFrames(lookahead))
	if n < 1 {
		n = 1
	}
	w := make([]float32, n)
	for i := range w {
		w[i] = 1
	}
	return &Limiter{ceiling: DBToGain(ceilingDB), release: timeCoef(release), delay: make([]float32, n*Channels),
		want: w, gain: 1}
}

// Process limita buf (entrelazado) en el lugar.
func (l *Limiter) Process(buf []float32) {
	n := len(l.want)
	minGain := float32(1)
	for i := 0; i+1 < len(buf); i += 2 {
		inL, inR := buf[i], buf[i+1]
		peak := max(abs32(inL), abs32(inR))
		w := float32(1)
		if peak > l.ceiling {
			w = l.ceiling / peak
		}
		l.want[l.pos] = w
		// mínimo de la ventana: la ganancia ya está baja cuando el pico sale del retardo
		target := float32(1)
		for _, x := range l.want {
			if x < target {
				target = x
			}
		}
		if target < l.gain {
			l.gain = target
		} else {
			l.gain += (target - l.gain) * l.release
		}
		outL, outR := l.delay[l.pos*2], l.delay[l.pos*2+1]
		l.delay[l.pos*2], l.delay[l.pos*2+1] = inL, inR
		buf[i], buf[i+1] = outL*l.gain, outR*l.gain
		l.pos++
		if l.pos == n {
			l.pos = 0
		}
		if l.gain < minGain {
			minGain = l.gain
		}
	}
	l.Reduction = minGain
}

// --- Medidor -----------------------------------------------------------------

// Levels son los niveles de un bloque (lineales, 0–1+).
type Levels struct {
	PeakL, PeakR, RMSL, RMSR float32
}

// measure calcula picos y RMS de un bloque entrelazado.
func measure(buf []float32) Levels {
	var lv Levels
	var sl, sr float64
	frames := len(buf) / Channels
	if frames == 0 {
		return lv
	}
	for i := 0; i+1 < len(buf); i += 2 {
		l, r := buf[i], buf[i+1]
		lv.PeakL, lv.PeakR = max(lv.PeakL, abs32(l)), max(lv.PeakR, abs32(r))
		sl += float64(l * l)
		sr += float64(r * r)
	}
	lv.RMSL = float32(math.Sqrt(sl / float64(frames)))
	lv.RMSR = float32(math.Sqrt(sr / float64(frames)))
	return lv
}

func peakOf(buf []float32) float32 {
	var p float32
	for _, v := range buf {
		p = max(p, abs32(v))
	}
	return p
}

func abs32(v float32) float32 {
	if v < 0 {
		return -v
	}
	return v
}
