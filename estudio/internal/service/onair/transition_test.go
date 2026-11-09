package onair

import (
	"testing"
	"time"

	"radioclub25/estudio/internal/domain"
)

func TestPlanTransition(t *testing.T) {
	r := TransitionRules{Crossfade: 3 * time.Second, BreakFade: time.Second}
	music := func(end domain.Ending, fade time.Duration) *Segment {
		return &Segment{Kind: domain.PlayMusic, Ending: end, FadeOut: fade, Length: 200 * time.Second, Segue: 190 * time.Second}
	}
	jingle := Segment{Kind: domain.PlayJingle, Length: 2 * time.Second}
	cases := []struct {
		name string
		prev *Segment
		next Segment
		want Transition
	}{
		{"nada antes", nil, Segment{Kind: domain.PlayMusic}, Transition{}},
		{"música con fundido → música: se funde a lo largo de su fundido", music(domain.EndingFade, 5*time.Second),
			Segment{Kind: domain.PlayMusic}, Transition{OutFade: 5 * time.Second}},
		{"fundido muy largo: tope de 2× crossfade", music(domain.EndingFade, 12*time.Second),
			Segment{Kind: domain.PlayMusic}, Transition{OutFade: 6 * time.Second}},
		{"final seco → música: no se corta el golpe", music(domain.EndingCold, 0),
			Segment{Kind: domain.PlayMusic}, Transition{OutFade: -1}},
		{"música → jingle corto: fundido del largo del jingle", music(domain.EndingFade, 5*time.Second),
			jingle, Transition{OutFade: 2 * time.Second}},
		{"jingle → música: la cola del jingle termina sola", &jingle,
			Segment{Kind: domain.PlayMusic}, Transition{OutFade: -1}},
		{"música → tanda: se funde 1 s y la tanda entra al terminar", music(domain.EndingFade, 5*time.Second),
			Segment{Kind: domain.PlayJingle, Break: true}, Transition{OutFade: time.Second, Delay: time.Second}},
		{"a la música le quedan 400 ms: la tanda entra sin hueco",
			&Segment{Kind: domain.PlayMusic, Length: 10 * time.Second, Segue: 9600 * time.Millisecond},
			Segment{Kind: domain.PlaySpot, Break: true}, Transition{OutFade: 400 * time.Millisecond, Delay: 400 * time.Millisecond}},
	}
	for _, c := range cases {
		if got := planTransition(c.prev, c.next, r); got != c.want {
			t.Errorf("%s: %+v, quería %+v", c.name, got, c.want)
		}
	}
}
