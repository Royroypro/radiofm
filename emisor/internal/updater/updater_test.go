package updater

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestSwapAndCleanup(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "club25-emisor.exe")
	nw := filepath.Join(dir, "club25-emisor.exe.new")
	os.WriteFile(exe, []byte("viejo"), 0o755)
	os.WriteFile(nw, []byte("nuevo"), 0o755)
	os.WriteFile(exe+".old", []byte("de antes"), 0o755) // queda de una actualización anterior
	if err := Swap(exe, nw); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(exe); string(b) != "nuevo" {
		t.Fatalf("exe = %s", b)
	}
	if b, _ := os.ReadFile(exe + ".old"); string(b) != "viejo" {
		t.Fatalf(".old = %s", b)
	}
	if _, err := os.Stat(nw); !os.IsNotExist(err) {
		t.Fatal("el .new debía moverse")
	}
	CleanupOld(exe)
	if _, err := os.Stat(exe + ".old"); !os.IsNotExist(err) {
		t.Fatal("CleanupOld debía borrar el .old")
	}
	// Sin archivo nuevo: error y el programa actual queda intacto.
	if err := Swap(exe, filepath.Join(dir, "no-existe")); err == nil {
		t.Fatal("debía fallar")
	}
	if b, _ := os.ReadFile(exe); string(b) != "nuevo" {
		t.Fatal("el programa actual no debe tocarse si falla")
	}
}

func TestRelaunchArgs(t *testing.T) {
	got := RelaunchArgs(1234, []string{"radio", "tv"}, `D:\datos emisor`)
	want := []string{"--esperar-pid", "1234", "--reanudar", "radio,tv", "-datos", `D:\datos emisor`}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("RelaunchArgs = %q", got)
	}
	if got := RelaunchArgs(1, nil, ""); !reflect.DeepEqual(got, []string{"--esperar-pid", "1"}) {
		t.Fatalf("sin reanudar = %q", got)
	}
	if got := ParseResume(" Radio, tv ,otra"); !reflect.DeepEqual(got, []string{"radio", "tv"}) {
		t.Fatalf("ParseResume = %q", got)
	}
}

func TestWaitPID(t *testing.T) {
	// Un proceso que ya terminó: vuelve en seguida.
	start := time.Now()
	WaitPID(999999, 5*time.Second)
	if time.Since(start) > 2*time.Second {
		t.Fatal("con un pid inexistente no debe esperar")
	}
	// Un proceso vivo: espera a que termine.
	cmd := exec.Command(os.Args[0], "-test.run=^$")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() { _ = cmd.Wait(); close(done) }()
	WaitPID(cmd.Process.Pid, 20*time.Second)
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("WaitPID volvió antes de que terminara el proceso")
	}
}

func TestNewer(t *testing.T) {
	for _, c := range []struct {
		a, b string
		want bool
	}{{"1.5.0", "1.4.0", true}, {"1.10.0", "1.9.3", true}, {"1.4.0", "1.4.0", false}, {"1.3.9", "1.4.0", false}, {"v2.0.0", "1.99.99", true}} {
		if got := Newer(c.a, c.b); got != c.want {
			t.Errorf("Newer(%s, %s) = %v", c.a, c.b, got)
		}
	}
}
