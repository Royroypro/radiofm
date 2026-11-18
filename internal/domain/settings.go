package domain

import (
	"context"
	"strconv"
)

// SiteSettings son los datos editables de la radio: marca, contacto, redes y
// streams. Se guardan como pares clave/valor (tabla settings) para poder
// añadir campos sin migraciones; ToMap/SiteSettingsFromMap hacen la conversión.
//
// Para añadir un ajuste nuevo:
//  1. Añadir el campo aquí y su clave en settingKeys.
//  2. Darle valor por defecto en DefaultSiteSettings.
//  3. Leerlo en el apply de su apartado (transport/httpserver/admin/settings.go) y añadir el
//     input en web/templates/admin/settings_<apartado>.html (brand, contact, app o ads).
type SiteSettings struct {
	// Marca
	SiteName     string
	Frequency    string
	Slogan       string
	Categories   string // separadas por coma: "Música, Noticias, ..."
	About        string
	LogoURL      string
	FaviconURL   string
	HeroImageURL string
	ColorNavy    string
	ColorRoyal   string
	ColorRed     string
	ColorOrange  string

	// Contacto
	Phone       string
	WhatsApp    string
	Email       string
	Address     string
	MapURL      string
	OfficeHours string

	// Redes sociales
	Facebook  string
	Instagram string
	TikTok    string
	YouTube   string
	X         string

	// Streams
	RadioEnabled   bool
	TVEnabled      bool
	RadioStreamURL string // URL pública del audio (normalmente /radio)
	TVStreamURL    string // URL pública del HLS (normalmente /hls/tv/index.m3u8)
	TVPublishUser  string // usuario que debe usar OBS para publicar el video
	TVPublishPass  string // clave que debe usar OBS para publicar el video

	// Publicidad
	PrerollIntervalMin int // minutos mínimos entre dos pre-roll/cuñas para el mismo oyente

	// App Android (los lee /api/app/config al abrir la app)
	AppNotice       string // aviso que la app muestra arriba (vacío = ninguno)
	AppNoticeURL    string // enlace opcional del aviso
	AppMinVersion   int    // versionCode mínimo; las versiones anteriores piden actualizar (0 = sin mínimo)
	AppPlayStoreURL string // ficha de la app en Google Play (para «Actualizar» y «Compartir»)
	AppPrivacyURL   string // política de privacidad (la pide Google Play)
	AppAPKURL       string // APK para descarga directa: subida al panel (/uploads/app/…) o enlace https
	AppAPKVersion   string // versión del APK que se muestra en la web ("1.0.0"), opcional
	// Colores de la app, aparte de los de la web (#RRGGBB)
	AppColorBg     string // fondo, arriba del degradado
	AppColorBg2    string // fondo, abajo del degradado
	AppColorCard   string // tarjetas de radio y TV
	AppColorAccent string // avisos, «Señal externa», detalles
	AppColorPlay   string // botón de play
	AppColorText   string // textos
}

// settingKeys enumera las claves en el orden en que se guardan.
var settingKeys = []string{
	"site_name", "frequency", "slogan", "categories", "about", "logo_url", "favicon_url", "hero_image_url",
	"color_navy", "color_royal", "color_red", "color_orange",
	"phone", "whatsapp", "email", "address", "map_url", "office_hours",
	"facebook", "instagram", "tiktok", "youtube", "x",
	"radio_enabled", "tv_enabled", "radio_stream_url", "tv_stream_url", "tv_publish_user", "tv_publish_pass",
	"preroll_interval_min",
	"app_notice", "app_notice_url", "app_min_version", "app_play_store_url", "app_privacy_url",
	"app_apk_url", "app_apk_version",
	"app_color_bg", "app_color_bg2", "app_color_card", "app_color_accent", "app_color_play", "app_color_text",
}

// DefaultSiteSettings devuelve los valores iniciales (marca provisional Radio Club 25).
func DefaultSiteSettings() SiteSettings {
	return SiteSettings{
		SiteName:           "Radio Club 25",
		Frequency:          "93.3 FM",
		Slogan:             "La música que va contigo",
		Categories:         "Música, Noticias, Éxitos, Retro, Entretenimiento",
		About:              "Radio Club 25 te acompaña las 24 horas con la mejor música, noticias y entretenimiento.",
		LogoURL:            "/static/img/brand/logo.svg",
		FaviconURL:         "/static/img/brand/isotipo.svg",
		ColorNavy:          "#0A1A6B",
		ColorRoyal:         "#223CB8",
		ColorRed:           "#FF1E1E",
		ColorOrange:        "#FF961E",
		AppColorBg:         "#3A0A4B",
		AppColorBg2:        "#1E0526",
		AppColorCard:       "#6A1B9A",
		AppColorAccent:     "#FF961E",
		AppColorPlay:       "#FF1E1E",
		AppColorText:       "#FFFFFF",
		RadioEnabled:       true,
		TVEnabled:          true,
		RadioStreamURL:     "/radio",
		TVStreamURL:        "/hls/tv/index.m3u8",
		TVPublishUser:      "emisora",
		PrerollIntervalMin: 30,
	}
}

// ToMap convierte los ajustes al formato clave/valor de la base.
func (s SiteSettings) ToMap() map[string]string {
	b := func(v bool) string { return strconv.FormatBool(v) }
	return map[string]string{
		"site_name": s.SiteName, "frequency": s.Frequency, "slogan": s.Slogan, "categories": s.Categories,
		"about": s.About, "logo_url": s.LogoURL, "favicon_url": s.FaviconURL, "hero_image_url": s.HeroImageURL,
		"color_navy": s.ColorNavy, "color_royal": s.ColorRoyal, "color_red": s.ColorRed, "color_orange": s.ColorOrange,
		"phone": s.Phone, "whatsapp": s.WhatsApp, "email": s.Email, "address": s.Address, "map_url": s.MapURL,
		"office_hours": s.OfficeHours,
		"facebook":     s.Facebook, "instagram": s.Instagram, "tiktok": s.TikTok, "youtube": s.YouTube, "x": s.X,
		"radio_enabled": b(s.RadioEnabled), "tv_enabled": b(s.TVEnabled),
		"radio_stream_url": s.RadioStreamURL, "tv_stream_url": s.TVStreamURL,
		"tv_publish_user": s.TVPublishUser, "tv_publish_pass": s.TVPublishPass,
		"preroll_interval_min": strconv.Itoa(s.PrerollIntervalMin),
		"app_notice":           s.AppNotice, "app_notice_url": s.AppNoticeURL,
		"app_min_version":    strconv.Itoa(s.AppMinVersion),
		"app_play_store_url": s.AppPlayStoreURL, "app_privacy_url": s.AppPrivacyURL,
		"app_apk_url": s.AppAPKURL, "app_apk_version": s.AppAPKVersion,
		"app_color_bg": s.AppColorBg, "app_color_bg2": s.AppColorBg2, "app_color_card": s.AppColorCard,
		"app_color_accent": s.AppColorAccent, "app_color_play": s.AppColorPlay, "app_color_text": s.AppColorText,
	}
}

// SiteSettingsFromMap construye los ajustes desde la base. Las claves que
// falten toman el valor por defecto.
func SiteSettingsFromMap(m map[string]string) SiteSettings {
	s := DefaultSiteSettings()
	str := func(key string, dst *string) {
		if v, ok := m[key]; ok {
			*dst = v
		}
	}
	boolean := func(key string, dst *bool) {
		if v, ok := m[key]; ok {
			if p, err := strconv.ParseBool(v); err == nil {
				*dst = p
			}
		}
	}
	str("site_name", &s.SiteName)
	str("frequency", &s.Frequency)
	str("slogan", &s.Slogan)
	str("categories", &s.Categories)
	str("about", &s.About)
	str("logo_url", &s.LogoURL)
	str("favicon_url", &s.FaviconURL)
	str("hero_image_url", &s.HeroImageURL)
	str("color_navy", &s.ColorNavy)
	str("color_royal", &s.ColorRoyal)
	str("color_red", &s.ColorRed)
	str("color_orange", &s.ColorOrange)
	str("phone", &s.Phone)
	str("whatsapp", &s.WhatsApp)
	str("email", &s.Email)
	str("address", &s.Address)
	str("map_url", &s.MapURL)
	str("office_hours", &s.OfficeHours)
	str("facebook", &s.Facebook)
	str("instagram", &s.Instagram)
	str("tiktok", &s.TikTok)
	str("youtube", &s.YouTube)
	str("x", &s.X)
	boolean("radio_enabled", &s.RadioEnabled)
	boolean("tv_enabled", &s.TVEnabled)
	str("radio_stream_url", &s.RadioStreamURL)
	str("tv_stream_url", &s.TVStreamURL)
	str("tv_publish_user", &s.TVPublishUser)
	str("tv_publish_pass", &s.TVPublishPass)
	if v, ok := m["preroll_interval_min"]; ok {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			s.PrerollIntervalMin = n
		}
	}
	str("app_notice", &s.AppNotice)
	str("app_notice_url", &s.AppNoticeURL)
	if v, ok := m["app_min_version"]; ok {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			s.AppMinVersion = n
		}
	}
	str("app_play_store_url", &s.AppPlayStoreURL)
	str("app_privacy_url", &s.AppPrivacyURL)
	str("app_apk_url", &s.AppAPKURL)
	str("app_apk_version", &s.AppAPKVersion)
	str("app_color_bg", &s.AppColorBg)
	str("app_color_bg2", &s.AppColorBg2)
	str("app_color_card", &s.AppColorCard)
	str("app_color_accent", &s.AppColorAccent)
	str("app_color_play", &s.AppColorPlay)
	str("app_color_text", &s.AppColorText)
	return s
}

// SettingKeys devuelve la lista de claves conocidas (útil para tests y docs).
func SettingKeys() []string { return append([]string(nil), settingKeys...) }

// SettingsRepository persiste los ajustes como pares clave/valor.
type SettingsRepository interface {
	GetAll(ctx context.Context) (map[string]string, error)
	SetMany(ctx context.Context, values map[string]string) error
}
