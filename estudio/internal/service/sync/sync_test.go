package sync

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	stdsync "sync"
	"sync/atomic"
	"testing"
	"time"

	"radioclub25/estudio/internal/platform/apiclient"
	"radioclub25/estudio/internal/platform/sqlite"
)

// panel simula el panel web: dos cuñas, heartbeat, nowplaying y airplays.
type panel struct {
	srv        *httptest.Server
	audio      []byte
	spotsJSON  atomic.Value
	heartbeats atomic.Int32
	nowPlaying atomic.Value
	mu         stdsync.Mutex
	airplays   []apiclient.Airplay
	down       atomic.Bool
	cmdSent    atomic.Bool
	cmdDone    atomic.Value // resultado informado por la PC
}

func newPanel(t *testing.T) *panel {
	p := &panel{audio: []byte("ID3 cuña de prueba")}
	mux := http.NewServeMux()
	ok := func(h http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if p.down.Load() {
				http.Error(w, "caído", http.StatusBadGateway)
				return
			}
			if r.Header.Get("Authorization") != "Bearer c25s_x" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			h(w, r)
		}
	}
	mux.HandleFunc("/api/studio/config", ok(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"station":{"name":"Radio Club 25","timezone":"America/Lima","latitude":-11.1,"longitude":-77.6},"icecast":{"host":"h","port":8010,"mount":"/radio"}}`))
	}))
	mux.HandleFunc("/api/studio/spots", ok(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(p.spotsJSON.Load().(string)))
	}))
	mux.HandleFunc("/api/studio/heartbeat", ok(func(w http.ResponseWriter, r *http.Request) {
		p.heartbeats.Add(1)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	mux.HandleFunc("/api/studio/nowplaying", ok(func(w http.ResponseWriter, r *http.Request) {
		var np apiclient.NowPlaying
		_ = json.NewDecoder(r.Body).Decode(&np)
		p.nowPlaying.Store(np.Artist + " - " + np.Title)
		w.WriteHeader(http.StatusNoContent)
	}))
	mux.HandleFunc("/api/studio/airplays", ok(func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Airplays []apiclient.Airplay `json:"airplays"`
		}
		_ = json.NewDecoder(r.Body).Decode(&in)
		p.mu.Lock()
		p.airplays = append(p.airplays, in.Airplays...)
		p.mu.Unlock()
		_, _ = w.Write([]byte(`{"received":1,"inserted":1,"ignored":0,"rejected":[]}`))
	}))
	mux.HandleFunc("/api/studio/commands", ok(func(w http.ResponseWriter, r *http.Request) {
		if p.cmdSent.CompareAndSwap(false, true) { // una orden, una sola vez
			_, _ = w.Write([]byte(`{"commands":[{"id":7,"kind":"next","arg":"","created_at":"2026-11-11T10:00:00Z"}]}`))
			return
		}
		_, _ = w.Write([]byte(`{"commands":[]}`))
	}))
	mux.HandleFunc("/api/studio/commands/7/done", ok(func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			OK     bool   `json:"ok"`
			Result string `json:"result"`
		}
		_ = json.NewDecoder(r.Body).Decode(&in)
		if in.OK {
			p.cmdDone.Store(in.Result)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	mux.HandleFunc("/uploads/fm/", func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(p.audio) })
	p.srv = httptest.NewServer(mux)
	t.Cleanup(p.srv.Close)
	return p
}

func (p *panel) setSpots(ids ...int) {
	sum := sha256.Sum256(p.audio)
	s := `{"spots":[`
	for i, id := range ids {
		if i > 0 {
			s += ","
		}
		s += `{"id":` + itoa(id) + `,"advertiser":"Anunciante ` + itoa(id) + `","name":"Cuña","url":"` + p.srv.URL +
			`/uploads/fm/c` + itoa(id) + `.mp3","sha256":"` + hex.EncodeToString(sum[:]) +
			`","duration_ms":20000,"plays_per_day":4,"hour_from":0,"hour_to":24,"updated_at":"2026-11-08T10:00:00Z"}`
	}
	p.spotsJSON.Store(s + `]}`)
}

func itoa(n int) string { return string(rune('0' + n)) }

func waitFor(d time.Duration, f func() bool) bool {
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if f() {
			return true
		}
		time.Sleep(20 * time.Millisecond)
	}
	return false
}

func TestSync(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	db, err := sqlite.Open(ctx, filepath.Join(t.TempDir(), "e.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	p := newPanel(t)
	p.setSpots(1, 2)
	spots, queue := sqlite.NewSpotRepo(db), sqlite.NewAirplayQueue(db)
	var applied atomic.Value
	var replans atomic.Int32
	dir := t.TempDir()
	svc := New(Deps{
		ServerURL: func() string { return p.srv.URL }, Token: func() (string, error) { return "c25s_x", nil }, Version: "test",
		Spots: spots, Queue: queue, KV: sqlite.NewKV(db), SpotsDir: dir,
		ApplyConfig:  func(c apiclient.Config) { applied.Store(c.Station.Timezone) },
		SpotsChanged: func() { replans.Add(1) },
		Mode:         func() string { return "auto" }, Encoder: func() any { return map[string]any{"connected": true} },
		Command: func(kind, arg string) (string, error) {
			if kind != "next" {
				return "", errors.New("inesperada")
			}
			return "pasó a lo siguiente", nil
		},
		Log: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	go svc.Run(ctx)

	if !waitFor(5*time.Second, func() bool { return svc.Status().Spots == 2 && p.heartbeats.Load() > 0 }) {
		t.Fatalf("estado = %+v, heartbeats %d", svc.Status(), p.heartbeats.Load())
	}
	if applied.Load() != "America/Lima" || replans.Load() == 0 {
		t.Fatal("debe aplicar la configuración y replanificar al llegar cuñas")
	}
	list, _ := spots.List(ctx)
	for _, s := range list {
		b, err := os.ReadFile(s.Path)
		if err != nil || string(b) != string(p.audio) || s.Duration != 20*time.Second || !s.Active {
			t.Fatalf("cuña local mal sincronizada: %+v (%v)", s, err)
		}
	}

	// control remoto: la orden del panel se ejecuta y se informa
	if !waitFor(8*time.Second, func() bool { return p.cmdDone.Load() == "pasó a lo siguiente" }) {
		t.Fatal("la orden del panel debe ejecutarse y responderse")
	}

	// lo que suena y emisiones
	svc.NowPlaying("Grupo 5", "Motor y motivo", "music")
	svc.EnqueueAirplay(ctx, 1, "uuid-00000001", time.Now(), 20*time.Second, true)
	if !waitFor(5*time.Second, func() bool {
		p.mu.Lock()
		defer p.mu.Unlock()
		return len(p.airplays) == 1 && p.nowPlaying.Load() == "Grupo 5 - Motor y motivo"
	}) {
		t.Fatalf("no llegaron la emisión o lo que suena: %+v", svc.Status())
	}
	if n, _ := queue.Count(ctx); n != 0 {
		t.Fatalf("la cola debe vaciarse, quedan %d", n)
	}

	// sin red: la emisión queda en cola y se envía al volver
	p.down.Store(true)
	svc.EnqueueAirplay(ctx, 2, "uuid-00000002", time.Now(), 20*time.Second, true)
	if !waitFor(3*time.Second, func() bool { return !svc.Status().Online }) {
		t.Fatal("debe marcarse sin conexión")
	}
	if n, _ := queue.Count(ctx); n != 1 {
		t.Fatalf("sin red la emisión debe quedar en cola: %d", n)
	}
	p.down.Store(false)
	svc.SyncNow()
	if !waitFor(5*time.Second, func() bool { n, _ := queue.Count(ctx); return n == 0 }) {
		t.Fatal("al volver la red la cola debe enviarse")
	}

	// el panel deja de enviar la cuña 2: se desactiva localmente
	p.setSpots(1)
	svc.SyncNow()
	if !waitFor(5*time.Second, func() bool {
		s, err := spots.ByRemoteID(ctx, 2)
		return err == nil && !s.Active
	}) {
		t.Fatal("una cuña que el panel ya no envía debe desactivarse")
	}
	if s, _ := spots.ByRemoteID(ctx, 1); s == nil || !s.Active {
		t.Fatal("la cuña 1 sigue activa")
	}
}
