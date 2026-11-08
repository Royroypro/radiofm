// Tests de punta a punta del servidor HTTP con httptest: la app completa
// (router, middlewares, plantillas, servicios, SQLite) con fuentes de
// medios falsas.
package app_test

import (
	"context"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"radioclub25/internal/app"
	"radioclub25/internal/testutil"
)

type env struct {
	t      *testing.T
	srv    *httptest.Server
	app    *app.App
	client *http.Client
	radio  *testutil.FakeRadio
	tv     *testutil.FakeTV
}

func newEnv(t *testing.T) *env {
	t.Helper()
	cfg := testutil.Config(t)
	cfg.BootstrapSuperEmail, cfg.BootstrapSuperPass = "super@test.pe", "supersecreta"
	radio, tv := &testutil.FakeRadio{Online: true, Listeners: 7}, &testutil.FakeTV{Online: true, Viewers: 3}
	a, err := app.New(cfg, testutil.Logger(), testutil.DB(t), app.Options{Radio: radio, TV: tv})
	if err != nil {
		t.Fatal(err)
	}
	if err := a.Prepare(context.Background()); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(a.Router)
	t.Cleanup(srv.Close)
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	return &env{t: t, srv: srv, app: a, client: client, radio: radio, tv: tv}
}

func (e *env) get(path string) (int, string) {
	e.t.Helper()
	res, err := e.client.Get(e.srv.URL + path)
	if err != nil {
		e.t.Fatal(err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	return res.StatusCode, string(b)
}

func (e *env) csrf() string {
	u, _ := url.Parse(e.srv.URL)
	for _, c := range e.client.Jar.Cookies(u) {
		if c.Name == "rc_csrf" {
			return c.Value
		}
	}
	e.get("/login") // obtiene la cookie
	return e.csrf()
}

func (e *env) post(path string, form url.Values) (int, string) {
	e.t.Helper()
	form.Set("csrf_token", e.csrf())
	res, err := e.client.PostForm(e.srv.URL+path, form)
	if err != nil {
		e.t.Fatal(err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	return res.StatusCode, res.Header.Get("Location") + string(b)
}

func (e *env) login(email, pass string) {
	e.t.Helper()
	code, loc := e.post("/login", url.Values{"email": {email}, "password": {pass}})
	if code != http.StatusSeeOther {
		e.t.Fatalf("login %s: código %d", email, code)
	}
	_ = loc
}

func TestPublicPages(t *testing.T) {
	e := newEnv(t)
	for _, p := range []string{"/", "/programacion", "/contacto", "/login", "/healthz", "/static/css/site.css", "/static/js/site.js"} {
		if code, _ := e.get(p); code != http.StatusOK {
			t.Errorf("GET %s = %d", p, code)
		}
	}
	if code, _ := e.get("/no-existe"); code != http.StatusNotFound {
		t.Errorf("404 esperado, dio %d", code)
	}
	_, body := e.get("/api/status")
	if !strings.Contains(body, `"listeners":7`) || !strings.Contains(body, `"viewers":3`) {
		t.Errorf("/api/status inesperado: %s", body)
	}
	_, home := e.get("/")
	if !strings.Contains(home, "Radio Club 25") || !strings.Contains(home, "--red: #FF1E1E") {
		t.Error("la portada no muestra la marca o los colores")
	}
}

func TestPanelsRequireLoginAndRoles(t *testing.T) {
	e := newEnv(t)
	if code, _ := e.get("/admin"); code != http.StatusSeeOther {
		t.Fatalf("/admin sin sesión debe redirigir, dio %d", code)
	}
	// CSRF
	res, _ := e.client.PostForm(e.srv.URL+"/login", url.Values{"email": {"x"}, "password": {"y"}, "csrf_token": {"falso"}})
	if res.StatusCode != http.StatusForbidden {
		t.Fatalf("POST con CSRF falso debe dar 403, dio %d", res.StatusCode)
	}

	e.login("super@test.pe", "supersecreta")
	pages := []string{"/admin", "/admin/ads", "/admin/ads/new", "/admin/stats", "/admin/programs", "/admin/programs/new",
		"/admin/settings", "/admin/streams", "/admin/users", "/admin/users/new", "/admin/password",
		"/superadmin", "/superadmin/client", "/superadmin/plans", "/superadmin/plans/new", "/superadmin/contracts",
		"/superadmin/contracts/new", "/superadmin/payments", "/superadmin/payments/new", "/superadmin/license", "/superadmin/audit"}
	for _, p := range pages {
		if code, body := e.get(p); code != http.StatusOK || strings.Contains(body, "ZgotmplZ") {
			t.Errorf("GET %s = %d", p, code)
		}
	}

	// crear un editor y comprobar que no entra a ajustes ni a superadmin
	code, _ := e.post("/admin/users", url.Values{"name": {"Ed"}, "email": {"ed@test.pe"}, "role": {"editor"},
		"password": {"editor123"}, "active": {"on"}})
	if code != http.StatusSeeOther {
		t.Fatalf("crear editor = %d", code)
	}
	ed := newClientSameServer(e)
	ed.login("ed@test.pe", "editor123")
	if code, _ := ed.get("/admin/ads"); code != http.StatusOK {
		t.Errorf("editor → publicidad = %d", code)
	}
	for _, p := range []string{"/admin/settings", "/superadmin"} {
		if code, _ := ed.get(p); code != http.StatusForbidden {
			t.Errorf("editor → %s debe dar 403, dio %d", p, code)
		}
	}
}

func newClientSameServer(e *env) *env {
	jar, _ := cookiejar.New(nil)
	c := *e
	c.client = &http.Client{Jar: jar, CheckRedirect: e.client.CheckRedirect}
	return &c
}

func TestLicenseBlocksEverything(t *testing.T) {
	e := newEnv(t)
	e.login("super@test.pe", "supersecreta")
	code, _ := e.post("/superadmin/license/block", url.Values{"level": {"full"}, "reason": {"prueba"}})
	if code != http.StatusSeeOther {
		t.Fatalf("bloquear = %d", code)
	}
	if e.radio.Killed != 1 || e.tv.Kicked != 1 {
		t.Fatal("el bloqueo total debe cortar los streams")
	}
	anon := newClientSameServer(e)
	if code, body := anon.get("/"); code != http.StatusServiceUnavailable || !strings.Contains(body, "suspendido") {
		t.Fatalf("portada con bloqueo total = %d", code)
	}
	if code, _ := anon.get("/internal/gate/radio"); code != http.StatusForbidden {
		t.Fatalf("puerta de radio con bloqueo = %d", code)
	}
	req, _ := http.NewRequest(http.MethodPost, e.srv.URL+"/internal/mediamtx/auth?token=token-interno",
		strings.NewReader(`{"action":"read","path":"tv"}`))
	res, _ := http.DefaultClient.Do(req)
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("MediaMTX read con bloqueo = %d", res.StatusCode)
	}
	// el superadmin sigue entrando
	if code, _ := e.get("/superadmin"); code != http.StatusOK {
		t.Fatalf("superadmin con bloqueo = %d", code)
	}
	if code, _ := e.post("/superadmin/license/unblock", url.Values{}); code != http.StatusSeeOther {
		t.Fatal("desbloquear")
	}
	if code, _ := anon.get("/"); code != http.StatusOK {
		t.Fatalf("portada tras desbloquear = %d", code)
	}
}

func TestInternalEndpointsNeedToken(t *testing.T) {
	e := newEnv(t)
	res, _ := http.Post(e.srv.URL+"/internal/mediamtx/auth?token=malo", "application/json", strings.NewReader(`{"action":"read","path":"tv"}`))
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("token malo = %d", res.StatusCode)
	}
	res, _ = http.Post(e.srv.URL+"/internal/mediamtx/auth?token=token-interno", "application/json", strings.NewReader(`{"action":"read","path":"tv"}`))
	if res.StatusCode != http.StatusOK {
		t.Fatalf("read con token = %d", res.StatusCode)
	}
	res, _ = http.PostForm(e.srv.URL+"/internal/icecast/source?token=token-interno", url.Values{"pass": {"fuente123"}, "mount": {"/radio"}})
	if res.Header.Get("icecast-auth-user") != "1" {
		t.Fatal("Icecast debe aceptar la contraseña de fuente correcta")
	}
	res, _ = http.PostForm(e.srv.URL+"/internal/icecast/source?token=token-interno", url.Values{"pass": {"mala"}})
	if res.Header.Get("icecast-auth-user") == "1" {
		t.Fatal("Icecast no debe aceptar una contraseña incorrecta")
	}
}
