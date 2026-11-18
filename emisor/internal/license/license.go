// Package license autentica al emisor contra la web de la radio y habla con
// sus endpoints (docs/api.md → "Club25 Emisor"):
//
//	GET /api/emisor/provider        contacto del proveedor (público)
//	GET /api/studio/emisor          autenticación + última versión (token Bearer)
//	GET /api/studio/emisor/download el .exe de la última versión
//	PUT/GET /api/studio/emisor/config copia de la configuración (sin claves)
//
// El programa solo funciona autenticado. Para que un corte de internet no deje a
// la radio sin poder emitir, una autenticación correcta vale OfflineGrace sin
// conexión (Decide); un token rechazado o un servicio suspendido la anulan al instante.
package license

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// OfflineGrace es cuánto vale la última autenticación correcta sin conexión con la web.
const OfflineGrace = 72 * time.Hour

// Errores de Check.
var (
	ErrNoCredentials = errors.New("Escriba el dominio de la web y el token del estudio.")
	ErrUnauthorized  = errors.New("El token no es válido para esa web. Genere uno en Admin → Estudio (PC).")
	ErrSuspended     = errors.New("El servicio de la radio está suspendido. Contáctese con el proveedor.")
	ErrNotEmisor     = errors.New("Esa web no tiene el servicio Club25 Emisor (¿dominio equivocado o versión antigua del servidor?).")
)

// OfflineError: no se pudo hablar con la web (sin internet, DNS, servidor caído).
type OfflineError struct{ Err error }

func (e *OfflineError) Error() string { return "No se pudo conectar con la web: " + e.Err.Error() }
func (e *OfflineError) Unwrap() error { return e.Err }

// Provider es el contacto del proveedor del servicio.
type Provider struct {
	Phone string `json:"phone"`
	URL   string `json:"url"`
}

// DefaultProvider es el contacto de fábrica (el mismo que el servidor trae por defecto).
var DefaultProvider = Provider{Phone: "9488933154", URL: "https://sehuacho.com/radio"}

// Release es una versión publicada del programa.
type Release struct {
	Version     string `json:"version"`
	SHA256      string `json:"sha256"`
	Size        int64  `json:"size"`
	Notes       string `json:"notes"`
	PublishedAt string `json:"published_at"`
}

// Station es la radio y frecuencia del token.
type Station struct {
	Name        string `json:"name"`
	Frequency   string `json:"frequency"`
	FrequencyID int64  `json:"frequency_id"`
	City        string `json:"city"`
}

// Info es la respuesta de una autenticación correcta.
type Info struct {
	Station  Station  `json:"station"`
	Provider Provider `json:"provider"`
	Latest   *Release `json:"latest"`
}

// client tiene tiempo límite: nunca cuelga la ventana.
var client = &http.Client{Timeout: 20 * time.Second}

// NormalizeBase acepta «radio.pe», «https://radio.pe/» o «https://radio.pe/admin» y devuelve «https://radio.pe».
func NormalizeBase(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", ErrNoCredentials
	}
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") {
		return "", errors.New("El dominio no es válido. Escriba, por ejemplo, radioclub25peru.com")
	}
	return u.Scheme + "://" + u.Host, nil
}

func newRequest(ctx context.Context, method, base, path, token string, body io.Reader) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, base+path, body)
	if err != nil {
		return nil, err
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	req.Header.Set("User-Agent", "Club25Emisor")
	return req, nil
}

func do(req *http.Request) (*http.Response, error) {
	resp, err := client.Do(req)
	if err != nil {
		return nil, &OfflineError{Err: err}
	}
	switch resp.StatusCode {
	case http.StatusUnauthorized, http.StatusForbidden:
		resp.Body.Close()
		return nil, ErrUnauthorized
	case http.StatusServiceUnavailable:
		var e struct {
			Error string `json:"error"`
		}
		_ = json.NewDecoder(io.LimitReader(resp.Body, 4096)).Decode(&e)
		resp.Body.Close()
		if e.Error == "suspended" {
			return nil, ErrSuspended
		}
		return nil, &OfflineError{Err: fmt.Errorf("la web respondió %d", resp.StatusCode)}
	}
	return resp, nil
}

// Check autentica: GET /api/studio/emisor con el token.
func Check(ctx context.Context, base, token string) (Info, error) {
	if strings.TrimSpace(token) == "" {
		return Info{}, ErrNoCredentials
	}
	base, err := NormalizeBase(base)
	if err != nil {
		return Info{}, err
	}
	req, err := newRequest(ctx, http.MethodGet, base, "/api/studio/emisor", strings.TrimSpace(token), nil)
	if err != nil {
		return Info{}, err
	}
	resp, err := do(req)
	if err != nil {
		return Info{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return Info{}, ErrNotEmisor
	}
	if resp.StatusCode != http.StatusOK {
		return Info{}, &OfflineError{Err: fmt.Errorf("la web respondió %d", resp.StatusCode)}
	}
	var out struct {
		Info
		OK bool `json:"ok"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&out); err != nil || !out.OK {
		return Info{}, ErrNotEmisor
	}
	if out.Provider.Phone == "" {
		out.Provider = DefaultProvider
	}
	return out.Info, nil
}

// FetchProvider lee el contacto del proveedor sin token (para el aviso de licencia).
func FetchProvider(ctx context.Context, base string) (Provider, error) {
	base, err := NormalizeBase(base)
	if err != nil {
		return DefaultProvider, err
	}
	req, err := newRequest(ctx, http.MethodGet, base, "/api/emisor/provider", "", nil)
	if err != nil {
		return DefaultProvider, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return DefaultProvider, err
	}
	defer resp.Body.Close()
	var p Provider
	if resp.StatusCode != http.StatusOK || json.NewDecoder(io.LimitReader(resp.Body, 4096)).Decode(&p) != nil || p.Phone == "" {
		return DefaultProvider, fmt.Errorf("respuesta %d", resp.StatusCode)
	}
	return p, nil
}

// ---------- estado guardado ----------

// Cache es la última autenticación correcta (se guarda cifrada en disco).
type Cache struct {
	Base      string    `json:"base"`
	TokenHash string    `json:"token_hash"`
	OKAt      time.Time `json:"ok_at"`
	Info      Info      `json:"info"`
}

// TokenHash identifica el token sin guardarlo.
func TokenHash(token string) string {
	h := sha256.Sum256([]byte(strings.TrimSpace(token)))
	return hex.EncodeToString(h[:])
}

// State es lo que la ventana muestra del estado de la licencia.
type State struct {
	Authenticated bool      `json:"authenticated"`
	Offline       bool      `json:"offline"` // válida por la última autenticación, sin conexión ahora
	Reason        string    `json:"reason"`  // por qué no está autenticado (o aviso)
	Base          string    `json:"base"`
	Station       Station   `json:"station"`
	Provider      Provider  `json:"provider"`
	Latest        *Release  `json:"latest"`
	CheckedAt     time.Time `json:"checked_at"`
	ValidUntil    time.Time `json:"valid_until"` // fin de la gracia sin conexión
}

// Decide combina el resultado de Check (info/err) con la última autenticación
// guardada (cache, puede ser nil) para el dominio y token actuales.
// Devuelve el estado y la caché a guardar (nil = borrarla).
func Decide(base, token string, info Info, checkErr error, cache *Cache, now time.Time) (State, *Cache) {
	nb, _ := NormalizeBase(base)
	st := State{Base: nb, Provider: DefaultProvider, CheckedAt: now}
	if checkErr == nil {
		c := &Cache{Base: nb, TokenHash: TokenHash(token), OKAt: now, Info: info}
		st.Authenticated, st.Station, st.Provider, st.Latest = true, info.Station, info.Provider, info.Latest
		st.ValidUntil = now.Add(OfflineGrace)
		return st, c
	}
	st.Reason = checkErr.Error()
	var off *OfflineError
	if errors.As(checkErr, &off) && cache != nil && cache.Base == nb && cache.TokenHash == TokenHash(token) {
		st.Provider, st.Station = cache.Info.Provider, cache.Info.Station
		until := cache.OKAt.Add(OfflineGrace)
		st.ValidUntil = until
		if now.Before(until) {
			st.Authenticated, st.Offline = true, true
			st.Latest = cache.Info.Latest
			st.Reason = "Sin conexión con la web: se usa la última autenticación (válida hasta " + until.Local().Format("02/01 15:04") + ")."
			return st, cache
		}
		st.Reason = "Sin conexión con la web desde hace más de 72 horas: conéctese a internet para volver a autenticar."
		return st, cache
	}
	if errors.As(checkErr, &off) {
		return st, cache // sin conexión y sin autenticación previa válida: se conserva la caché tal cual
	}
	if cache != nil && cache.Info.Provider.Phone != "" {
		st.Provider = cache.Info.Provider
	}
	return st, nil // token rechazado, suspendido o sin datos: se borra la autenticación guardada
}

// ---------- copia de la configuración ----------

// UploadConfig guarda la configuración (ya sin claves) en el servidor.
func UploadConfig(ctx context.Context, base, token string, cfgJSON []byte) error {
	base, err := NormalizeBase(base)
	if err != nil {
		return err
	}
	req, err := newRequest(ctx, http.MethodPut, base, "/api/studio/emisor/config", token, strings.NewReader(string(cfgJSON)))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		return fmt.Errorf("la web respondió %d al guardar la copia", resp.StatusCode)
	}
	return nil
}

// DownloadConfig trae la última copia guardada y su fecha.
func DownloadConfig(ctx context.Context, base, token string) ([]byte, time.Time, error) {
	base, err := NormalizeBase(base)
	if err != nil {
		return nil, time.Time{}, err
	}
	req, err := newRequest(ctx, http.MethodGet, base, "/api/studio/emisor/config", token, nil)
	if err != nil {
		return nil, time.Time{}, err
	}
	resp, err := do(req)
	if err != nil {
		return nil, time.Time{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, time.Time{}, errors.New("Todavía no hay una copia de la configuración en el servidor.")
	}
	if resp.StatusCode != http.StatusOK {
		return nil, time.Time{}, fmt.Errorf("la web respondió %d", resp.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	ts, _ := time.Parse(time.RFC3339, resp.Header.Get("X-Saved-At"))
	return b, ts, err
}

// ---------- descarga de la actualización ----------

// Download baja el .exe de la última versión a dst y comprueba tamaño y SHA-256.
// progress (opcional) recibe los bytes bajados.
func Download(ctx context.Context, base, token string, rel Release, dst string, progress func(done, total int64)) error {
	base, err := NormalizeBase(base)
	if err != nil {
		return err
	}
	dl, cancel := context.WithTimeout(ctx, 15*time.Minute)
	defer cancel()
	req, err := newRequest(dl, http.MethodGet, base, "/api/studio/emisor/download", token, nil)
	if err != nil {
		return err
	}
	slow := *client
	slow.Timeout = 0 // el tiempo lo pone el contexto (archivo grande)
	resp, err := slow.Do(req)
	if err != nil {
		return &OfflineError{Err: err}
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusUnauthorized, http.StatusForbidden:
		return ErrUnauthorized
	case http.StatusServiceUnavailable:
		return ErrSuspended
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("la web respondió %d al descargar la actualización", resp.StatusCode)
	}
	if v := resp.Header.Get("X-Emisor-Version"); v != "" && v != rel.Version {
		return fmt.Errorf("se publicó otra versión (%s) mientras tanto: vuelva a intentar", v)
	}
	f, err := os.Create(dst)
	if err != nil {
		return err
	}
	h := sha256.New()
	pw := &progressWriter{total: rel.Size, fn: progress}
	n, err := io.Copy(io.MultiWriter(f, h, pw), io.LimitReader(resp.Body, rel.Size+1))
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err == nil && n != rel.Size {
		err = fmt.Errorf("la descarga llegó incompleta (%d de %d bytes)", n, rel.Size)
	}
	if err == nil && !strings.EqualFold(hex.EncodeToString(h.Sum(nil)), rel.SHA256) {
		err = errors.New("el archivo descargado no coincide con el publicado (SHA-256): no se instala")
	}
	if err != nil {
		_ = os.Remove(dst)
		return err
	}
	return nil
}

type progressWriter struct {
	done, total int64
	fn          func(done, total int64)
	last        time.Time
}

func (p *progressWriter) Write(b []byte) (int, error) {
	p.done += int64(len(b))
	if p.fn != nil && (time.Since(p.last) > 150*time.Millisecond || p.done >= p.total) {
		p.last = time.Now()
		p.fn(p.done, p.total)
	}
	return len(b), nil
}

// ---------- latido ----------

// Output es el estado de una salida para el panel (Admin → Monitoreo del estudio).
type Output struct {
	State       string  `json:"state"`
	BitrateKbps float64 `json:"bitrate_kbps"`
	Message     string  `json:"message"`
	Target      string  `json:"target"`
}

// Report es lo que el emisor informa en cada latido.
type Report struct {
	Radio      Output `json:"radio"`
	TV         Output `json:"tv"`
	NowPlaying string `json:"now_playing"`
}

// Heartbeat avisa al servidor que el emisor está funcionando:
// POST /api/studio/emisor/heartbeat. Devuelve la versión publicada más alta
// ("" si no hay o el servidor no la informa) y ErrUnauthorized si el token fue
// revocado o reemplazado (el servidor responde 401 al instante).
func Heartbeat(ctx context.Context, base, token, version string, r Report) (string, error) {
	base, err := NormalizeBase(base)
	if err != nil {
		return "", err
	}
	body, err := json.Marshal(map[string]any{"version": version, "status": r})
	if err != nil {
		return "", err
	}
	req, err := newRequest(ctx, http.MethodPost, base, "/api/studio/emisor/heartbeat", token, strings.NewReader(string(body)))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusNotFound, http.StatusNoContent:
		return "", nil // servidor anterior: sin latido o sin versión en la respuesta
	case http.StatusOK:
		var out struct {
			Latest string `json:"latest"`
		}
		_ = json.NewDecoder(io.LimitReader(resp.Body, 4096)).Decode(&out)
		return out.Latest, nil
	}
	return "", fmt.Errorf("la web respondió %d al latido", resp.StatusCode)
}
