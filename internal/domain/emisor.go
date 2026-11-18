package domain

import (
	"context"
	"encoding/json"
	"time"
)

// Club25 Emisor (carpeta emisor/ del repositorio): el emisor en vivo de la PC.
// El servidor guarda el contacto del proveedor del servicio (para el aviso de
// licencia del programa), las versiones publicadas del programa (actualización
// automática) y una copia de la configuración de cada PC.

// ProviderInfo es el contacto del proveedor del servicio de radio. El emisor lo
// muestra cuando no está autenticado ("contáctese para adquirir una licencia").
// Se guarda en la tabla settings con las claves ProviderPhoneKey y ProviderURLKey
// (no forma parte de SiteSettings: solo lo edita el superadmin).
type ProviderInfo struct {
	Phone string `json:"phone"`
	URL   string `json:"url"`
}

// Claves de ProviderInfo en la tabla settings.
const (
	ProviderPhoneKey = "provider_phone"
	ProviderURLKey   = "provider_url"
)

// DefaultProviderInfo es el contacto de fábrica.
func DefaultProviderInfo() ProviderInfo {
	return ProviderInfo{Phone: "9488933154", URL: "https://sehuacho.com/radio"}
}

// EmisorRelease es una versión publicada del programa Club25 Emisor.
type EmisorRelease struct {
	ID        int64
	Version   string // "1.5.0"
	FileName  string // nombre del archivo guardado (club25-emisor.exe)
	SHA256    string // hex
	Size      int64
	Notes     string
	CreatedAt time.Time
	CreatedBy string
}

// EmisorReleaseRepository persiste las versiones del emisor.
type EmisorReleaseRepository interface {
	Create(ctx context.Context, r *EmisorRelease) error
	List(ctx context.Context) ([]EmisorRelease, error) // más nuevas primero
	Get(ctx context.Context, id int64) (*EmisorRelease, error)
	Delete(ctx context.Context, id int64) error
}

// EmisorOutput es el estado de una salida (radio o TV) de Club25 Emisor.
type EmisorOutput struct {
	State       string  `json:"state"` // detenido, conectando, en_vivo, reconectando
	BitrateKbps float64 `json:"bitrate_kbps"`
	Message     string  `json:"message"`
	Target      string  `json:"target"` // servidor:puerto/montaje o rtmp://…
}

// Live indica si la salida está al aire.
func (o EmisorOutput) Live() bool { return o.State == "en_vivo" }

// Label es el estado para el panel.
func (o EmisorOutput) Label() string {
	switch o.State {
	case "en_vivo":
		return "al aire"
	case "conectando":
		return "conectando"
	case "reconectando":
		return "reconectando"
	}
	return "detenida"
}

// EmisorReport es lo que Club25 Emisor envía en cada latido.
type EmisorReport struct {
	Radio      EmisorOutput `json:"radio"`
	TV         EmisorOutput `json:"tv"`
	NowPlaying string       `json:"now_playing"`
}

// EmisorOnline indica si Club25 Emisor dio señal en los últimos maxAge.
func (s StudioState) EmisorOnline(now time.Time, maxAge time.Duration) bool {
	return s.signalValid(s.EmisorLastSeen, now, maxAge)
}

// EmisorReport devuelve el último estado informado por Club25 Emisor.
func (s StudioState) EmisorReport() EmisorReport {
	var r EmisorReport
	if s.EmisorStatusJSON != "" {
		_ = json.Unmarshal([]byte(s.EmisorStatusJSON), &r)
	}
	return r
}

// LastSignal es la última señal de cualquiera de los dos programas (cero si ninguna).
func (s StudioState) LastSignal() time.Time {
	var t time.Time
	if s.LastSeen != nil {
		t = *s.LastSeen
	}
	if s.EmisorLastSeen != nil && s.EmisorLastSeen.After(t) {
		t = *s.EmisorLastSeen
	}
	return t
}

// SignalSources dice qué programas dan señal ahora: "Club25 Estudio", "Club25 Emisor" o ambos.
func (s StudioState) SignalSources(now time.Time, maxAge time.Duration) []string {
	var out []string
	if s.Online(now, maxAge) {
		out = append(out, "Club25 Estudio")
	}
	if s.EmisorOnline(now, maxAge) {
		out = append(out, "Club25 Emisor")
	}
	return out
}

// emisorAlarms avisa cuando una salida de Club25 Emisor no llega al servidor.
func (s StudioState) emisorAlarms() []Alarm {
	r := s.EmisorReport()
	var out []Alarm
	for _, o := range []struct {
		name string
		out  EmisorOutput
	}{{"la radio", r.Radio}, {"la TV", r.TV}} {
		if o.out.State == "reconectando" {
			out = append(out, Alarm{AlarmWarning, "Club25 Emisor: " + o.name + " no llega al servidor" + suffix(o.out.Message)})
		}
	}
	return out
}
