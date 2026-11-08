// Package storage guarda en disco los archivos subidos desde los paneles
// (imágenes, videos y audios de anuncios, logos, comprobantes de pago).
//
// Los archivos se guardan en <DATA_DIR>/uploads/<categoría>/<aleatorio>.<ext>
// y se sirven públicamente en /uploads/<categoría>/<archivo>. El nombre
// original nunca se usa en disco (evita colisiones y rutas maliciosas).
//
// El tipo real se detecta por el contenido (http.DetectContentType), no por
// la extensión que manda el navegador.
package storage

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// Kind agrupa los tipos de archivo permitidos para cada uso.
type Kind string

const (
	KindImage    Kind = "image"
	KindVideo    Kind = "video"
	KindAudio    Kind = "audio"
	KindDocument Kind = "document" // PDF o imagen (contratos, comprobantes)
)

// allowed asocia cada tipo MIME detectado con su extensión.
var allowed = map[Kind]map[string]string{
	KindImage:    {"image/jpeg": ".jpg", "image/png": ".png", "image/gif": ".gif", "image/webp": ".webp", "image/svg+xml": ".svg"},
	KindVideo:    {"video/mp4": ".mp4", "video/webm": ".webm"},
	KindAudio:    {"audio/mpeg": ".mp3", "audio/aac": ".aac", "audio/ogg": ".ogg", "audio/wave": ".wav", "audio/wav": ".wav", "audio/mp4": ".m4a"},
	KindDocument: {"application/pdf": ".pdf", "image/jpeg": ".jpg", "image/png": ".png", "image/webp": ".webp"},
}

// ErrFileType indica que el archivo no es de un tipo permitido.
var ErrFileType = errors.New("tipo de archivo no permitido")

// ErrFileTooLarge indica que el archivo supera el tamaño permitido.
var ErrFileTooLarge = errors.New("el archivo supera el tamaño permitido")

// Store guarda archivos en una carpeta raíz.
type Store struct {
	root      string // carpeta en disco
	publicURL string // prefijo público, p. ej. "/uploads"
}

// New crea un Store y su carpeta raíz.
func New(root, publicURL string) (*Store, error) {
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, fmt.Errorf("storage: crear %s: %w", root, err)
	}
	return &Store{root: root, publicURL: strings.TrimRight(publicURL, "/")}, nil
}

// Root devuelve la carpeta raíz en disco (para servir los archivos).
func (s *Store) Root() string { return s.root }

// Save valida y guarda un archivo subido. category es la subcarpeta ("ads",
// "brand", "billing"). maxBytes es el tamaño máximo. Devuelve la URL pública.
func (s *Store) Save(fh *multipart.FileHeader, kind Kind, category string, maxBytes int64) (string, error) {
	if fh.Size > maxBytes {
		return "", ErrFileTooLarge
	}
	f, err := fh.Open()
	if err != nil {
		return "", fmt.Errorf("storage: abrir subida: %w", err)
	}
	defer f.Close()

	head := make([]byte, 512)
	n, _ := io.ReadFull(f, head)
	mime := detect(head[:n], fh.Filename)
	ext, ok := allowed[kind][mime]
	if !ok {
		return "", fmt.Errorf("%w: %s", ErrFileType, mime)
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return "", err
	}

	dir := filepath.Join(s.root, category)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("storage: crear carpeta: %w", err)
	}
	name := randomName() + ext
	dst, err := os.Create(filepath.Join(dir, name))
	if err != nil {
		return "", fmt.Errorf("storage: crear archivo: %w", err)
	}
	defer dst.Close()
	written, err := io.Copy(dst, io.LimitReader(f, maxBytes+1))
	if err != nil {
		return "", fmt.Errorf("storage: escribir: %w", err)
	}
	if written > maxBytes {
		_ = os.Remove(dst.Name())
		return "", ErrFileTooLarge
	}
	return s.publicURL + "/" + category + "/" + name, nil
}

// Delete borra un archivo a partir de su URL pública. Ignora URLs que no son de este Store.
func (s *Store) Delete(publicURL string) error {
	if !strings.HasPrefix(publicURL, s.publicURL+"/") {
		return nil
	}
	rel := strings.TrimPrefix(publicURL, s.publicURL+"/")
	clean := filepath.Clean(rel)
	if strings.HasPrefix(clean, "..") || filepath.IsAbs(clean) {
		return nil
	}
	err := os.Remove(filepath.Join(s.root, clean))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

// detect identifica el tipo MIME por contenido. SVG, AAC y M4A no los detecta
// la librería estándar, así que se reconocen aparte.
func detect(head []byte, filename string) string {
	mime := http.DetectContentType(head)
	if i := strings.Index(mime, ";"); i >= 0 {
		mime = mime[:i]
	}
	lower := strings.ToLower(filename)
	switch {
	case (mime == "text/xml" || mime == "text/plain") && strings.HasSuffix(lower, ".svg") && strings.Contains(string(head), "<svg"):
		return "image/svg+xml"
	case mime == "application/octet-stream" && strings.HasSuffix(lower, ".aac") && len(head) > 1 && head[0] == 0xFF && head[1]&0xF0 == 0xF0:
		return "audio/aac"
	case mime == "video/mp4" && strings.HasSuffix(lower, ".m4a"):
		return "audio/mp4"
	case mime == "audio/mpeg" || (mime == "application/octet-stream" && strings.HasSuffix(lower, ".mp3")):
		return "audio/mpeg"
	}
	return mime
}

func randomName() string {
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
