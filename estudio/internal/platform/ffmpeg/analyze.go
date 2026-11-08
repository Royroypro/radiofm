package ffmpeg

import (
	"bufio"
	"context"
	"fmt"
	"regexp"
	"strconv"
	"time"
)

// Loudness es la medición EBU R128 de un archivo.
type Loudness struct {
	Integrated float64 // LUFS integrado del archivo
	// Momentary es la sonoridad momentánea (ventana de 400 ms) cada 100 ms: Momentary[i] es el
	// valor en t = (i+1)·100 ms. Con ella se buscan silencios al inicio/fin y la caída final.
	Momentary []float64
}

var (
	reFrame = regexp.MustCompile(`t:\s*([0-9.]+)\s+TARGET:.*?M:\s*(-?[0-9.]+|-inf|nan)`)
	reInteg = regexp.MustCompile(`^\s*I:\s+(-?[0-9.]+|-inf)\s+LUFS`)
)

// Analyze mide la sonoridad de un archivo con el filtro ebur128 de ffmpeg (una sola pasada).
func (t Tools) Analyze(ctx context.Context, path string) (Loudness, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	cmd := command(ctx, t.FFmpeg, "-hide_banner", "-nostats", "-nostdin", "-i", path, "-vn",
		"-af", "ebur128=framelog=info", "-f", "null", "-")
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return Loudness{}, err
	}
	if err := cmd.Start(); err != nil {
		return Loudness{}, fmt.Errorf("ffmpeg: iniciar: %w", err)
	}
	var l Loudness
	integSet := false
	sc := bufio.NewScanner(stderr)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	var lastLines []string
	for sc.Scan() {
		line := sc.Text()
		if m := reFrame.FindStringSubmatch(line); m != nil {
			l.Momentary = append(l.Momentary, parseLU(m[2]))
			continue
		}
		if m := reInteg.FindStringSubmatch(line); m != nil {
			l.Integrated, integSet = parseLU(m[1]), true
			continue
		}
		if len(lastLines) < 5 {
			lastLines = append(lastLines, line)
		}
	}
	if err := cmd.Wait(); err != nil {
		return l, fmt.Errorf("ffmpeg: analizar %s: %w %v", path, err, lastLines)
	}
	if !integSet {
		return l, fmt.Errorf("ffmpeg: no se obtuvo el loudness de %s", path)
	}
	return l, nil
}

func parseLU(s string) float64 {
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return -120 // -inf o nan: silencio
	}
	return f
}
