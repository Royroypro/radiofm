package domain

// StudioMode es el modo de trabajo del aire. Los valores coinciden con los que
// recibe el servidor en POST /api/studio/heartbeat.
type StudioMode string

const (
	ModeAuto     StudioMode = "auto"     // automático: la playlist avanza sola
	ModeAssisted StudioMode = "assisted" // asistido: cada elemento espera al operador (Espacio)
	ModeLive     StudioMode = "live"     // en vivo: la música se pausa; micrófono y botonera
)

// Valid indica si el modo es conocido.
func (m StudioMode) Valid() bool { return m == ModeAuto || m == ModeAssisted || m == ModeLive }

// Label devuelve el nombre del modo para la interfaz.
func (m StudioMode) Label() string {
	switch m {
	case ModeAuto:
		return "Automático"
	case ModeAssisted:
		return "Asistido"
	case ModeLive:
		return "En vivo"
	}
	return string(m)
}
