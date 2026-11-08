// Package public contiene los handlers de la web pública (inicio,
// programación, contacto), la API JSON que usa el reproductor y los
// endpoints internos que consultan MediaMTX y nginx.
//
// Las rutas se registran en httpserver/router.go.
package public

import (
	"crypto/subtle"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"radioclub25/internal/domain"
	"radioclub25/internal/service/ads"
	"radioclub25/internal/service/gate"
	"radioclub25/internal/service/license"
	"radioclub25/internal/service/media"
	"radioclub25/internal/service/programs"
	"radioclub25/internal/service/settings"
	"radioclub25/internal/transport/httpserver/form"
	"radioclub25/internal/transport/httpserver/middleware"
	"radioclub25/internal/transport/httpserver/views"
)

// Handler agrupa las dependencias de la web pública.
type Handler struct {
	Views         *views.Renderer
	Settings      *settings.Service
	Programs      *programs.Service
	Ads           *ads.Service
	Media         *media.Service
	License       *license.Service
	Gate          *gate.Service
	Log           *slog.Logger
	InternalToken string
}

// page prepara un *views.Page con los banners de la web pública.
func (h *Handler) page(r *http.Request, title string, data any) *views.Page {
	banners, err := h.Ads.PickBanners(r.Context())
	if err != nil {
		h.Log.Error("elegir banners", "err", err)
	}
	return &views.Page{Title: title, Data: data, Banners: banners}
}

// Home es la página principal con los reproductores de radio y TV.
func (h *Handler) Home(w http.ResponseWriter, r *http.Request) {
	now := time.Now()
	onAir, err := h.Programs.OnAir(r.Context(), now)
	if err != nil {
		h.Views.Error(w, r, err)
		return
	}
	week, err := h.Programs.Week(r.Context(), now)
	if err != nil {
		h.Views.Error(w, r, err)
		return
	}
	var today []domain.Program
	for _, d := range week {
		if d.IsToday {
			today = d.Programs
		}
	}
	h.Views.RenderPage(w, r, http.StatusOK, "public/home", h.page(r, "", map[string]any{
		"OnAir":     onAir,
		"Today":     today,
		"Status":    h.Media.Status(r.Context()),
		"TVAllowed": h.License.Current().Limits.TVEnabled,
	}))
}

// Schedule muestra la programación semanal completa.
func (h *Handler) Schedule(w http.ResponseWriter, r *http.Request) {
	week, err := h.Programs.Week(r.Context(), time.Now())
	if err != nil {
		h.Views.Error(w, r, err)
		return
	}
	h.Views.RenderPage(w, r, http.StatusOK, "public/schedule", h.page(r, "Programación", week))
}

// Contact muestra los datos de contacto.
func (h *Handler) Contact(w http.ResponseWriter, r *http.Request) {
	h.Views.RenderPage(w, r, http.StatusOK, "public/contact", h.page(r, "Contacto", nil))
}

// Suspended es la página que se muestra con bloqueo total de licencia.
func (h *Handler) Suspended(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/api/") {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "suspended"})
		return
	}
	w.Header().Set("Retry-After", "3600")
	h.Views.Render(w, r, http.StatusServiceUnavailable, "errors/suspended", "Servicio suspendido",
		h.License.SuspendedMessage())
}

// NotFound es la página 404.
func (h *Handler) NotFound(w http.ResponseWriter, r *http.Request) {
	h.Views.Error(w, r, domain.ErrNotFound)
}

// ---------- API JSON para el reproductor ----------

// APIStatus devuelve el estado en vivo: GET /api/status
func (h *Handler) APIStatus(w http.ResponseWriter, r *http.Request) {
	st := h.Media.Status(r.Context())
	resp := map[string]any{"radio": st.Radio, "tv": st.TV, "on_air": nil}
	if p, err := h.Programs.OnAir(r.Context(), time.Now()); err == nil && p != nil {
		resp["on_air"] = map[string]string{"title": p.Title, "host": p.Host, "start": p.StartTime, "end": p.EndTime,
			"image": p.ImageURL}
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, resp)
}

// APIPreroll elige un pre-roll de video o una cuña de audio:
// GET /api/ads/preroll?kind=video|audio → 200 JSON o 204 si no hay.
func (h *Handler) APIPreroll(w http.ResponseWriter, r *http.Request) {
	t := domain.AdPrerollVideo
	if r.URL.Query().Get("kind") == "audio" {
		t = domain.AdAudioSpot
	}
	ad, err := h.Ads.Pick(r.Context(), t)
	if err != nil {
		h.Log.Error("elegir pre-roll", "err", err)
	}
	w.Header().Set("Cache-Control", "no-store")
	if ad == nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	set, _ := h.Settings.Get(r.Context())
	writeJSON(w, http.StatusOK, map[string]any{
		"id": ad.ID, "url": ad.FileURL, "skip_after": ad.SkipAfterSec, "has_link": ad.LinkURL != "",
		"click_url": "/ad/click/" + itoa(ad.ID), "interval_min": set.PrerollIntervalMin,
	})
}

// APIEvent registra un evento de anuncio: POST /api/ads/event {"ad_id":1,"type":"impression"}.
// Lo usa navigator.sendBeacon, por eso acepta JSON en text/plain.
func (h *Handler) APIEvent(w http.ResponseWriter, r *http.Request) {
	var in struct {
		AdID int64              `json:"ad_id"`
		Type domain.AdEventType `json:"type"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024)).Decode(&in); err != nil || in.AdID <= 0 {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	if in.Type == domain.EventClick { // los clics se registran en /ad/click para no duplicarlos
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if err := h.Ads.RecordEvent(r.Context(), in.AdID, in.Type, middleware.IPFrom(r.Context())); err != nil {
		h.Log.Debug("evento de anuncio rechazado", "err", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// AdClick registra un clic y redirige al anunciante: GET /ad/click/{id}
func (h *Handler) AdClick(w http.ResponseWriter, r *http.Request) {
	ad, err := h.Ads.Get(r.Context(), form.ID(chi.URLParam(r, "id")))
	if err != nil || ad.LinkURL == "" {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}
	if err := h.Ads.RecordEvent(r.Context(), ad.ID, domain.EventClick, middleware.IPFrom(r.Context())); err != nil {
		h.Log.Error("registrar clic", "err", err)
	}
	w.Header().Set("Cache-Control", "no-store")
	http.Redirect(w, r, ad.LinkURL, http.StatusFound)
}

// ---------- Endpoints internos (no expuestos por nginx) ----------

// MediaMTXAuth responde a MediaMTX si acepta una publicación o reproducción:
// POST /internal/mediamtx/auth?token=INTERNAL_TOKEN
func (h *Handler) MediaMTXAuth(w http.ResponseWriter, r *http.Request) {
	if !h.validInternalToken(r.URL.Query().Get("token")) {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	var req gate.TVRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192)).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	ok := h.Gate.AuthorizeTV(r.Context(), req)
	if req.Action == "publish" {
		h.Log.Info("autorización de publicación TV", "ok", ok, "ip", req.IP, "protocol", req.Protocol, "user", req.User)
	}
	if !ok {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	w.WriteHeader(http.StatusOK)
}

// IcecastSourceAuth responde a Icecast (stream_auth) si acepta a la PC emisora:
// POST /internal/icecast/source?token=INTERNAL_TOKEN con el formulario
// action=stream_auth&mount=/radio&user=source&pass=...&ip=...
// Para aceptar, Icecast exige la cabecera "icecast-auth-user: 1".
func (h *Handler) IcecastSourceAuth(w http.ResponseWriter, r *http.Request) {
	if !h.validInternalToken(r.URL.Query().Get("token")) {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	ok := h.Gate.RadioSourceAllowed(r.Context(), r.PostFormValue("pass"))
	h.Log.Info("autorización de fuente de radio", "ok", ok, "ip", r.PostFormValue("ip"), "mount", r.PostFormValue("mount"))
	if ok {
		w.Header().Set("icecast-auth-user", "1")
	} else {
		w.Header().Set("icecast-auth-message", "rechazado")
	}
	w.WriteHeader(http.StatusOK)
}

// RadioGate responde a nginx (auth_request) si deja pasar a un oyente:
// GET /internal/gate/radio → 204 permitido / 403 denegado.
func (h *Handler) RadioGate(w http.ResponseWriter, r *http.Request) {
	if !h.Gate.RadioAllowed(r.Context()) {
		w.WriteHeader(http.StatusForbidden)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) validInternalToken(t string) bool {
	if h.InternalToken == "" { // desarrollo sin token configurado
		return true
	}
	return subtle.ConstantTimeCompare([]byte(t), []byte(h.InternalToken)) == 1
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }
