package feed

import (
	"context"
	"encoding/binary"
	"io"
	"math"
	"net"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	"radioclub25/emisor/internal/config"
)

// TestHelperProcess hace de ffmpeg de captura: escribe PCM (tono fuerte) por la salida.
func TestHelperProcess(t *testing.T) {
	if os.Getenv("FEED_FAKE_CAPTURE") == "" {
		return
	}
	defer os.Exit(0)
	stop := time.After(time.Duration(0))
	if os.Getenv("FEED_FAKE_CAPTURE") == "corta" {
		stop = time.After(300 * time.Millisecond) // simula un micrófono desconectado
	} else {
		stop = time.After(time.Hour)
	}
	buf := make([]byte, 960*4)
	for i := 0; i < 960; i++ {
		binary.LittleEndian.PutUint16(buf[i*4:], uint16(int16(20000)))
		binary.LittleEndian.PutUint16(buf[i*4+2:], uint16(int16(20000)))
	}
	t2 := time.NewTicker(20 * time.Millisecond)
	for {
		select {
		case <-stop:
			return
		case <-t2.C:
			if _, err := os.Stdout.Write(buf); err != nil {
				return
			}
		}
	}
}

func fakeCmd(mode string, starts *int, mu *sync.Mutex) func(ctx context.Context, name string, args ...string) *exec.Cmd {
	return func(ctx context.Context, name string, args ...string) *exec.Cmd {
		mu.Lock()
		*starts++
		mu.Unlock()
		cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestHelperProcess$")
		cmd.Env = append(os.Environ(), "FEED_FAKE_CAPTURE="+mode)
		return cmd
	}
}

// peak lee del bus durante d y devuelve el pico (0..32767).
func peak(t *testing.T, c net.Conn, d time.Duration) int {
	t.Helper()
	_ = c.SetReadDeadline(time.Now().Add(d))
	buf := make([]byte, 4096)
	max := 0
	for {
		n, err := c.Read(buf)
		for i := 0; i+1 < n; i += 2 {
			v := int(int16(binary.LittleEndian.Uint16(buf[i:])))
			if v < 0 {
				v = -v
			}
			if v > max {
				max = v
			}
		}
		if err != nil {
			return max
		}
	}
}

func dial(t *testing.T, f *Feed) net.Conn {
	t.Helper()
	c, err := net.Dial("tcp", strings.TrimPrefix(f.Addr(), "tcp://"))
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(100 * time.Millisecond)
	return c
}

func TestSwitchWithoutReconnecting(t *testing.T) {
	var logs []string
	var lmu sync.Mutex
	var starts int
	var mu sync.Mutex
	f, err := New(Options{Name: "radio", FFmpeg: "ffmpeg", NewCmd: fakeCmd("ok", &starts, &mu),
		OnLog: func(l string) { lmu.Lock(); logs = append(logs, l); lmu.Unlock() },
		Capture: func(name string, on func([]byte)) (func(), error) {
			return Tone(on, 440, -6), nil // «audio de la PC» falso
		}})
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	c := dial(t, f) // el «codificador»: UNA sola conexión durante toda la prueba
	defer c.Close()

	if p := peak(t, c, 400*time.Millisecond); p != 0 {
		t.Fatalf("sin fuente debe ser silencio, pico %d", p)
	}
	steps := []struct {
		src      config.Source
		min, max int
	}{
		{config.Source{Kind: config.SourceTest}, 3000, 5000},                     // tono a -18 dBFS ≈ 4125
		{config.Source{Kind: config.SourceDShow, Name: "Mic USB"}, 19000, 21000}, // captura falsa a 20000
		{config.Source{Kind: config.SourceLoopback}, 15000, 17000},               // -6 dBFS ≈ 16422
		{config.Source{Kind: config.SourceNone}, 0, 0},
	}
	for _, st := range steps {
		if err := f.Switch(st.src); err != nil {
			t.Fatalf("%s: %v", st.src.Kind, err)
		}
		_ = peak(t, c, 300*time.Millisecond) // descartar la transición
		if p := peak(t, c, 500*time.Millisecond); p < st.min || p > st.max {
			t.Fatalf("%s: pico %d, esperaba entre %d y %d", st.src.Kind, p, st.min, st.max)
		}
		if f.Source().Kind != st.src.Kind {
			t.Fatalf("Source = %+v", f.Source())
		}
	}
	// La misma fuente dos veces no reinicia nada.
	_ = f.Switch(config.Source{Kind: config.SourceDShow, Name: "Mic USB"})
	_ = f.Switch(config.Source{Kind: config.SourceDShow, Name: "Mic USB"})
	time.Sleep(500 * time.Millisecond) // la captura arranca en segundo plano
	mu.Lock()
	n := starts
	mu.Unlock()
	if n != 2 {
		t.Fatalf("procesos de captura = %d, esperaba 2", n)
	}
	if err := f.Switch(config.Source{Kind: config.SourceFromVideo}); err == nil {
		t.Fatal("«el audio del video» no pasa por el bus")
	}
	lmu.Lock()
	defer lmu.Unlock()
	if len(logs) == 0 || !strings.Contains(logs[0], "sin reconectar") {
		t.Fatalf("registro: %q", logs)
	}
}

func TestCaptureRestartsWhenDeviceDrops(t *testing.T) {
	var starts int
	var mu sync.Mutex
	var logs []string
	var lmu sync.Mutex
	f, err := New(Options{Name: "radio", FFmpeg: "ffmpeg", NewCmd: fakeCmd("corta", &starts, &mu),
		OnLog: func(l string) { lmu.Lock(); logs = append(logs, l); lmu.Unlock() }})
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	c := dial(t, f)
	defer c.Close()
	if err := f.Switch(config.Source{Kind: config.SourceDShow, Name: "Mic"}); err != nil {
		t.Fatal(err)
	}
	// Se corta a los 300 ms; el bus sigue entregando (silencio) y la captura se reabre a los 3 s.
	deadline := time.Now().Add(6 * time.Second)
	for time.Now().Before(deadline) {
		go io.Copy(io.Discard, c)
		mu.Lock()
		n := starts
		mu.Unlock()
		if n >= 2 {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	mu.Lock()
	n := starts
	mu.Unlock()
	if n < 2 {
		t.Fatalf("la captura debía reintentarse, procesos = %d", n)
	}
	lmu.Lock()
	defer lmu.Unlock()
	if len(logs) < 2 || !strings.Contains(strings.Join(logs, "|"), "se reintenta") {
		t.Fatalf("registro: %q", logs)
	}
}

func TestToneLevelAndArgs(t *testing.T) {
	var got []byte
	var mu sync.Mutex
	stop := Tone(func(b []byte) { mu.Lock(); got = append(got, b...); mu.Unlock() }, 1000, -6)
	time.Sleep(150 * time.Millisecond)
	stop()
	mu.Lock()
	defer mu.Unlock()
	max := 0
	for i := 0; i+1 < len(got); i += 2 {
		v := int(int16(binary.LittleEndian.Uint16(got[i:])))
		if v > max {
			max = v
		}
	}
	want := int(32767 * math.Pow(10, -6.0/20))
	if len(got) == 0 || max < want-200 || max > want+50 {
		t.Fatalf("tono: %d bytes, pico %d, esperaba ≈ %d", len(got), max, want)
	}
	a := strings.Join(CaptureArgs("Micrófono (USB)"), " ")
	if !strings.Contains(a, "-f dshow") || !strings.Contains(a, "-i audio=Micrófono (USB)") || !strings.HasSuffix(a, "-f s16le -ar 48000 -ac 2 pipe:1") {
		t.Fatalf("CaptureArgs = %s", a)
	}
	if Describe(config.Source{Kind: config.SourceLoopback}) != "audio de la PC (salida predeterminada)" {
		t.Fatal("Describe")
	}
}
