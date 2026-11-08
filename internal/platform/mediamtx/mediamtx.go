// Package mediamtx es un cliente mínimo de la API v3 de MediaMTX (servidor de TV).
//
// MediaMTX recibe el video de OBS (RTMP o SRT) y lo publica como HLS.
// Desde la app solo necesitamos:
//   - saber si el path de la TV está recibiendo señal y cuántos lo ven;
//   - expulsar las conexiones de publicación al aplicar un bloqueo total.
//
// Documentación de la API: https://github.com/bluenviron/mediamtx (apidocs).
package mediamtx

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

// Client habla con la API de MediaMTX.
type Client struct {
	apiURL string
	path   string
	http   *http.Client
}

// New crea un cliente. apiURL es la URL interna de la API (p. ej. http://mediamtx:9997).
func New(apiURL, path string) *Client {
	return &Client{apiURL: apiURL, path: path, http: &http.Client{Timeout: 4 * time.Second}}
}

// PathStatus es el estado del canal de TV.
type PathStatus struct {
	Online  bool
	Viewers int
	Tracks  []string // p. ej. ["H264", "MPEG-4 Audio"]
	Source  string   // tipo de fuente: rtmpConn, srtConn...
}

type pathDoc struct {
	Name   string `json:"name"`
	Ready  bool   `json:"ready"`
	Tracks []string
	Source *struct {
		Type string `json:"type"`
		ID   string `json:"id"`
	} `json:"source"`
	Readers []struct {
		Type string `json:"type"`
		ID   string `json:"id"`
	} `json:"readers"`
}

// Status consulta /v3/paths/get/{path}. Si el path no existe (nadie publica) devuelve Online=false.
func (c *Client) Status(ctx context.Context) (PathStatus, error) {
	var doc pathDoc
	code, err := c.getJSON(ctx, "/v3/paths/get/"+url.PathEscape(c.path), &doc)
	if err != nil {
		if code == http.StatusNotFound {
			return PathStatus{}, nil
		}
		return PathStatus{}, err
	}
	st := PathStatus{Online: doc.Ready, Viewers: len(doc.Readers), Tracks: doc.Tracks}
	if doc.Source != nil {
		st.Source = doc.Source.Type
	}
	return st, nil
}

// KickPublisher expulsa la conexión que publica en el path de la TV (RTMP o SRT).
// MediaMTX volverá a pedir autorización a la app si OBS reconecta, y la app la
// rechazará mientras dure el bloqueo.
func (c *Client) KickPublisher(ctx context.Context) error {
	var doc pathDoc
	code, err := c.getJSON(ctx, "/v3/paths/get/"+url.PathEscape(c.path), &doc)
	if err != nil {
		if code == http.StatusNotFound {
			return nil
		}
		return err
	}
	if doc.Source == nil || doc.Source.ID == "" {
		return nil
	}
	var endpoint string
	switch doc.Source.Type {
	case "rtmpConn":
		endpoint = "/v3/rtmpconns/kick/"
	case "srtConn":
		endpoint = "/v3/srtconns/kick/"
	case "rtspSession":
		endpoint = "/v3/rtspsessions/kick/"
	case "webRTCSession":
		endpoint = "/v3/webrtcsessions/kick/"
	default:
		return fmt.Errorf("mediamtx: tipo de fuente desconocido %q", doc.Source.Type)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.apiURL+endpoint+doc.Source.ID, bytes.NewReader(nil))
	if err != nil {
		return err
	}
	res, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("mediamtx: kick: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode >= 400 {
		return fmt.Errorf("mediamtx: kick: HTTP %d", res.StatusCode)
	}
	return nil
}

func (c *Client) getJSON(ctx context.Context, path string, dst any) (int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.apiURL+path, nil)
	if err != nil {
		return 0, err
	}
	res, err := c.http.Do(req)
	if err != nil {
		return 0, fmt.Errorf("mediamtx: %s: %w", path, err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return res.StatusCode, fmt.Errorf("mediamtx: %s: HTTP %d", path, res.StatusCode)
	}
	if err := json.NewDecoder(res.Body).Decode(dst); err != nil {
		return res.StatusCode, fmt.Errorf("mediamtx: decodificar %s: %w", path, err)
	}
	return res.StatusCode, nil
}
