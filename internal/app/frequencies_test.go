package app_test

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
)

// Frecuencias por ciudad: alta y validación en el panel, orden, y lo que
// muestran la portada y la API del estudio.
func TestFrequencies(t *testing.T) {
	e := newEnv(t)
	e.login("super@test.pe", "supersecreta")

	for _, p := range []string{"/admin/frequencies", "/admin/frequencies/new"} {
		if code, body := e.get(p); code != http.StatusOK || strings.Contains(body, "ZgotmplZ") {
			t.Fatalf("GET %s = %d", p, code)
		}
	}
	add := func(city, freq string) {
		t.Helper()
		if code, body := e.post("/admin/frequencies", url.Values{"city": {city}, "frequency": {freq},
			"band": {"FM"}, "active": {"on"}}); code != http.StatusSeeOther {
			t.Fatalf("crear %s = %d %s", city, code, body)
		}
	}
	add("Huacho", "93.3")
	add("Barranca", "103,2") // coma decimal: se guarda como 103.2
	add("Huaral", "105.4")

	if code, _ := e.post("/admin/frequencies", url.Values{"city": {"Lima"}, "frequency": {"200"}, "band": {"FM"}}); code != http.StatusUnprocessableEntity {
		t.Fatalf("frecuencia fuera de banda debe dar 422, dio %d", code)
	}
	if code, _ := e.post("/admin/frequencies", url.Values{"city": {""}, "frequency": {"99.1"}}); code != http.StatusUnprocessableEntity {
		t.Fatalf("sin ciudad debe dar 422, dio %d", code)
	}

	_, home := e.get("/")
	for _, want := range []string{"93.3 FM", "103.2 FM", "Barranca", "105.4 FM", "Huaral", "<title>"} {
		if !strings.Contains(home, want) {
			t.Errorf("la portada no muestra %q", want)
		}
	}
	if !strings.Contains(home, "93.3 FM</p>") {
		t.Error("la frecuencia principal debe ser la primera (Huacho)")
	}

	// subir Barranca al primer lugar: pasa a ser la principal
	_, list := e.get("/admin/frequencies")
	i := strings.Index(list, `/admin/frequencies/2/up`)
	if i < 0 {
		t.Fatal("la lista no tiene el botón de subir de Barranca")
	}
	if code, _ := e.post("/admin/frequencies/2/up", url.Values{}); code != http.StatusSeeOther {
		t.Fatalf("subir = %d", code)
	}
	if _, home = e.get("/"); !strings.Contains(home, "103.2 FM</p>") {
		t.Error("tras subir Barranca, 103.2 FM debe ser la principal")
	}

	// ocultar Huaral
	if code, _ := e.post("/admin/frequencies/3", url.Values{"city": {"Huaral"}, "frequency": {"105.4"}, "band": {"FM"},
		"sort_order": {"20"}}); code != http.StatusSeeOther {
		t.Fatalf("editar = %d", code)
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
