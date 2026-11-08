package timeannounce

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Files ubica las locuciones: <Dir>/<HourPrefix>00.mp3 … 23, <MinutePrefix>00 … 59,
// <TempPrefix>-10 … 45. Se aceptan .mp3, .wav, .ogg, .flac y .m4a.
type Files struct {
	Dir          string
	HourPrefix   string // HRS
	MinutePrefix string // MIN
	TempPrefix   string // TMP
	// Exists permite sustituir la comprobación de archivos en los tests (nil = os.Stat).
	Exists func(path string) bool
}

var exts = []string{".mp3", ".wav", ".ogg", ".flac", ".m4a"}

// TempMin y TempMax son el rango de temperaturas con locución.
const (
	TempMin = -10
	TempMax = 45
)

func (f Files) find(name string) (string, bool) {
	exists := f.Exists
	if exists == nil {
		exists = func(p string) bool { st, err := os.Stat(p); return err == nil && !st.IsDir() }
	}
	for _, e := range exts {
		p := filepath.Join(f.Dir, name+e)
		if exists(p) {
			return p, true
		}
	}
	return "", false
}

// Time devuelve los archivos para locutar la hora t (en la zona que traiga t):
// [HRShh, MINmm]. Error si falta alguno (el aire lo omite y lo registra).
func (f Files) Time(t time.Time) ([]string, error) {
	if f.Dir == "" {
		return nil, fmt.Errorf("no se configuró la carpeta de locuciones")
	}
	names := []string{fmt.Sprintf("%s%02d", f.HourPrefix, t.Hour()), fmt.Sprintf("%s%02d", f.MinutePrefix, t.Minute())}
	return f.resolve(names)
}

// Temperature devuelve el archivo para locutar una temperatura (redondeada a grados enteros).
func (f Files) Temperature(celsius float64) ([]string, error) {
	if f.Dir == "" {
		return nil, fmt.Errorf("no se configuró la carpeta de locuciones")
	}
	c := int(math.Round(celsius))
	if c < TempMin || c > TempMax {
		return nil, fmt.Errorf("temperatura %d °C fuera del rango de locuciones (%d a %d)", c, TempMin, TempMax)
	}
	return f.resolve([]string{fmt.Sprintf("%s%d", f.TempPrefix, c)})
}

func (f Files) resolve(names []string) ([]string, error) {
	var out, missing []string
	for _, n := range names {
		if p, ok := f.find(n); ok {
			out = append(out, p)
		} else {
			missing = append(missing, n)
		}
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("faltan locuciones: %s", strings.Join(missing, ", "))
	}
	return out, nil
}

// Check revisa qué locuciones faltan en la carpeta (para la pantalla de ajustes).
func (f Files) Check() []string {
	var missing []string
	for h := 0; h < 24; h++ {
		if _, ok := f.find(fmt.Sprintf("%s%02d", f.HourPrefix, h)); !ok {
			missing = append(missing, fmt.Sprintf("%s%02d", f.HourPrefix, h))
		}
	}
	for m := 0; m < 60; m++ {
		if _, ok := f.find(fmt.Sprintf("%s%02d", f.MinutePrefix, m)); !ok {
			missing = append(missing, fmt.Sprintf("%s%02d", f.MinutePrefix, m))
		}
	}
	for c := TempMin; c <= TempMax; c++ {
		if _, ok := f.find(fmt.Sprintf("%s%d", f.TempPrefix, c)); !ok {
			missing = append(missing, fmt.Sprintf("%s%d", f.TempPrefix, c))
		}
	}
	return missing
}
