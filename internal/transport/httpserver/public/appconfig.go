package public

import (
	"net/http"
	"strings"
)

// APIAppConfig da a la app Android todo lo que necesita al abrirse:
// GET /api/app/config. Es de solo lectura y sale de los ajustes (Admin → Marca
// y contacto, Streams) y de las frecuencias activas, así un cambio en el panel
// se ve en la app sin publicar una versión nueva. El estado en vivo (al aire,
// oyentes) no va aquí: la app lo consulta en /api/status.
//
// Las URL relativas ("/radio", "/static/...") se resuelven contra la URL base
// del servidor con la que se compiló la app; las de servidores externos ya
// vienen completas.
func (h *Handler) APIAppConfig(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	s, err := h.Settings.Get(ctx)
	if err != nil {
		h.Log.Error("leer ajustes para la app", "err", err)
	}
	freqs, err := h.Frequencies.Active(ctx)
	if err != nil {
		h.Log.Error("leer frecuencias para la app", "err", err)
	}
	stations := make([]map[string]any, 0, len(freqs))
	if s.RadioEnabled {
		for _, f := range freqs {
			if f.ListenURL() == "" {
				continue
			}
			stations = append(stations, map[string]any{
				"id": f.ID, "label": f.Label(), "city": f.City, "listen_url": f.ListenURL(), "external": f.IsExternal(),
			})
		}
	}
	tvEnabled := s.TVEnabled && h.License.Current().Limits.TVEnabled
	categories := []string{}
	for _, c := range strings.Split(s.Categories, ",") {
		if c = strings.TrimSpace(c); c != "" {
			categories = append(categories, c)
		}
	}
	w.Header().Set("Cache-Control", "no-cache")
	writeJSON(w, http.StatusOK, map[string]any{
		"name": s.SiteName, "slogan": s.Slogan, "about": s.About, "categories": categories,
		"logo_url": s.LogoURL, "icon_url": s.FaviconURL,
		"colors":        map[string]string{"navy": s.ColorNavy, "royal": s.ColorRoyal, "red": s.ColorRed, "orange": s.ColorOrange},
		"radio_enabled": s.RadioEnabled,
		"stations":      stations,
		"tv":            map[string]any{"enabled": tvEnabled, "hls_url": s.TVStreamURL},
		"contact": map[string]string{"phone": s.Phone, "whatsapp": s.WhatsApp, "email": s.Email, "address": s.Address,
			"office_hours": s.OfficeHours},
		"social": map[string]string{"facebook": s.Facebook, "instagram": s.Instagram, "tiktok": s.TikTok,
			"youtube": s.YouTube, "x": s.X},
		"app": map[string]any{"notice": s.AppNotice, "notice_url": s.AppNoticeURL, "min_version": s.AppMinVersion,
			"play_store_url": s.AppPlayStoreURL, "privacy_url": s.AppPrivacyURL},
		"status_url": "/api/status",
	})
}
