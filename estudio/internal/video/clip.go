package video

import (
	"context"
	"io"
	"sync/atomic"
	"time"
)

// clip es un video que se reproduce sincronizado con el audio: el cuadro que
// se muestra sale de la hora (start = momento en que empezó a sonar el audio
// desde la posición pedida), no del ritmo de ffmpeg. ffmpeg decodifica por
// delante (hasta clipQueue cuadros) y, si arrancó tarde o se atrasa, se
// descartan cuadros para alcanzar al audio; si va adelantado, se espera.
type clip struct {
	start  time.Time
	fps    int
	q      chan *Frame
	free   chan *Frame
	cur    *Frame
	idx    int64 // número del cuadro actual (-1 = ninguno todavía)
	ended  atomic.Bool
	rc     io.ReadCloser
	cancel context.CancelFunc
}

const clipQueue = 8

// decodeFunc es Tools.DecodeVideo (los tests usan un falso).
type decodeFunc func(ctx context.Context, path string, start time.Duration, w, h, fps int) (io.ReadCloser, error)

// startClip arranca la decodificación de path desde pos; at es el momento en
// que suena el audio de esa posición.
func startClip(ctx context.Context, dec decodeFunc, path string, pos time.Duration, at time.Time, w, h, fps int) (*clip, error) {
	ctx, cancel := context.WithCancel(ctx)
	rc, err := dec(ctx, path, pos, w, h, fps)
	if err != nil {
		cancel()
		return nil, err
	}
	c := &clip{start: at, fps: fps, q: make(chan *Frame, clipQueue), free: make(chan *Frame, clipQueue+2),
		idx: -1, rc: rc, cancel: cancel}
	for i := 0; i < clipQueue+2; i++ {
		c.free <- NewFrame(w, h)
	}
	go c.read(ctx)
	return c, nil
}

func (c *clip) read(ctx context.Context) {
	defer close(c.q)
	defer c.ended.Store(true)
	for {
		var f *Frame
		select {
		case f = <-c.free:
		case <-ctx.Done():
			return
		}
		if _, err := io.ReadFull(c.rc, f.Y); err != nil {
			return
		}
		if _, err := io.ReadFull(c.rc, f.U); err != nil {
			return
		}
		if _, err := io.ReadFull(c.rc, f.V); err != nil {
			return
		}
		select {
		case c.q <- f:
		case <-ctx.Done():
			return
		}
	}
}

// frame devuelve el cuadro que corresponde a now. ok = false si todavía no hay
// ninguno o si el video terminó (y ya se mostró su último cuadro).
func (c *clip) frame(now time.Time) (*Frame, bool) {
	want := int64(now.Sub(c.start).Seconds() * float64(c.fps))
	for c.idx < want {
		select {
		case f, open := <-c.q:
			if !open {
				return nil, false // terminó: vuelve el visualizador
			}
			if c.cur != nil {
				c.free <- c.cur
			}
			c.cur = f
			c.idx++
			continue
		default:
		}
		break // ffmpeg atrasado: se repite el cuadro actual
	}
	return c.cur, c.cur != nil
}

// close detiene ffmpeg.
func (c *clip) close() {
	c.cancel()
	_ = c.rc.Close()
}
