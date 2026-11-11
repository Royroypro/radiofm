package video

import (
	"image"
	"image/color"
	"strings"
	"time"
)

// Look es el aspecto del grafismo (Ajustes → TV).
type Look struct {
	Station  string      // nombre de la radio (logo de texto si no hay imagen)
	Logo     image.Image // nil = el nombre en texto
	LogoPos  string      // tl | tr | bl | br (esquina)
	Accent   color.NRGBA // color principal (barras abajo, franja del cintillo)
	Accent2  color.NRGBA // color secundario (barras arriba, picos)
	Panel    color.NRGBA // fondo de los recuadros (con transparencia)
	Text     color.NRGBA
	ShowLogo bool
	ShowNow  bool // cintillo «ahora suena»
	ShowClk  bool
	ShowTick bool
	Ticker   string
	Loc      *time.Location
}

// Graphics son las capas de grafismo: se renderizan solo cuando cambian.
type Graphics struct {
	w, h int
	look Look
	fn   *fonts

	logo   *Layer
	now    *Layer
	nowKey string
	nowAt  time.Time // cuándo cambió (entra con fundido)
	clk    *Layer
	clkKey string
	brk    *Layer
	tick   *Layer
	tickX  float64
	tickP  int    // ancho de una repetición del texto (el desplazamiento vuelve a 0)
	band   *Layer // franja del ticker

	artist, title string
	isBreak       bool
	breakName     string
}

// NewGraphics prepara el grafismo para cuadros de w×h.
func NewGraphics(w, h int, look Look) *Graphics {
	g := &Graphics{w: w, h: h, look: look, fn: newFonts()}
	if look.Loc == nil {
		g.look.Loc = time.Local
	}
	g.renderLogo()
	g.renderTicker()
	return g
}

func (g *Graphics) px(f float64) int { return int(f * float64(g.h)) } // medidas relativas a la altura

// SetNowPlaying cambia lo que suena. brk = es parte de una tanda (se muestra
// la placa «Publicidad» con el anunciante en vez del cintillo).
func (g *Graphics) SetNowPlaying(artist, title string, brk bool) {
	g.artist, g.title, g.isBreak = artist, title, brk
	if brk {
		g.breakName = artist
	}
}

func (g *Graphics) renderLogo() {
	h := g.px(0.11)
	if g.look.Logo != nil {
		g.logo = NewLayer(Fit(g.look.Logo, h*4, h, false))
		return
	}
	name := strings.ToUpper(g.look.Station)
	if name == "" {
		return
	}
	size := g.px(0.045)
	w := g.fn.measure(true, size, name) + 2*g.px(0.025)
	img := image.NewNRGBA(image.Rect(0, 0, w, g.px(0.075)))
	roundRect(img, img.Rect, g.px(0.012), g.look.Accent)
	g.fn.text(img, true, size, g.px(0.025), g.px(0.054), name, color.White)
	g.logo = NewLayer(img)
}

func (g *Graphics) renderNow() {
	key := g.artist + "\x00" + g.title
	if key == g.nowKey && g.now != nil {
		return
	}
	g.nowKey, g.nowAt = key, time.Now()
	if g.title == "" && g.artist == "" {
		g.now = nil
		return
	}
	pad := g.px(0.022)
	tSize, aSize, lSize := g.px(0.048), g.px(0.034), g.px(0.022)
	maxW := g.w * 62 / 100
	title := g.fn.fitText(true, tSize, g.title, maxW-2*pad)
	artist := g.fn.fitText(false, aSize, g.artist, maxW-2*pad)
	label := "AHORA SUENA"
	w := max(g.fn.measure(true, tSize, title), g.fn.measure(false, aSize, artist), g.fn.measure(true, lSize, label)) + 2*pad + g.px(0.012)
	h := pad*2 + lSize + tSize + aSize + g.px(0.03)
	if artist == "" {
		h -= aSize
	}
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	roundRect(img, img.Rect, g.px(0.014), g.look.Panel)
	roundRect(img, image.Rect(0, 0, g.px(0.012), h), 0, g.look.Accent) // franja de color
	x := pad + g.px(0.012)
	y := pad + lSize
	g.fn.text(img, true, lSize, x, y, label, g.look.Accent2)
	y += tSize + g.px(0.012)
	g.fn.text(img, true, tSize, x, y, title, g.look.Text)
	if artist != "" {
		y += aSize + g.px(0.01)
		g.fn.text(img, false, aSize, x, y, artist, color.NRGBA{g.look.Text.R, g.look.Text.G, g.look.Text.B, 210})
	}
	g.now = NewLayer(img)
}

func (g *Graphics) renderClock(now time.Time) {
	key := now.In(g.look.Loc).Format("15:04")
	if key == g.clkKey {
		return
	}
	g.clkKey = key
	size := g.px(0.042)
	pad := g.px(0.018)
	w := g.fn.measure(true, size, key) + 2*pad
	img := image.NewNRGBA(image.Rect(0, 0, w, size+2*pad))
	roundRect(img, img.Rect, g.px(0.012), g.look.Panel)
	g.fn.text(img, true, size, pad, pad+size*86/100, key, g.look.Text)
	g.clk = NewLayer(img)
}

func (g *Graphics) renderBreak() {
	size := g.px(0.03)
	pad := g.px(0.016)
	label := "PUBLICIDAD"
	w := g.fn.measure(true, size, label) + 2*pad
	img := image.NewNRGBA(image.Rect(0, 0, w, size+2*pad))
	roundRect(img, img.Rect, g.px(0.01), g.look.Accent)
	g.fn.text(img, true, size, pad, pad+size*86/100, label, color.White)
	g.brk = NewLayer(img)
}

func (g *Graphics) renderTicker() {
	text := strings.TrimSpace(g.look.Ticker)
	if text == "" {
		g.tick, g.band = nil, nil
		return
	}
	bh := g.px(0.065)
	band := image.NewNRGBA(image.Rect(0, 0, g.w, bh))
	roundRect(band, band.Rect, 0, g.look.Panel)
	roundRect(band, image.Rect(0, 0, g.w, g.px(0.006)), 0, g.look.Accent)
	g.band = NewLayer(band)
	size := g.px(0.034)
	text += "   •   "
	w := g.fn.measure(false, size, text)
	reps := g.w/max(w, 1) + 2 // repetido para que el desplazamiento no deje huecos
	img := image.NewNRGBA(image.Rect(0, 0, w*reps, bh))
	for i := 0; i < reps; i++ {
		g.fn.text(img, false, size, i*w, bh/2+size*36/100+g.px(0.003), text, g.look.Text)
	}
	g.tick = NewLayer(img)
	g.tickX = 0
	g.tickP = max(w&^1, 2)
}

// Draw pone las capas sobre f. fps sirve para la velocidad del ticker.
func (g *Graphics) Draw(f *Frame, now time.Time, fps int) {
	m := g.px(0.04) // margen
	bottom := g.h - m
	if g.look.ShowTick && g.tick != nil {
		bh := g.band.H
		f.Draw(g.band, 0, g.h-bh, 0, 0, 1)
		f.Draw(g.tick, 0, g.h-bh, int(g.tickX), g.w, 1)
		g.tickX += float64(g.h) / 720 * 120 / float64(max(fps, 1)) // ~120 px/s a 720p
		if int(g.tickX) >= g.tickP {
			g.tickX -= float64(g.tickP)
		}
		bottom = g.h - bh - m/2
	}
	if g.look.ShowLogo && g.logo != nil {
		x, y := g.w-m-g.logo.W, m
		switch g.look.LogoPos {
		case "tl":
			x = m
		case "bl":
			x, y = m, bottom-g.logo.H
		case "br":
			y = bottom - g.logo.H
		}
		f.Draw(g.logo, x, y, 0, 0, 0.95)
	}
	top := m
	if g.look.ShowClk {
		g.renderClock(now)
		x := m
		if g.look.LogoPos == "tl" && g.look.ShowLogo {
			x = g.w - m - g.clk.W
		}
		f.Draw(g.clk, x, top, 0, 0, 1)
		top += g.clk.H + m/3
	}
	if g.isBreak {
		if g.brk == nil {
			g.renderBreak()
		}
		x := m
		if g.look.LogoPos == "tl" && g.look.ShowLogo {
			x = g.w - m - g.brk.W
		}
		f.Draw(g.brk, x, top, 0, 0, 1)
	}
	if g.look.ShowNow {
		g.renderNow()
		if g.now != nil {
			op := float32(now.Sub(g.nowAt).Seconds() / 0.6) // entra con fundido
			x := m
			if g.look.LogoPos == "bl" && g.look.ShowLogo {
				x = g.w - m - g.now.W
			}
			f.Draw(g.now, x, bottom-g.now.H, 0, 0, min(op, 1))
		}
	}
}
