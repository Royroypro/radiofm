// Package studio integra la web con "Club25 Estudio", el programa de
// automatización de radio que corre en la PC de la emisora (estudio/).
//
// Hay una PC por frecuencia (93.3 Huacho, 103.2 Barranca…): cada una tiene su
// token, su estado y lo que suena; recibe las cuñas de su frecuencia más las
// de «todas»; y sus emisiones quedan marcadas con su frecuencia.
//
// Qué hace:
//   - Token de acceso de la PC: se genera desde el panel, se muestra una sola
//     vez y solo se guarda su SHA-256. La API /api/studio/* lo exige como
//     "Authorization: Bearer <token>".
//   - Cuñas FM: alta/edición/baja desde el panel (con auditoría) y la lista
//     de cuñas vigentes que descarga la PC.
//   - Estado de la PC: heartbeat (versión, modo, codificador) y "lo que suena".
//   - Emisiones (as-run): la PC reporta cada cuña emitida con un play_uuid
//     propio; reenviar el mismo lote no duplica. Con eso se arma el
//     comprobante para el anunciante.
//
// Qué no hace: no programa nada (la PC decide qué y cuándo suena) ni
// habla con Icecast. El bloqueo total de licencia se aplica en el router
// (PublicLicenseGuard), no aquí.
package studio

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"radioclub25/internal/domain"
	"radioclub25/internal/service/audit"
)

// TokenPrefix identifica los tokens del estudio (ayuda a reconocerlos si se filtran).
const TokenPrefix = "c25s_"

// MaxAirplayBatch es el máximo de emisiones por petición.
const MaxAirplayBatch = 500

// Files es lo que el servicio necesita del almacenamiento de archivos (platform/storage).
type Files interface {
	Delete(publicURL string) error
	SHA256(publicURL string) (string, error)
}

// Service implementa los casos de uso del estudio.
type Service struct {
	spots domain.FMSpotRepository
	plays domain.AirplayRepository
	state domain.StudioRepository
	files Files
	audit *audit.Service
	loc   *time.Location
	now   func() time.Time
}

// New crea el servicio. loc es la zona horaria de la radio (franjas y "hoy").
func New(spots domain.FMSpotRepository, plays domain.AirplayRepository, state domain.StudioRepository,
	files Files, au *audit.Service, loc *time.Location) *Service {
	return &Service{spots: spots, plays: plays, state: state, files: files, audit: au, loc: loc, now: time.Now}
}

// ---------- Token y estado de la PC ----------

// State devuelve el estado conocido de la PC de una frecuencia.
func (s *Service) State(ctx context.Context, fid int64) (*domain.StudioState, error) {
	return s.state.Get(ctx, fid)
}

// States devuelve el estado de las PC de todas las frecuencias que tienen una.
func (s *Service) States(ctx context.Context) (map[int64]*domain.StudioState, error) {
	return s.state.List(ctx)
}

// GenerateToken crea un token nuevo (invalida el anterior) y lo devuelve en
// claro. Es la única vez que se puede ver: solo se guarda su hash.
func (s *Service) GenerateToken(ctx context.Context, actor domain.Actor, fid int64) (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	token := TokenPrefix + hex.EncodeToString(b)
	if err := s.state.SetToken(ctx, fid, hashToken(token), s.now()); err != nil {
		return "", err
	}
	s.audit.Log(ctx, actor, "generate_token", "studio", fid, "token del estudio generado")
	return token, nil
}

// RevokeToken borra el token: la PC deja de poder sincronizar.
func (s *Service) RevokeToken(ctx context.Context, actor domain.Actor, fid int64) error {
	if err := s.state.SetToken(ctx, fid, "", s.now()); err != nil {
		return err
	}
	s.audit.Log(ctx, actor, "revoke_token", "studio", fid, "token del estudio revocado")
	return nil
}

// Authenticate comprueba el token que envía la PC y devuelve su frecuencia.
// Devuelve domain.ErrUnauthorized si no vale. Se busca por el SHA-256 del
// token (32 bytes aleatorios): medir tiempos de esa búsqueda no ayuda a
// adivinar un token.
func (s *Service) Authenticate(ctx context.Context, token string) (int64, error) {
	if !strings.HasPrefix(token, TokenPrefix) || len(token) != len(TokenPrefix)+64 {
		return 0, domain.ErrUnauthorized
	}
	fid, err := s.state.FindToken(ctx, hashToken(token))
	if errors.Is(err, domain.ErrNotFound) {
		return 0, domain.ErrUnauthorized
	}
	return fid, err
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// HeartbeatInput es lo que envía la PC periódicamente.
type HeartbeatInput struct {
	Version string            `json:"version"`
	Mode    domain.StudioMode `json:"mode"`
	Encoder json.RawMessage   `json:"encoder"` // estado libre del codificador (conectado, kbps, reintentos...)
}

// Heartbeat registra la señal de vida de la PC.
func (s *Service) Heartbeat(ctx context.Context, fid int64, in HeartbeatInput) error {
	in.Version = strings.TrimSpace(in.Version)
	if in.Version == "" || len(in.Version) > 40 {
		return domain.Invalid("version", "Versión inválida")
	}
	if !in.Mode.Valid() {
		return domain.Invalid("mode", "Modo inválido")
	}
	enc := "{}"
	if len(in.Encoder) > 0 {
		if len(in.Encoder) > 2048 || !json.Valid(in.Encoder) {
			return domain.Invalid("encoder", "Estado del codificador inválido")
		}
		enc = string(in.Encoder)
	}
	return s.state.Heartbeat(ctx, fid, s.now(), in.Version, in.Mode, enc)
}

// SetNowPlaying guarda lo que suena en la PC.
func (s *Service) SetNowPlaying(ctx context.Context, fid int64, np domain.NowPlaying) error {
	np.Artist, np.Title, np.Kind = strings.TrimSpace(np.Artist), strings.TrimSpace(np.Title), strings.TrimSpace(np.Kind)
	if np.Artist == "" && np.Title == "" {
		return domain.Invalid("title", "Falta el título")
	}
	if utf8.RuneCountInString(np.Artist) > 200 || utf8.RuneCountInString(np.Title) > 200 || len(np.Kind) > 20 {
		return domain.Invalid("title", "Texto demasiado largo")
	}
	return s.state.SetNowPlaying(ctx, fid, np, s.now())
}

// CurrentNowPlaying devuelve lo que suena si la PC lo informó hace menos de
// maxAge; si no, nil (la web usará el título que tenga Icecast).
func (s *Service) CurrentNowPlaying(ctx context.Context, fid int64, maxAge time.Duration) *domain.NowPlaying {
	st, err := s.state.Get(ctx, fid)
	if err != nil || st.NowPlaying == nil || st.NowPlayingAt == nil || s.now().Sub(*st.NowPlayingAt) > maxAge {
		return nil
	}
	return st.NowPlaying
}

// ---------- Cuñas FM ----------

// ListSpots devuelve todas las cuñas.
func (s *Service) ListSpots(ctx context.Context) ([]domain.FMSpot, error) {
	return s.spots.List(ctx, false)
}

// GetSpot devuelve una cuña.
func (s *Service) GetSpot(ctx context.Context, id int64) (*domain.FMSpot, error) {
	return s.spots.Get(ctx, id)
}

// CreateSpot valida y crea una cuña; calcula el sha256 del archivo.
func (s *Service) CreateSpot(ctx context.Context, actor domain.Actor, sp *domain.FMSpot) error {
	if err := validateSpot(sp); err != nil {
		return err
	}
	if err := s.hashFile(sp); err != nil {
		return err
	}
	if err := s.spots.Create(ctx, sp); err != nil {
		return err
	}
	s.audit.Log(ctx, actor, "create", "fm_spot", sp.ID, "%s — %s", sp.Advertiser, sp.Name)
	return nil
}

// UpdateSpot valida y modifica una cuña. Si se reemplazó el archivo, recalcula
// el hash y borra el archivo anterior.
func (s *Service) UpdateSpot(ctx context.Context, actor domain.Actor, sp *domain.FMSpot) error {
	if err := validateSpot(sp); err != nil {
		return err
	}
	old, err := s.spots.Get(ctx, sp.ID)
	if err != nil {
		return err
	}
	if old.FileURL != sp.FileURL || sp.SHA256 == "" {
		if err := s.hashFile(sp); err != nil {
			return err
		}
	}
	if err := s.spots.Update(ctx, sp); err != nil {
		return err
	}
	if old.FileURL != sp.FileURL {
		_ = s.files.Delete(old.FileURL)
	}
	s.audit.Log(ctx, actor, "update", "fm_spot", sp.ID, "%s — %s (activa=%t)", sp.Advertiser, sp.Name, sp.Active)
	return nil
}

// DeleteSpot borra una cuña y su archivo. Las emisiones ya registradas se conservan.
func (s *Service) DeleteSpot(ctx context.Context, actor domain.Actor, id int64) error {
	sp, err := s.spots.Get(ctx, id)
	if err != nil {
		return err
	}
	if err := s.spots.Delete(ctx, id); err != nil {
		return err
	}
	_ = s.files.Delete(sp.FileURL)
	s.audit.Log(ctx, actor, "delete", "fm_spot", id, "%s — %s", sp.Advertiser, sp.Name)
	return nil
}

func (s *Service) hashFile(sp *domain.FMSpot) error {
	h, err := s.files.SHA256(sp.FileURL)
	if err != nil {
		return domain.Invalid("file", "No se pudo leer el archivo de la cuña")
	}
	sp.SHA256 = h
	return nil
}

// SpotsForStudio devuelve las cuñas que la PC de la frecuencia fid debe
// tener: las de su frecuencia y las de todas, activas y no vencidas (incluye
// las que empiezan en el futuro, para que la PC pueda programar con antelación).
func (s *Service) SpotsForStudio(ctx context.Context, fid int64) ([]domain.FMSpot, error) {
	list, err := s.spots.List(ctx, true)
	if err != nil {
		return nil, err
	}
	now := s.now()
	out := list[:0]
	for _, sp := range list {
		if !sp.Expired(now) && sp.AppliesTo(fid) {
			out = append(out, sp)
		}
	}
	return out, nil
}

// TodayCounts devuelve cuántas veces sonó cada cuña hoy (hora de la radio).
func (s *Service) TodayCounts(ctx context.Context) (map[int64]int, error) {
	now := s.now().In(s.loc)
	return s.plays.CountSince(ctx, time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, s.loc))
}

func validateSpot(sp *domain.FMSpot) error {
	sp.Advertiser, sp.Name = strings.TrimSpace(sp.Advertiser), strings.TrimSpace(sp.Name)
	switch {
	case sp.Advertiser == "":
		return domain.Invalid("advertiser", "El anunciante es obligatorio")
	case utf8.RuneCountInString(sp.Advertiser) > 120:
		return domain.Invalid("advertiser", "Máximo 120 caracteres")
	case sp.Name == "":
		return domain.Invalid("name", "El nombre de la cuña es obligatorio")
	case utf8.RuneCountInString(sp.Name) > 120:
		return domain.Invalid("name", "Máximo 120 caracteres")
	case sp.FileURL == "":
		return domain.Invalid("file", "Suba el audio de la cuña")
	case sp.PlaysPerDay < 1 || sp.PlaysPerDay > 96:
		return domain.Invalid("plays_per_day", "Debe estar entre 1 y 96 veces por día")
	case sp.HourFrom < 0 || sp.HourFrom > 23 || sp.HourTo < 1 || sp.HourTo > 24 || sp.HourFrom >= sp.HourTo:
		return domain.Invalid("hour_to", "La franja debe ir de 00 a 24 h y terminar después de empezar")
	case sp.DurationMs < 0 || sp.DurationMs > 10*60*1000:
		return domain.Invalid("duration", "La duración debe estar entre 0 y 600 segundos")
	case sp.StartsAt != nil && sp.EndsAt != nil && !sp.EndsAt.After(*sp.StartsAt):
		return domain.Invalid("ends_at", "La fecha de fin debe ser posterior al inicio")
	}
	return nil
}

// ---------- Emisiones (as-run) ----------

// AirplayInput es una emisión tal como la envía la PC.
type AirplayInput struct {
	SpotID     int64     `json:"spot_id"`
	AiredAt    time.Time `json:"aired_at"`
	DurationMs int64     `json:"duration_ms"`
	Complete   bool      `json:"complete"`
	PlayUUID   string    `json:"play_uuid"`
}

// AirplayResult resume lo que pasó con un lote.
type AirplayResult struct {
	Received int      `json:"received"`
	Inserted int      `json:"inserted"` // nuevas
	Ignored  int      `json:"ignored"`  // ya registradas o de cuñas que no existen
	Rejected []string `json:"rejected"` // play_uuid con datos inválidos: la PC no debe reenviarlas
}

// RecordAirplays registra un lote de emisiones de forma idempotente. Las
// emisiones con datos inválidos se rechazan una a una (el resto se guarda)
// para que un dato malo no bloquee la cola de la PC para siempre.
func (s *Service) RecordAirplays(ctx context.Context, fid int64, in []AirplayInput) (AirplayResult, error) {
	res := AirplayResult{Received: len(in), Rejected: []string{}}
	if len(in) == 0 {
		return res, nil
	}
	if len(in) > MaxAirplayBatch {
		return res, domain.Invalid("airplays", "Lote demasiado grande (máximo 500)")
	}
	now := s.now()
	valid := make([]domain.Airplay, 0, len(in))
	for _, p := range in {
		if !validUUID(p.PlayUUID) {
			res.Rejected = append(res.Rejected, p.PlayUUID)
			continue
		}
		if p.SpotID <= 0 || p.AiredAt.IsZero() || p.AiredAt.After(now.Add(10*time.Minute)) ||
			p.AiredAt.Before(now.AddDate(-1, 0, 0)) || p.DurationMs < 0 || p.DurationMs > 3_600_000 {
			res.Rejected = append(res.Rejected, p.PlayUUID)
			continue
		}
		valid = append(valid, domain.Airplay{SpotID: p.SpotID, AiredAt: p.AiredAt, DurationMs: p.DurationMs,
			Complete: p.Complete, PlayUUID: p.PlayUUID, FrequencyID: fid})
	}
	n, err := s.plays.Record(ctx, valid)
	if err != nil {
		return res, err
	}
	res.Inserted, res.Ignored = n, len(valid)-n
	return res, nil
}

// validUUID acepta identificadores de 8 a 64 caracteres [A-Za-z0-9-].
func validUUID(u string) bool {
	if len(u) < 8 || len(u) > 64 {
		return false
	}
	for _, c := range u {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-') {
			return false
		}
	}
	return true
}

// Report devuelve las emisiones del filtro (el comprobante).
func (s *Service) Report(ctx context.Context, f domain.AirplayFilter) ([]domain.Airplay, error) {
	return s.plays.List(ctx, f)
}
