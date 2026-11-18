package panel

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"radioclub25/emisor/internal/config"
)

// studioResponse imita GET /api/studio/config (studioapi.Config en el servidor).
func studioResponse(icecastHost string) map[string]any {
	return map[string]any{
		"station": map[string]any{"name": "Radio Club 25", "frequency": "93.3 FM"},
		"icecast": map[string]any{"host": icecastHost, "port": 8010, "mount": "/radio-barranca", "user": "source",
			"radio_enabled": true, "source": "local"},
		"tv": map[string]any{"enabled": true, "url": "rtmp://ingesta.example:1935/tv", "user": "emisora", "pass": "K3y"},
	}
}

func TestImport(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/studio/config" {
			http.NotFound(w, r)
			return
		}
		gotAuth = r.Header.Get("Authorization")
		if gotAuth != "Bearer tok" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		json.NewEncoder(w).Encode(studioResponse("ingesta.example"))
	}))
	defer srv.Close()

	c := config.Defaults()
	c.Panel = config.Panel{URL: srv.URL + "/", Token: "tok"}
	res, err := Import(context.Background(), &c)
	if err != nil {
		t.Fatal(err)
	}
	if c.Radio.Host != "ingesta.example" || c.Radio.Mount != "/radio-barranca" || c.Radio.Port != 8010 {
		t.Fatalf("radio: %+v", c.Radio)
	}
	if c.TV.Host != "ingesta.example" || c.TV.RTMPPort != 1935 || c.TV.Path != "tv" || c.TV.User != "emisora" || c.TV.Password != "K3y" {
		t.Fatalf("tv: %+v", c.TV)
	}
	if res.Station != "Radio Club 25 93.3 FM" || !strings.Contains(strings.Join(res.Notes, "|"), "contraseña de fuente") {
		t.Fatalf("resultado: %+v", res)
	}

	c.Panel.Token = "malo"
	if _, err := Import(context.Background(), &c); err == nil || !strings.Contains(err.Error(), "token") {
		t.Fatalf("token malo: %v", err)
	}
	c.Panel.Token = ""
	if _, err := Import(context.Background(), &c); err == nil {
		t.Fatal("sin token debe pedirlo")
	}
}

func TestImportExternalFrequencyKeepsServer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(studioResponse(""))
	}))
	defer srv.Close()
	c := config.Defaults()
	c.Panel = config.Panel{URL: srv.URL, Token: "t"}
	c.Radio.Password = "ya-estaba"
	res, err := Import(context.Background(), &c)
	if err != nil {
		t.Fatal(err)
	}
	if c.Radio.Host != "radioclub25peru.com" || c.Radio.Password != "ya-estaba" {
		t.Fatalf("no debía cambiar la radio: %+v", c.Radio)
	}
	if !strings.Contains(strings.Join(res.Notes, "|"), "externo") {
		t.Fatalf("notas: %v", res.Notes)
	}
}

func TestParsePasted(t *testing.T) {
	c := config.Defaults()
	got, err := ParsePasted(&c, "Servidor completo:\n  rtmp://tv.example:1936/tv2?user=emi&pass=AbC123\n")
	if err != nil || len(got) != 1 {
		t.Fatalf("%v %v", got, err)
	}
	if c.TV.Host != "tv.example" || c.TV.RTMPPort != 1936 || c.TV.Path != "tv2" || c.TV.User != "emi" || c.TV.Password != "AbC123" ||
		c.TV.Protocol != config.ProtoRTMP {
		t.Fatalf("rtmp: %+v", c.TV)
	}

	c = config.Defaults()
	// Como OBS: servidor y clave en líneas separadas.
	if _, err := ParsePasted(&c, "rtmp://tv.example:1935\ntv?user=emisora&pass=Z9"); err != nil {
		t.Fatal(err)
	}
	if c.TV.Path != "tv" || c.TV.Password != "Z9" || c.TV.Host != "tv.example" {
		t.Fatalf("estilo OBS: %+v", c.TV)
	}

	c = config.Defaults()
	if _, err := ParsePasted(&c, "srt://tv.example:9000?streamid=publish:tv:emisora:Q:W&pkt_size=1316"); err != nil {
		t.Fatal(err)
	}
	if c.TV.Protocol != config.ProtoSRT || c.TV.SRTPort != 9000 || c.TV.Password != "Q:W" || c.TV.User != "emisora" {
		t.Fatalf("srt: %+v", c.TV)
	}

	c = config.Defaults()
	if _, err := ParsePasted(&c, "icecast://source:p%40ss@radio.example:8010/radio-huaral"); err != nil {
		t.Fatal(err)
	}
	if c.Radio.Host != "radio.example" || c.Radio.Mount != "/radio-huaral" || c.Radio.Password != "p@ss" {
		t.Fatalf("icecast: %+v", c.Radio)
	}

	c = config.Defaults()
	if _, err := ParsePasted(&c, "hola, esto no es nada"); err == nil {
		t.Fatal("texto sin datos debe dar error")
	}
	// La dirección de la web (sin puerto) no se confunde con Icecast.
	if _, err := ParsePasted(&c, "https://radioclub25peru.com/"); err == nil {
		t.Fatal("la URL de la web no es un servidor de Icecast")
	}
}

func TestUpdateSong(t *testing.T) {
	var got url.Values
	var user, pass string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.URL.Query()
		user, pass, _ = r.BasicAuth()
		if pass != "fuente" {
			w.WriteHeader(http.StatusUnauthorized)
		}
	}))
	defer srv.Close()
	host, port, _ := net.SplitHostPort(strings.TrimPrefix(srv.URL, "http://"))
	r := config.Defaults().Radio
	r.Host, r.Password = host, "fuente"
	r.Port, _ = strconv.Atoi(port)
	if err := UpdateSong(context.Background(), r, "Grupo 5 — Motor y motivo"); err != nil {
		t.Fatal(err)
	}
	if got.Get("mount") != "/radio" || got.Get("mode") != "updinfo" || got.Get("song") != "Grupo 5 — Motor y motivo" || user != "source" {
		t.Fatalf("consulta: %v (%s)", got, user)
	}
	r.Password = "mala"
	if err := UpdateSong(context.Background(), r, "x"); err == nil {
		t.Fatal("contraseña mala debe dar error")
	}
	r.Codec = config.CodecOpus
	if err := UpdateSong(context.Background(), r, "x"); err == nil {
		t.Fatal("Opus no admite el título por /admin/metadata")
	}
}
