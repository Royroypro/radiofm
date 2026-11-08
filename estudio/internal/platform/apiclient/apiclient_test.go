package apiclient

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// fakeServer implementa la API del estudio con las respuestas de ejemplo de docs/api.md.
func fakeServer(t *testing.T, audio []byte) *httptest.Server {
	sum := sha256.Sum256(audio)
	mux := http.NewServeMux()
	var srv *httptest.Server
	auth := func(h http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Authorization") != "Bearer c25s_bueno" {
				w.Header().Set("WWW-Authenticate", `Bearer realm="studio"`)
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = w.Write([]byte(`{"error":"unauthorized"}`))
				return
			}
			h(w, r)
		}
	}
	mux.HandleFunc("/api/studio/config", auth(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"station":{"name":"Radio Club 25","frequency":"93.3 FM","slogan":"La música que va contigo",
			"timezone":"America/Lima","latitude":-11.1067,"longitude":-77.605},
			"icecast":{"host":"radioclub25.sehuacho.com","port":8010,"mount":"/radio","user":"source","radio_enabled":true},
			"server_time":"2026-11-08T17:00:00Z"}`))
	}))
	mux.HandleFunc("/api/studio/spots", auth(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("ETag", `"abc"`)
		if r.Header.Get("If-None-Match") == `"abc"` {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		_, _ = w.Write([]byte(`{"spots":[{"id":7,"advertiser":"Ferretería El Sol","name":"Verano 20 s","url":"` + srv.URL +
			`/uploads/fm/a.mp3","sha256":"` + hex.EncodeToString(sum[:]) + `","duration_ms":20000,"starts_at":null,
			"ends_at":"2026-12-31T05:00:00Z","plays_per_day":6,"hour_from":7,"hour_to":13,"updated_at":"2026-11-08T16:55:10Z"}]}`))
	}))
	mux.HandleFunc("/api/studio/heartbeat", auth(func(w http.ResponseWriter, r *http.Request) {
		var in map[string]any
		_ = json.NewDecoder(r.Body).Decode(&in)
		if in["mode"] == "x" {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":"invalid","field":"mode","message":"Modo inválido"}`))
			return
		}
		_, _ = w.Write([]byte(`{"ok":true,"server_time":"2026-11-08T17:00:00Z"}`))
	}))
	mux.HandleFunc("/api/studio/nowplaying", auth(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) }))
	mux.HandleFunc("/api/studio/airplays", auth(func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Airplays []Airplay `json:"airplays"`
		}
		_ = json.NewDecoder(r.Body).Decode(&in)
		_ = json.NewEncoder(w).Encode(AirplayResult{Received: len(in.Airplays), Inserted: len(in.Airplays), Rejected: []string{}})
	}))
	mux.HandleFunc("/uploads/fm/a.mp3", func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(audio) })
	srv = httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestClient(t *testing.T) {
	ctx := context.Background()
	audio := []byte(strings.Repeat("ID3audio", 500))
	srv := fakeServer(t, audio)
	c := New(srv.URL, "c25s_bueno", "test")

	cfg, err := c.GetConfig(ctx)
	if err != nil || cfg.Station.Timezone != "America/Lima" || cfg.Icecast.Port != 8010 {
		t.Fatalf("GetConfig = %+v, %v", cfg, err)
	}
	spots, etag, nm, err := c.GetSpots(ctx, "")
	if err != nil || nm || len(spots) != 1 || etag != `"abc"` || spots[0].HourTo != 13 || spots[0].EndsAt == nil {
		t.Fatalf("GetSpots = %+v %q %v %v", spots, etag, nm, err)
	}
	if _, _, nm, err := c.GetSpots(ctx, etag); err != nil || !nm {
		t.Fatalf("con el mismo ETag debe dar 304: %v %v", nm, err)
	}
	dst := filepath.Join(t.TempDir(), "spots", "a.mp3")
	if err := c.Download(ctx, spots[0].URL, dst, spots[0].SHA256); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(dst); string(b) != string(audio) {
		t.Fatal("descarga incompleta")
	}
	if err := c.Download(ctx, spots[0].URL, dst+"2", "00ff"); err == nil {
		t.Fatal("un sha256 distinto debe rechazarse")
	}
	if _, err := os.Stat(dst + "2"); err == nil {
		t.Fatal("no debe quedar el archivo si el sha256 no coincide")
	}
	if err := c.SendHeartbeat(ctx, Heartbeat{Version: "1", Mode: "auto", Encoder: map[string]any{"connected": true}}); err != nil {
		t.Fatal(err)
	}
	var apiErr *APIError
	if err := c.SendHeartbeat(ctx, Heartbeat{Version: "1", Mode: "x"}); !errors.As(err, &apiErr) || apiErr.Field != "mode" {
		t.Fatalf("error de validación = %v", err)
	}
	if err := c.SendNowPlaying(ctx, NowPlaying{Artist: "A", Title: "B", Kind: "music"}); err != nil {
		t.Fatal(err)
	}
	res, err := c.SendAirplays(ctx, []Airplay{{SpotID: 7, AiredAt: time.Now(), DurationMs: 20000, Complete: true, PlayUUID: "u-12345678"}})
	if err != nil || res.Inserted != 1 {
		t.Fatalf("SendAirplays = %+v, %v", res, err)
	}
	bad := New(srv.URL, "c25s_malo", "test")
	if _, err := bad.GetConfig(ctx); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("token malo = %v", err)
	}
}
