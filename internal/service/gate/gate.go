// Package gate decide quién puede publicar y quién puede ver/escuchar los streams.
//
//   - TV: MediaMTX está configurado con authMethod=http y pregunta a la app
//     (POST /internal/mediamtx/auth) antes de aceptar a OBS (publish) o a un
//     espectador (read). La app responde 200 (permitido) o 401 (denegado).
//   - Radio (fuente): Icecast usa autenticación por URL (stream_auth) y
//     pregunta a la app (POST /internal/icecast/source) antes de aceptar a la
//     PC emisora. La app responde con la cabecera "icecast-auth-user: 1".
//   - Radio (oyentes): nginx usa auth_request contra /internal/gate/radio
//     antes de dejar pasar a un oyente hacia Icecast.
//
// Reglas:
//   - Con bloqueo total de licencia todo se deniega.
//   - Publicar en TV exige el usuario/clave guardados en Ajustes → Streams.
//   - Ver la TV exige que la TV esté habilitada en ajustes y en el plan.
//   - Escuchar la radio exige que la radio esté habilitada en ajustes.
package gate

import (
	"context"
	"crypto/subtle"

	"radioclub25/internal/domain"
)

// LicenseReader da el estado de licencia actual.
type LicenseReader interface {
	Current() domain.LicenseState
}

// SettingsReader da los ajustes actuales.
type SettingsReader interface {
	Get(ctx context.Context) (domain.SiteSettings, error)
}

// Service toma las decisiones de acceso.
type Service struct {
	license    LicenseReader
	settings   SettingsReader
	tvPath     string
	sourcePass string // contraseña de fuente de Icecast (ICECAST_SOURCE_PASSWORD)
	mounts     MountChecker
}

// New crea el servicio. tvPath es el nombre del path de MediaMTX (p. ej. "tv");
// sourcePass es la contraseña que debe usar la PC emisora en Icecast.
func New(lic LicenseReader, set SettingsReader, tvPath, sourcePass string) *Service {
	return &Service{license: lic, settings: set, tvPath: tvPath, sourcePass: sourcePass}
}

// TVRequest es la petición de autorización que envía MediaMTX.
// Ver https://github.com/bluenviron/mediamtx#authentication (HTTP-based).
type TVRequest struct {
	User     string `json:"user"`
	Password string `json:"password"`
	IP       string `json:"ip"`
	Action   string `json:"action"`   // publish, read, playback, api, metrics, pprof
	Path     string `json:"path"`     // p. ej. "tv"
	Protocol string `json:"protocol"` // rtmp, srt, hls, webrtc, rtsp
	ID       string `json:"id"`
	Query    string `json:"query"`
}

// AuthorizeTV indica si MediaMTX debe aceptar la petición.
func (s *Service) AuthorizeTV(ctx context.Context, r TVRequest) bool {
	st := s.license.Current()
	if st.Level.PublicBlocked() || !st.Limits.TVEnabled {
		return false
	}
	set, err := s.settings.Get(ctx)
	if err != nil || !set.TVEnabled {
		return false
	}
	if r.Path != s.tvPath {
		return false
	}
	switch r.Action {
	case "read":
		return true
	case "publish":
		return equal(r.User, set.TVPublishUser) && equal(r.Password, set.TVPublishPass)
	}
	return false // api, metrics, pprof, playback: no se exponen
}

// RadioAllowed indica si nginx debe dejar pasar a un oyente de la radio.
func (s *Service) RadioAllowed(ctx context.Context) bool {
	if s.license.Current().Level.PublicBlocked() {
		return false
	}
	set, err := s.settings.Get(ctx)
	return err == nil && set.RadioEnabled
}

// MountChecker dice si un montaje de Icecast pertenece a una frecuencia
// activa con audio local (lo implementa service/frequencies).
type MountChecker interface {
	MountAllowed(ctx context.Context, mount string) bool
}

// SetMounts configura qué montajes acepta stream_auth. Sin configurar se
// acepta cualquiera (solo se comprueba la contraseña).
func (s *Service) SetMounts(m MountChecker) { s.mounts = m }

// RadioSourceAllowed indica si Icecast debe aceptar a una PC emisora en un
// montaje (stream_auth): radio habilitada, licencia sin bloqueo, contraseña
// correcta y montaje de una frecuencia activa.
func (s *Service) RadioSourceAllowed(ctx context.Context, password, mount string) bool {
	if !s.RadioAllowed(ctx) || !equal(password, s.sourcePass) {
		return false
	}
	return s.mounts == nil || s.mounts.MountAllowed(ctx, mount)
}

func equal(a, b string) bool {
	return b != "" && subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}
