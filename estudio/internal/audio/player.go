package audio

import (
	"context"
	"errors"
	"io"
	"math"
	"sync/atomic"
	"time"
)

// LoadOptions describe cómo reproducir una fuente en un reproductor.
type LoadOptions struct {
	Tag         uint64        // identificador del llamador; vuelve en los eventos
	GainDB      float64       // ganancia de la pista (normalización de loudness)
	Length      time.Duration // reproducir como máximo esto (cue-out − cue-in); 0 = hasta el final
	SegueAt     time.Duration // emitir EventSegue al llegar aquí (desde el inicio); 0 = no
	BufferAhead time.Duration // tamaño del ring buffer; 0 = 6 s
}

// playItem es una fuente cargada en un reproductor (deck o cart). Se crea
// fuera del hilo de audio; después, sus campos "de audio" solo los toca Render.
type playItem struct {
	ring   *RingBuffer
	done   atomic.Bool // la goroutine de llenado llegó al final de la fuente
	cancel context.CancelFunc
	gain   float32
	length int64 // frames máximos (0 = sin límite)
	segue  int64 // frame del aviso de mezcla (0 = no)
	tag    uint64

	// estado del hilo de audio
	pos                int64
	wait               int64 // frames que faltan para empezar (PlayAfter)
	vol, target, step  float32
	stopAtTarget       bool
	playing            bool
	segueSent, endSent bool
}

func newPlayItem(src Source, o LoadOptions) *playItem {
	ahead := o.BufferAhead
	if ahead <= 0 {
		ahead = 6 * time.Second
	}
	ctx, cancel := context.WithCancel(context.Background())
	it := &playItem{
		ring:   NewRingBuffer(int(DurationToFrames(ahead)) * Channels),
		cancel: cancel,
		gain:   DBToGain(o.GainDB),
		length: DurationToFrames(o.Length),
		segue:  DurationToFrames(o.SegueAt),
		tag:    o.Tag,
		vol:    1, target: 1,
	}
	go it.fill(ctx, src)
	return it
}

// fill lee la fuente y llena el ring buffer hasta el final o hasta que se cancele.
func (it *playItem) fill(ctx context.Context, src Source) {
	defer src.Close()
	defer it.done.Store(true)
	chunk := make([]float32, 4096)
	limit := it.length * Channels // muestras máximas
	var total int64
	for {
		if ctx.Err() != nil {
			return
		}
		if it.ring.Free() < len(chunk) {
			time.Sleep(5 * time.Millisecond)
			continue
		}
		want := chunk
		if limit > 0 {
			left := limit - total
			if left <= 0 {
				return
			}
			if int64(len(want)) > left {
				want = want[:left]
			}
		}
		n, err := src.Read(want)
		if n > 0 {
			it.ring.Write(want[:n]) // hay espacio: se comprobó Free arriba
			total += int64(n)
		}
		if err != nil {
			if !errors.Is(err, io.EOF) {
				// error de decodificación: se trata como fin de la pista
				return
			}
			return
		}
	}
}

// ready indica si ya hay audio suficiente en el buffer para empezar sin cortes.
func (it *playItem) ready(min int) bool { return it.done.Load() || it.ring.Len() >= min }

// setFade programa una rampa de volumen hasta target en frames.
func (it *playItem) setFade(target float32, frames int64, stop bool) {
	it.target, it.stopAtTarget = target, stop
	if frames <= 0 {
		it.vol, it.step = target, 0
		return
	}
	it.step = (target - it.vol) / float32(frames)
}

// render suma este reproductor en dst (muestras entrelazadas) usando tmp como
// buffer de trabajo. Devuelve los eventos ocurridos en el bloque.
func (it *playItem) render(dst, tmp []float32) (segue, ended bool, dropped int) {
	if !it.playing {
		return
	}
	if it.wait > 0 { // todavía esperando para empezar
		it.wait -= int64(len(dst) / Channels)
		return
	}
	frames := len(dst) / Channels
	if it.length > 0 && it.pos+int64(frames) > it.length {
		frames = int(max(it.length-it.pos, 0))
	}
	want := frames * Channels
	got := it.ring.Read(tmp[:want])
	for i := 0; i < got; i += 2 {
		v := fadeShape(it.vol) * it.gain
		dst[i] += tmp[i] * v
		dst[i+1] += tmp[i+1] * v
		if it.step != 0 {
			it.vol += it.step
			if (it.step < 0 && it.vol <= it.target) || (it.step > 0 && it.vol >= it.target) {
				it.vol, it.step = it.target, 0
			}
		}
	}
	it.pos += int64(got / Channels)
	if it.segue > 0 && !it.segueSent && it.pos >= it.segue {
		it.segueSent, segue = true, true
	}
	if it.stopAtTarget && it.step == 0 && it.vol == it.target {
		it.playing = false
	}
	finished := (it.length > 0 && it.pos >= it.length) || (got < want && it.done.Load() && it.ring.Len() == 0)
	if got < want && !finished {
		dropped = (want - got) / Channels // el decodificador no llegó a tiempo
	}
	if finished || !it.playing {
		it.playing = false
		if !it.endSent {
			it.endSent, ended = true, true
		}
	}
	return
}

// fadeShape convierte la posición lineal del fundido (0…1) en ganancia con curva
// de potencia constante (equal-power, como el crossfader de Mixxx): seno de un
// cuarto de ciclo. Al cruzar dos pistas, una sube con sin(x·π/2) y la otra baja con
// cos = sin((1−x)·π/2): la suma de potencias es constante y no hay "bache" de
// volumen en medio del crossfade. Fuera de un fundido (0 o 1) no cambia nada.
func fadeShape(x float32) float32 {
	if x <= 0 || x >= 1 {
		return x
	}
	return float32(math.Sin(float64(x) * math.Pi / 2))
}
