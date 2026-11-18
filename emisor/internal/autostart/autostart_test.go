//go:build windows

package autostart

import (
	"testing"

	"golang.org/x/sys/windows/registry"
)

func TestCommand(t *testing.T) {
	got := Command(`C:\Program Files\Club25 Emisor\club25-emisor.exe`, "")
	if got != `"C:\Program Files\Club25 Emisor\club25-emisor.exe" --autoarranque` {
		t.Fatalf("Command = %s", got)
	}
	got = Command(`E:\emisor\club25-emisor.exe`, `D:\datos emisor`)
	if got != `"E:\emisor\club25-emisor.exe" --autoarranque -datos "D:\datos emisor"` {
		t.Fatalf("Command con datos = %s", got)
	}
}

// TestSetGet usa una clave de prueba propia (no la de arranque real) y la borra al terminar.
func TestSetGet(t *testing.T) {
	key := `Software\Club25EmisorPrueba\Run`
	t.Cleanup(func() {
		_ = registry.DeleteKey(registry.CURRENT_USER, key)
		_ = registry.DeleteKey(registry.CURRENT_USER, `Software\Club25EmisorPrueba`)
	})
	if _, ok, err := Get(key); ok || err != nil {
		t.Fatalf("al principio no debe existir: %v %v", ok, err)
	}
	cmd := Command(`C:\x\club25-emisor.exe`, "")
	if err := Set(key, true, cmd); err != nil {
		t.Fatal(err)
	}
	if v, ok, err := Get(key); !ok || err != nil || v != cmd {
		t.Fatalf("Get = %q %v %v", v, ok, err)
	}
	// Se movió el programa: la línea se actualiza.
	cmd2 := Command(`D:\nuevo\club25-emisor.exe`, "")
	if err := Set(key, true, cmd2); err != nil {
		t.Fatal(err)
	}
	if v, _, _ := Get(key); v != cmd2 {
		t.Fatalf("no se actualizó: %q", v)
	}
	if err := Set(key, false, ""); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := Get(key); ok {
		t.Fatal("debía quedar quitado")
	}
	if err := Set(key, false, ""); err != nil {
		t.Fatalf("quitar dos veces no es error: %v", err)
	}
}
