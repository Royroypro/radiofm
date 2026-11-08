package weather

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestCurrentWithCache(t *testing.T) {
	var calls atomic.Int32
	fail := atomic.Bool{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if fail.Load() {
			http.Error(w, "caído", http.StatusBadGateway)
			return
		}
		if !strings.Contains(r.URL.RawQuery, "current=temperature_2m") || !strings.Contains(r.URL.RawQuery, "latitude=-11.1067") {
			t.Errorf("consulta inesperada: %s", r.URL.RawQuery)
		}
		_, _ = w.Write([]byte(`{"current":{"time":"2026-11-08T14:00","temperature_2m":21.4}}`))
	}))
	defer srv.Close()
	c := New()
	c.BaseURL, c.HTTP = srv.URL, srv.Client()
	v, err := c.Current(context.Background(), -11.1067, -77.605)
	if err != nil || v != 21.4 {
		t.Fatalf("Current = %v, %v", v, err)
	}
	if v, _ := c.Current(context.Background(), -11.1067, -77.605); v != 21.4 || calls.Load() != 1 {
		t.Fatalf("debe usar la caché: llamadas=%d", calls.Load())
	}
	// caché vencida y red caída: usa el último valor (menos de 1 h)
	c.TTL = 0
	fail.Store(true)
	if v, err := c.Current(context.Background(), -11.1067, -77.605); err != nil || v != 21.4 {
		t.Fatalf("sin red debe devolver el último valor: %v, %v", v, err)
	}
	// otra ubicación sin datos y sin red: error
	if _, err := c.Current(context.Background(), 0, 0); err == nil {
		t.Fatal("sin datos ni red debe dar error")
	}
}
