package icecast

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestUpdateSong(t *testing.T) {
	var gotQuery string
	var gotUser, gotPass string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		gotUser, gotPass, _ = r.BasicAuth()
		if r.URL.Path != "/admin/metadata" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte("<?xml version=\"1.0\"?><iceresponse><message>Metadata update successful</message><return>1</return></iceresponse>"))
	}))
	defer srv.Close()
	c := &Client{BaseURL: srv.URL, Mount: "/radio", User: "source", Password: "cl@ve&1", HTTP: srv.Client()}
	if err := c.UpdateSong(context.Background(), "Grupo 5 - Motor y motivo ñ"); err != nil {
		t.Fatal(err)
	}
	if gotUser != "source" || gotPass != "cl@ve&1" {
		t.Fatalf("credenciales = %q/%q", gotUser, gotPass)
	}
	for _, want := range []string{"mode=updinfo", "mount=%2Fradio", "charset=UTF-8", "song=Grupo+5+-+Motor+y+motivo+%C3%B1"} {
		if !strings.Contains(gotQuery, want) {
			t.Fatalf("query %q no contiene %q", gotQuery, want)
		}
	}
}

func TestUpdateSongRejected(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "Authentication Required", http.StatusUnauthorized)
	}))
	defer srv.Close()
	c := &Client{BaseURL: srv.URL, Mount: "/radio", User: "source", Password: "mala", HTTP: srv.Client()}
	if err := c.UpdateSong(context.Background(), "x"); err == nil || !strings.Contains(err.Error(), "401") {
		t.Fatalf("debe fallar con 401, dio %v", err)
	}
}
