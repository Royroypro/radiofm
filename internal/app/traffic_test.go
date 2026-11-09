package app_test

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"
)

// postMultipart envía un formulario con archivo opcional (campo "file").
func (e *env) postMultipart(path string, fields url.Values, withFile bool) (int, string, string) {
	e.t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fields.Set("csrf_token", e.csrf())
	for k, vs := range fields {
		for _, v := range vs {
			_ = mw.WriteField(k, v)
		}
	}
	if withFile {
		fw, _ := mw.CreateFormFile("file", "cuna.mp3")
		_, _ = fw.Write(fakeMP3)
	}
	_ = mw.Close()
	res, err := e.client.Post(e.srv.URL+path, mw.FormDataContentType(), &buf)
	if err != nil {
		e.t.Fatal(err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	return res.StatusCode, string(b), res.Header.Get("Location")
}

// Publicidad FM: anunciante con rubro, campaña con dos versiones y reglas, y lo
// que recibe la PC (una versión por campaña para estudios viejos, todas con ?v=2).
func TestTrafficPanelAndAPI(t *testing.T) {
	e := newEnv(t)
	e.login("super@test.pe", "supersecreta")
	if code, _ := e.post("/admin/fm-advertisers", url.Values{"name": {"Banco Andino"}, "sector": {"Banca"}}); code != http.StatusSeeOther {
		t.Fatalf("crear anunciante = %d", code)
	}
	code, body, loc := e.postMultipart("/admin/fm-spots", url.Values{"advertiser_id": {"1"}, "name": {"Cuenta sueldo"},
		"plays_per_day": {"10"}, "hour_from": {"7"}, "hour_to": {"22"}, "days": {"1", "2", "3", "4", "5"},
		"position": {"first"}, "priority": {"1"}, "active": {"on"}, "version_name": {"20 s"}, "duration": {"20"}}, true)
	if code != http.StatusSeeOther || !strings.HasPrefix(loc, "/admin/fm-spots/") {
		t.Fatalf("crear campaña = %d %s", code, body)
	}
	if code, body, _ := e.postMultipart(loc+"/versions", url.Values{"name": {"30 s"}, "weight": {"2"}}, true); code != http.StatusSeeOther {
		t.Fatalf("segunda versión = %d %s", code, body)
	}
	for _, p := range []string{"/admin/fm-spots", loc, "/admin/fm-advertisers", "/admin/fm-spots/new", "/admin/fm-spots/report", "/admin"} {
		if code, body := e.get(p); code != http.StatusOK || strings.Contains(body, "ZgotmplZ") {
			t.Fatalf("GET %s = %d", p, code)
		}
	}
	if _, page := e.get(loc); !strings.Contains(page, "20 s") || !strings.Contains(page, "30 s") || !strings.Contains(page, "Banco Andino") {
		t.Fatal("la campaña debe mostrar sus dos versiones y el anunciante")
	}
	if _, list := e.get("/admin/fm-spots"); !strings.Contains(list, "Lun–Vie") || !strings.Contains(list, "banca") {
		t.Fatal("la lista debe mostrar días y rubro")
	}

	// token de la PC de Huacho
	code, page := e.post("/admin/studio/1/token", url.Values{})
	tok := regexp.MustCompile(`c25s_[0-9a-f]{64}`).FindString(page)
	if code != http.StatusOK || tok == "" {
		t.Fatal("no se generó el token")
	}
	type spot struct {
		ID, CampaignID   int64
		Weight, Days     int
		Position, Sector string
		Priority         int
	}
	get := func(path string) []spot {
		_, body, _ := e.studioReq(http.MethodGet, path, tok, "")
		var out struct {
			Spots []struct {
				ID         int64  `json:"id"`
				CampaignID int64  `json:"campaign_id"`
				Weight     int    `json:"weight"`
				Days       int    `json:"days"`
				Position   string `json:"position"`
				Priority   int    `json:"priority"`
				Sector     string `json:"sector"`
			} `json:"spots"`
		}
		if err := json.Unmarshal([]byte(body), &out); err != nil {
			t.Fatalf("%s: %s", path, body)
		}
		var r []spot
		for _, s := range out.Spots {
			r = append(r, spot{s.ID, s.CampaignID, s.Weight, s.Days, s.Position, s.Sector, s.Priority})
		}
		return r
	}
	if v1 := get("/api/studio/spots"); len(v1) != 1 || v1[0].Weight != 2 {
		t.Fatalf("un estudio viejo debe recibir una versión por campaña (la de más peso): %+v", v1)
	}
	v2 := get("/api/studio/spots?v=2")
	if len(v2) != 2 {
		t.Fatalf("con ?v=2 llegan todas las versiones: %+v", v2)
	}
	for _, s := range v2 {
		if s.CampaignID == 0 || s.Days != 0b0111110 || s.Position != "first" || s.Priority != 1 || s.Sector != "banca" {
			t.Fatalf("versión sin las reglas de la campaña: %+v", s)
		}
	}
}
