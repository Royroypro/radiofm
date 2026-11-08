package logger

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
)

// New crea un logger que escribe en dir/estudio.log (rotativo) y, si console
// no es nil, también allí. Devuelve el logger y el archivo para cerrarlo al salir.
func New(dir string, console io.Writer) (*slog.Logger, io.Closer, error) {
	w, err := NewRotating(filepath.Join(dir, "estudio.log"), 5<<20, 5)
	if err != nil {
		return nil, nil, err
	}
	var out io.Writer = w
	if console != nil {
		out = io.MultiWriter(w, console)
	}
	return slog.New(slog.NewTextHandler(out, &slog.HandlerOptions{Level: slog.LevelInfo})), w, nil
}

// Rotating es un archivo que, al superar maxBytes, se renombra a .1 (y .1 a .2…)
// conservando keep copias.
type Rotating struct {
	mu       sync.Mutex
	path     string
	maxBytes int64
	keep     int
	f        *os.File
	size     int64
}

// NewRotating abre (o crea) el archivo de log.
func NewRotating(path string, maxBytes int64, keep int) (*Rotating, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	r := &Rotating{path: path, maxBytes: maxBytes, keep: keep}
	return r, r.open()
}

func (r *Rotating) open() error {
	f, err := os.OpenFile(r.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	st, err := f.Stat()
	if err != nil {
		f.Close()
		return err
	}
	r.f, r.size = f, st.Size()
	return nil
}

// Write implementa io.Writer.
func (r *Rotating) Write(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.size+int64(len(p)) > r.maxBytes {
		if err := r.rotate(); err != nil {
			return 0, err
		}
	}
	n, err := r.f.Write(p)
	r.size += int64(n)
	return n, err
}

func (r *Rotating) rotate() error {
	r.f.Close()
	for i := r.keep - 1; i >= 1; i-- {
		_ = os.Rename(fmt.Sprintf("%s.%d", r.path, i), fmt.Sprintf("%s.%d", r.path, i+1))
	}
	_ = os.Rename(r.path, r.path+".1")
	return r.open()
}

// Close cierra el archivo.
func (r *Rotating) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.f.Close()
}
