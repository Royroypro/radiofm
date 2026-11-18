package studioapi

import (
	"io"
	"net/http"
	"os"
	"strconv"
	"time"

	"radioclub25/internal/domain"
	"radioclub25/internal/service/emisor"
	"radioclub25/internal/service/studio"
	"radioclub25/internal/transport/httpserver/middleware"
)

// Endpoints de Club25 Emisor (emisor/). Usan el mismo token que Club25 Estudio
// (Admin → Estudio (PC)); el contrato está en docs/api.md → "Club25 Emisor".

// EmisorProvider devuelve el contacto del proveedor: GET /api/emisor/provider
// (público, sin token: el programa lo muestra cuando NO está autenticado).
func (h *Handler) EmisorProvider(w http.ResponseWriter, r *http.Request) {
	p, err := h.Emisor.Provider(r.Context())
	if err != nil {
		h.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, p)
}

// EmisorInfo autentica al emisor y le dice qué versión hay: GET /api/studio/emisor
// → {"ok":true,"station":{…},"provider":{…},"latest":{"version":"1.5.0","sha256":"…","size":123,"notes":"…"}}
func (h *Handler) EmisorInfo(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	set, err := h.Settings.Get(ctx)
	if err != nil {
		h.fail(w, err)
		return
	}
	f, err := h.Frequencies.Get(ctx, middleware.StudioFrequencyFrom(ctx))
	if err != nil {
		h.fail(w, err)
		return
	}
	prov, err := h.Emisor.Provider(ctx)
	if err != nil {
		h.fail(w, err)
		return
	}
	out := map[string]any{
		"ok":       true,
		"station":  map[string]any{"name": set.SiteName, "frequency": f.Label(), "frequency_id": f.ID, "city": f.City},
		"provider": prov,
		"latest":   nil,
	}
	if rel, err := h.Emisor.Latest(ctx); err != nil {
		h.fail(w, err)
		return
	} else if rel != nil {
		out["latest"] = map[string]any{"version": rel.Version, "sha256": rel.SHA256, "size": rel.Size, "notes": rel.Notes,
			"published_at": rel.CreatedAt.UTC().Format(time.RFC3339)}
	}
	writeJSON(w, http.StatusOK, out)
}

// EmisorDownload entrega el .exe de la última versión: GET /api/studio/emisor/download
func (h *Handler) EmisorDownload(w http.ResponseWriter, r *http.Request) {
	rel, err := h.Emisor.Latest(r.Context())
	if err != nil {
		h.fail(w, err)
		return
	}
	if rel == nil {
		h.fail(w, domain.ErrNotFound)
		return
	}
	f, err := os.Open(h.Emisor.ReleasePath(rel))
	if err != nil {
		h.fail(w, domain.ErrNotFound)
		return
	}
	defer f.Close()
	w.Header().Set("Content-Type", "application/vnd.microsoft.portable-executable")
	w.Header().Set("Content-Disposition", `attachment; filename="`+emisor.ReleaseFileName+`"`)
	w.Header().Set("X-Emisor-Version", rel.Version)
	w.Header().Set("X-Emisor-SHA256", rel.SHA256)
	w.Header().Set("Cache-Control", "no-store")
	http.ServeContent(w, r, emisor.ReleaseFileName, rel.CreatedAt, f)
}

// EmisorConfigPut guarda la copia de la configuración de esta PC (sin claves):
// PUT /api/studio/emisor/config (cuerpo JSON) → 204
func (h *Handler) EmisorConfigPut(w http.ResponseWriter, r *http.Request) {
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, emisor.MaxConfigBytes+1))
	if err != nil {
		h.fail(w, domain.Invalid("config", "La configuración es demasiado grande."))
		return
	}
	if err := h.Emisor.SaveConfig(middleware.StudioFrequencyFrom(r.Context()), raw); err != nil {
		h.fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// EmisorConfigGet devuelve la última copia guardada: GET /api/studio/emisor/config
// (404 si esta frecuencia todavía no tiene copia). Cabecera X-Saved-At con la fecha.
func (h *Handler) EmisorConfigGet(w http.ResponseWriter, r *http.Request) {
	b, ts, err := h.Emisor.LatestConfig(middleware.StudioFrequencyFrom(r.Context()))
	if err != nil {
		h.fail(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Saved-At", ts.UTC().Format(time.RFC3339))
	w.Header().Set("Content-Length", strconv.Itoa(len(b)))
	_, _ = w.Write(b)
}

// EmisorHeartbeat recibe la señal de vida de Club25 Emisor:
// POST /api/studio/emisor/heartbeat {"version":"1.5.0","status":{"radio":{…},"tv":{…},"now_playing":"…"}}
// → 200 {"latest":"1.5.3"} (versión publicada más alta, "" si no hay): así el emisor
// se entera de una actualización en un minuto, sin esperar la autenticación periódica.
func (h *Handler) EmisorHeartbeat(w http.ResponseWriter, r *http.Request) {
	var in studio.EmisorHeartbeatInput
	if !decode(w, r, &in, 16<<10) {
		return
	}
	if err := h.Studio.EmisorHeartbeat(r.Context(), middleware.StudioFrequencyFrom(r.Context()), in); err != nil {
		h.fail(w, err)
		return
	}
	latest := ""
	if rel, err := h.Emisor.Latest(r.Context()); err == nil && rel != nil {
		latest = rel.Version
	}
	writeJSON(w, http.StatusOK, map[string]string{"latest": latest})
}
