package secrets

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStore(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(dir)
	if v, err := s.Get(ServerToken); err != nil || v != "" || s.Has(ServerToken) {
		t.Fatalf("vacío: %q %v", v, err)
	}
	if err := s.Set(ServerToken, "c25s_abc123"); err != nil {
		t.Fatal(err)
	}
	if v, err := s.Get(ServerToken); err != nil || v != "c25s_abc123" || !s.Has(ServerToken) {
		t.Fatalf("Get = %q, %v", v, err)
	}
	b, _ := os.ReadFile(filepath.Join(dir, "secrets.json"))
	if strings.Contains(string(b), "c25s_abc123") {
		t.Fatal("el secreto no debe quedar en claro en el archivo")
	}
	if err := s.Set(ServerToken, ""); err != nil || s.Has(ServerToken) {
		t.Fatal("Set vacío debe borrar")
	}
}
