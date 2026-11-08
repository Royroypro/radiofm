package audio

// DeviceKind indica si un dispositivo reproduce o captura audio.
type DeviceKind string

const (
	Playback DeviceKind = "playback" // salida: consola/transmisor, pre-escucha
	Capture  DeviceKind = "capture"  // entrada: micrófono o línea (salida de la consola)
)

// Device es una tarjeta de sonido visible para miniaudio.
type Device struct {
	ID        string     `json:"id"` // identificador de WASAPI, se guardará en la configuración
	Name      string     `json:"name"`
	Kind      DeviceKind `json:"kind"`
	IsDefault bool       `json:"is_default"`
}
