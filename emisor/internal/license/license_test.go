package license

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// fakeServer imita los endpoints del emisor en la web (studioapi/emisor.go).
func fakeServer(t *testing.T, exe []byte, suspended *bool) *httptest.Server {
	sum := sha256.Sum256(exe)
	var saved []byte
	mux := http.NewServeMux()
	auth := func(w http.ResponseWriter, r *http.Request) bool {
		if suspended != nil && *suspended {
			w.WriteHeader(http.StatusServiceUnavailable)
			io.WriteString(w, `{"error":"suspended"}`)
			return false
		}
		if r.Header.Get("Authorization") != "Bearer buen-token" {
			w.WriteHeader(http.StatusUnauthorized)
			return false
		}
		return true
	}
	mux.HandleFunc("/api/emisor/provider", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"phone":"999111222","url":"https://proveedor.pe"}`)
	})
	mux.HandleFunc("/api/studio/emisor", func(w http.ResponseWriter, r *http.Request) {
		if !auth(w, r) {
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"ok": true,
			"station":  map[string]any{"name": "Radio Club 25", "frequency": "93.3 FM", "frequency_id": 1, "city": "Huacho"},
			"provider": map[string]any{"phone": "999111222", "url": "https://proveedor.pe"},
			"latest":   map[string]any{"version": "1.5.0", "sha256": hex.EncodeToString(sum[:]), "size": len(exe), "notes": "nuevo"}})
	})
	mux.HandleFunc("/api/studio/emisor/download", func(w http.ResponseWriter, r *http.Request) {
		if !auth(w, r) {
			return
		}
		w.Header().Set("X-Emisor-Version", "1.5.0")
		w.Write(exe)
	})
	mux.HandleFunc("/api/studio/emisor/heartbeat", func(w http.ResponseWriter, r *http.Request) {
		if !auth(w, r) {
			return
		}
		var in struct {
			Version string `json:"version"`
			Status  Report `json:"status"`
		}
		if json.NewDecoder(r.Body).Decode(&in) != nil || in.Version == "" || in.Status.Radio.State == "" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		io.WriteString(w, `{"latest":"1.5.0"}`)
	})
	mux.HandleFunc("/api/studio/emisor/config", func(w http.ResponseWriter, r *http.Request) {
		if !auth(w, r) {
			return
		}
		if r.Method == http.MethodPut {
			saved, _ = io.ReadAll(r.Body)
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if saved == nil {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("X-Saved-At", "2026-11-18T10:00:00Z")
		w.Write(saved)
	})
	return httptest.NewServer(mux)
}

func TestNormalizeBase(t *testing.T) {
	for in, want := range map[string]string{"radioclub25peru.com": "https://radioclub25peru.com",
		" https://radio.pe/admin/studio ": "https://radio.pe", "http://127.0.0.1:8121/": "http://127.0.0.1:8121"} {
		if got, err := NormalizeBase(in); err != nil || got != want {
			t.Errorf("NormalizeBase(%q) = %q, %v", in, got, err)
		}
	}
	for _, bad := range []string{"", "ftp://x.pe", "https://"} {
		if _, err := NormalizeBase(bad); err == nil {
			t.Errorf("%q debía ser inválido", bad)
		}
	}
}

func TestCheckAndProvider(t *testing.T) {
	ctx := context.Background()
	suspended := false
	srv := fakeServer(t, []byte("MZ"), &suspended)
	defer srv.Close()
	info, err := Check(ctx, srv.URL, "buen-token")
	if err != nil || info.Station.Frequency != "93.3 FM" || info.Latest == nil || info.Latest.Version != "1.5.0" || info.Provider.Phone != "999111222" {
		t.Fatalf("Check = %+v, %v", info, err)
	}
	if _, err := Check(ctx, srv.URL, "malo"); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("token malo: %v", err)
	}
	if _, err := Check(ctx, srv.URL, ""); !errors.Is(err, ErrNoCredentials) {
		t.Fatalf("sin token: %v", err)
	}
	suspended = true
	if _, err := Check(ctx, srv.URL, "buen-token"); !errors.Is(err, ErrSuspended) {
		t.Fatalf("suspendido: %v", err)
	}
	var off *OfflineError
	if _, err := Check(ctx, "http://127.0.0.1:1", "buen-token"); !errors.As(err, &off) {
		t.Fatalf("sin conexión: %v", err)
	}
	if p, err := FetchProvider(ctx, srv.URL); err != nil || p.URL != "https://proveedor.pe" {
		t.Fatalf("FetchProvider = %+v, %v", p, err)
	}
	if p, err := FetchProvider(ctx, "http://127.0.0.1:1"); err == nil || p != DefaultProvider {
		t.Fatalf("sin conexión debe dar el de fábrica: %+v", p)
	}
}

func TestDecide(t *testing.T) {
	now := time.Date(2026, 11, 18, 12, 0, 0, 0, time.UTC)
	info := Info{Station: Station{Name: "Radio"}, Provider: Provider{Phone: "1", URL: "https://p.pe"}, Latest: &Release{Version: "1.5.0"}}
	// Autenticación correcta → caché nueva.
	st, cache := Decide("radio.pe", "tok", info, nil, nil, now)
	if !st.Authenticated || st.Offline || cache == nil || cache.TokenHash != TokenHash("tok") || st.Base != "https://radio.pe" {
		t.Fatalf("ok: %+v %+v", st, cache)
	}
	offErr := &OfflineError{Err: errors.New("sin red")}
	// Sin conexión dentro de la gracia → sigue autenticado (offline).
	st, c2 := Decide("https://radio.pe", "tok", Info{}, offErr, cache, now.Add(10*time.Hour))
	if !st.Authenticated || !st.Offline || c2 != cache || st.Latest == nil {
		t.Fatalf("offline en gracia: %+v", st)
	}
	// Sin conexión fuera de la gracia → no autenticado, pero la caché se conserva.
	st, c3 := Decide("radio.pe", "tok", Info{}, offErr, cache, now.Add(OfflineGrace+time.Minute))
	if st.Authenticated || c3 != cache || !strings.Contains(st.Reason, "72 horas") {
		t.Fatalf("offline vencido: %+v", st)
	}
	// Otro token u otro dominio no aprovechan la caché.
	if st, _ := Decide("radio.pe", "otro", Info{}, offErr, cache, now); st.Authenticated {
		t.Fatal("otro token no debe usar la caché")
	}
	if st, _ := Decide("otra.pe", "tok", Info{}, offErr, cache, now); st.Authenticated {
		t.Fatal("otro dominio no debe usar la caché")
	}
	// Token rechazado o suspendido → se borra la caché al instante.
	for _, e := range []error{ErrUnauthorized, ErrSuspended} {
		st, c := Decide("radio.pe", "tok", Info{}, e, cache, now)
		if st.Authenticated || c != nil || st.Provider.Phone != "1" {
			t.Fatalf("%v: %+v %+v", e, st, c)
		}
	}
}

func TestConfigAndDownload(t *testing.T) {
	ctx := context.Background()
	exe := append([]byte("MZ"), []byte(strings.Repeat("x", 5000))...)
	srv := fakeServer(t, exe, nil)
	defer srv.Close()
	if _, _, err := DownloadConfig(ctx, srv.URL, "buen-token"); err == nil {
		t.Fatal("sin copia debe avisar")
	}
	if err := UploadConfig(ctx, srv.URL, "buen-token", []byte(`{"radio":{"host":"x"}}`)); err != nil {
		t.Fatal(err)
	}
	b, ts, err := DownloadConfig(ctx, srv.URL, "buen-token")
	if err != nil || string(b) != `{"radio":{"host":"x"}}` || ts.IsZero() {
		t.Fatalf("DownloadConfig = %s %v %v", b, ts, err)
	}
	info, _ := Check(ctx, srv.URL, "buen-token")
	dst := filepath.Join(t.TempDir(), "nuevo.exe")
	var last int64
	if err := Download(ctx, srv.URL, "buen-token", *info.Latest, dst, func(d, _ int64) { last = d }); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(dst); string(got) != string(exe) || last != int64(len(exe)) {
		t.Fatalf("descarga: %d bytes, progreso %d", len(got), last)
	}
	// SHA-256 distinto: no se instala y no queda el archivo.
	bad := *info.Latest
	bad.SHA256 = strings.Repeat("0", 64)
	if err := Download(ctx, srv.URL, "buen-token", bad, dst+"2", nil); err == nil || !strings.Contains(err.Error(), "SHA-256") {
		t.Fatalf("SHA distinto: %v", err)
	}
	if _, err := os.Stat(dst + "2"); !os.IsNotExist(err) {
		t.Fatal("no debe quedar el archivo malo")
	}
	if err := Download(ctx, srv.URL, "malo", *info.Latest, dst+"3", nil); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("token malo: %v", err)
	}
}

func TestHeartbeat(t *testing.T) {
	ctx := context.Background()
	srv := fakeServer(t, []byte("MZ"), nil)
	defer srv.Close()
	r := Report{Radio: Output{State: "en_vivo", BitrateKbps: 128, Target: "x:8010/radio"}, TV: Output{State: "detenido"}}
	if latest, err := Heartbeat(ctx, srv.URL, "buen-token", "1.4.0", r); err != nil || latest != "1.5.0" {
		t.Fatalf("latido = %q, %v", latest, err)
	}
	// Token revocado o reemplazado: el servidor responde 401 al instante.
	if _, err := Heartbeat(ctx, srv.URL, "token-viejo", "1.5.0", r); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("token revocado: %v", err)
	}
	// Servidor anterior sin el endpoint: no es error.
	old := httptest.NewServer(http.NotFoundHandler())
	defer old.Close()
	if _, err := Heartbeat(ctx, old.URL, "buen-token", "1.5.0", r); err != nil {
		t.Fatalf("servidor sin latido: %v", err)
	}
}
