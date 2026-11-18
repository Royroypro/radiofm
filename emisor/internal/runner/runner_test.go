package runner

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"
)

// TestHelperProcess hace de ffmpeg falso. Solo actúa si lo lanza fakeCmd.
func TestHelperProcess(t *testing.T) {
	mode := os.Getenv("EMISOR_FAKE_FFMPEG")
	if mode == "" {
		return
	}
	defer os.Exit(0)
	switch mode {
	case "live": // emite progreso y niveles hasta recibir «q»
		quit := make(chan struct{})
		go func() {
			sc := bufio.NewScanner(os.Stdin)
			for sc.Scan() {
				l := strings.TrimSpace(sc.Text())
				if l == "q" {
					close(quit)
					return
				}
				if strings.HasPrefix(l, "c") { // comando a un filtro, como ffmpeg
					fmt.Fprintf(os.Stderr, "recibido %s\n", l)
				}
			}
		}()
		for i := 1; ; i++ {
			fmt.Printf("frame:%d    pts:%d    pts_time:%d\nlavfi.astats.1.Peak_level=-6.0\nlavfi.astats.1.RMS_level=-12.0\n"+
				"lavfi.astats.2.Peak_level=-7.0\nlavfi.astats.2.RMS_level=-13.0\n", i, i, i)
			fmt.Fprintf(os.Stderr, "total_size=%d\nout_time_us=%d\nbitrate=128.0kbits/s\nspeed=1.00x\nprogress=continue\n", i*16000, i*100000)
			select {
			case <-quit:
				fmt.Fprintln(os.Stderr, "progress=end")
				return
			case <-time.After(20 * time.Millisecond):
			}
		}
	case "fail401":
		fmt.Fprintln(os.Stderr, "[http @ 0x1] HTTP error 401 Unauthorized")
		os.Exit(1)
	case "failnet":
		fmt.Fprintln(os.Stderr, "[tcp @ 0x1] Connection to tcp://h:8010 failed: Connection refused")
		os.Exit(1)
	case "hang": // conecta pero nunca envía datos
		time.Sleep(time.Minute)
	}
}

// fakeCmd lanza este mismo binario de test como ffmpeg falso.
func fakeCmd(mode string, starts *int, mu *sync.Mutex) func(ctx context.Context, name string, args ...string) *exec.Cmd {
	return func(ctx context.Context, name string, args ...string) *exec.Cmd {
		mu.Lock()
		*starts++
		mu.Unlock()
		cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestHelperProcess$")
		cmd.Env = append(os.Environ(), "EMISOR_FAKE_FFMPEG="+mode)
		return cmd
	}
}

// recorder guarda los estados y los registros.
type recorder struct {
	mu     sync.Mutex
	states []Status
	logs   []string
}

func (r *recorder) status(s Status) { r.mu.Lock(); r.states = append(r.states, s); r.mu.Unlock() }
func (r *recorder) log(l string)    { r.mu.Lock(); r.logs = append(r.logs, l); r.mu.Unlock() }

func (r *recorder) waitFor(t *testing.T, what string, ok func(Status) bool) Status {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		r.mu.Lock()
		for _, s := range r.states {
			if ok(s) {
				r.mu.Unlock()
				return s
			}
		}
		r.mu.Unlock()
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("no llegó el estado: %s", what)
	return Status{}
}

func newRunner(mode string, rec *recorder, starts *int, mu *sync.Mutex) *Runner {
	return New(Options{
		Name: "radio", FFmpeg: "ffmpeg",
		Args:       func() ([]string, error) { return []string{"-i", "x", "icecast://source:secreta@h:8010/radio"}, nil },
		Redact:     func(s string) string { return strings.ReplaceAll(s, "secreta", "****") },
		OnStatus:   rec.status,
		OnLog:      rec.log,
		MinBackoff: 50 * time.Millisecond, MaxBackoff: 200 * time.Millisecond,
		StallTimeout: 1500 * time.Millisecond, StableAfter: time.Hour, StopGrace: 2 * time.Second,
		NewCmd: fakeCmd(mode, starts, mu),
	})
}

func TestLiveAndStop(t *testing.T) {
	rec := &recorder{}
	var starts int
	var mu sync.Mutex
	r := newRunner("live", rec, &starts, &mu)
	if err := r.Start(); err != nil {
		t.Fatal(err)
	}
	if err := r.Start(); err != nil { // segundo Start: no hace nada
		t.Fatal(err)
	}
	rec.waitFor(t, "conectando", func(s Status) bool { return s.State == Connecting })
	live := rec.waitFor(t, "en vivo con niveles", func(s Status) bool {
		return s.State == Live && len(s.Levels) == 2 && s.SentBytes > 0
	})
	if live.Levels[0].PeakDB != -6 || live.Levels[1].RMSDB != -13 || live.BitrateKbps != 128 || live.LiveSince.IsZero() {
		t.Fatalf("estado en vivo: %+v", live)
	}
	start := time.Now()
	r.Stop()
	if time.Since(start) > 1500*time.Millisecond {
		t.Fatalf("Stop tardó %s: debía cerrar con «q» sin esperar a matarlo", time.Since(start))
	}
	if s := r.Status(); s.State != Stopped || r.Running() {
		t.Fatalf("tras Stop: %+v", s)
	}
	mu.Lock()
	defer mu.Unlock()
	if starts != 1 {
		t.Fatalf("ffmpeg se lanzó %d veces, esperaba 1", starts)
	}
	rec.mu.Lock()
	defer rec.mu.Unlock()
	for _, l := range rec.logs {
		if strings.Contains(l, "secreta") {
			t.Fatalf("la clave quedó en el registro: %s", l)
		}
	}
}

func TestReconnectsWithHint(t *testing.T) {
	rec := &recorder{}
	var starts int
	var mu sync.Mutex
	r := newRunner("fail401", rec, &starts, &mu)
	if err := r.Start(); err != nil {
		t.Fatal(err)
	}
	s := rec.waitFor(t, "reconectando (2º intento)", func(s Status) bool { return s.State == Reconnecting && s.Attempt >= 2 })
	if !strings.Contains(s.Message, "401") {
		t.Fatalf("el motivo debe explicar el 401: %q", s.Message)
	}
	// Con la clave rechazada deja de insistir solo, tras MaxAuthFails intentos.
	s = rec.waitFor(t, "detenido por clave", func(s Status) bool { return s.State == Stopped && strings.Contains(s.Message, "401") })
	time.Sleep(300 * time.Millisecond)
	mu.Lock()
	n := starts
	mu.Unlock()
	if n != MaxAuthFails || r.Running() {
		t.Fatalf("debía intentar %d veces y quedar detenido; intentos=%d, activo=%v", MaxAuthFails, n, r.Running())
	}
	// Se puede volver a iniciar (por ejemplo, tras corregir la clave).
	if err := r.Start(); err != nil || !r.Running() {
		t.Fatalf("no se pudo reiniciar: %v", err)
	}
	r.Stop()
}

func TestStallRestarts(t *testing.T) {
	rec := &recorder{}
	var starts int
	var mu sync.Mutex
	r := newRunner("hang", rec, &starts, &mu)
	if err := r.Start(); err != nil {
		t.Fatal(err)
	}
	s := rec.waitFor(t, "reconectando tras cuelgue", func(s Status) bool { return s.State == Reconnecting })
	if !strings.Contains(s.Message, "sin avanzar") {
		t.Fatalf("motivo: %q", s.Message)
	}
	r.Stop()
}

func TestStartRejectsInvalidConfig(t *testing.T) {
	r := New(Options{Name: "tv", Args: func() ([]string, error) { return nil, errors.New("Falta la clave de la TV.") }})
	if err := r.Start(); err == nil || r.Running() {
		t.Fatal("con configuración inválida no debe arrancar")
	}
	r.Stop() // detener algo detenido no falla
}

func TestBackoffGrowsAndCaps(t *testing.T) {
	rec := &recorder{}
	var starts int
	var mu sync.Mutex
	r := newRunner("failnet", rec, &starts, &mu)
	r.opt.MinBackoff, r.opt.MaxBackoff = 1*time.Second, 2*time.Second
	_ = r.Start()
	// 1 s, luego 2 s, luego sigue en 2 s (tope): el 3er reintento muestra a lo sumo 2 s.
	s := rec.waitFor(t, "tercer reintento", func(s Status) bool { return s.State == Reconnecting && s.Attempt == 3 })
	r.Stop()
	if s.RetryInSec > 2 {
		t.Fatalf("la espera pasó el tope: %d s", s.RetryInSec)
	}
	rec.mu.Lock()
	defer rec.mu.Unlock()
	saw2 := false
	for _, st := range rec.states {
		if st.State == Reconnecting && st.Attempt == 2 && st.RetryInSec == 2 {
			saw2 = true
		}
	}
	if !saw2 {
		t.Fatal("el segundo reintento debía esperar 2 s (espera creciente)")
	}
}

func TestSendAndRestart(t *testing.T) {
	rec := &recorder{}
	var starts int
	var mu sync.Mutex
	r := newRunner("live", rec, &starts, &mu)
	if err := r.Send([]string{"cx -1 g 1\n"}); err != ErrNotRunning {
		t.Fatalf("Send sin proceso: %v", err)
	}
	r.Restart() // detenido: no hace nada
	if r.Running() {
		t.Fatal("Restart no debe iniciar una salida detenida")
	}
	if err := r.Start(); err != nil {
		t.Fatal(err)
	}
	defer r.Stop()
	rec.waitFor(t, "en vivo", func(s Status) bool { return s.State == Live })

	// Send llega al ffmpeg que está al aire.
	if err := r.Send([]string{"cbass@mejora -1 g 4\n"}); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for !logged(rec, "recibido cbass@mejora -1 g 4") {
		if time.Now().After(deadline) {
			t.Fatal("el comando no llegó a ffmpeg")
		}
		time.Sleep(20 * time.Millisecond)
	}

	// Restart: un ffmpeg nuevo en seguida, sin pasar por «Reconectando».
	rec.mu.Lock()
	rec.states = nil
	rec.mu.Unlock()
	start := time.Now()
	r.Restart()
	rec.waitFor(t, "reconectando para aplicar", func(s Status) bool { return s.State == Connecting })
	rec.mu.Lock()
	rec.states = nil
	rec.mu.Unlock()
	rec.waitFor(t, "en vivo otra vez", func(s Status) bool { return s.State == Live })
	mu.Lock()
	n := starts
	mu.Unlock()
	if n != 2 {
		t.Fatalf("ffmpeg se lanzó %d veces, esperaba 2", n)
	}
	rec.mu.Lock()
	defer rec.mu.Unlock()
	for _, st := range rec.states {
		if st.State == Reconnecting {
			t.Fatal("Aplicar no debe pasar por la espera de reconexión")
		}
	}
	if time.Since(start) > 3*time.Second {
		t.Fatalf("el reinicio tardó %s", time.Since(start))
	}
}

func logged(rec *recorder, sub string) bool {
	rec.mu.Lock()
	defer rec.mu.Unlock()
	for _, l := range rec.logs {
		if strings.Contains(l, sub) {
			return true
		}
	}
	return false
}
