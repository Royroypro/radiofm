package admin

import (
	"fmt"
	"net/http"

	"radioclub25/internal/domain"
	"radioclub25/internal/platform/storage"
	"radioclub25/internal/service/settings"
	"radioclub25/internal/transport/httpserver/form"
	"radioclub25/internal/transport/httpserver/middleware"
	"radioclub25/internal/transport/httpserver/views"
)

// SettingsForm muestra los ajustes de marca y contacto: GET /admin/settings
func (h *Handler) SettingsForm(w http.ResponseWriter, r *http.Request) {
	s, err := h.Settings.Get(r.Context())
	if err != nil {
		h.Views.Error(w, r, err)
		return
	}
	h.Views.Render(w, r, http.StatusOK, "admin/settings", "Marca y contacto", h.settingsData(s))
}

func (h *Handler) settingsData(s domain.SiteSettings) map[string]any {
	return map[string]any{"S": s, "Defaults": domain.DefaultSiteSettings(), "MaxUploadMB": h.maxUploadMB()}
}

// SettingsSave guarda marca, contacto y redes: POST /admin/settings
// Los campos de streams se editan en /admin/streams y aquí se conservan.
func (h *Handler) SettingsSave(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	s, err := h.Settings.Get(ctx)
	if err != nil {
		h.Views.Error(w, r, err)
		return
	}
	s.SiteName = form.String(r, "site_name")
	s.Frequency = form.String(r, "frequency")
	s.Slogan = form.String(r, "slogan")
	s.Categories = form.String(r, "categories")
	s.About = form.String(r, "about")
	s.LogoURL = form.String(r, "logo_url")
	s.FaviconURL = form.String(r, "favicon_url")
	s.HeroImageURL = form.String(r, "hero_image_url")
	s.ColorNavy = form.String(r, "color_navy")
	s.ColorRoyal = form.String(r, "color_royal")
	s.ColorRed = form.String(r, "color_red")
	s.ColorOrange = form.String(r, "color_orange")
	s.Phone = form.String(r, "phone")
	s.WhatsApp = form.String(r, "whatsapp")
	s.Email = form.String(r, "email")
	s.Address = form.String(r, "address")
	s.MapURL = form.String(r, "map_url")
	s.OfficeHours = form.String(r, "office_hours")
	s.Facebook = form.String(r, "facebook")
	s.Instagram = form.String(r, "instagram")
	s.TikTok = form.String(r, "tiktok")
	s.YouTube = form.String(r, "youtube")
	s.X = form.String(r, "x")
	s.PrerollIntervalMin = form.Int(r, "preroll_interval_min")

	for field, dst := range map[string]*string{"logo_file": &s.LogoURL, "favicon_file": &s.FaviconURL, "hero_file": &s.HeroImageURL} {
		url, err := h.upload(r, field, storage.KindImage, "brand")
		if err != nil {
			h.formError(w, r, "admin/settings", "Marca y contacto", h.settingsData(s), err)
			return
		}
		if url != "" {
			*dst = url
		}
	}
	if err := h.Settings.Update(ctx, middleware.ActorFrom(r), s); err != nil {
		h.formError(w, r, "admin/settings", "Marca y contacto", h.settingsData(s), err)
		return
	}
	views.Redirect(w, r, "/admin/settings", "ok", "Ajustes guardados.")
}

// StreamInfo son los datos que necesita la persona que configura la PC emisora.
type StreamInfo struct {
	IcecastHost, IcecastMount, IcecastUser, IcecastPass string
	IcecastPort                                         int
	RTMPServer, RTMPKey, RTMPFull                       string
	SRTURL                                              string
	PublicRadioURL, PublicTVURL                         string
}

// streamInfo arma las URLs de ingesta para BUTT/OBS.
func (h *Handler) streamInfo(s domain.SiteSettings) StreamInfo {
	c := h.Cfg
	return StreamInfo{
		IcecastHost: c.IngestHost, IcecastPort: c.IcecastPublicPt, IcecastMount: c.IcecastMount,
		IcecastUser: "source", IcecastPass: c.IcecastSourcePass,
		RTMPServer: fmt.Sprintf("rtmp://%s:%d", c.IngestHost, c.RTMPPublicPort),
		RTMPKey:    fmt.Sprintf("%s?user=%s&pass=%s", c.MediaMTXPath, s.TVPublishUser, s.TVPublishPass),
		RTMPFull: fmt.Sprintf("rtmp://%s:%d/%s?user=%s&pass=%s", c.IngestHost, c.RTMPPublicPort, c.MediaMTXPath,
			s.TVPublishUser, s.TVPublishPass),
		SRTURL: fmt.Sprintf("srt://%s:%d?streamid=publish:%s:%s:%s&pkt_size=1316", c.IngestHost, c.SRTPublicPort,
			c.MediaMTXPath, s.TVPublishUser, s.TVPublishPass),
		PublicRadioURL: c.BaseURL + s.RadioStreamURL,
		PublicTVURL:    c.BaseURL + s.TVStreamURL,
	}
}

// Streams muestra el estado y la configuración de emisión: GET /admin/streams
func (h *Handler) Streams(w http.ResponseWriter, r *http.Request) {
	s, err := h.Settings.Get(r.Context())
	if err != nil {
		h.Views.Error(w, r, err)
		return
	}
	h.Views.Render(w, r, http.StatusOK, "admin/streams", "Streams y emisión", map[string]any{
		"S": s, "Info": h.streamInfo(s), "Status": h.Media.Status(r.Context()),
		"TVPlanEnabled": h.License.Current().Limits.TVEnabled,
	})
}

// StreamsSave guarda los ajustes de streams: POST /admin/streams
func (h *Handler) StreamsSave(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	s, err := h.Settings.Get(ctx)
	if err != nil {
		h.Views.Error(w, r, err)
		return
	}
	s.RadioEnabled = form.Bool(r, "radio_enabled")
	s.TVEnabled = form.Bool(r, "tv_enabled")
	s.TVPublishUser = form.String(r, "tv_publish_user")
	if form.Bool(r, "regenerate_key") {
		s.TVPublishPass = settings.RandomKey()
	}
	if err := h.Settings.Update(ctx, middleware.ActorFrom(r), s); err != nil {
		h.formError(w, r, "admin/streams", "Streams y emisión", map[string]any{
			"S": s, "Info": h.streamInfo(s), "Status": h.Media.Status(ctx),
			"TVPlanEnabled": h.License.Current().Limits.TVEnabled,
		}, err)
		return
	}
	msg := "Ajustes de emisión guardados."
	if form.Bool(r, "regenerate_key") {
		msg += " Se generó una clave nueva: actualícela en OBS."
	}
	views.Redirect(w, r, "/admin/streams", "ok", msg)
}
