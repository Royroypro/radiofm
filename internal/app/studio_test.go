package app_test

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"radioclub25/internal/domain"
)

// fakeMP3 empieza con una cabecera ID3 para que storage lo detecte como audio/mpeg.
var fakeMP3 = append([]byte("ID3\x03\x00\x00\x00\x00\x00\x00"), bytes.Repeat([]byte{0xFF, 0xFB, 0x90, 0x00}, 256)...)

// studioReq hace una petición a la API del estudio con el token indicado.
func (e *env) studioReq(method, path, token, body string) (int, string, http.Header) {
	e.t.Helper()
	var rd io.Reader
	if body != "" {
		rd = strings.NewReader(body)
	}
	req, _ := http.NewRequest(method, e.srv.URL+path, rd)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		e.t.Fatal(err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	return res.StatusCode, string(b), res.Header
}

// uploadSpot crea una cuña FM desde el formulario del panel (multipart).
func (e *env) uploadSpot(fields map[string]string) (int, string) {
	e.t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fields["csrf_token"] = e.csrf()
	for k, v := range fields {
		_ = mw.WriteField(k, v)
	}
	fw, _ := mw.CreateFormFile("file", "cuna.mp3")
	_, _ = fw.Write(fakeMP3)
	_ = mw.Close()
	res, err := e.client.Post(e.srv.URL+"/admin/fm-spots", mw.FormDataContentType(), &buf)
	if err != nil {
		e.t.Fatal(err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	return res.StatusCode, string(b)
}

func TestStudioPanelAndAPI(t *testing.T) {
	e := newEnv(t)
	e.login("super@test.pe", "supersecreta")
	for _, p := range []string{"/admin/studio", "/admin/fm-spots", "/admin/fm-spots/new", "/admin/fm-spots/report",
		"/admin/fm-spots/report.csv"} {
		if code, body := e.get(p); code != http.StatusOK || strings.Contains(body, "ZgotmplZ") {
			t.Errorf("GET %s = %d", p, code)
		}
	}

	// sin token la API responde 401
	if code, _, h := e.studioReq(http.MethodGet, "/api/studio/spots", "", ""); code != http.StatusUnauthorized ||
		h.Get("WWW-Authenticate") == "" {
		t.Fatalf("sin token = %d", code)
	}

	// generar el token desde el panel: se muestra una sola vez
	code, page := e.post("/admin/studio/1/token", url.Values{})
	if code != http.StatusOK {
		t.Fatalf("generar token = %d", code)
	}
	i := strings.Index(page, "c25s_")
	if i < 0 {
		t.Fatal("la página no muestra el token nuevo")
	}
	token := page[i : i+len("c25s_")+64]
	if _, again := e.get("/admin/studio"); strings.Contains(again, token) {
		t.Fatal("el token no debe volver a mostrarse")
	}
	if code, _, _ := e.studioReq(http.MethodGet, "/api/studio/spots", "c25s_"+strings.Repeat("0", 64), ""); code != http.StatusUnauthorized {
		t.Fatalf("token falso = %d", code)
	}

	// crear una cuña FM desde el panel (con archivo)
	if code, body := e.uploadSpot(map[string]string{"advertiser": "Ferretería El Sol", "name": "Verano 20 s",
		"duration": "20", "plays_per_day": "6", "hour_from": "7", "hour_to": "13", "active": "on"}); code != http.StatusSeeOther {
		t.Fatalf("crear cuña = %d %s", code, body)
	}

	// config
	code, body, _ := e.studioReq(http.MethodGet, "/api/studio/config", token, "")
	if code != http.StatusOK || !strings.Contains(body, `"mount":"/radio"`) || !strings.Contains(body, `"timezone":"America/Lima"`) ||
		!strings.Contains(body, `"mount":"/radio"`) || !strings.Contains(body, `"city":"Huacho"`) || strings.Contains(body, "fuente123") {
		t.Fatalf("config = %d %s", code, body)
	}

	// spots: URL absoluta, sha256 real del archivo y ETag
	code, body, h := e.studioReq(http.MethodGet, "/api/studio/spots", token, "")
	var spots struct {
		Spots []struct {
			ID         int64  `json:"id"`
			URL        string `json:"url"`
			SHA256     string `json:"sha256"`
			DurationMs int64  `json:"duration_ms"`
			HourTo     int    `json:"hour_to"`
		} `json:"spots"`
	}
	if code != http.StatusOK || json.Unmarshal([]byte(body), &spots) != nil || len(spots.Spots) != 1 {
		t.Fatalf("spots = %d %s", code, body)
	}
	sp := spots.Spots[0]
	sum := sha256.Sum256(fakeMP3)
	if !strings.HasPrefix(sp.URL, "http://test.local/uploads/fm/") || sp.SHA256 != hex.EncodeToString(sum[:]) ||
		sp.DurationMs != 20000 || sp.HourTo != 13 {
		t.Fatalf("cuña inesperada: %+v", sp)
	}
	req, _ := http.NewRequest(http.MethodGet, e.srv.URL+"/api/studio/spots", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("If-None-Match", h.Get("ETag"))
	if res, _ := http.DefaultClient.Do(req); res.StatusCode != http.StatusNotModified {
		t.Fatalf("mismo ETag debe dar 304, dio %d", res.StatusCode)
	}

	// heartbeat y lo que suena; /api/status usa el título de la PC
	if code, body, _ := e.studioReq(http.MethodPost, "/api/studio/heartbeat", token,
		`{"version":"0.1.0","mode":"auto","encoder":{"connected":true,"kbps":128}}`); code != http.StatusOK {
		t.Fatalf("heartbeat = %d %s", code, body)
	}
	if code, body, _ := e.studioReq(http.MethodPost, "/api/studio/heartbeat", token, `{"version":"0.1.0","mode":"x"}`); code != http.StatusBadRequest ||
		!strings.Contains(body, `"field":"mode"`) {
		t.Fatalf("heartbeat inválido = %d %s", code, body)
	}
	if code, _, _ := e.studioReq(http.MethodPost, "/api/studio/nowplaying", token,
		`{"artist":"Grupo 5","title":"Motor y motivo","kind":"music"}`); code != http.StatusNoContent {
		t.Fatalf("nowplaying = %d", code)
	}
	if _, status := e.get("/api/status"); !strings.Contains(status, "Grupo 5 - Motor y motivo") {
		t.Fatalf("/api/status debe mostrar lo que informa la PC: %s", status)
	}

	// emisiones: reenviar no duplica
	aired := time.Now().UTC().Add(-time.Minute).Format(time.RFC3339)
	batch := `{"airplays":[{"spot_id":` + strconv.FormatInt(sp.ID, 10) + `,"aired_at":"` + aired +
		`","duration_ms":20000,"complete":true,"play_uuid":"5f0c2b9e-1d2a-4c3b-9e8f-000000000001"}]}`
	for i, want := range []string{`"inserted":1`, `"inserted":0`} {
		code, body, _ := e.studioReq(http.MethodPost, "/api/studio/airplays", token, batch)
		if code != http.StatusOK || !strings.Contains(body, want) {
			t.Fatalf("airplays #%d = %d %s", i+1, code, body)
		}
	}
	if code, _, _ := e.studioReq(http.MethodPost, "/api/studio/airplays", token, `{"airplays":`); code != http.StatusBadRequest {
		t.Fatalf("JSON roto = %d", code)
	}
	_, csv := e.get("/admin/fm-spots/report.csv?advertiser=Ferreter%C3%ADa+El+Sol")
	if !strings.Contains(csv, "Ferretería El Sol;Verano 20 s;20.0;Sí;5f0c2b9e") {
		t.Fatalf("CSV del comprobante inesperado:\n%s", csv)
	}
	if _, list := e.get("/admin/studio"); !strings.Contains(list, "PC conectada") || !strings.Contains(list, "Motor y motivo") {
		t.Fatal("el panel debe mostrar la PC conectada y lo que suena")
	}

	// estado completo con un problema: alarmas en la página del estudio, el resumen y el control remoto
	if code, body, _ := e.studioReq(http.MethodPost, "/api/studio/heartbeat", token,
		`{"version":"1.10.0","mode":"auto","encoder":{},"status":{"running":true,"silence_for_s":60,"left_today_s":40000,`+
			`"queue":[{"id":41,"at":"2026-11-11T15:00:00Z","title":"Motor y motivo","artist":"Grupo 5","kind":"category","playing":true},`+
			`{"id":42,"at":"2026-11-11T15:03:30Z","title":"Tanda de cuñas","kind":"spots","hard":true}]}}`); code != http.StatusOK {
		t.Fatalf("heartbeat con estado = %d %s", code, body)
	}
	for _, p := range []string{"/admin/studio", "/admin", "/admin/control"} {
		if code, page := e.get(p); code != http.StatusOK || !strings.Contains(page, "Silencio al aire hace 60 s") {
			t.Fatalf("%s debe mostrar la alarma de silencio (%d)", p, code)
		}
	}
	if _, page := e.get("/admin/control/1"); !strings.Contains(page, "Tanda de cuñas") || !strings.Contains(page, "▶ sonando") {
		t.Fatal("el control remoto debe mostrar lo que viene")
	}
	// orden del panel → la PC la toma una vez y responde
	if code, _ := e.post("/admin/control/1", url.Values{"kind": {"remove"}, "arg": {"42"}}); code != http.StatusSeeOther {
		t.Fatalf("enviar orden = %d", code)
	}
	if code, _ := e.post("/admin/control/1", url.Values{"kind": {"borrar-todo"}}); code != http.StatusSeeOther {
		t.Fatalf("orden inválida debe volver con error = %d", code)
	}
	code, body, _ = e.studioReq(http.MethodGet, "/api/studio/commands", token, "")
	var cmds struct {
		Commands []struct {
			ID   int64  `json:"id"`
			Kind string `json:"kind"`
			Arg  string `json:"arg"`
		} `json:"commands"`
	}
	if code != http.StatusOK || json.Unmarshal([]byte(body), &cmds) != nil || len(cmds.Commands) != 1 ||
		cmds.Commands[0].Kind != "remove" || cmds.Commands[0].Arg != "42" {
		t.Fatalf("órdenes = %d %s", code, body)
	}
	if _, body, _ := e.studioReq(http.MethodGet, "/api/studio/commands", token, ""); strings.Contains(body, `"remove"`) {
		t.Fatal("una orden se entrega una sola vez")
	}
	if code, _, _ := e.studioReq(http.MethodPost, "/api/studio/commands/"+strconv.FormatInt(cmds.Commands[0].ID, 10)+"/done", token,
		`{"ok":true,"result":"elemento quitado"}`); code != http.StatusNoContent {
		t.Fatalf("respuesta de la orden = %d", code)
	}
	if _, page := e.get("/admin/control/1"); !strings.Contains(page, "elemento quitado") {
		t.Fatal("el panel debe mostrar el resultado de la orden")
	}
	// respaldo de la base de la PC
	var gz bytes.Buffer
	zw := gzip.NewWriter(&gz)
	_, _ = zw.Write([]byte(strings.Repeat("SQLite format 3 ", 100)))
	_ = zw.Close()
	breq, _ := http.NewRequest(http.MethodPost, e.srv.URL+"/api/studio/backup", bytes.NewReader(gz.Bytes()))
	breq.Header.Set("Authorization", "Bearer "+token)
	breq.Header.Set("X-Backup-Name", "estudio-20261111.db")
	if res, err := http.DefaultClient.Do(breq); err != nil || res.StatusCode != http.StatusNoContent {
		t.Fatalf("respaldo = %v %v", res, err)
	}
	if _, page := e.get("/admin/studio"); !strings.Contains(page, "estudio-20261111.db.gz") {
		t.Fatal("el panel debe listar las copias guardadas")
	}

	// bloqueo total: la API responde 503
	if code, _ := e.post("/superadmin/license/block", url.Values{"level": {"full"}, "reason": {"prueba"}}); code != http.StatusSeeOther {
		t.Fatalf("bloquear = %d", code)
	}
	if code, body, _ := e.studioReq(http.MethodGet, "/api/studio/spots", token, ""); code != http.StatusServiceUnavailable ||
		!strings.Contains(body, "suspended") {
		t.Fatalf("API con bloqueo total = %d %s", code, body)
	}

	// revocar invalida el token
	if code, _ := e.post("/superadmin/license/unblock", url.Values{}); code != http.StatusSeeOther {
		t.Fatal("desbloquear")
	}
	if code, _ := e.post("/admin/studio/1/token/revoke", url.Values{}); code != http.StatusSeeOther {
		t.Fatal("revocar")
	}
	if code, _, _ := e.studioReq(http.MethodGet, "/api/studio/spots", token, ""); code != http.StatusUnauthorized {
		t.Fatalf("token revocado = %d", code)
	}
	st, _ := e.app.Studio.State(context.Background(), 1)
	if st.Mode != domain.StudioAuto {
		t.Fatalf("modo guardado = %q", st.Mode)
	}
}
