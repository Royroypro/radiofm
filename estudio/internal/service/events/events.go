// Package events decide qué hace el aire a continuación según la hora y la
// playlist: arrancar el siguiente elemento (evento blando), cortar para un
// evento duro, o saltar elementos atrasados u opcionales. Es lógica pura: no
// toca audio ni base de datos; la usa service/onair en cada aviso del motor
// (punto de mezcla, fin) y en un temporizador.
package events

import (
	"time"

	"radioclub25/estudio/internal/domain"
)

// Action es lo que hay que hacer.
type Action int

const (
	None    Action = iota // seguir como estamos
	Start                 // arrancar Item (el anterior terminó o llegó a su punto de mezcla)
	HardCut               // fundir lo que suena y arrancar Item (evento duro en hora)
)

// Rules son los márgenes de decisión.
type Rules struct {
	HardLead      time.Duration // un evento duro se dispara si faltan menos de esto (absorbe la latencia del tick)
	HardMissAfter time.Duration // un evento duro que pasó hace más de esto se salta (no se locuta una hora vieja)
	StaleAfter    time.Duration // un elemento blando que debía terminar hace más de esto se salta (recuperación tras un corte)
	OptionalLate  time.Duration // un opcional se salta si se va más tarde que esto
}

// DefaultRules son los valores del plan.
func DefaultRules() Rules {
	return Rules{HardLead: 150 * time.Millisecond, HardMissAfter: 3 * time.Minute, StaleAfter: 2 * time.Minute,
		OptionalLate: 30 * time.Second}
}

// Decision es el resultado de Next.
type Decision struct {
	Action Action
	Item   domain.LogItem
	Skip   []int64 // elementos a marcar como saltados (en orden)
}

// Next decide qué hacer.
//
//   - now: hora actual.
//   - wantNext: true si el aire está libre o lo que suena llegó a su punto de mezcla
//     (hay que arrancar algo); false en un tick normal (solo se miran los eventos duros).
//   - upcoming: elementos pendientes en orden (el primero es el siguiente según la playlist).
//
// Reglas:
//  1. El primer evento duro pendiente cuya hora llegó (now ≥ At − HardLead) corta lo que
//     suena (HardCut). Los elementos blandos anteriores a él se saltan.
//     Si pasó hace más de HardMissAfter, se salta y se sigue buscando.
//  2. Si hay que arrancar algo: el primer elemento blando pendiente que no esté
//     atrasado (At+Length < now − StaleAfter) ni sea un opcional tardío.
//     Un evento duro futuro no se adelanta: si lo siguiente es duro y todavía no es la
//     hora, se busca el siguiente blando que cabe antes (si no, None: el aire espera).
func Next(now time.Time, wantNext bool, upcoming []domain.LogItem, r Rules) Decision {
	var d Decision
	// 1. eventos duros vencidos
	for i, it := range upcoming {
		if !it.Hard {
			continue
		}
		if now.Before(it.At.Add(-r.HardLead)) {
			break // el primer duro todavía no llega; los siguientes tampoco
		}
		if now.Sub(it.At) > r.HardMissAfter {
			continue // muy viejo: se salta más abajo
		}
		for _, prev := range upcoming[:i] {
			d.Skip = append(d.Skip, prev.ID)
		}
		d.Action, d.Item = HardCut, it
		return d
	}
	if !wantNext {
		return d
	}
	// 2. siguiente blando
	for _, it := range upcoming {
		if it.Hard {
			if now.Sub(it.At) > r.HardMissAfter {
				d.Skip = append(d.Skip, it.ID) // evento duro perdido
				continue
			}
			// un duro futuro: solo se puede arrancar algo blando que esté antes en la lista
			return d
		}
		end := it.At.Add(it.Length)
		if now.Sub(end) > r.StaleAfter {
			d.Skip = append(d.Skip, it.ID) // atrasado (p. ej. tras reiniciar la PC)
			continue
		}
		if it.Optional && now.Sub(it.At) > r.OptionalLate {
			d.Skip = append(d.Skip, it.ID)
			continue
		}
		d.Action, d.Item = Start, it
		return d
	}
	return d
}
