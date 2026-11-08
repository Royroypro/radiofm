// Package tools revisa la PC y descarga desde el servidor de la radio las
// herramientas que falten (hoy: ffmpeg y ffprobe).
//
// El servidor publica un manifiesto en
//
//	https://radioclub25.sehuacho.com/uploads/estudio/manifest.json
//
// con la versión del programa, el instalador y cada herramienta con la URL y el
// SHA-256 de sus archivos. Lo descargado se verifica antes de usarse y se
// guarda en <carpeta de datos>\tools\<herramienta>\ (escribible sin ser
// administrador). Lo usa el instalador (club25-estudio.exe --setup) y el
// programa al arrancar si falta algo.
package tools

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// ManifestPath es la ruta del manifiesto en el servidor.
const ManifestPath = "/uploads/estudio/manifest.json"

// File es un archivo descargable.
type File struct {
	Name   string `json:"name"` // nombre local (ffmpeg.exe)
	URL    string `json:"url"`  // absoluta o relativa al servidor
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
}

// Tool es una herramienta con sus archivos.
type Tool struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	Files   []File `json:"files"`
}

// Manifest es lo que publica el servidor.
type Manifest struct {
	AppVersion string `json:"app_version"`
	Installer  File   `json:"installer"`
	Tools      []Tool `json:"tools"`
}

// Check es el resultado de revisar algo de la PC.
type Check struct {
	Name   string `json:"name"`
	OK     bool   `json:"ok"`
	Detail string `json:"detail"`
}

// Fetch descarga el manifiesto del servidor.
func Fetch(ctx context.Context, server string) (Manifest, error) {
	var m Manifest
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(server, "/")+ManifestPath, nil)
	if err != nil {
		return m, err
	}
	req.Header.Set("Cache-Control", "no-cache")
	res, err := (&http.Client{Timeout: 20 * time.Second}).Do(req)
	if err != nil {
		return m, fmt.Errorf("servidor de herramientas: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return m, fmt.Errorf("servidor de herramientas: HTTP %d", res.StatusCode)
	}
	if err := json.NewDecoder(res.Body).Decode(&m); err != nil {
		return m, fmt.Errorf("manifiesto inválido: %w", err)
	}
	return m, nil
}

// Find devuelve la herramienta con ese nombre.
func (m Manifest) Find(name string) (Tool, bool) {
	for _, t := range m.Tools {
		if t.Name == name {
			return t, true
		}
	}
	return Tool{}, false
}

// Install descarga los archivos de t en dir (verificando SHA-256) y escribe VERSION.txt.
// progress recibe (descargado, total) en bytes; puede ser nil.
func Install(ctx context.Context, server string, t Tool, dir string, progress func(done, total int64)) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	var total, done int64
	for _, f := range t.Files {
		total += f.Size
	}
	for _, f := range t.Files {
		url := f.URL
		if strings.HasPrefix(url, "/") {
			url = strings.TrimRight(server, "/") + url
		}
		n, err := download(ctx, url, filepath.Join(dir, f.Name), f.SHA256, func(n int64) {
			if progress != nil {
				progress(done+n, total)
			}
		})
		if err != nil {
			return fmt.Errorf("%s: %w", f.Name, err)
		}
		done += n
	}
	return os.WriteFile(filepath.Join(dir, "VERSION.txt"), []byte(t.Version), 0o644)
}

// Installed indica si en dir están todos los archivos de t con la versión de t.
func Installed(t Tool, dir string) bool {
	v, err := os.ReadFile(filepath.Join(dir, "VERSION.txt"))
	if err != nil || strings.TrimSpace(string(v)) != t.Version {
		return false
	}
	for _, f := range t.Files {
		if st, err := os.Stat(filepath.Join(dir, f.Name)); err != nil || st.IsDir() {
			return false
		}
	}
	return true
}

type counter struct {
	n  int64
	cb func(int64)
}

func (c *counter) Write(p []byte) (int, error) {
	c.n += int64(len(p))
	if c.cb != nil {
		c.cb(c.n)
	}
	return len(p), nil
}

func download(ctx context.Context, url, dst, wantSHA string, cb func(int64)) (int64, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0, err
	}
	res, err := (&http.Client{Timeout: 30 * time.Minute}).Do(req)
	if err != nil {
		return 0, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("HTTP %d en %s", res.StatusCode, url)
	}
	tmp := dst + ".part"
	f, err := os.Create(tmp)
	if err != nil {
		return 0, err
	}
	h := sha256.New()
	c := &counter{cb: cb}
	_, err = io.Copy(io.MultiWriter(f, h, c), res.Body)
	f.Close()
	if err != nil {
		_ = os.Remove(tmp)
		return 0, err
	}
	if got := hex.EncodeToString(h.Sum(nil)); !strings.EqualFold(got, wantSHA) {
		_ = os.Remove(tmp)
		return 0, fmt.Errorf("SHA-256 no coincide (descargado %s, esperado %s): no se instala", got[:12], shortSHA(wantSHA))
	}
	if err := os.Rename(tmp, dst); err != nil {
		return 0, err
	}
	return c.n, nil
}

func shortSHA(s string) string {
	if len(s) > 12 {
		return s[:12]
	}
	return s
}

// System revisa la PC: sistema operativo y arquitectura, carpeta de datos escribible.
func System(dataDir string) []Check {
	var out []Check
	out = append(out, Check{Name: "Sistema", OK: runtime.GOOS == "windows" && runtime.GOARCH == "amd64",
		Detail: runtime.GOOS + "/" + runtime.GOARCH})
	f, err := os.CreateTemp(dataDir, ".check*")
	if err == nil {
		name := f.Name()
		f.Close()
		_ = os.Remove(name)
	}
	out = append(out, Check{Name: "Carpeta de datos", OK: err == nil, Detail: dataDir})
	return out
}
