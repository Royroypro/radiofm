package tools

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestFetchAndInstall(t *testing.T) {
	exe := []byte("MZ falso ffmpeg")
	sum := sha256.Sum256(exe)
	m := Manifest{AppVersion: "0.2.0", Tools: []Tool{{Name: "ffmpeg", Version: "n8.1.3", Files: []File{
		{Name: "ffmpeg.exe", URL: "/uploads/estudio/tools/ffmpeg.exe", SHA256: hex.EncodeToString(sum[:]), Size: int64(len(exe))},
		{Name: "ffprobe.exe", URL: "/uploads/estudio/tools/ffprobe.exe", SHA256: hex.EncodeToString(sum[:]), Size: int64(len(exe))},
	}}}}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case ManifestPath:
			_ = json.NewEncoder(w).Encode(m)
		case "/uploads/estudio/tools/ffmpeg.exe", "/uploads/estudio/tools/ffprobe.exe":
			_, _ = w.Write(exe)
		case "/uploads/estudio/tools/malo.exe":
			_, _ = w.Write([]byte("otro contenido"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	ctx := context.Background()
	got, err := Fetch(ctx, srv.URL)
	if err != nil || got.AppVersion != "0.2.0" {
		t.Fatalf("Fetch = %+v, %v", got, err)
	}
	tool, ok := got.Find("ffmpeg")
	dir := filepath.Join(t.TempDir(), "tools", "ffmpeg")
	if !ok || Installed(tool, dir) {
		t.Fatal("antes de instalar no debe figurar como instalada")
	}
	var last int64
	if err := Install(ctx, srv.URL, tool, dir, func(done, total int64) { last = done }); err != nil {
		t.Fatal(err)
	}
	if !Installed(tool, dir) || last != 2*int64(len(exe)) {
		t.Fatalf("instalada=%v progreso=%d", Installed(tool, dir), last)
	}
	bad := Tool{Name: "x", Version: "1", Files: []File{{Name: "malo.exe", URL: "/uploads/estudio/tools/malo.exe", SHA256: hex.EncodeToString(sum[:])}}}
	if err := Install(ctx, srv.URL, bad, dir, nil); err == nil {
		t.Fatal("un archivo con otro SHA-256 no debe instalarse")
	}
	if _, err := os.Stat(filepath.Join(dir, "malo.exe")); err == nil {
		t.Fatal("no debe quedar el archivo rechazado")
	}
}
