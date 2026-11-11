package updater

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestNewer(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"1.9.2", "1.9.1", true}, {"1.10.0", "1.9.9", true}, {"2.0.0", "1.99.99", true},
		{"1.9.1", "1.9.1", false}, {"1.8.9", "1.9.0", false}, {"v1.9.2", "1.9.1", true},
		{"1.9.2-beta", "1.9.1", true}, {"basura", "1.0.0", false}, {"1.0.0", "dev", false},
	}
	for _, c := range cases {
		if got := Newer(c.a, c.b); got != c.want {
			t.Errorf("Newer(%q, %q) = %v, quería %v", c.a, c.b, got, c.want)
		}
	}
}

// Con una versión más nueva en el servidor: la descarga, la verifica y queda
// lista; instalar arranca el ayudante con el instalador y el programa actual.
func TestCheckDownloadAndInstall(t *testing.T) {
	installer := []byte(strings.Repeat("instalador falso ", 1000))
	sum := sha256.Sum256(installer)
	latest := "1.9.2"
	mux := http.NewServeMux()
	mux.HandleFunc("/uploads/estudio/manifest.json", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"app_version": latest, "installer": map[string]any{
			"name": "x.exe", "url": "/uploads/estudio/app/x.exe", "sha256": hex.EncodeToString(sum[:]), "size": len(installer)}})
	})
	mux.HandleFunc("/uploads/estudio/app/x.exe", func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(installer) })
	srv := httptest.NewServer(mux)
	defer srv.Close()

	s := New(func() string { return srv.URL }, "1.9.1", t.TempDir(), slog.Default())
	var launched []string
	s.launch = func(script string, args ...string) error {
		launched = append([]string{script}, args...)
		return nil
	}
	if err := s.Install(); err == nil {
		t.Fatal("sin descargar no debe instalar")
	}
	s.check(context.Background())
	st := s.Status()
	if !st.Available || !st.Ready || st.Latest != latest || st.Error != "" {
		t.Fatalf("debe quedar lista: %+v", st)
	}
	if err := s.Install(); err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(launched, " ")
	if !strings.Contains(joined, "-Installer") || !strings.Contains(joined, "club25-estudio-installer.exe") || !strings.Contains(joined, "-ProcId") {
		t.Fatalf("el ayudante debe recibir el instalador y el proceso: %v", launched)
	}
	if b, err := os.ReadFile(launched[0]); err != nil || !strings.Contains(string(b), "-Verb RunAs") {
		t.Fatalf("el ayudante debe escribirse junto a la descarga: %v", err)
	}

	// misma versión: nada que hacer
	s2 := New(func() string { return srv.URL }, latest, t.TempDir(), slog.Default())
	s2.check(context.Background())
	if st := s2.Status(); st.Available || st.Ready {
		t.Fatalf("al día no debe ofrecer nada: %+v", st)
	}
	// instalador alterado: no queda lista
	installer[0] = 'X'
	s3 := New(func() string { return srv.URL }, "1.0.0", t.TempDir(), slog.Default())
	s3.check(context.Background())
	if st := s3.Status(); st.Ready || st.Error == "" {
		t.Fatalf("con el SHA-256 distinto no debe quedar lista: %+v", st)
	}
}
