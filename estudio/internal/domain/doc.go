// Package domain contiene las entidades y reglas puras de Club25 Estudio:
// Track (pista), Category (categoría), Clock y ClockSlot (reloj horario),
// GridCell (celda de la parrilla semanal), Event (evento duro o blando),
// LogItem (elemento de la playlist del día), Spot (cuña), Play (emisión
// registrada) y Cart (botón de la botonera), más las interfaces de
// repositorio que implementa platform/sqlite.
//
// Reglas: Go puro, sin dependencias internas, sin E/S. Todo lo de aquí se
// testea sin audio ni base de datos. Glosario de términos en docs/glossary.md.
package domain
