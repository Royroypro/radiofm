package icecast

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Client actualiza la metadata ("lo que suena") de un mount de Icecast con las
// credenciales de fuente, como hacen BUTT o Mixxx.
type Client struct {
	BaseURL  string // http://host:puerto
	Mount    string // /radio
	User     string // source
	Password string
	HTTP     *http.Client
}

// New crea un cliente para host:port.
func New(host string, port int, mount, user, password string) *Client {
	if user == "" {
		user = "source"
	}
	return &Client{BaseURL: fmt.Sprintf("http://%s:%d", host, port), Mount: mount, User: user, Password: password,
		HTTP: &http.Client{Timeout: 5 * time.Second}}
}

// UpdateSong cambia el título que ven los oyentes ("Artista - Título").
// Llama a GET /admin/metadata?mount=/radio&mode=updinfo&song=...&charset=UTF-8.
func (c *Client) UpdateSong(ctx context.Context, song string) error {
	q := url.Values{"mount": {c.Mount}, "mode": {"updinfo"}, "song": {song}, "charset": {"UTF-8"}}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(c.BaseURL, "/")+"/admin/metadata?"+q.Encode(), nil)
	if err != nil {
		return err
	}
	req.SetBasicAuth(c.User, c.Password)
	req.Header.Set("User-Agent", "Club25Estudio")
	res, err := c.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("icecast: metadata: %w", err)
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(res.Body, 2048))
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("icecast: metadata: HTTP %d %s", res.StatusCode, strings.TrimSpace(string(body)))
	}
	// Icecast responde 200 con <return>1</return> si se aplicó.
	if len(body) > 0 && strings.Contains(string(body), "<return>0</return>") {
		return fmt.Errorf("icecast: metadata rechazada: %s", strings.TrimSpace(string(body)))
	}
	return nil
}
