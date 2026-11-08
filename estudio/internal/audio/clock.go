package audio

import (
	"context"
	"time"
)

// RunClock hace avanzar el motor con un reloj de software cuando no hay
// tarjeta de salida (pruebas, servidor sin audio, emisión solo a Icecast).
// Cada tick renderiza los frames que tocan según el tiempo real transcurrido,
// así no acumula deriva. Bloquea hasta que ctx se cancele.
func RunClock(ctx context.Context, e *Engine) {
	buf := make([]float32, maxBlockFrames*Channels)
	start := time.Now()
	var done int64
	t := time.NewTicker(10 * time.Millisecond)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-t.C:
			due := DurationToFrames(now.Sub(start)) - done
			for due > 0 {
				n := min(due, maxBlockFrames)
				e.Render(buf[:n*Channels])
				done += n
				due -= n
			}
		}
	}
}

// RenderOffline produce d de audio sin tiempo real (tests y estudio-cli render).
// Llama a each después de cada bloque, si no es nil (p. ej. para atender eventos).
func RenderOffline(e *Engine, d time.Duration, each func(block []float32)) {
	frames := DurationToFrames(d)
	buf := make([]float32, BlockFrames*Channels)
	for frames > 0 {
		n := min(frames, BlockFrames)
		e.Render(buf[:n*Channels])
		if each != nil {
			each(buf[:n*Channels])
		}
		frames -= n
	}
}

// RenderOfflineWaiting es como RenderOffline pero, antes de cada bloque, espera
// (hasta 2 s) a que los reproductores que suenan tengan audio decodificado.
// Sirve para renderizar a archivo pistas que decodifica ffmpeg sin meter
// silencios falsos. Debe llamarse desde la misma goroutine que usa el motor.
func RenderOfflineWaiting(e *Engine, d time.Duration, each func(block []float32)) {
	frames := DurationToFrames(d)
	buf := make([]float32, BlockFrames*Channels)
	need := BlockFrames * Channels
	for frames > 0 {
		e.renderBlock(buf[:0]) // aplica comandos pendientes
		deadline := time.Now().Add(2 * time.Second)
		for _, it := range e.slots {
			for it != nil && it.playing && !it.ready(need) && time.Now().Before(deadline) {
				time.Sleep(time.Millisecond)
			}
		}
		n := min(frames, BlockFrames)
		e.Render(buf[:n*Channels])
		if each != nil {
			each(buf[:n*Channels])
		}
		frames -= n
	}
}
