package video

import (
	"image/color"
	"math"
	"math/cmplx"
)

// Visualizer dibuja el espectro del audio que suena (barras con picos y
// reflejo): el «efecto de audio dinámico» cuando no hay video. Analiza el mismo
// audio que sale en la TV, así que se mueve exactamente con lo que se oye.
type Visualizer struct {
	n      int       // tamaño de la FFT
	hist   []float32 // últimas n muestras mono
	win    []float64 // ventana de Hann
	buf    []complex128
	tw     []complex128 // factores de giro
	rev    []int        // permutación de bits
	edges  []int        // límites de bin de cada barra (len = barras+1)
	level  []float32    // nivel suavizado de cada barra (0–1)
	peak   []float32
	hold   []int
	low    YUV // color de abajo de las barras
	high   YUV // color de arriba
	shadow YUV
}

// NewVisualizer crea un visualizador de bars barras entre 40 Hz y 16 kHz
// (escala logarítmica, como un analizador de consola).
func NewVisualizer(bars int, low, high color.NRGBA) *Visualizer {
	const n = 2048
	v := &Visualizer{n: n, hist: make([]float32, n), win: make([]float64, n), buf: make([]complex128, n),
		tw: make([]complex128, n/2), rev: make([]int, n), level: make([]float32, bars),
		peak: make([]float32, bars), hold: make([]int, bars),
		low: FromColor(low), high: FromColor(high), shadow: ToYUV(0, 0, 0)}
	for i := range v.win {
		v.win[i] = 0.5 - 0.5*math.Cos(2*math.Pi*float64(i)/float64(n-1))
	}
	for i := range v.tw {
		v.tw[i] = cmplx.Exp(complex(0, -2*math.Pi*float64(i)/float64(n)))
	}
	bits := 0
	for 1<<bits < n {
		bits++
	}
	for i := range v.rev {
		r := 0
		for b := 0; b < bits; b++ {
			if i&(1<<b) != 0 {
				r |= 1 << (bits - 1 - b)
			}
		}
		v.rev[i] = r
	}
	const sr = 44100.0
	lo, hi := math.Log(40), math.Log(16000)
	v.edges = make([]int, bars+1)
	for i := range v.edges {
		f := math.Exp(lo + (hi-lo)*float64(i)/float64(bars))
		v.edges[i] = int(f / sr * n)
		if i > 0 && v.edges[i] <= v.edges[i-1] {
			v.edges[i] = v.edges[i-1] + 1
		}
	}
	return v
}

// Push agrega audio estéreo entrelazado (se analiza en mono).
func (v *Visualizer) Push(stereo []float32) {
	k := len(stereo) / 2
	if k >= v.n {
		stereo, k = stereo[len(stereo)-2*v.n:], v.n
	}
	copy(v.hist, v.hist[k:])
	base := v.n - k
	for i := 0; i < k; i++ {
		v.hist[base+i] = (stereo[2*i] + stereo[2*i+1]) / 2
	}
}

// Update calcula el espectro y actualiza los niveles (una vez por cuadro).
func (v *Visualizer) Update() {
	for i, s := range v.hist {
		v.buf[v.rev[i]] = complex(float64(s)*v.win[i], 0)
	}
	// FFT radix-2 iterativa (Cooley-Tukey)
	for size := 2; size <= v.n; size <<= 1 {
		half, step := size/2, v.n/size
		for start := 0; start < v.n; start += size {
			for j := 0; j < half; j++ {
				t := v.tw[j*step] * v.buf[start+j+half]
				u := v.buf[start+j]
				v.buf[start+j], v.buf[start+j+half] = u+t, u-t
			}
		}
	}
	norm := 4 / float64(v.n) // ventana de Hann: un seno a escala completa ≈ 0 dB
	for b := range v.level {
		m := 0.0
		for i := v.edges[b]; i < v.edges[b+1] && i < v.n/2; i++ {
			m = max(m, cmplx.Abs(v.buf[i])*norm)
		}
		db := 20 * math.Log10(m+1e-9)
		// realce leve de agudos: la música tiene menos energía arriba (como los analizadores «pink»)
		db += 3 * math.Log2(float64(v.edges[b]+1)/float64(v.edges[0]+1)) / 2
		target := float32(min(max((db+66)/60, 0), 1))
		if target > v.level[b] {
			v.level[b] += (target - v.level[b]) * 0.65
		} else {
			v.level[b] = max(target, v.level[b]-0.035)
		}
		if v.level[b] >= v.peak[b] {
			v.peak[b], v.hold[b] = v.level[b], 18
		} else if v.hold[b] > 0 {
			v.hold[b]--
		} else {
			v.peak[b] = max(0, v.peak[b]-0.012)
		}
	}
}

// Levels devuelve los niveles actuales (0–1) de cada barra (para los tests).
func (v *Visualizer) Levels() []float32 { return v.level }

// Draw pinta las barras en f dentro del área (x, y, w, h): la base de las
// barras está a 3/4 de la altura y debajo va el reflejo.
func (v *Visualizer) Draw(f *Frame, x, y, w, h int) {
	bars := len(v.level)
	slot := float64(w) / float64(bars)
	bw := max(int(slot*0.68), 2)
	base := y + h*3/4
	maxH := h * 3 / 4
	refl := h / 4
	for b := 0; b < bars; b++ {
		bx := x + int(float64(b)*slot+(slot-float64(bw))/2)
		bh := max(int(v.level[b]*float32(maxH)), 2)
		// barra con degradado (de low abajo a high arriba) en tramos de 8 px
		for yy := 0; yy < bh; yy += 8 {
			seg := min(8, bh-yy)
			t := float64(yy) / float64(maxH)
			c := YUV{lerp8(v.low.Y, v.high.Y, t), lerp8(v.low.U, v.high.U, t), lerp8(v.low.V, v.high.V, t)}
			f.FillRect(bx, base-yy-seg, bw, seg, c, 255)
		}
		// reflejo atenuado
		rh := min(bh*refl/maxH, refl)
		f.FillRect(bx, base+4, bw, rh, v.low, 70)
		// pico
		py := base - int(v.peak[b]*float32(maxH)) - 6
		f.FillRect(bx, py, bw, 4, v.high, 230)
	}
}

// DrawBand pinta las barras en una franja (x, y, w, h) encima de un video: sin
// reflejo, desde el borde de abajo de la franja, con opacidad alpha (0–255) y
// una sombra suave detrás para que se lean sobre cualquier imagen.
func (v *Visualizer) DrawBand(f *Frame, x, y, w, h int, alpha uint8) {
	bars := len(v.level)
	slot := float64(w) / float64(bars)
	bw := max(int(slot*0.62), 2)
	base := y + h
	// sombra a todo el ancho, más oscura abajo (en tramos), para que las barras se lean
	for i, a := range []uint8{25, 40, 55, 70} {
		f.FillRect(0, y+h*(40+15*i)/100, f.W, h*15/100+2, v.shadow, a)
	}
	for b := 0; b < bars; b++ {
		bx := x + int(float64(b)*slot+(slot-float64(bw))/2)
		bh := max(int(v.level[b]*float32(h)*0.92), 2)
		for yy := 0; yy < bh; yy += 6 {
			seg := min(6, bh-yy)
			t := float64(yy) / float64(h)
			c := YUV{lerp8(v.low.Y, v.high.Y, t), lerp8(v.low.U, v.high.U, t), lerp8(v.low.V, v.high.V, t)}
			f.FillRect(bx, base-yy-seg, bw, seg, c, alpha)
		}
		py := base - int(v.peak[b]*float32(h)*0.92) - 5
		f.FillRect(bx, py, bw, 3, v.high, alpha)
	}
}
