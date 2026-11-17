package app_test

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

// appConfig es la parte de /api/app/config que revisan los tests.
type appConfig struct {
	Name         string            `json:"name"`
	Slogan       string            `json:"slogan"`
	LogoURL      string            `json:"logo_url"`
	Colors       map[string]string `json:"colors"`
	RadioEnabled bool              `json:"radio_enabled"`
	Stations     []struct {
		ID        int64  `json:"id"`
		Label     string `json:"label"`
		City      string `json:"city"`
		ListenURL string `json:"listen_url"`
		External  bool   `json:"external"`
	} `json:"stations"`
	TV struct {
		Enabled bool   `json:"enabled"`
		HLSURL  string `json:"hls_url"`
	} `json:"tv"`
	App struct {
		Notice       string `json:"notice"`
		MinVersion   int    `json:"min_version"`
		PlayStoreURL string `json:"play_store_url"`
		PrivacyURL   string `json:"privacy_url"`
	} `json:"app"`
	StatusURL string `json:"status_url"`
}

func (e *env) appConfig() appConfig {
	e.t.Helper()
	res, err := http.Get(e.srv.URL + "/api/app/config") // sin sesión: es público
	if err != nil {
		e.t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK || !strings.HasPrefix(res.Header.Get("Content-Type"), "application/json") {
		e.t.Fatalf("/api/app/config = %d %s", res.StatusCode, res.Header.Get("Content-Type"))
	}
	var c appConfig
	if err := json.NewDecoder(res.Body).Decode(&c); err != nil {
		e.t.Fatal(err)
	}
	return c
}

func TestAppConfig(t *testing.T) {
	e := newEnv(t)

	// valores por defecto: marca provisional, la frecuencia inicial y la TV
	c := e.appConfig()
	if c.Name != "Radio Club 25" || c.Colors["red"] != "#FF1E1E" || c.LogoURL == "" || c.StatusURL != "/api/status" {
		t.Errorf("marca inesperada: %+v", c)
	}
	if !c.RadioEnabled || len(c.Stations) != 1 || c.Stations[0].ListenURL != "/radio" || c.Stations[0].External {
		t.Errorf("frecuencias inesperadas: %+v", c.Stations)
	}
	if !c.TV.Enabled || c.TV.HLSURL != "/hls/tv/index.m3u8" {
		t.Errorf("TV inesperada: %+v", c.TV)
	}
	if c.App.PrivacyURL != "/privacidad" { // sin URL en el panel: la página propia
		t.Errorf("privacy_url por defecto = %q", c.App.PrivacyURL)
	}

	// una frecuencia externa y los ajustes de la app editados desde el panel
	e.login("super@test.pe", "supersecreta")
	if code, body := e.post("/admin/frequencies", url.Values{"city": {"Huaral"}, "frequency": {"105.4"}, "band": {"FM"},
		"source": {"external"}, "stream_url": {"https://stream.ejemplo.pe/huaral"}, "active": {"on"}}); code != http.StatusSeeOther {
		t.Fatalf("crear Huaral = %d %s", code, body)
	}
	set := url.Values{"site_name": {"Club25"}, "slogan": {"Huacho en tu bolsillo"}, "color_navy": {"#000000"},
		"color_royal": {"#111111"}, "color_red": {"#FF0000"}, "color_orange": {"#FF9900"},
		"app_notice": {"Transmisión especial"}, "app_min_version": {"3"},
		"app_play_store_url": {"https://play.google.com/store/apps/details?id=pe.club25peru.radio"}}
	if code, body := e.post("/admin/settings", set); code != http.StatusSeeOther {
		t.Fatalf("guardar ajustes = %d %s", code, body)
	}
	c = e.appConfig()
	if c.Name != "Club25" || c.Slogan != "Huacho en tu bolsillo" || c.Colors["red"] != "#FF0000" {
		t.Errorf("la marca no se actualizó: %+v", c)
	}
	if c.App.Notice != "Transmisión especial" || c.App.MinVersion != 3 || !strings.Contains(c.App.PlayStoreURL, "pe.club25peru.radio") {
		t.Errorf("ajustes de la app no se actualizaron: %+v", c.App)
	}
	if len(c.Stations) != 2 || !c.Stations[1].External || c.Stations[1].ListenURL != "https://stream.ejemplo.pe/huaral" ||
		c.Stations[1].City != "Huaral" || c.Stations[1].Label != "105.4 FM" {
		t.Errorf("frecuencia externa inesperada: %+v", c.Stations)
	}

	// enlaces inválidos se rechazan
	set.Set("app_privacy_url", "privacidad")
	if code, body := e.post("/admin/settings", set); code == http.StatusSeeOther || !strings.Contains(body, "https://") {
		t.Errorf("enlace inválido aceptado: %d", code)
	}

	// radio y TV apagadas en Streams: la app no muestra reproductores
	if code, _ := e.post("/admin/streams", url.Values{"tv_publish_user": {"emisora"}}); code != http.StatusSeeOther {
		t.Fatalf("guardar streams = %d", code)
	}
	c = e.appConfig()
	if c.RadioEnabled || len(c.Stations) != 0 || c.TV.Enabled {
		t.Errorf("con radio y TV apagadas: %+v", c)
	}
}
