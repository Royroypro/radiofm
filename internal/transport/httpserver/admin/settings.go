package admin

import (
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"

	"radioclub25/internal/domain"
	"radioclub25/internal/platform/storage"
	"radioclub25/internal/service/settings"
	"radioclub25/internal/transport/httpserver/form"
	"radioclub25/internal/transport/httpserver/middleware"
	"radioclub25/internal/transport/httpserver/views"
)

// Ajustes del sitio, separados por apartado (cada uno es una página del panel
// con su propio formulario; guardar uno no toca los demás):
//
//	brand    Personalización → Marca e identidad (nombre, textos, imágenes, colores de la web)
//	contact  Personalización → Contacto y redes
//	app      App Android (aviso, versión mínima, Google Play, colores de la app, APK)
//	ads      Publicidad → Ajustes de publicidad (frecuencia de pre-roll/cuñas web)
//
// Los de emisión están en Streams (/admin/streams) y las claves en Tokens y
// claves (/admin/access). Para añadir un campo: domain/settings.go, el apply
// del apartado aquí y el input en web/templates/admin/settings_<apartado>.html.

// settingsSection describe un apartado de ajustes.
type settingsSection struct {
	Title string
	apply func(h *Handler, r *http.Request, s *domain.SiteSettings) error // lee sus campos del formulario
}

var settingsSections = map[string]settingsSection{
	"brand":   {Title: "Marca e identidad", apply: applyBrand},
	"contact": {Title: "Contacto y redes", apply: applyContact},
	"app":     {Title: "App Android", apply: applyApp},
	"ads":     {Title: "Ajustes de publicidad", apply: applyAds},
}

// settingsOrder es el orden en que se aplican todos (POST /admin/settings).
var settingsOrder = []string{"brand", "contact", "ads", "app"}

func applyBrand(h *Handler, r *http.Request, s *domain.SiteSettings) error {
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
	for field, dst := range map[string]*string{"logo_file": &s.LogoURL, "favicon_file": &s.FaviconURL, "hero_file": &s.HeroImageURL} {
		url, err := h.upload(r, field, storage.KindImage, "brand")
		if err != nil {
			return err
		}
		if url != "" {
			*dst = url
		}
	}
	return nil
}

func applyContact(_ *Handler, r *http.Request, s *domain.SiteSettings) error {
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
	return nil
}

func applyAds(_ *Handler, r *http.Request, s *domain.SiteSettings) error {
	s.PrerollIntervalMin = form.Int(r, "preroll_interval_min")
	return nil
}

func applyApp(h *Handler, r *http.Request, s *domain.SiteSettings) error {
	s.AppNotice = form.String(r, "app_notice")
	s.AppNoticeURL = form.String(r, "app_notice_url")
	s.AppMinVersion = form.Int(r, "app_min_version")
	s.AppPlayStoreURL = form.String(r, "app_play_store_url")
	s.AppPrivacyURL = form.String(r, "app_privacy_url")
	s.AppAPKURL = form.String(r, "app_apk_url")
	s.AppAPKVersion = form.String(r, "app_apk_version")
	s.AppColorBg = form.String(r, "app_color_bg")
	s.AppColorBg2 = form.String(r, "app_color_bg2")
	s.AppColorCard = form.String(r, "app_color_card")
	s.AppColorAccent = form.String(r, "app_color_accent")
	s.AppColorPlay = form.String(r, "app_color_play")
	s.AppColorText = form.String(r, "app_color_text")
	apk, err := h.upload(r, "app_apk_file", storage.KindAPK, "app")
	if err != nil {
		return err
	}
	if apk != "" {
		s.AppAPKURL = apk
	}
	return nil
}

func (h *Handler) settingsData(s domain.SiteSettings) map[string]any {
	return map[string]any{"S": s, "Defaults": domain.DefaultSiteSettings(), "MaxUploadMB": h.maxUploadMB()}
}

// SettingsIndex lleva al primer apartado: GET /admin/settings
func (h *Handler) SettingsIndex(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/admin/settings/brand", http.StatusSeeOther)
}

// SettingsForm muestra un apartado: GET /admin/settings/{section}
func (h *Handler) SettingsForm(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "section")
	sec, ok := settingsSections[name]
	if !ok {
		h.Views.Error(w, r, domain.ErrNotFound)
		return
	}
	s, err := h.Settings.Get(r.Context())
	if err != nil {
		h.Views.Error(w, r, err)
		return
	}
	h.Views.Render(w, r, http.StatusOK, "admin/settings_"+name, sec.Title, h.settingsData(s))
}

// SettingsSave guarda solo los campos de un apartado: POST /admin/settings/{section}
func (h *Handler) SettingsSave(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "section")
	if _, ok := settingsSections[name]; !ok {
		h.Views.Error(w, r, domain.ErrNotFound)
		return
	}
	h.saveSettings(w, r, name, []string{name})
}

// SettingsSaveAll guarda todos los apartados a la vez: POST /admin/settings.
// Se mantiene por compatibilidad (scripts y tests); el panel usa un formulario por apartado.
func (h *Handler) SettingsSaveAll(w http.ResponseWriter, r *http.Request) {
	h.saveSettings(w, r, "brand", settingsOrder)
}

func (h *Handler) saveSettings(w http.ResponseWriter, r *http.Request, page string, sections []string) {
	ctx := r.Context()
	s, err := h.Settings.Get(ctx)
	if err != nil {
		h.Views.Error(w, r, err)
		return
	}
	title, tpl := settingsSections[page].Title, "admin/settings_"+page
	for _, name := range sections {
		if err := settingsSections[name].apply(h, r, &s); err != nil {
			h.formError(w, r, tpl, title, h.settingsData(s), err)
			return
		}
	}
	if err := h.Settings.Update(ctx, middleware.ActorFrom(r), s); err != nil {
		// muestra el apartado del campo con error
		if ve := views.AsValidation(err); ve != nil {
			if sec := settingsFieldSection(ve.Field); sec != "" {
				title, tpl = settingsSections[sec].Title, "admin/settings_"+sec
			}
		}
		h.formError(w, r, tpl, title, h.settingsData(s), err)
		return
	}
	views.Redirect(w, r, "/admin/settings/"+page, "ok", "Cambios guardados.")
}

// settingsFieldSection dice en qué apartado está un campo (para mostrar el error ahí).
func settingsFieldSection(field string) string {
	switch {
	case len(field) > 4 && field[:4] == "app_":
		return "app"
	case field == "preroll_interval_min":
		return "ads"
	}
	for _, f := range []string{"phone", "whatsapp", "email", "address", "map_url", "office_hours", "facebook",
		"instagram", "tiktok", "youtube", "x"} {
		if f == field {
			return "contact"
		}
	}
	return "brand"
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
	h.Views.Render(w, r, http.StatusOK, "admin/streams", "Streams (radio y TV)", map[string]any{
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
	if r.PostForm.Has("tv_publish_user") { // el usuario y la clave de TV se cambian en Tokens y claves
		s.TVPublishUser = form.String(r, "tv_publish_user")
	}
	if form.Bool(r, "regenerate_key") {
		s.TVPublishPass = settings.RandomKey()
	}
	if err := h.Settings.Update(ctx, middleware.ActorFrom(r), s); err != nil {
		h.formError(w, r, "admin/streams", "Streams (radio y TV)", map[string]any{
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
