package app_test

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

// El panel está organizado por apartados: cada página de ajustes guarda solo
// sus campos, y los tokens y claves están en Accesos y seguridad.
func TestPanelSections(t *testing.T) {
	e := newEnv(t)
	e.login("super@test.pe", "supersecreta")

	if code, _ := e.get("/admin/settings"); code != http.StatusSeeOther {
		t.Fatalf("/admin/settings debe llevar a Marca e identidad, dio %d", code)
	}
	if code, _ := e.get("/admin/settings/otra"); code != http.StatusNotFound {
		t.Fatalf("apartado inexistente = %d", code)
	}
	_, menu := e.get("/admin")
	for _, want := range []string{"Emisión", "Publicidad", "Personalización", "App Android", "Accesos y seguridad",
		"Tokens y claves", "Marca e identidad", "Monitoreo del estudio"} {
		if !strings.Contains(menu, want) {
			t.Errorf("el menú no tiene %q", want)
		}
	}

	// guardar Contacto no toca la marca ni la app
	ctx := context.Background()
	before, _ := e.app.Settings.Get(ctx)
	if code, body := e.post("/admin/settings/contact", url.Values{"phone": {"01 234 5678"}, "email": {"radio@ejemplo.pe"}}); code != http.StatusSeeOther {
		t.Fatalf("guardar contacto = %d %s", code, body)
	}
	after, _ := e.app.Settings.Get(ctx)
	if after.Phone != "01 234 5678" || after.SiteName != before.SiteName || after.LogoURL != before.LogoURL ||
		after.ColorRed != before.ColorRed || after.AppColorBg != before.AppColorBg {
		t.Fatalf("guardar Contacto cambió otros apartados: antes %+v después %+v", before, after)
	}
	// un error se muestra en su apartado
	if code, body := e.post("/admin/settings/contact", url.Values{"email": {"no-es-email"}}); code != http.StatusUnprocessableEntity ||
		!strings.Contains(body, "Contacto y redes") {
		t.Fatalf("email inválido = %d", code)
	}
	if code, _ := e.post("/admin/settings/ads", url.Values{"preroll_interval_min": {"15"}}); code != http.StatusSeeOther {
		t.Fatal("guardar ajustes de publicidad")
	}
	if s, _ := e.app.Settings.Get(ctx); s.PrerollIntervalMin != 15 || s.Phone != "01 234 5678" {
		t.Fatalf("ajustes de publicidad = %+v", s)
	}

	// Tokens y claves: la clave de la TV se cambia aquí; Streams ya no la toca
	old := after.TVPublishPass
	if code, _ := e.post("/admin/streams", url.Values{"radio_enabled": {"on"}}); code != http.StatusSeeOther {
		t.Fatal("guardar streams")
	}
	if s, _ := e.app.Settings.Get(ctx); s.TVPublishUser != after.TVPublishUser || s.TVPublishPass != old {
		t.Fatal("guardar Streams no debe cambiar el usuario ni la clave de la TV")
	}
	if code, _ := e.post("/admin/access/tv", url.Values{"tv_publish_user": {"obs"}, "regenerate_key": {"on"}}); code != http.StatusSeeOther {
		t.Fatal("cambiar la clave de la TV")
	}
	if s, _ := e.app.Settings.Get(ctx); s.TVPublishUser != "obs" || s.TVPublishPass == old {
		t.Fatalf("clave de TV = %q/%q", s.TVPublishUser, s.TVPublishPass)
	}
	code, page := e.post("/admin/access/studio/1/token", url.Values{})
	if code != http.StatusOK || !strings.Contains(page, "c25s_") || !strings.Contains(page, "Tokens y claves") {
		t.Fatalf("generar token desde Tokens y claves = %d", code)
	}
	if _, mon := e.get("/admin/studio"); strings.Contains(mon, "/admin/access/studio/1/token") {
		t.Error("el monitoreo ya no debe tener los botones de token")
	}
}
