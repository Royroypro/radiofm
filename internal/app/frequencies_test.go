package app_test

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"radioclub25/internal/platform/icecast"
)

// Frecuencias por ciudad: alta y validación en el panel, orden, y lo que
// muestran la portada y /api/status.
func TestFrequencies(t *testing.T) {
	e := newEnv(t)
	e.login("super@test.pe", "supersecreta")

	for _, p := range []string{"/admin/frequencies", "/admin/frequencies/new", "/admin/frequencies/1"} {
		if code, body := e.get(p); code != http.StatusOK || strings.Contains(body, "ZgotmplZ") {
			t.Fatalf("GET %s = %d", p, code)
		}
	}
	// Prepare creó la principal: Huacho, 93.3 FM, montaje /radio.
	if _, list := e.get("/admin/frequencies"); !strings.Contains(list, "Huacho") || !strings.Contains(list, "<code>/radio</code>") {
		t.Fatal("debe existir la frecuencia principal con el montaje /radio")
	}
	add := func(v url.Values) (int, string) {
		t.Helper()
		v.Set("band", "FM")
		v.Set("active", "on")
		return e.post("/admin/frequencies", v)
	}
	if code, body := add(url.Values{"city": {"Barranca"}, "frequency": {"103,2"}, "source": {"local"}}); code != http.StatusSeeOther {
		t.Fatalf("crear Barranca = %d %s", code, body)
	}
	if code, body := add(url.Values{"city": {"Huaral"}, "frequency": {"105.4"}, "source": {"local"}}); code != http.StatusSeeOther {
		t.Fatalf("crear Huaral = %d %s", code, body)
	}
	for name, v := range map[string]url.Values{
		"fuera de banda":   {"city": {"Lima"}, "frequency": {"200"}, "source": {"local"}},
		"sin ciudad":       {"city": {""}, "frequency": {"99.1"}, "source": {"local"}},
		"montaje inválido": {"city": {"Lima"}, "frequency": {"99.1"}, "source": {"local"}, "mount": {"/otra"}},
		"montaje repetido": {"city": {"Lima"}, "frequency": {"99.1"}, "source": {"local"}, "mount": {"/radio-barranca"}},
		"URL externa mala": {"city": {"Lima"}, "frequency": {"99.1"}, "source": {"external"}, "stream_url": {"ftp://x"}},
	} {
		if code, _ := add(v); code != http.StatusUnprocessableEntity {
			t.Errorf("%s debe dar 422, dio %d", name, code)
		}
	}

	_, home := e.get("/")
	for _, want := range []string{"93.3 FM", "103.2 FM", "Barranca", "105.4 FM", "Huaral",
		`data-src="/radio"`, `data-src="/radio-barranca"`, `data-src="/radio-huaral"`, "radio-stack multi"} {
		if !strings.Contains(home, want) {
			t.Errorf("la portada no muestra %q", want)
		}
	}

	// subir Barranca al primer lugar: pasa a ser la principal
	if code, _ := e.post("/admin/frequencies/2/up", url.Values{}); code != http.StatusSeeOther {
		t.Fatalf("subir = %d", code)
	}
	if _, home = e.get("/"); !strings.Contains(home, "<title>Radio Club 25 103.2 FM") &&
		!strings.Contains(home, "103.2 FM</title>") {
		t.Error("tras subir Barranca, 103.2 FM debe ser la principal")
	}

	// Huaral pasa a un servidor externo y luego se oculta
	if code, body := e.post("/admin/frequencies/3", url.Values{"city": {"Huaral"}, "frequency": {"105.4"}, "band": {"FM"},
		"sort_order": {"20"}, "active": {"on"}, "source": {"external"}, "stream_url": {"https://stream.ejemplo.pe/huaral"}}); code != http.StatusSeeOther {
		t.Fatalf("editar = %d %s", code, body)
	}
	if _, home = e.get("/"); !strings.Contains(home, `data-src="https://stream.ejemplo.pe/huaral"`) || !strings.Contains(home, "data-external") {
		t.Error("Huaral debe reproducir la URL externa")
	}
	if code, _ := e.post("/admin/frequencies/3", url.Values{"city": {"Huaral"}, "frequency": {"105.4"}, "band": {"FM"},
		"source": {"external"}, "stream_url": {"https://stream.ejemplo.pe/huaral"}}); code != http.StatusSeeOther {
		t.Fatalf("ocultar = %d", code)
	}
	if _, home = e.get("/"); strings.Contains(home, "Huaral") {
		t.Error("una frecuencia oculta no debe salir en la web")
	}
	if code, _ := e.post("/admin/frequencies/3/delete", url.Values{}); code != http.StatusSeeOther {
		t.Fatalf("eliminar = %d", code)
	}
	if code, _ := e.get("/admin/frequencies/3"); code != http.StatusNotFound {
		t.Fatalf("frecuencia eliminada = %d", code)
	}
}

// Cada frecuencia es una emisora: su montaje, su estado, su programación,
// sus cuñas y su PC del estudio.
func TestStations(t *testing.T) {
	e := newEnv(t)
	// en Icecast están conectadas /radio (7 oyentes) y /radio-barranca (4)
	e.radio.Mounts = map[string]icecast.MountStatus{"/radio-barranca": {Online: true, Listeners: 4, Title: "Barranca - Tema"}}
	e.login("super@test.pe", "supersecreta")
	if code, body := e.post("/admin/frequencies", url.Values{"city": {"Barranca"}, "frequency": {"103.2"}, "band": {"FM"},
		"source": {"local"}, "active": {"on"}}); code != http.StatusSeeOther {
		t.Fatalf("crear Barranca = %d %s", code, body)
	}
	if code, body := e.post("/admin/frequencies", url.Values{"city": {"Huaral"}, "frequency": {"105.4"}, "band": {"FM"},
		"source": {"external"}, "stream_url": {"https://stream.ejemplo.pe/huaral"}, "active": {"on"}}); code != http.StatusSeeOther {
		t.Fatalf("crear Huaral = %d %s", code, body)
	}

	// estado por frecuencia
	_, body := e.get("/api/status")
	var st struct {
		Stations []struct {
			ID        int64  `json:"id"`
			City      string `json:"city"`
			Online    *bool  `json:"online"`
			Listeners *int   `json:"listeners"`
			Title     string `json:"title"`
		} `json:"stations"`
	}
	if err := json.Unmarshal([]byte(body), &st); err != nil || len(st.Stations) != 3 {
		t.Fatalf("/api/status = %s", body)
	}
	if s := st.Stations[1]; s.City != "Barranca" || s.Online == nil || !*s.Online || *s.Listeners != 4 || s.Title != "Barranca - Tema" {
		t.Errorf("Barranca = %+v", s)
	}
	if s := st.Stations[0]; s.Online == nil || !*s.Online || *s.Listeners != 7 {
		t.Errorf("Huacho = %+v", s)
	}
	if s := st.Stations[2]; s.Online != nil || s.Listeners != nil {
		t.Errorf("de un servidor externo no se sabe el estado: %+v", s)
	}

	// Icecast acepta fuentes solo en montajes de frecuencias locales activas
	for mount, want := range map[string]bool{"/radio": true, "/radio-barranca": true, "/radio-otra": false} {
		res, err := http.PostForm(e.srv.URL+"/internal/icecast/source?token=token-interno", url.Values{"action": {"stream_auth"},
			"mount": {mount}, "user": {"source"}, "pass": {"fuente123"}})
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		if got := res.Header.Get("icecast-auth-user") == "1"; got != want {
			t.Errorf("stream_auth %s = %v, se esperaba %v", mount, got, want)
		}
	}

	// programa solo de Barranca
	if code, body := e.post("/admin/programs", url.Values{"title": {"Mañanas de Barranca"}, "weekday": {"1"},
		"start_time": {"06:00"}, "end_time": {"08:00"}, "active": {"on"}, "frequency_id": {"2"}}); code != http.StatusSeeOther {
		t.Fatalf("crear programa = %d %s", code, body)
	}
	if _, s := e.get("/programacion?f=2"); !strings.Contains(s, "Mañanas de Barranca") {
		t.Error("la programación de Barranca debe tener su programa")
	}
	if _, s := e.get("/programacion?f=1"); strings.Contains(s, "Mañanas de Barranca") {
		t.Error("la programación de Huacho no debe tener el programa de Barranca")
	}
	if _, s := e.get("/programacion"); !strings.Contains(s, `<span class="pcity">Barranca</span>`) {
		t.Error("en «Todas» el programa propio debe llevar la ciudad")
	}
	// no se puede borrar una frecuencia con programas propios
	if code, _ := e.post("/admin/frequencies/2/delete", url.Values{}); code != http.StatusSeeOther {
		t.Fatalf("borrar con programas = %d", code)
	}
	if code, _ := e.get("/admin/frequencies/2"); code != http.StatusOK {
		t.Fatal("Barranca no debe borrarse mientras tenga programas propios")
	}

	// una cuña solo para Huacho y otra para todas
	for name, fid := range map[string]string{"Solo Huacho": "1", "Para todas": "0"} {
		if code, body := e.uploadSpot(map[string]string{"advertiser": "Anunciante", "name": name, "duration": "20",
			"plays_per_day": "4", "hour_from": "6", "hour_to": "22", "active": "on", "frequency_id": fid}); code != http.StatusSeeOther {
			t.Fatalf("crear cuña %s = %d %s", name, code, body)
		}
	}

	// la PC de Barranca: su token, su montaje, sus cuñas y lo que suena
	code, page := e.post("/admin/studio/2/token", url.Values{})
	i := strings.Index(page, "c25s_")
	if code != http.StatusOK || i < 0 {
		t.Fatalf("token de Barranca = %d", code)
	}
	tok := page[i : i+len("c25s_")+64]
	if _, cfg, _ := e.studioReq(http.MethodGet, "/api/studio/config", tok, ""); !strings.Contains(cfg, `"mount":"/radio-barranca"`) ||
		!strings.Contains(cfg, `"city":"Barranca"`) {
		t.Fatalf("config de Barranca = %s", cfg)
	}
	if _, spots, _ := e.studioReq(http.MethodGet, "/api/studio/spots", tok, ""); strings.Contains(spots, "Solo Huacho") ||
		!strings.Contains(spots, "Para todas") {
		t.Fatalf("cuñas de Barranca = %s", spots)
	}
	if _, progs, _ := e.studioReq(http.MethodGet, "/api/studio/programs", tok, ""); !strings.Contains(progs, "Mañanas de Barranca") {
		t.Fatalf("programas de Barranca = %s", progs)
	}
	if code, _, _ := e.studioReq(http.MethodPost, "/api/studio/nowplaying", tok, `{"artist":"PC Barranca","title":"En vivo"}`); code != http.StatusNoContent {
		t.Fatalf("nowplaying = %d", code)
	}
	e.app.Media.CutStreams(t.Context()) // vacía la caché de estado
	if _, body = e.get("/api/status"); !strings.Contains(body, "PC Barranca - En vivo") {
		t.Errorf("lo que suena en Barranca debe venir de su PC: %s", body)
	}
	if _, s := e.get("/admin/studio"); !strings.Contains(s, "PC Barranca - En vivo") || !strings.Contains(s, "test.local:8010/radio-barranca") {
		t.Error("el panel debe mostrar la PC de Barranca y su montaje")
	}
	// la PC de Huaral (servidor externo) no recibe nuestro Icecast
	code, page = e.post("/admin/studio/3/token", url.Values{})
	i = strings.Index(page, "c25s_")
	if _, cfg, _ := e.studioReq(http.MethodGet, "/api/studio/config", page[i:i+len("c25s_")+64], ""); !strings.Contains(cfg, `"host":""`) {
		t.Fatalf("config de Huaral (externo) = %s", cfg)
	}
}
