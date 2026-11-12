package admin

import (
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"radioclub25/internal/domain"
	"radioclub25/internal/transport/httpserver/form"
	"radioclub25/internal/transport/httpserver/middleware"
	"radioclub25/internal/transport/httpserver/views"
)

// Control remoto de la PC del estudio (editor o superior): ver lo que suena y lo
// que viene, y mandar órdenes (siguiente, iniciar, detener, modo, mover, quitar,
// TV). La PC las pide cada 5 s, las ejecuta y responde; el estado se actualiza
// con su próxima señal (cada 30 s, o al momento después de una orden).

// ControlPage muestra el control de una frecuencia: GET /admin/control y /admin/control/{fid}
func (h *Handler) ControlPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	freqs, err := h.Frequencies.List(ctx)
	if err != nil {
		h.Views.Error(w, r, err)
		return
	}
	states, err := h.Studio.States(ctx)
	if err != nil {
		h.Views.Error(w, r, err)
		return
	}
	var withPC []domain.Frequency
	for _, f := range freqs {
		if st := states[f.ID]; st != nil && st.HasToken() {
			withPC = append(withPC, f)
		}
	}
	if len(withPC) == 0 {
		h.Views.Render(w, r, http.StatusOK, "admin/control", "Control remoto", map[string]any{"Freqs": withPC})
		return
	}
	cur := withPC[0]
	if fid := chi.URLParam(r, "fid"); fid != "" {
		id, _ := strconv.ParseInt(fid, 10, 64)
		for _, f := range withPC {
			if f.ID == id {
				cur = f
			}
		}
	}
	st := states[cur.ID]
	now := time.Now()
	cmds, _ := h.Studio.RecentCommands(ctx, cur.ID)
	h.Views.Render(w, r, http.StatusOK, "admin/control", "Control remoto", map[string]any{
		"Freqs": withPC, "Freq": cur, "State": st, "Report": st.Report(), "Online": st.Online(now, studioOnline),
		"Alarms": st.Alarms(now, studioOnline), "Commands": cmds,
	})
}

// ControlCommand encola una orden: POST /admin/control/{fid}  (kind, arg)
func (h *Handler) ControlCommand(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	f, err := h.Frequencies.Get(ctx, form.ID(chi.URLParam(r, "fid")))
	if err != nil {
		h.Views.Error(w, r, err)
		return
	}
	back := "/admin/control/" + strconv.FormatInt(f.ID, 10)
	c, err := h.Studio.SendCommand(ctx, middleware.ActorFrom(r), f.ID, r.PostFormValue("kind"), r.PostFormValue("arg"))
	if err != nil {
		views.Redirect(w, r, back, "error", err.Error())
		return
	}
	views.Redirect(w, r, back, "ok", "Orden «"+c.Label()+"» enviada a la PC de "+f.City+": se ejecuta en unos segundos.")
}
