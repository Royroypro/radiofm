package video

import (
	"image"
	"image/color"
	"image/draw"
	_ "image/gif" // formatos de imagen para logo y fondos
	_ "image/jpeg"
	_ "image/png"
	"os"

	xdraw "golang.org/x/image/draw"
	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

// Layer es una capa de grafismo ya convertida a YUV con alfa: se pre-renderiza
// una vez (cuando cambia el texto o la imagen) y se mezcla en cada cuadro.
type Layer struct {
	W, H  int    // pares
	Y, A  []byte // luma y alfa a resolución completa
	U, V  []byte // croma a media resolución
	CA    []byte // alfa de croma (promedio del bloque 2×2)
	Empty bool
}

// NewLayer convierte una imagen (con transparencia) en capa.
func NewLayer(img image.Image) *Layer {
	b := img.Bounds()
	w, h := (b.Dx()+1)&^1, (b.Dy()+1)&^1
	l := &Layer{W: w, H: h, Y: make([]byte, w*h), A: make([]byte, w*h),
		U: make([]byte, w*h/4), V: make([]byte, w*h/4), CA: make([]byte, w*h/4), Empty: true}
	rgba := image.NewNRGBA(image.Rect(0, 0, w, h))
	draw.Draw(rgba, rgba.Bounds(), img, b.Min, draw.Src)
	cw := w / 2
	usum := make([]int32, cw*(h/2))
	vsum := make([]int32, cw*(h/2))
	asum := make([]int32, cw*(h/2))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			p := rgba.NRGBAAt(x, y)
			c := ToYUV(p.R, p.G, p.B)
			i := y*w + x
			l.Y[i], l.A[i] = c.Y, p.A
			if p.A > 0 {
				l.Empty = false
			}
			ci := (y/2)*cw + x/2
			a := int32(p.A)
			usum[ci] += int32(c.U) * a
			vsum[ci] += int32(c.V) * a
			asum[ci] += a
		}
	}
	for i := range l.U {
		if asum[i] > 0 {
			l.U[i], l.V[i] = byte(usum[i]/asum[i]), byte(vsum[i]/asum[i])
		} else {
			l.U[i], l.V[i] = 128, 128
		}
		l.CA[i] = byte(asum[i] / 4)
	}
	return l
}

// Draw mezcla en f la capa l con la esquina en (x, y), tomando de la capa las
// columnas [sx, sx+sw) (para el ticker; sw ≤ 0 = toda) y con opacidad op (0–1).
func (f *Frame) Draw(l *Layer, x, y, sx, sw int, op float32) {
	if l == nil || l.Empty || op <= 0 {
		return
	}
	if sw <= 0 || sx+sw > l.W {
		sw = l.W - sx
	}
	x, y, sx = x&^1, y&^1, max(sx, 0)&^1
	k := int32(min(op, 1) * 256)
	x0, y0 := max(x, 0), max(y, 0)
	x1, y1 := min(x+sw, f.W), min(y+l.H, f.H)
	for yy := y0; yy < y1; yy++ {
		ly := yy - y
		drow, lrow, arow := f.Y[yy*f.W:], l.Y[ly*l.W:], l.A[ly*l.W:]
		for xx := x0; xx < x1; xx++ {
			lx := xx - x + sx
			a := int32(arow[lx]) * k >> 8
			if a != 0 {
				drow[xx] = blend(drow[xx], lrow[lx], a)
			}
		}
	}
	fcw, lcw := f.W/2, l.W/2
	for yy := y0 / 2; yy < y1/2; yy++ {
		ly := yy - y/2
		for xx := x0 / 2; xx < x1/2; xx++ {
			lx := xx - x/2 + sx/2
			li := ly*lcw + lx
			a := int32(l.CA[li]) * k >> 8
			if a != 0 {
				fi := yy*fcw + xx
				f.U[fi] = blend(f.U[fi], l.U[li], a)
				f.V[fi] = blend(f.V[fi], l.V[li], a)
			}
		}
	}
}

// LoadImage abre un PNG, JPG o GIF.
func LoadImage(path string) (image.Image, error) {
	fh, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer fh.Close()
	img, _, err := image.Decode(fh)
	return img, err
}

// Fit escala img a w×h: cover = llena y recorta (fondos); si no, cabe entera (logos).
func Fit(img image.Image, w, h int, cover bool) *image.NRGBA {
	b := img.Bounds()
	sw, sh := float64(b.Dx()), float64(b.Dy())
	scale := min(float64(w)/sw, float64(h)/sh)
	if cover {
		scale = max(float64(w)/sw, float64(h)/sh)
	}
	tw, th := int(sw*scale+0.5), int(sh*scale+0.5)
	if !cover {
		w, h = tw, th
	}
	dst := image.NewNRGBA(image.Rect(0, 0, w, h))
	r := image.Rect((w-tw)/2, (h-th)/2, (w-tw)/2+tw, (h-th)/2+th)
	xdraw.CatmullRom.Scale(dst, r, img, b, xdraw.Over, nil)
	return dst
}

// FrameFromImage arma un cuadro de fondo con la imagen (cubriendo todo).
func FrameFromImage(img image.Image, w, h int) *Frame {
	f := NewFrame(w, h)
	f.Draw(NewLayer(Fit(img, w, h, true)), 0, 0, 0, 0, 1)
	return f
}

// GradientFrame arma un fondo con degradado vertical de top a bottom.
func GradientFrame(w, h int, top, bottom color.NRGBA) *Frame {
	f := NewFrame(w, h)
	for y := 0; y < h; y += 2 {
		t := float64(y) / float64(h)
		c := ToYUV(lerp8(top.R, bottom.R, t), lerp8(top.G, bottom.G, t), lerp8(top.B, bottom.B, t))
		for yy := y; yy < y+2; yy++ {
			row := f.Y[yy*w : (yy+1)*w]
			for i := range row {
				row[i] = c.Y
			}
		}
		crow := (y / 2) * (w / 2)
		for i := 0; i < w/2; i++ {
			f.U[crow+i], f.V[crow+i] = c.U, c.V
		}
	}
	return f
}

func lerp8(a, b uint8, t float64) uint8 { return uint8(float64(a) + (float64(b)-float64(a))*t + 0.5) }

// fonts son las caras de letra usadas en el grafismo (fuentes Go, licencia BSD).
type fonts struct {
	bold, regular *opentype.Font
	cache         map[[2]int]font.Face // [negrita, tamaño] → cara
}

func newFonts() *fonts {
	b, _ := opentype.Parse(gobold.TTF)
	r, _ := opentype.Parse(goregular.TTF)
	return &fonts{bold: b, regular: r, cache: map[[2]int]font.Face{}}
}

func (f *fonts) face(bold bool, size int) font.Face {
	k := [2]int{0, size}
	src := f.regular
	if bold {
		k[0], src = 1, f.bold
	}
	if fc, ok := f.cache[k]; ok {
		return fc
	}
	fc, _ := opentype.NewFace(src, &opentype.FaceOptions{Size: float64(size), DPI: 72, Hinting: font.HintingFull})
	f.cache[k] = fc
	return fc
}

// measure devuelve el ancho en píxeles de s.
func (f *fonts) measure(bold bool, size int, s string) int {
	return font.MeasureString(f.face(bold, size), s).Ceil()
}

// text dibuja s en dst con la línea base en (x, y).
func (f *fonts) text(dst draw.Image, bold bool, size, x, y int, s string, c color.Color) {
	d := font.Drawer{Dst: dst, Src: image.NewUniform(c), Face: f.face(bold, size), Dot: fixed.P(x, y)}
	d.DrawString(s)
}

// fitText recorta s con «…» para que no pase de maxW.
func (f *fonts) fitText(bold bool, size int, s string, maxW int) string {
	if f.measure(bold, size, s) <= maxW {
		return s
	}
	r := []rune(s)
	for len(r) > 1 {
		r = r[:len(r)-1]
		if t := string(r) + "…"; f.measure(bold, size, t) <= maxW {
			return t
		}
	}
	return s
}

// roundRect pinta un rectángulo redondeado (radio rad) de color c en dst.
func roundRect(dst *image.NRGBA, r image.Rectangle, rad int, c color.NRGBA) {
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			dx := max(r.Min.X+rad-x, x-(r.Max.X-1-rad), 0)
			dy := max(r.Min.Y+rad-y, y-(r.Max.Y-1-rad), 0)
			if dx*dx+dy*dy <= rad*rad {
				dst.SetNRGBA(x, y, c)
			}
		}
	}
}
