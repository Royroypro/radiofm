package ffmpeg

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

// tools localiza ffmpeg o salta el test (los tests de lógica no deben depender de él).
func tools(t *testing.T) Tools {
	t.Helper()
	tl, err := Locate()
	if err != nil {
		t.Skip("ffmpeg no disponible: ", err)
	}
	return tl
}

// makeTone crea un MP3 de prueba con ffmpeg (seno de 440 Hz).
func makeTone(t *testing.T, tl Tools, secs float64) string {
	t.Helper()
	out := filepath.Join(t.TempDir(), "tono.mp3")
	cmd := command(context.Background(), tl.FFmpeg, "-hide_banner", "-loglevel", "error", "-f", "lavfi",
		"-i", "sine=frequency=440:duration="+ftoa(secs), "-ac", "2", "-b:a", "128k", out)
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("crear tono: %v %s", err, b)
	}
	return out
}

func ftoa(f float64) string { return strconv.FormatFloat(f, 'f', -1, 64) }

func TestDecode(t *testing.T) {
	tl := tools(t)
	path := makeTone(t, tl, 2)
	r, err := tl.Decode(context.Background(), path, 500*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	b, err := io.ReadAll(r)
	r.Close()
	if err != nil {
		t.Fatal(err)
	}
	frames := len(b) / 8
	// 2 s − 0,5 s ≈ 1,5 s (el MP3 añade algo de relleno)
	if frames < 44100*14/10 || frames > 44100*17/10 {
		t.Fatalf("frames decodificados = %d, esperaba ~66150", frames)
	}
}

func TestDecodeMissingFile(t *testing.T) {
	tl := tools(t)
	r, err := tl.Decode(context.Background(), filepath.Join(os.TempDir(), "no-existe-club25.mp3"), 0)
	if err != nil {
		return // también vale fallar al iniciar
	}
	defer r.Close()
	if _, err := io.ReadAll(r); err == nil {
		t.Fatal("decodificar un archivo inexistente debe dar error")
	}
}
