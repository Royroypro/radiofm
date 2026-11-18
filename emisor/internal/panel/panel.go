// Package panel trae los datos de ingesta desde la web de la radio y entiende
// los datos copiados del panel.
//
// Tres formas de cargar los datos (además de los valores por defecto):
//   - Import: GET /api/studio/config con el token de Admin → Estudio (PC), el
//     mismo que usa Club25 Estudio. Trae servidor, puerto y montaje de Icecast
//     y la URL, usuario y clave de la TV. La contraseña de fuente de Icecast NO
//     viene por diseño (docs/api.md): se escribe una vez y se guarda cifrada.
//   - ParsePasted: pegar el texto de Admin → Streams y emisión (URL RTMP
//     completa, URL SRT, icecast://…).
//   - UpdateSong: además envía el título de la canción a Icecast, como BUTT.
//
// No cambia nada en el servidor: solo lee.
package panel

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"radioclub25/emisor/internal/config"
)

// client es el cliente HTTP con tiempo límite (no cuelga la interfaz).
var client = &http.Client{Timeout: 15 * time.Second}

// studioConfig es la parte de GET /api/studio/config que usa el emisor.
type studioConfig struct {
	Station struct {
		Name      string `json:"name"`
		Frequency string `json:"frequency"`
	} `json:"station"`
	Icecast struct {
		Host         string `json:"host"`
		Port         int    `json:"port"`
		Mount        string `json:"mount"`
		User         string `json:"user"`
		RadioEnabled bool   `json:"radio_enabled"`
		Source       string `json:"source"`
	} `json:"icecast"`
	TV struct {
		Enabled bool   `json:"enabled"`
		URL     string `json:"url"`
		User    string `json:"user"`
		Pass    string `json:"pass"`
	} `json:"tv"`
}

// ImportResult resume lo que se cargó, para mostrarlo.
type ImportResult struct {
	Station      string   `json:"station"`
	Notes        []string `json:"notes"`
	RadioEnabled bool     `json:"radio_enabled"`
	TVEnabled    bool     `json:"tv_enabled"`
}

// Import pide los datos al panel y los aplica sobre c. No toca la contraseña de Icecast.
func Import(ctx context.Context, c *config.Config) (ImportResult, error) {
	base := strings.TrimRight(strings.TrimSpace(c.Panel.URL), "/")
	if base == "" || c.Panel.Token == "" {
		return ImportResult{}, errors.New("Escriba la dirección de la web y el token de Admin → Estudio (PC).")
	}
	if !strings.Contains(base, "://") {
		base = "https://" + base
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/api/studio/config", nil)
	if err != nil {
		return ImportResult{}, fmt.Errorf("Dirección de la web no válida: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.Panel.Token)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "Club25Emisor")
	resp, err := client.Do(req)
	if err != nil {
		return ImportResult{}, fmt.Errorf("No se pudo conectar con la web: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusUnauthorized:
		return ImportResult{}, errors.New("El token no es válido. Genere uno en Admin → Estudio (PC).")
	case http.StatusServiceUnavailable:
		return ImportResult{}, errors.New("El servicio está suspendido en la web (licencia).")
	default:
		return ImportResult{}, fmt.Errorf("La web respondió %d.", resp.StatusCode)
	}
	var sc studioConfig
	if err := json.Unmarshal(body, &sc); err != nil {
		return ImportResult{}, errors.New("La web respondió algo que no es la configuración del estudio.")
	}
	res := ImportResult{Station: strings.TrimSpace(sc.Station.Name + " " + sc.Station.Frequency),
		RadioEnabled: sc.Icecast.RadioEnabled, TVEnabled: sc.TV.Enabled}

	if sc.Icecast.Host != "" {
		c.Radio.Host, c.Radio.Port, c.Radio.Mount = sc.Icecast.Host, sc.Icecast.Port, sc.Icecast.Mount
		if sc.Icecast.User != "" {
			c.Radio.User = sc.Icecast.User
		}
		res.Notes = append(res.Notes, fmt.Sprintf("Radio: %s:%d%s", c.Radio.Host, c.Radio.Port, c.Radio.Mount))
	} else {
		res.Notes = append(res.Notes, "Esta frecuencia sale de un servidor externo: se conserva el servidor de radio actual.")
	}
	if c.Radio.Password == "" {
		res.Notes = append(res.Notes, "Falta la contraseña de fuente de Icecast: la web no la envía; cópiela de Admin → Streams y emisión.")
	}
	if !sc.Icecast.RadioEnabled {
		res.Notes = append(res.Notes, "Aviso: la radio está desactivada en el panel; Icecast rechazará la emisión.")
	}
	if sc.TV.URL != "" {
		if err := applyRTMP(c, sc.TV.URL); err != nil {
			return res, err
		}
		if sc.TV.User != "" {
			c.TV.User = sc.TV.User
		}
		if sc.TV.Pass != "" {
			c.TV.Password = sc.TV.Pass
		}
		res.Notes = append(res.Notes, fmt.Sprintf("TV: %s (usuario %s, clave cargada)", sc.TV.URL, c.TV.User))
	}
	if !sc.TV.Enabled {
		res.Notes = append(res.Notes, "Aviso: la TV está desactivada en el panel; MediaMTX rechazará la emisión.")
	}
	c.Normalize()
	return res, nil
}

// applyRTMP toma host, puerto, ruta y (si vienen) usuario y clave de una URL rtmp://.
func applyRTMP(c *config.Config, raw string) error {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Scheme != "rtmp" || u.Hostname() == "" {
		return fmt.Errorf("URL RTMP no válida: %q", raw)
	}
	c.TV.Host = u.Hostname()
	c.TV.RTMPPort = portOr(u.Port(), 1935)
	if p := strings.Trim(u.Path, "/"); p != "" {
		c.TV.Path = p
	}
	q := u.Query()
	if v := q.Get("user"); v != "" {
		c.TV.User = v
	}
	if v := q.Get("pass"); v != "" {
		c.TV.Password = v
	}
	return nil
}

func portOr(p string, def int) int {
	if n, err := strconv.Atoi(p); err == nil && n > 0 && n <= 65535 {
		return n
	}
	return def
}

// ParsePasted entiende el texto copiado del panel (una o varias líneas):
//
//	rtmp://host:1935/tv?user=emisora&pass=CLAVE     (URL completa)
//	rtmp://host:1935  +  tv?user=emisora&pass=CLAVE  (servidor y clave por separado, como OBS)
//	srt://host:8890?streamid=publish:tv:emisora:CLAVE&pkt_size=1316
//	icecast://source:CLAVE@host:8010/radio  ·  http://host:8010/radio
//
// Devuelve qué entendió. Lo que no reconoce lo ignora.
func ParsePasted(c *config.Config, text string) ([]string, error) {
	var got []string
	var rtmpServer string
	for _, line := range strings.FieldsFunc(text, func(r rune) bool { return r == '\n' || r == '\r' }) {
		for _, tok := range strings.Fields(line) {
			tok = strings.Trim(tok, "`'\"<>,;")
			low := strings.ToLower(tok)
			switch {
			case strings.HasPrefix(low, "rtmp://"):
				if err := applyRTMP(c, tok); err != nil {
					return got, err
				}
				rtmpServer = tok
				if strings.Contains(tok, "pass=") {
					got = append(got, "TV por RTMP: servidor, ruta, usuario y clave")
				} else {
					got = append(got, "TV por RTMP: servidor")
				}
				c.TV.Protocol = config.ProtoRTMP
			case strings.HasPrefix(low, "srt://"):
				if err := applySRT(c, tok); err != nil {
					return got, err
				}
				got = append(got, "TV por SRT: servidor, puerto, ruta, usuario y clave")
				c.TV.Protocol = config.ProtoSRT
			case strings.HasPrefix(low, "icecast://"), strings.HasPrefix(low, "http://"), strings.HasPrefix(low, "https://"):
				if applyIcecast(c, tok) {
					got = append(got, "Radio: servidor, puerto y montaje"+map[bool]string{true: " y contraseña", false: ""}[strings.Contains(tok, "@")])
				}
			case rtmpServer != "" && strings.Contains(tok, "?") && strings.Contains(tok, "pass="):
				// Clave de retransmisión al estilo OBS: tv?user=…&pass=…
				if err := applyRTMP(c, strings.TrimRight(rtmpServer, "/")+"/"+strings.TrimLeft(tok, "/")); err != nil {
					return got, err
				}
				got = append(got, "TV por RTMP: ruta, usuario y clave")
			}
		}
	}
	if len(got) == 0 {
		return nil, errors.New("No se reconoció ningún dato. Pegue la URL RTMP completa, la URL SRT o icecast://… del panel.")
	}
	c.Normalize()
	return got, nil
}

// applySRT lee srt://host:puerto?streamid=publish:ruta:usuario:clave.
func applySRT(c *config.Config, raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" {
		return fmt.Errorf("URL SRT no válida: %q", raw)
	}
	c.TV.Host = u.Hostname()
	c.TV.SRTPort = portOr(u.Port(), 8890)
	sid := u.Query().Get("streamid")
	parts := strings.Split(sid, ":")
	if len(parts) >= 2 && parts[0] == "publish" {
		c.TV.Path = parts[1]
	}
	if len(parts) >= 4 {
		c.TV.User, c.TV.Password = parts[2], strings.Join(parts[3:], ":")
	}
	return nil
}

// applyIcecast lee icecast://usuario:clave@host:puerto/montaje o http://host:puerto/montaje.
// Solo la acepta si trae puerto (para no confundirla con la dirección de la web).
func applyIcecast(c *config.Config, raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" || u.Port() == "" || strings.Trim(u.Path, "/") == "" {
		return false
	}
	c.Radio.Host = u.Hostname()
	c.Radio.Port = portOr(u.Port(), 8010)
	c.Radio.Mount = u.Path
	if u.User != nil {
		if n := u.User.Username(); n != "" {
			c.Radio.User = n
		}
		if p, ok := u.User.Password(); ok {
			c.Radio.Password = p
		}
	}
	return true
}

// UpdateSong envía el título actual a Icecast (/admin/metadata, como BUTT).
// Solo sirve para MP3 y AAC; en Ogg (Opus/Vorbis) el título va dentro del stream
// y Icecast lo rechaza por esta vía.
func UpdateSong(ctx context.Context, r config.Radio, title string) error {
	if strings.TrimSpace(title) == "" {
		return errors.New("Escriba el título.")
	}
	if r.Codec == config.CodecOpus || r.Codec == config.CodecVorbis {
		return errors.New("Con Opus u Ogg Vorbis Icecast no acepta el título por esta vía.")
	}
	q := url.Values{"mount": {r.Mount}, "mode": {"updinfo"}, "song": {title}, "charset": {"UTF-8"}}
	u := url.URL{Scheme: "http", Host: net.JoinHostPort(r.Host, strconv.Itoa(r.Port)), Path: "/admin/metadata", RawQuery: q.Encode()}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return err
	}
	req.SetBasicAuth(r.User, r.Password)
	req.Header.Set("User-Agent", "Club25Emisor")
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("No se pudo enviar el título: %w", err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
	switch resp.StatusCode {
	case http.StatusOK:
		return nil
	case http.StatusUnauthorized, http.StatusForbidden:
		return errors.New("Icecast rechazó la contraseña al enviar el título.")
	default:
		return fmt.Errorf("Icecast respondió %d al enviar el título.", resp.StatusCode)
	}
}
