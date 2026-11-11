// Package studioapi contiene los handlers de la API que usa la PC del estudio
// ("Club25 Estudio", carpeta estudio/): /api/studio/*.
//
// Seguridad (aplicada en router.go):
//   - middleware.StudioToken: "Authorization: Bearer <token>" generado en
//     Admin → Estudio (PC). Sin token válido: 401 {"error":"unauthorized"}.
//   - middleware.PublicLicenseGuard: con bloqueo total de licencia responde
//     503 {"error":"suspended"}; la PC sigue sonando por FM.
//   - Exenta de CSRF como el resto de /api/.
//
// El contrato (campos y ejemplos) está en docs/api.md → "API del estudio".
// Cualquier cambio aquí debe reflejarse allí y en el cliente de estudio/.
package studioapi

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"radioclub25/internal/domain"
	"radioclub25/internal/platform/config"
	"radioclub25/internal/service/frequencies"
	"radioclub25/internal/service/programs"
	"radioclub25/internal/service/settings"
	"radioclub25/internal/service/studio"
	"radioclub25/internal/transport/httpserver/middleware"
)

// Handler agrupa las dependencias de la API del estudio.
type Handler struct {
	Studio      *studio.Service
	Settings    *settings.Service
	Programs    *programs.Service
	Frequencies *frequencies.Service
	Cfg         config.Config
	Log         *slog.Logger
}

// Config devuelve los datos de la radio que la PC necesita: GET /api/studio/config
func (h *Handler) Config(w http.ResponseWriter, r *http.Request) {
	set, err := h.Settings.Get(r.Context())
	if err != nil {
		h.fail(w, err)
		return
	}
	// Cada PC es de una frecuencia (la del token): su ciudad, su frecuencia y su montaje.
	f, err := h.Frequencies.Get(r.Context(), middleware.StudioFrequencyFrom(r.Context()))
	if err != nil {
		h.fail(w, err)
		return
	}
	freqs := []map[string]any{}
	if list, err := h.Frequencies.Active(r.Context()); err == nil {
		for _, o := range list {
			freqs = append(freqs, map[string]any{"id": o.ID, "city": o.City, "frequency": o.Frequency, "band": o.Band,
				"label": o.Label()})
		}
	}
	// La clave de fuente NO se envía: se escribe una vez en la PC y se guarda cifrada (DPAPI).
	// Si el audio de esta frecuencia sale de otro servidor, host va vacío y la PC
	// conserva el servidor que tenga configurado.
	ice := map[string]any{"host": "", "port": 0, "mount": "", "user": "source", "radio_enabled": set.RadioEnabled,
		"source": string(f.Source)}
	if !f.IsExternal() {
		ice["host"], ice["port"], ice["mount"] = h.Cfg.IngestHost, h.Cfg.IcecastPublicPt, f.Mount
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"station": map[string]any{
			"name": set.SiteName, "frequency": f.Label(), "frequency_id": f.ID, "city": f.City,
			"frequencies": freqs, "slogan": set.Slogan,
			"timezone": h.Cfg.Location.String(),
			"latitude": h.Cfg.StationLat, "longitude": h.Cfg.StationLon,
		},
		"icecast": ice,
		// Modo TV del Estudio: a dónde publicar y con qué usuario y clave (los de
		// Ajustes → Streams). A diferencia de la clave de Icecast, esta sí se envía,
		// para no copiarla a mano en cada PC: solo por HTTPS y con el token de estudio.
		"tv": map[string]any{
			"enabled": set.TVEnabled,
			"url":     fmt.Sprintf("rtmp://%s:%d/%s", h.Cfg.IngestHost, h.Cfg.RTMPPublicPort, h.Cfg.MediaMTXPath),
			"user":    set.TVPublishUser, "pass": set.TVPublishPass,
		},
		"server_time": time.Now().UTC().Format(time.RFC3339),
	})
}

// spotJSON es una cuña tal como la recibe la PC.
type spotJSON struct {
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
	// Tráfico (estudio 1.2+): reglas de la campaña de esta versión.
	CampaignID int64  `json:"campaign_id"`
	Weight     int    `json:"weight"`
	Days       int    `json:"days"` // máscara: bit 0 = domingo … bit 6 = sábado
	Position   string `json:"position"`
	Priority   int    `json:"priority"`
	Sector     string `json:"sector"`
}

// Spots devuelve las cuñas FM activas y no vencidas: GET /api/studio/spots
// Responde con ETag; si la PC envía If-None-Match igual, 304 sin cuerpo.
func (h *Handler) Spots(w http.ResponseWriter, r *http.Request) {
	list, err := h.Studio.SpotsForStudio(r.Context(), middleware.StudioFrequencyFrom(r.Context()))
	if err != nil {
		h.fail(w, err)
		return
	}
	// Un estudio anterior a 1.2 no conoce las campañas: trataría cada versión
	// como una cuña aparte y duplicaría las veces por día. Sin ?v=2 se envía
	// solo una versión por campaña (la de más peso).
	if r.URL.Query().Get("v") != "2" {
		list = onePerCampaign(list)
	}
	out := make([]spotJSON, 0, len(list))
	for _, s := range list {
		out = append(out, spotJSON{
			ID: s.ID, Advertiser: s.Advertiser, Name: s.Name, URL: h.Cfg.BaseURL + s.FileURL, SHA256: s.SHA256,
			DurationMs: s.DurationMs, StartsAt: utcPtr(s.StartsAt), EndsAt: utcPtr(s.EndsAt), PlaysPerDay: s.PlaysPerDay,
			HourFrom: s.HourFrom, HourTo: s.HourTo, UpdatedAt: s.UpdatedAt.UTC(),
			CampaignID: s.CampaignID, Weight: max(s.Weight, 1), Days: s.DaysMask, Position: string(s.Position),
			Priority: s.Priority, Sector: s.Sector,
		})
	}
	body, err := json.Marshal(map[string]any{"spots": out})
	if err != nil {
		h.fail(w, err)
		return
	}
	sum := sha256.Sum256(body)
	etag := `"` + hex.EncodeToString(sum[:12]) + `"`
	w.Header().Set("ETag", etag)
	w.Header().Set("Cache-Control", "no-cache")
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_, _ = w.Write(append(body, '\n'))
}

// Schedule devuelve la programación semanal activa de la frecuencia de la PC
// (sus programas y los de todas): GET /api/studio/programs
func (h *Handler) Schedule(w http.ResponseWriter, r *http.Request) {
	fid := middleware.StudioFrequencyFrom(r.Context())
	list, err := h.Programs.List(r.Context())
	if err != nil {
		h.fail(w, err)
		return
	}
	out := []map[string]any{}
	for _, p := range list {
		if !p.Active || !p.AppliesTo(fid) {
			continue
		}
		out = append(out, map[string]any{"id": p.ID, "title": p.Title, "host": p.Host, "weekday": int(p.Weekday),
			"start": p.StartTime, "end": p.EndTime})
	}
	writeJSON(w, http.StatusOK, map[string]any{"programs": out})
}

// Heartbeat registra la señal de vida de la PC: POST /api/studio/heartbeat
func (h *Handler) Heartbeat(w http.ResponseWriter, r *http.Request) {
	var in studio.HeartbeatInput
	if !decode(w, r, &in, 8<<10) {
		return
	}
	if err := h.Studio.Heartbeat(r.Context(), middleware.StudioFrequencyFrom(r.Context()), in); err != nil {
		h.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "server_time": time.Now().UTC().Format(time.RFC3339)})
}

// NowPlaying guarda lo que suena: POST /api/studio/nowplaying
func (h *Handler) NowPlaying(w http.ResponseWriter, r *http.Request) {
	var in domain.NowPlaying
	if !decode(w, r, &in, 4<<10) {
		return
	}
	if err := h.Studio.SetNowPlaying(r.Context(), middleware.StudioFrequencyFrom(r.Context()), in); err != nil {
		h.fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Airplays registra un lote de emisiones de cuñas: POST /api/studio/airplays
// Cuerpo: {"airplays":[{...}]}. Respuesta: studio.AirplayResult.
func (h *Handler) Airplays(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Airplays []studio.AirplayInput `json:"airplays"`
	}
	if !decode(w, r, &in, 256<<10) {
		return
	}
	res, err := h.Studio.RecordAirplays(r.Context(), middleware.StudioFrequencyFrom(r.Context()), in.Airplays)
	if err != nil {
		h.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

// fail traduce un error del servicio a una respuesta JSON.
func (h *Handler) fail(w http.ResponseWriter, err error) {
	var ve *domain.ValidationError
	switch {
	case errors.As(err, &ve):
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid", "field": ve.Field, "message": ve.Msg})
	case errors.Is(err, domain.ErrNotFound):
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found"})
	default:
		h.Log.Error("api del estudio", "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
	}
}

// decode lee un cuerpo JSON de hasta maxBytes. Si falla responde 400 y devuelve false.
func decode(w http.ResponseWriter, r *http.Request, v any, maxBytes int64) bool {
	if ct := r.Header.Get("Content-Type"); ct != "" && !strings.HasPrefix(ct, "application/json") {
		writeJSON(w, http.StatusUnsupportedMediaType, map[string]string{"error": "content_type"})
		return false
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBytes))
	if err := dec.Decode(v); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad_json", "message": err.Error()})
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func utcPtr(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	u := t.UTC()
	return &u
}

// onePerCampaign deja una versión por campaña (la de más peso; a igual peso, la primera).
func onePerCampaign(list []domain.FMSpot) []domain.FMSpot {
	best := map[int64]int{}
	var out []domain.FMSpot
	for _, s := range list {
		if s.CampaignID == 0 {
			out = append(out, s)
			continue
		}
		if i, ok := best[s.CampaignID]; ok {
			if s.Weight > out[i].Weight {
				out[i] = s
			}
			continue
		}
		best[s.CampaignID] = len(out)
		out = append(out, s)
	}
	return out
}
