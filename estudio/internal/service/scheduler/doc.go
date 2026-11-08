// Package scheduler genera la playlist del día (log) a partir de la parrilla
// semanal y los relojes: resuelve cada slot con rotation, añade relleno o
// marca pistas recortables para cuadrar la hora (back-timing) y respeta los
// elementos bloqueados al regenerar.
//
// Fase 4 del plan. Ejemplo paso a paso en docs/scheduling.md (fase 4).
package scheduler
