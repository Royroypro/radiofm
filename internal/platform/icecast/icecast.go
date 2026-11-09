// Package icecast es un cliente mínimo del servidor de radio Icecast2.
//
// Solo hace dos cosas:
//   - leer /status-json.xsl para saber si la fuente (la PC emisora) está
//     conectada, cuántos oyentes hay y qué canción suena;
//   - expulsar la fuente (/admin/killsource) cuando la licencia se bloquea.
package icecast

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

// Client habla con Icecast por HTTP.
type Client struct {
	baseURL   string
	mount     string
	adminUser string
	adminPass string
	http      *http.Client
}

// New crea un cliente. baseURL es la URL interna (p. ej. http://icecast:8000).
func New(baseURL, mount, adminUser, adminPass string) *Client {
	return &Client{
		baseURL: baseURL, mount: mount, adminUser: adminUser, adminPass: adminPass,
		http: &http.Client{Timeout: 4 * time.Second},
	}
}

// MountStatus es el estado de la radio.
type MountStatus struct {
	Online    bool
	Listeners int
	Peak      int
	Title     string // "Artista - Canción" que envía el software de la PC
	Bitrate   int    // kbps
	Format    string // audio/mpeg, audio/aac...
}

// status-json.xsl devuelve "source" como objeto si hay un mount o como lista si hay varios.
type statusDoc struct {
	Icestats struct {
		Source json.RawMessage `json:"source"`
	} `json:"icestats"`
}

// sourceDoc son los campos de un mount que nos interesan. Icecast envía
// algunos números como texto según la versión, por eso Bitrate es any.
type sourceDoc struct {
	ListenURL    string `json:"listenurl"`
	Listeners    int    `json:"listeners"`
	ListenerPeak int    `json:"listener_peak"`
	Title        string `json:"title"`
	Artist       string `json:"artist"`
	ServerType   string `json:"server_type"`
	Bitrate      any    `json:"bitrate"`
	AudioBitrate any    `json:"audio_bitrate"`
	IceBitrate   any    `json:"ice-bitrate"`
}

// Status consulta el estado del mount configurado (el principal).
func (c *Client) Status(ctx context.Context) (MountStatus, error) {
	all, err := c.StatusAll(ctx)
	if err != nil {
		return MountStatus{}, err
	}
	return all[c.mount], nil // ausente = fuera del aire
}

// StatusAll consulta todos los montajes con fuente conectada (una frecuencia
// por montaje: /radio, /radio-barranca…). Los que no aparecen están fuera del aire.
func (c *Client) StatusAll(ctx context.Context) (map[string]MountStatus, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/status-json.xsl", nil)
	if err != nil {
		return nil, err
	}
	res, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("icecast: status: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("icecast: status: HTTP %d", res.StatusCode)
	}
	var doc statusDoc
	if err := json.NewDecoder(res.Body).Decode(&doc); err != nil {
		return nil, fmt.Errorf("icecast: decodificar status: %w", err)
	}
	sources, err := parseSources(doc.Icestats.Source)
	if err != nil {
		return nil, err
	}
	out := map[string]MountStatus{}
	for _, s := range sources {
		u, err := url.Parse(s.ListenURL)
		if err != nil {
			continue
		}
		title := s.Title
		if s.Artist != "" && title != "" {
			title = s.Artist + " - " + title
		}
		out[u.Path] = MountStatus{
			Online: true, Listeners: s.Listeners, Peak: s.ListenerPeak, Title: title,
			Bitrate: toInt(firstNonNil(s.AudioBitrate, s.Bitrate, s.IceBitrate)), Format: s.ServerType,
		}
	}
	return out, nil
}

// KillSource desconecta las fuentes de todos los montajes conectados (las PC
// emisoras de cada frecuencia). Se usa al aplicar un bloqueo total. Si una PC
// reconecta, los oyentes igualmente están bloqueados por nginx (auth_request
// a /internal/gate/radio) y stream_auth la rechaza.
func (c *Client) KillSource(ctx context.Context) error {
	mounts := []string{c.mount}
	if all, err := c.StatusAll(ctx); err == nil {
		for m := range all {
			if m != c.mount {
				mounts = append(mounts, m)
			}
		}
	}
	var errs []error
	for _, m := range mounts {
		errs = append(errs, c.killMount(ctx, m))
	}
	return errors.Join(errs...)
}

func (c *Client) killMount(ctx context.Context, mount string) error {
	u := c.baseURL + "/admin/killsource?mount=" + url.QueryEscape(mount)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	req.SetBasicAuth(c.adminUser, c.adminPass)
	res, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("icecast: killsource: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode >= 400 && res.StatusCode != http.StatusBadRequest { // 400 = no hay fuente conectada
		return fmt.Errorf("icecast: killsource: HTTP %d", res.StatusCode)
	}
	return nil
}

func parseSources(raw json.RawMessage) ([]sourceDoc, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	if raw[0] == '[' {
		var list []sourceDoc
		if err := json.Unmarshal(raw, &list); err != nil {
			return nil, fmt.Errorf("icecast: decodificar sources: %w", err)
		}
		return list, nil
	}
	var one sourceDoc
	if err := json.Unmarshal(raw, &one); err != nil {
		return nil, fmt.Errorf("icecast: decodificar source: %w", err)
	}
	return []sourceDoc{one}, nil
}

func firstNonNil(vals ...any) any {
	for _, v := range vals {
		if v != nil {
			return v
		}
	}
	return nil
}

func toInt(v any) int {
	switch x := v.(type) {
	case float64:
		return int(x)
	case string:
		var n int
		_, _ = fmt.Sscanf(x, "%d", &n)
		return n
	}
	return 0
}
