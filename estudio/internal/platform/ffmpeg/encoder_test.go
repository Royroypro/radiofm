package ffmpeg

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// fakeIcecast acepta el PUT de ffmpeg como haría Icecast y cuenta los bytes.
type fakeIcecast struct {
	srv         *httptest.Server
	bytes       atomic.Int64
	mu          sync.Mutex
	contentType string
	user, pass  string
	path        string
	reject      atomic.Bool
}

func newFakeIcecast(t *testing.T) *fakeIcecast {
	f := &fakeIcecast{}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if f.reject.Load() {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}
		f.mu.Lock()
		f.contentType = r.Header.Get("Content-Type")
		f.user, f.pass, _ = r.BasicAuth()
		f.path = r.URL.Path
		f.mu.Unlock()
		// ffmpeg manda el audio como flujo crudo (sin Content-Length ni chunked),
		// igual que espera Icecast: se toma la conexión y se lee el socket.
		hj, ok := w.(http.Hijacker)
		if !ok {
			http.Error(w, "sin hijack", http.StatusInternalServerError)
			return
		}
		conn, rw, err := hj.Hijack()
		if err != nil {
			return
		}
		defer conn.Close()
		if r.Header.Get("Expect") != "" {
			_, _ = rw.WriteString("HTTP/1.1 100 Continue\r\n\r\n")
		}
		_, _ = rw.WriteString("HTTP/1.0 200 OK\r\n\r\n")
		_ = rw.Flush()
		buf := make([]byte, 4096)
		for {
			n, err := rw.Read(buf)
			f.bytes.Add(int64(n))
			if err != nil {
				return
			}
		}
	}))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeIcecast) hostPort(t *testing.T) (string, int) {
	host, port, err := net.SplitHostPort(strings.TrimPrefix(f.srv.URL, "http://"))
	if err != nil {
		t.Fatal(err)
	}
	p, _ := strconv.Atoi(port)
	return host, p
}

// sineFeed es un SampleReader que siempre tiene audio (simula el motor).
type sineFeed struct{ discarded atomic.Int64 }

func (s *sineFeed) Read(p []float32) int {
	n := min(len(p), 44100*2/50) // 20 ms por lectura
	for i := 0; i < n; i++ {
		p[i] = 0.1
	}
	return n
}
func (s *sineFeed) Len() int          { return 44100 * 2 / 50 }
func (s *sineFeed) Discard(n int) int { s.discarded.Add(int64(n)); return n }

func TestEncoderStreamsToIcecast(t *testing.T) {
	tl := tools(t)
	ice := newFakeIcecast(t)
	host, port := ice.hostPort(t)
	enc := tl.NewEncoder(EncoderConfig{Host: host, Port: port, Mount: "/radio", Password: "cl@ve:1",
		Name: "Radio Club 25"}, &sineFeed{})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { enc.Run(ctx); close(done) }()
	deadline := time.Now().Add(15 * time.Second)
	for !enc.Status().Connected && time.Now().Before(deadline) {
		time.Sleep(100 * time.Millisecond)
	}
	time.Sleep(time.Second)
	st := enc.Status()
	cancel()
	<-done
	if !st.Connected {
		t.Fatalf("no llegó a conectarse: %+v", st)
	}
	// 128 kbps ≈ 16 KB/s; tras ~4 s debe haber recibido bastante
	if b := ice.bytes.Load(); b < 20000 {
		t.Fatalf("Icecast recibió %d bytes, esperaba > 20000", b)
	}
	ice.mu.Lock()
	defer ice.mu.Unlock()
	if ice.contentType != "audio/mpeg" || ice.user != "source" || ice.pass != "cl@ve:1" || ice.path != "/radio" {
		t.Fatalf("petición inesperada: tipo=%q user=%q pass=%q path=%q", ice.contentType, ice.user, ice.pass, ice.path)
	}
}

func TestEncoderReconnects(t *testing.T) {
	tl := tools(t)
	ice := newFakeIcecast(t)
	ice.reject.Store(true) // Icecast rechaza la fuente (p. ej. licencia bloqueada)
	host, port := ice.hostPort(t)
	enc := tl.NewEncoder(EncoderConfig{Host: host, Port: port, Password: "x"}, &sineFeed{})
	enc.MinBackoff, enc.MaxBackoff = 50*time.Millisecond, 200*time.Millisecond
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { enc.Run(ctx); close(done) }()
	deadline := time.Now().Add(15 * time.Second)
	for enc.Status().Retries < 2 && time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
	}
	if st := enc.Status(); st.Retries < 2 || st.Connected || st.LastError == "" {
		t.Fatalf("con rechazo debe reintentar y guardar el error: %+v", st)
	}
	// el servidor vuelve a aceptar: debe conectarse solo
	ice.reject.Store(false)
	deadline = time.Now().Add(20 * time.Second)
	for !enc.Status().Connected && time.Now().Before(deadline) {
		time.Sleep(100 * time.Millisecond)
	}
	connected := enc.Status().Connected
	cancel()
	<-done
	if !connected {
		t.Fatal("debe reconectar cuando Icecast vuelve a aceptar")
	}
}

func TestEncoderArgs(t *testing.T) {
	c := EncoderConfig{Host: "radio.example", Port: 8010, Password: "a b/c", Format: "aac"}.withDefaults()
	a := strings.Join(c.args(), " ")
	for _, want := range []string{"-c:a aac", "-b:a 96k", "-content_type audio/aac", "-f adts",
		"icecast://source:a%20b%2Fc@radio.example:8010/radio"} {
		if !strings.Contains(a, want) {
			t.Fatalf("args %q no contienen %q", a, want)
		}
	}
}
