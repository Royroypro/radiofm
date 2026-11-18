package superadmin

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"radioclub25/internal/domain"
	"radioclub25/internal/service/emisor"
	"radioclub25/internal/transport/httpserver/form"
	"radioclub25/internal/transport/httpserver/middleware"
	"radioclub25/internal/transport/httpserver/views"
)

// releaseRow es una versión lista para mostrar.
type releaseRow struct {
	domain.EmisorRelease
	SizeMB   string
	ShortSHA string
}

// emisorConfigRow es una frecuencia con sus copias de configuración del emisor.
type emisorConfigRow struct {
	Label string
	Count int
}

func (h *Handler) emisorData(r *http.Request, prov domain.ProviderInfo) (map[string]any, error) {
	ctx := r.Context()
	list, err := h.Emisor.Releases(ctx)
	if err != nil {
		return nil, err
	}
	rels := make([]releaseRow, 0, len(list))
	for _, e := range list {
		short := e.SHA256
		if len(short) > 16 {
			short = short[:16]
		}
		rels = append(rels, releaseRow{EmisorRelease: e, SizeMB: fmt.Sprintf("%.1f", float64(e.Size)/(1<<20)), ShortSHA: short})
	}
	var rows []emisorConfigRow
	if freqs, err := h.Frequencies.List(ctx); err == nil {
		for _, f := range freqs {
			rows = append(rows, emisorConfigRow{Label: f.Label(), Count: h.Emisor.ConfigCount(f.ID)})
		}
	}
	return map[string]any{"Provider": prov, "Releases": rels, "Configs": rows, "MaxMB": emisor.MaxReleaseBytes >> 20,
		"Default": domain.DefaultProviderInfo()}, nil
}

// EmisorPage muestra el contacto del proveedor y las versiones del programa: GET /superadmin/emisor
func (h *Handler) EmisorPage(w http.ResponseWriter, r *http.Request) {
	prov, err := h.Emisor.Provider(r.Context())
	if err != nil {
		h.Views.Error(w, r, err)
		return
	}
	data, err := h.emisorData(r, prov)
	if err != nil {
		h.Views.Error(w, r, err)
		return
	}
	h.Views.Render(w, r, http.StatusOK, "superadmin/emisor", "Club25 Emisor", data)
}

// EmisorProviderSave guarda el contacto del proveedor: POST /superadmin/emisor/provider
func (h *Handler) EmisorProviderSave(w http.ResponseWriter, r *http.Request) {
	p := domain.ProviderInfo{Phone: form.String(r, "phone"), URL: form.String(r, "url")}
	if err := h.Emisor.SaveProvider(r.Context(), middleware.ActorFrom(r), p); err != nil {
		data, derr := h.emisorData(r, p)
		if derr != nil {
			h.Views.Error(w, r, derr)
			return
		}
		h.formError(w, r, "superadmin/emisor", "Club25 Emisor", data, err)
		return
	}
	views.Redirect(w, r, "/superadmin/emisor", "ok", "Contacto del proveedor guardado.")
}

// EmisorReleaseUpload publica una versión nueva: POST /superadmin/emisor/releases (multipart)
func (h *Handler) EmisorReleaseUpload(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, emisor.MaxReleaseBytes+1<<20)
	fail := func(err error) {
		prov, _ := h.Emisor.Provider(r.Context())
		data, derr := h.emisorData(r, prov)
		if derr != nil {
			h.Views.Error(w, r, derr)
			return
		}
		h.formError(w, r, "superadmin/emisor", "Club25 Emisor", data, err)
	}
	if err := r.ParseMultipartForm(8 << 20); err != nil {
		fail(domain.Invalid("file", "No se pudo recibir el archivo (¿pasa del tamaño máximo?)."))
		return
	}
	file, _, err := r.FormFile("file")
	if err != nil {
		fail(domain.Invalid("file", "Elija el archivo club25-emisor.exe."))
		return
	}
	defer file.Close()
	rel, err := h.Emisor.Publish(r.Context(), middleware.ActorFrom(r), r.FormValue("version"), r.FormValue("notes"), file)
	if err != nil {
		fail(err)
		return
	}
	views.Redirect(w, r, "/superadmin/emisor", "ok", "Versión "+rel.Version+" publicada: los emisores se actualizarán al abrir sus opciones.")
}

// EmisorReleaseDelete borra una versión: POST /superadmin/emisor/releases/{id}/delete
func (h *Handler) EmisorReleaseDelete(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	err := h.Emisor.DeleteRelease(r.Context(), middleware.ActorFrom(r), id)
	h.redirectResult(w, r, "/superadmin/emisor", "Versión borrada.", err)
}
