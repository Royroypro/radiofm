package ffmpeg

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"time"
)

// Tools son las rutas de ffmpeg.exe y ffprobe.exe.
type Tools struct {
	FFmpeg  string
	FFprobe string
}

// ErrNotFound indica que no se encontró ffmpeg.
var ErrNotFound = errors.New("ffmpeg: no se encontró ffmpeg.exe (ejecute scripts/get-ffmpeg.ps1)")

// Locate busca ffmpeg y ffprobe en este orden:
//  1. la carpeta de la variable CLUB25_FFMPEG_DIR;
//  2. junto al ejecutable (instalación) o en su subcarpeta ffmpeg\;
//  3. third_party\ffmpeg subiendo desde el ejecutable y desde la carpeta actual (desarrollo);
//  4. el PATH.
func Locate() (Tools, error) {
	var dirs []string
	if d := os.Getenv("CLUB25_FFMPEG_DIR"); d != "" {
		dirs = append(dirs, d)
	}
	if exe, err := os.Executable(); err == nil {
		d := filepath.Dir(exe)
		dirs = append(dirs, d, filepath.Join(d, "ffmpeg"))
		dirs = append(dirs, upwards(d)...)
	}
	if wd, err := os.Getwd(); err == nil {
		dirs = append(dirs, upwards(wd)...)
	}
	for _, d := range dirs {
		t := Tools{FFmpeg: filepath.Join(d, exeName("ffmpeg")), FFprobe: filepath.Join(d, exeName("ffprobe"))}
		if fileExists(t.FFmpeg) && fileExists(t.FFprobe) {
			return t, nil
		}
	}
	ff, err1 := exec.LookPath("ffmpeg")
	fp, err2 := exec.LookPath("ffprobe")
	if err1 == nil && err2 == nil {
		return Tools{FFmpeg: ff, FFprobe: fp}, nil
	}
	return Tools{}, ErrNotFound
}

// upwards devuelve <d>/third_party/ffmpeg, <d>/estudio/third_party/ffmpeg y lo mismo en cada carpeta superior.
func upwards(d string) []string {
	var out []string
	for i := 0; i < 6; i++ {
		out = append(out, filepath.Join(d, "third_party", "ffmpeg"), filepath.Join(d, "estudio", "third_party", "ffmpeg"))
		parent := filepath.Dir(d)
		if parent == d {
			break
		}
		d = parent
	}
	return out
}

func fileExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}

// command crea un proceso sin ventana de consola (ver proc_windows.go).
func command(ctx context.Context, bin string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, bin, args...)
	hideWindow(cmd)
	return cmd
}

// Decode arranca ffmpeg para decodificar path a PCM float32 estéreo 44,1 kHz
// desde start. Devuelve un lector en streaming: leer de él consume la salida
// de ffmpeg. Close mata el proceso si sigue vivo.
func (t Tools) Decode(ctx context.Context, path string, start time.Duration) (io.ReadCloser, error) {
	args := []string{"-hide_banner", "-loglevel", "error", "-nostdin"}
	if start > 0 {
		args = append(args, "-ss", strconv.FormatFloat(start.Seconds(), 'f', 3, 64))
	}
	args = append(args, "-i", path, "-vn", "-f", "f32le", "-ac", "2", "-ar", "44100", "pipe:1")
	ctx, cancel := context.WithCancel(ctx)
	cmd := command(ctx, t.FFmpeg, args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		return nil, err
	}
	stderr := &limitedBuffer{max: 4096}
	cmd.Stderr = stderr
	if err := cmd.Start(); err != nil {
		cancel()
		return nil, fmt.Errorf("ffmpeg: iniciar: %w", err)
	}
	return &procReader{r: stdout, cmd: cmd, cancel: cancel, stderr: stderr}, nil
}

// procReader lee la salida de un proceso y lo termina al cerrar.
type procReader struct {
	r      io.Reader
	cmd    *exec.Cmd
	cancel context.CancelFunc
	stderr *limitedBuffer
	once   sync.Once
	err    error
}

func (p *procReader) Read(b []byte) (int, error) {
	n, err := p.r.Read(b)
	if err == io.EOF {
		if werr := p.wait(); werr != nil && p.stderr.Len() > 0 {
			return n, fmt.Errorf("ffmpeg: %s", bytes.TrimSpace(p.stderr.Bytes()))
		}
	}
	return n, err
}

func (p *procReader) wait() error {
	p.once.Do(func() { p.err = p.cmd.Wait() })
	return p.err
}

// Close termina el proceso.
func (p *procReader) Close() error {
	p.cancel()
	_ = p.wait()
	return nil
}

// limitedBuffer guarda como mucho max bytes (para el stderr de ffmpeg).
type limitedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
	max int
}

func (l *limitedBuffer) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if room := l.max - l.buf.Len(); room > 0 {
		l.buf.Write(p[:min(len(p), room)])
	}
	return len(p), nil
}

func (l *limitedBuffer) Len() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.buf.Len()
}

func (l *limitedBuffer) Bytes() []byte {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]byte(nil), l.buf.Bytes()...)
}

func (l *limitedBuffer) String() string { return string(l.Bytes()) }
