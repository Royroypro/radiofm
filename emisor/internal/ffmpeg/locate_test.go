package ffmpeg

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLocate(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "ffmpeg", "bin")
	os.MkdirAll(bin, 0o755)
	nested := filepath.Join(bin, exeName)
	os.WriteFile(nested, []byte("x"), 0o755)
	if got, err := Locate("", dir); err != nil || got != nested {
		t.Fatalf("Locate = %q, %v; esperaba %q", got, err, nested)
	}
	side := filepath.Join(dir, exeName)
	os.WriteFile(side, []byte("x"), 0o755)
	if got, _ := Locate("", dir); got != side {
		t.Fatalf("junto al programa tiene prioridad: %q", got)
	}
	if got, _ := Locate(nested, dir); got != nested {
		t.Fatalf("la ruta configurada tiene prioridad: %q", got)
	}
	if _, err := Locate(filepath.Join(dir, "no-existe.exe"), dir); err == nil {
		t.Fatal("una ruta configurada que no existe es error")
	}
}
