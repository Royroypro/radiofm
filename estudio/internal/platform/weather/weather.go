package weather

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"
)

// Client consulta la temperatura actual en Open-Meteo (https://open-meteo.com,
// gratis, sin clave) y la guarda en caché TTL (15 min por defecto).
type Client struct {
	BaseURL string // https://api.open-meteo.com
	TTL     time.Duration
	HTTP    *http.Client

	mu      sync.Mutex
	lat     float64
	lon     float64
	value   float64
	fetched time.Time
}

// New crea el cliente.
func New() *Client {
	return &Client{BaseURL: "https://api.open-meteo.com", TTL: 15 * time.Minute, HTTP: &http.Client{Timeout: 5 * time.Second}}
}

// Current devuelve la temperatura (°C) en lat/lon. Si la red falla pero hay un
// valor de hace menos de 1 h, devuelve ese valor.
func (c *Client) Current(ctx context.Context, lat, lon float64) (float64, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	same := c.lat == lat && c.lon == lon
	if same && !c.fetched.IsZero() && time.Since(c.fetched) < c.TTL {
		return c.value, nil
	}
	v, err := c.fetch(ctx, lat, lon)
	if err != nil {
		if same && !c.fetched.IsZero() && time.Since(c.fetched) < time.Hour {
			return c.value, nil
		}
		return 0, err
	}
	c.lat, c.lon, c.value, c.fetched = lat, lon, v, time.Now()
	return v, nil
}

func (c *Client) fetch(ctx context.Context, lat, lon float64) (float64, error) {
	q := url.Values{"latitude": {strconv.FormatFloat(lat, 'f', 4, 64)}, "longitude": {strconv.FormatFloat(lon, 'f', 4, 64)},
		"current": {"temperature_2m"}}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+"/v1/forecast?"+q.Encode(), nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("User-Agent", "Club25Estudio")
	res, err := c.HTTP.Do(req)
	if err != nil {
		return 0, fmt.Errorf("clima: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("clima: HTTP %d", res.StatusCode)
	}
	var doc struct {
		Current struct {
			Temperature *float64 `json:"temperature_2m"`
		} `json:"current"`
	}
	if err := json.NewDecoder(res.Body).Decode(&doc); err != nil {
		return 0, fmt.Errorf("clima: respuesta inválida: %w", err)
	}
	if doc.Current.Temperature == nil {
		return 0, fmt.Errorf("clima: la respuesta no trae temperatura")
	}
	return *doc.Current.Temperature, nil
}
