// Package video es el modo TV de Club25 Estudio: arma el video del canal de TV
// de la web (visualizador animado con el audio de la radio, fondo, logo,
// cintillo «ahora suena», reloj y ticker) y lo emite con ffmpeg por RTMP al
// servidor (mediamtx), junto con el mismo audio que va a Icecast.
//
// Formato interno: YUV 4:2:0 de 8 bits (BT.709, rango limitado), el que recibe
// directamente el codificador H.264. La composición es código propio en Go;
// ffmpeg solo codifica (ver docs/tv.md).
//
// Ideas de código abierto (reescritas, no copiadas): escenas → fuentes →
// transiciones de OBS Studio; capas de grafismo de CasparCG; fallback de
// Liquidsoap. Textos con golang.org/x/image (BSD) y las fuentes Go (BSD).
package video

import "image/color"

// Frame es un cuadro YUV 4:2:0 (planos separados). W y H son pares.
type Frame struct {
	W, H    int
	Y, U, V []byte
}

// NewFrame reserva un cuadro negro de w×h (se redondean a pares).
func NewFrame(w, h int) *Frame {
	w, h = w&^1, h&^1
	f := &Frame{W: w, H: h, Y: make([]byte, w*h), U: make([]byte, w*h/4), V: make([]byte, w*h/4)}
	f.Fill(YUV{16, 128, 128})
	return f
}

// Size es el tamaño en bytes del cuadro (lo que se envía al codificador).
func (f *Frame) Size() int { return f.W * f.H * 3 / 2 }

// CopyFrom copia src (mismo tamaño).
func (f *Frame) CopyFrom(src *Frame) {
	copy(f.Y, src.Y)
	copy(f.U, src.U)
	copy(f.V, src.V)
}

// AppendTo escribe los tres planos seguidos en dst (formato rawvideo yuv420p).
func (f *Frame) AppendTo(dst []byte) []byte {
	dst = append(dst, f.Y...)
	dst = append(dst, f.U...)
	return append(dst, f.V...)
}

// Fill pinta todo el cuadro de un color.
func (f *Frame) Fill(c YUV) {
	for i := range f.Y {
		f.Y[i] = c.Y
	}
	for i := range f.U {
		f.U[i], f.V[i] = c.U, c.V
	}
}

// Mix deja en f la mezcla de a y b: t = 0 → a, t = 1 → b (fundido).
func (f *Frame) Mix(a, b *Frame, t float32) {
	k := int32(min(max(t, 0), 1) * 256)
	mix := func(dst, x, y []byte) {
		for i := range dst {
			dst[i] = byte((int32(x[i])*(256-k) + int32(y[i])*k) >> 8)
		}
	}
	mix(f.Y, a.Y, b.Y)
	mix(f.U, a.U, b.U)
	mix(f.V, a.V, b.V)
}

// FillRect pinta un rectángulo con opacidad alpha (0–255), recortado al cuadro.
func (f *Frame) FillRect(x, y, w, h int, c YUV, alpha uint8) {
	x0, y0 := max(x, 0)&^1, max(y, 0)&^1
	x1, y1 := min(x+w, f.W), min(y+h, f.H)
	if x1 <= x0 || y1 <= y0 || alpha == 0 {
		return
	}
	a := int32(alpha)
	for yy := y0; yy < y1; yy++ {
		row := f.Y[yy*f.W:]
		for xx := x0; xx < x1; xx++ {
			row[xx] = blend(row[xx], c.Y, a)
		}
	}
	cw := f.W / 2
	for yy := y0 / 2; yy < (y1+1)/2; yy++ {
		for xx := x0 / 2; xx < (x1+1)/2; xx++ {
			i := yy*cw + xx
			f.U[i] = blend(f.U[i], c.U, a)
			f.V[i] = blend(f.V[i], c.V, a)
		}
	}
}

func blend(dst, src byte, a int32) byte {
	return byte(int32(dst) + (int32(src)-int32(dst))*a/255)
}

// YUV es un color en BT.709 de rango limitado (Y 16–235, U/V 16–240).
type YUV struct{ Y, U, V uint8 }

// ToYUV convierte un color RGB (sin alfa) a YUV BT.709 limitado.
func ToYUV(r, g, b uint8) YUV {
	R, G, B := float64(r), float64(g), float64(b)
	y := 16 + (0.2126*R+0.7152*G+0.0722*B)*219/255
	u := 128 + (-0.1146*R-0.3854*G+0.5*B)*224/255
	v := 128 + (0.5*R-0.4542*G-0.0458*B)*224/255
	return YUV{clamp8(y), clamp8(u), clamp8(v)}
}

// FromColor convierte un color.Color (se ignora el alfa).
func FromColor(c color.Color) YUV {
	n := color.NRGBAModel.Convert(c).(color.NRGBA)
	return ToYUV(n.R, n.G, n.B)
}

func clamp8(v float64) uint8 {
	switch {
	case v < 0:
		return 0
	case v > 255:
		return 255
	}
	return uint8(v + 0.5)
}

// ParseHex lee "#rrggbb" (o "rrggbb"); def si no se puede.
func ParseHex(s string, def color.NRGBA) color.NRGBA {
	if len(s) > 0 && s[0] == '#' {
		s = s[1:]
	}
	if len(s) != 6 {
		return def
	}
	var v [3]uint8
	for i := 0; i < 3; i++ {
		hi, ok1 := hexVal(s[2*i])
		lo, ok2 := hexVal(s[2*i+1])
		if !ok1 || !ok2 {
			return def
		}
		v[i] = hi<<4 | lo
	}
	return color.NRGBA{v[0], v[1], v[2], 255}
}

func hexVal(c byte) (uint8, bool) {
	switch {
	case c >= '0' && c <= '9':
		return c - '0', true
	case c >= 'a' && c <= 'f':
		return c - 'a' + 10, true
	case c >= 'A' && c <= 'F':
		return c - 'A' + 10, true
	}
	return 0, false
}
