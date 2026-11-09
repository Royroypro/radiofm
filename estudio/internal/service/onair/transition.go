package onair

import (
	"time"

	"radioclub25/estudio/internal/domain"
)

// Transition es cómo se pasa de lo que suena a lo siguiente en un punto de mezcla.
type Transition struct {
	// OutFade es el fundido de lo que suena. Negativo = no fundirlo: se deja
	// terminar solo (un final seco o la cola de un jingle no se cortan).
	OutFade time.Duration
	InFade  time.Duration // fundido de entrada de lo nuevo (0 = entra a tope)
	Delay   time.Duration // esperar esto antes de arrancar lo nuevo (sin solape)
}

// TransitionRules son los ajustes de las transiciones (Ajustes → Transiciones).
type TransitionRules struct {
	Crossfade time.Duration // fundido normal entre canciones (3 s)
	BreakFade time.Duration // fundido de la música al entrar una tanda (1 s): la tanda entra al terminar
}

// planTransition decide la transición automática de prev (lo que suena) a next.
// Función pura. Reglas (ideas de smart_crossfade de Liquidsoap y del AutoDJ de Mixxx):
//
//	música → tanda          la música se funde en BreakFade y la tanda entra al terminar (sin pisarse)
//	música → jingle/voz     el jingle entra encima del final; la música se funde mientras
//	música → música         final con fundido: se funde a lo largo de su propio fundido (FadeOut);
//	                        final seco: no se funde, se deja terminar el golpe final
//	jingle/voz → música     la cola del jingle termina sola, la música entra debajo
//	otros                   crossfade normal
func planTransition(prev *Segment, next Segment, r TransitionRules) Transition {
	if prev == nil {
		return Transition{}
	}
	switch {
	case next.Break && !prev.Break:
		d := r.BreakFade
		if rest := prev.Length - prev.Segue; prev.Length > 0 && prev.Segue > 0 && rest < d {
			d = max(rest, 0) // si a la música le queda menos, la tanda entra cuando termina (sin hueco)
		}
		return Transition{OutFade: d, Delay: d}
	case prev.Break:
		return Transition{OutFade: r.Crossfade}
	}
	prevMusic, nextMusic := prev.Kind == domain.PlayMusic, next.Kind == domain.PlayMusic
	switch {
	case prevMusic && !nextMusic:
		out := r.Crossfade
		if next.Length > 0 && next.Length < out {
			out = max(next.Length, time.Second)
		}
		return Transition{OutFade: out}
	case prevMusic && nextMusic:
		if prev.Ending == domain.EndingCold {
			return Transition{OutFade: -1}
		}
		out := r.Crossfade
		if prev.FadeOut > 0 {
			out = min(max(prev.FadeOut, time.Second), 2*r.Crossfade)
		}
		return Transition{OutFade: out}
	case !prevMusic && nextMusic:
		return Transition{OutFade: -1}
	}
	return Transition{OutFade: r.Crossfade}
}

// PreviewTransition calcula, para la pantalla de la biblioteca, la transición
// automática de la pista a a la b y el punto de mezcla de a (relativo a su
// cue-in). Usa las mismas reglas que el aire.
func PreviewTransition(a, b domain.Track, kinds map[int64]domain.CategoryKind, r TransitionRules) (Transition, time.Duration) {
	x := &expander{crossfade: r.Crossfade, kinds: kinds}
	sa, sb := x.trackSegment(a), x.trackSegment(b)
	if r.BreakFade <= 0 {
		r.BreakFade = time.Second
	}
	return planTransition(&sa, sb, r), sa.Segue
}
