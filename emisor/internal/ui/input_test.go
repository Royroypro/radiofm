package ui

import (
	"context"
	"net"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"radioclub25/emisor/internal/config"
	"radioclub25/emisor/internal/icefake"
	"radioclub25/emisor/internal/runner"
)

// TestSetInputLive: con la radio al aire, SetInput y un Apply que solo cambia la
// entrada no reconectan; un Apply que cambia el bitrate sí.
func TestSetInputLive(t *testing.T) {
	ff, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("sin ffmpeg")
	}
	ice := icefake.New("source", "fuente")
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go ice.Serve(ln)
	defer ln.Close()

	a := NewApp()
	a.Emit = func(context.Context, string, any) {}
	a.ffmpeg = ff
	a.cfgPath = filepath.Join(t.TempDir(), "emisor.json")
	a.lic.st.Authenticated = true
	c := config.Defaults()
	c.Radio.Host, c.Radio.Port, c.Radio.Password = "127.0.0.1", ln.Addr().(*net.TCPAddr).Port, "fuente"
	c.Radio.Input = config.Source{Kind: config.SourceTest}
	a.cfg = c
	a.radio = runner.New(runner.Options{Name: "radio", FFmpeg: ff, Args: a.radioArgs, Redact: a.redact, OnLog: a.addLog})
	a.tv = runner.New(runner.Options{Name: "tv", FFmpeg: ff, Args: a.tvArgs, OnLog: a.addLog})
	defer a.Shutdown(context.Background())

	if err := a.Start("radio", c); err != nil {
		t.Fatal(err)
	}
	live := func() runner.Status {
		for end := time.Now().Add(15 * time.Second); time.Now().Before(end); time.Sleep(100 * time.Millisecond) {
			if s := a.radio.Status(); s.State == runner.Live {
				return s
			}
		}
		t.Fatalf("la radio no salió al aire: %v", a.logs)
		return runner.Status{}
	}
	first := live()

	restarted, err := a.SetInput("radio", config.Source{Kind: config.SourceNone})
	if err == nil {
		t.Fatal("la radio no admite silencio como entrada: debía rechazarse")
	}
	restarted, err = a.SetInput("radio", config.Source{Kind: config.SourceLoopback, Name: "__no_existe__"})
	if restarted {
		t.Fatal("SetInput no debía reconectar")
	}
	t.Logf("audio de la PC inexistente: %v", err)
	time.Sleep(time.Second)
	c2 := a.cfg
	c2.Radio.Input = config.Source{Kind: config.SourceTest}
	if restarted, err = a.Apply("radio", c2); err != nil || restarted {
		t.Fatalf("Apply con solo la entrada: restarted=%v err=%v", restarted, err)
	}
	time.Sleep(time.Second)
	if s := a.radio.Status(); s.State != runner.Live || !s.LiveSince.Equal(first.LiveSince) {
		t.Fatalf("cambiar la entrada no debía reconectar: %+v", s)
	}
	c3 := a.cfg
	c3.Radio.BitrateKbps = 192
	if restarted, err = a.Apply("radio", c3); err != nil || !restarted {
		t.Fatalf("Apply con otro bitrate debía reconectar: restarted=%v err=%v", restarted, err)
	}
	time.Sleep(500 * time.Millisecond)
	if s := live(); s.LiveSince.Equal(first.LiveSince) {
		t.Fatal("el bitrate nuevo exige reconectar")
	}
	a.logMu.Lock()
	for _, l := range a.logs {
		t.Log(l)
	}
	a.logMu.Unlock()
}
