package app_test

import (
	"archive/zip"
	"bytes"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"
)

// fakeAPK es un ZIP mínimo con forma de APK (un APK es un ZIP).
func fakeAPK(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, _ := zw.Create("AndroidManifest.xml")
	_, _ = w.Write([]byte("manifiesto de prueba"))
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// saveSettingsWithFile envía Admin → Marca y contacto con un archivo en app_apk_file.
func (e *env) saveSettingsWithFile(fields url.Values, filename string, content []byte) (int, string) {
	e.t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fields.Set("csrf_token", e.csrf())
	for k, vs := range fields {
		for _, v := range vs {
			_ = mw.WriteField(k, v)
		}
	}
	if filename != "" {
		fw, _ := mw.CreateFormFile("app_apk_file", filename)
		_, _ = fw.Write(content)
	}
	_ = mw.Close()
	res, err := e.client.Post(e.srv.URL+"/admin/settings", mw.FormDataContentType(), &buf)
	if err != nil {
		e.t.Fatal(err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	return res.StatusCode, string(b)
}

func TestAppDownloadLinks(t *testing.T) {
	e := newEnv(t)
	if _, home := e.get("/"); strings.Contains(home, "Descarga la app") {
		t.Fatal("sin enlaces configurados no debe mostrarse la descarga")
	}
	e.login("super@test.pe", "supersecreta")
	base := url.Values{"site_name": {"Radio Club 25"}, "color_navy": {"#0A1A6B"}, "color_royal": {"#223CB8"},
		"color_red": {"#FF1E1E"}, "color_orange": {"#FF961E"}, "app_apk_version": {"1.2.0"}}

	// un archivo que no es APK se rechaza aunque se llame .apk
	if code, body := e.saveSettingsWithFile(base, "falso.apk", []byte("no soy un zip")); code == http.StatusSeeOther ||
		!strings.Contains(body, "Tipo de archivo no permitido") {
		t.Fatalf("APK falso aceptado: %d", code)
	}

	// subir el APK: aparece el botón en la web y en /api/app/config
	if code, body := e.saveSettingsWithFile(base, "club25.apk", fakeAPK(t)); code != http.StatusSeeOther {
		t.Fatalf("subir APK = %d %s", code, body)
	}
	_, home := e.get("/")
	link := regexp.MustCompile(`href="(/uploads/app/[^"]+\.apk)"`).FindStringSubmatch(home)
	if link == nil || !strings.Contains(home, "APK para Android") || !strings.Contains(home, "v1.2.0") ||
		strings.Contains(home, "Google Play</span>") {
		t.Fatalf("botón de descarga inesperado en la portada")
	}
	res, err := http.Get(e.srv.URL + link[1])
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusOK || res.Header.Get("Content-Type") != "application/vnd.android.package-archive" ||
		!strings.HasPrefix(res.Header.Get("Content-Disposition"), "attachment") {
		t.Errorf("descarga del APK: %d %q %q", res.StatusCode, res.Header.Get("Content-Type"), res.Header.Get("Content-Disposition"))
	}
	r2, err := http.Get(e.srv.URL + "/api/app/config")
	if err != nil {
		t.Fatal(err)
	}
	defer r2.Body.Close()
	b, _ := io.ReadAll(r2.Body)
	if !strings.Contains(string(b), `"apk_url":"`+link[1]+`"`) || !strings.Contains(string(b), `"apk_version":"1.2.0"`) {
		t.Errorf("/api/app/config sin el APK: %s", b)
	}

	// con Google Play y sin APK: solo el botón de Play
	base.Set("app_apk_url", "")
	base.Set("app_play_store_url", "https://play.google.com/store/apps/details?id=pe.club25peru.radio")
	if code, body := e.saveSettingsWithFile(base, "", nil); code != http.StatusSeeOther {
		t.Fatalf("guardar Play = %d %s", code, body)
	}
	_, home = e.get("/")
	if !strings.Contains(home, "Google Play</span>") || strings.Contains(home, "APK para Android") {
		t.Errorf("se esperaba solo Google Play")
	}

	// un enlace de APK que no es https ni subido se rechaza
	base.Set("app_apk_url", "ftp://ejemplo.pe/app.apk")
	if code, body := e.saveSettingsWithFile(base, "", nil); code == http.StatusSeeOther || !strings.Contains(body, "https://") {
		t.Errorf("enlace de APK inválido aceptado: %d", code)
	}
}
