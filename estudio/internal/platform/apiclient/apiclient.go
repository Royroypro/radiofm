package apiclient

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Errores con significado para la sincronización.
var (
	ErrUnauthorized = errors.New("token inválido o revocado (Admin → Estudio (PC))")
	ErrSuspended    = errors.New("servicio web suspendido")
)

// Client habla con /api/studio/* del panel web. Contrato: docs/api.md → "API del estudio".
type Client struct {
	BaseURL string // https://radioclub25peru.com
	Token   string
	HTTP    *http.Client
	Version string // versión del programa (User-Agent)
}

// New crea un cliente.
func New(baseURL, token, version string) *Client {
	return &Client{BaseURL: strings.TrimRight(baseURL, "/"), Token: token, Version: version,
		HTTP: &http.Client{Timeout: 20 * time.Second}}
}

// APIError es un error devuelto por la API.
type APIError struct {
	Status  int
	Code    string `json:"error"`
	Field   string `json:"field"`
	Message string `json:"message"`
}

func (e *APIError) Error() string {
	if e.Message != "" {
		return fmt.Sprintf("API %d: %s", e.Status, e.Message)
	}
	return fmt.Sprintf("API %d: %s", e.Status, e.Code)
}

func (c *Client) do(ctx context.Context, method, path string, body any, out any, header http.Header) (*http.Response, error) {
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+"/api/studio"+path, rd)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("User-Agent", "Club25Estudio/"+c.Version)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range header {
		req.Header[k] = v
	}
	res, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	switch {
	case res.StatusCode == http.StatusUnauthorized:
		return res, ErrUnauthorized
	case res.StatusCode == http.StatusServiceUnavailable:
		return res, ErrSuspended
	case res.StatusCode == http.StatusNotModified, res.StatusCode == http.StatusNoContent:
		return res, nil
	case res.StatusCode >= 400:
		e := &APIError{Status: res.StatusCode}
		_ = json.NewDecoder(io.LimitReader(res.Body, 4096)).Decode(e)
		return res, e
	}
	if out != nil {
		if err := json.NewDecoder(res.Body).Decode(out); err != nil {
			return res, fmt.Errorf("API: respuesta inválida: %w", err)
		}
	}
	return res, nil
}

// Config es GET /api/studio/config.
type Config struct {
	Station struct {
		Name      string  `json:"name"`
		Frequency string  `json:"frequency"`
		Slogan    string  `json:"slogan"`
		Timezone  string  `json:"timezone"`
		Latitude  float64 `json:"latitude"`
		Longitude float64 `json:"longitude"`
	} `json:"station"`
	Icecast struct {
		Host         string `json:"host"`
		Port         int    `json:"port"`
		Mount        string `json:"mount"`
		User         string `json:"user"`
		RadioEnabled bool   `json:"radio_enabled"`
	} `json:"icecast"`
	TV struct { // modo TV: dónde publicar y con qué credenciales (panel → Ajustes → Streams)
		Enabled bool   `json:"enabled"`
		URL     string `json:"url"`
		User    string `json:"user"`
		Pass    string `json:"pass"`
	} `json:"tv"`
	ServerTime time.Time `json:"server_time"`
}

// GetConfig lee la configuración de la radio.
func (c *Client) GetConfig(ctx context.Context) (Config, error) {
	var out Config
	_, err := c.do(ctx, http.MethodGet, "/config", nil, &out, nil)
	return out, err
}

// Spot es una cuña de GET /api/studio/spots.
type Spot struct {
	ID          int64      `json:"id"`
	Advertiser  string     `json:"advertiser"`
	Name        string     `json:"name"`
	URL         string     `json:"url"`
	SHA256      string     `json:"sha256"`
	DurationMs  int64      `json:"duration_ms"`
	StartsAt    *time.Time `json:"starts_at"`
	EndsAt      *time.Time `json:"ends_at"`
	PlaysPerDay int        `json:"plays_per_day"`
	HourFrom    int        `json:"hour_from"`
	HourTo      int        `json:"hour_to"`
	UpdatedAt   time.Time  `json:"updated_at"`
	// Reglas de la campaña (servidor con campañas; se piden con ?v=2).
	CampaignID int64  `json:"campaign_id"`
	Weight     int    `json:"weight"`
	Days       int    `json:"days"`
	Position   string `json:"position"`
	Priority   int    `json:"priority"`
	Sector     string `json:"sector"`
}

// GetSpots lee las cuñas vigentes. Con etag igual al del servidor devuelve notModified = true.
func (c *Client) GetSpots(ctx context.Context, etag string) (spots []Spot, newETag string, notModified bool, err error) {
	h := http.Header{}
	if etag != "" {
		h.Set("If-None-Match", etag)
	}
	var out struct {
		Spots []Spot `json:"spots"`
	}
	// v=2: todas las versiones de cada campaña con sus reglas (sin v el servidor
	// manda una versión por campaña, para estudios anteriores a 1.2).
	res, err := c.do(ctx, http.MethodGet, "/spots?v=2", nil, &out, h)
	if err != nil {
		return nil, "", false, err
	}
	if res.StatusCode == http.StatusNotModified {
		return nil, etag, true, nil
	}
	return out.Spots, res.Header.Get("ETag"), false, nil
}

// Program es un programa de la programación semanal.
type Program struct {
	ID      int64  `json:"id"`
	Title   string `json:"title"`
	Host    string `json:"host"`
	Weekday int    `json:"weekday"`
	Start   string `json:"start"`
	End     string `json:"end"`
}

// GetPrograms lee la programación semanal.
func (c *Client) GetPrograms(ctx context.Context) ([]Program, error) {
	var out struct {
		Programs []Program `json:"programs"`
	}
	_, err := c.do(ctx, http.MethodGet, "/programs", nil, &out, nil)
	return out.Programs, err
}

// Heartbeat es el cuerpo de POST /api/studio/heartbeat.
type Heartbeat struct {
	Version string `json:"version"`
	Mode    string `json:"mode"`
	Encoder any    `json:"encoder"`
	Status  any    `json:"status,omitempty"` // estado completo de la PC para el panel (ver app.StudioStatus)
}

// SendHeartbeat informa que la PC está viva.
func (c *Client) SendHeartbeat(ctx context.Context, hb Heartbeat) error {
	_, err := c.do(ctx, http.MethodPost, "/heartbeat", hb, nil, nil)
	return err
}

// NowPlaying es el cuerpo de POST /api/studio/nowplaying.
type NowPlaying struct {
	Artist string     `json:"artist"`
	Title  string     `json:"title"`
	Kind   string     `json:"kind"`
	EndsAt *time.Time `json:"ends_at,omitempty"`
}

// SendNowPlaying informa lo que suena.
func (c *Client) SendNowPlaying(ctx context.Context, np NowPlaying) error {
	_, err := c.do(ctx, http.MethodPost, "/nowplaying", np, nil, nil)
	return err
}

// Airplay es una emisión de cuña para POST /api/studio/airplays.
type Airplay struct {
	SpotID     int64     `json:"spot_id"`
	AiredAt    time.Time `json:"aired_at"`
	DurationMs int64     `json:"duration_ms"`
	Complete   bool      `json:"complete"`
	PlayUUID   string    `json:"play_uuid"`
}

// AirplayResult es la respuesta de POST /api/studio/airplays.
type AirplayResult struct {
	Received int      `json:"received"`
	Inserted int      `json:"inserted"`
	Ignored  int      `json:"ignored"`
	Rejected []string `json:"rejected"`
}

// SendAirplays envía un lote de emisiones (máx. 500).
func (c *Client) SendAirplays(ctx context.Context, batch []Airplay) (AirplayResult, error) {
	var out AirplayResult
	_, err := c.do(ctx, http.MethodPost, "/airplays", map[string]any{"airplays": batch}, &out, nil)
	return out, err
}

// Download descarga url a dst verificando el SHA-256 (si se da). Escribe en un
// temporal y renombra: nunca deja un archivo a medias con el nombre final.
func (c *Client) Download(ctx context.Context, url, dst, wantSHA string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "Club25Estudio/"+c.Version)
	hc := &http.Client{Timeout: 5 * time.Minute, Transport: c.HTTP.Transport}
	res, err := hc.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("descargar %s: HTTP %d", url, res.StatusCode)
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	tmp := dst + ".part"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	h := sha256.New()
	_, err = io.Copy(io.MultiWriter(f, h), io.LimitReader(res.Body, 200<<20))
	f.Close()
	if err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if got := hex.EncodeToString(h.Sum(nil)); wantSHA != "" && !strings.EqualFold(got, wantSHA) {
		_ = os.Remove(tmp)
		return fmt.Errorf("el archivo descargado no coincide (sha256 %s, se esperaba %s)", got, wantSHA)
	}
	return os.Rename(tmp, dst)
}

// UploadBackup sube la copia diaria de la base del Estudio (comprimida con gzip)
// al servidor: POST /api/studio/backup. name es el nombre del archivo
// (estudio-AAAAMMDD.db). El servidor guarda las últimas copias de cada frecuencia.
func (c *Client) UploadBackup(ctx context.Context, name string, gz io.Reader) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/api/studio/backup", gz)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("User-Agent", "Club25Estudio/"+c.Version)
	req.Header.Set("Content-Type", "application/gzip")
	req.Header.Set("X-Backup-Name", name)
	cl := *c.HTTP
	cl.Timeout = 10 * time.Minute // la base puede pesar varios MB
	res, err := cl.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode/100 != 2 {
		b, _ := io.ReadAll(io.LimitReader(res.Body, 512))
		return fmt.Errorf("respaldo: HTTP %d %s", res.StatusCode, strings.TrimSpace(string(b)))
	}
	return nil
}

// Command es una orden del panel (control remoto).
type Command struct {
	ID        int64     `json:"id"`
	Kind      string    `json:"kind"`
	Arg       string    `json:"arg"`
	CreatedAt time.Time `json:"created_at"`
}

// GetCommands pide las órdenes pendientes del panel: GET /api/studio/commands.
func (c *Client) GetCommands(ctx context.Context) ([]Command, error) {
	var out struct {
		Commands []Command `json:"commands"`
	}
	_, err := c.do(ctx, http.MethodGet, "/commands", nil, &out, nil)
	return out.Commands, err
}

// CommandDone informa el resultado de una orden.
func (c *Client) CommandDone(ctx context.Context, id int64, ok bool, result string) error {
	_, err := c.do(ctx, http.MethodPost, fmt.Sprintf("/commands/%d/done", id),
		map[string]any{"ok": ok, "result": result}, nil, nil)
	return err
}
