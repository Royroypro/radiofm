// Package airlog es el grabador legal de la emisión ("logger"): graba sin
// parar lo que sale por la tarjeta (después del procesador y el limitador) en
// archivos MP3 mono por hora, borra lo más viejo según el límite de tamaño y de
// días, permite exportar un tramo y hace las grabaciones programadas
// (recortes de un programa al terminar).
//
// Ideas de Rotter y Chronicle (grabadores de emisión para radios); escrito desde
// cero sobre ffmpeg. En Perú el reglamento de la Ley de Radio y Televisión pide
// conservar lo emitido (al menos 30 días; verificar el texto vigente).
//
// Archivos: <dir>/2026-11-09_14-00-00.mp3 (hora de inicio, hora local de la PC).
// ffmpeg corta en cada cambio de hora del reloj (segment_atclocktime).
package airlog

import (
	"bufio"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"radioclub25/estudio/internal/audio"
)

// Settings son los ajustes del grabador.
type Settings struct {
	Enabled bool
	Dir     string
	MaxMB   int // tamaño máximo total (0 = sin límite)
	MaxDays int // días máximos (0 = sin límite)
	Kbps    int // 64 por defecto (mono)
}

// Engine es lo que el grabador usa del motor.
type Engine interface {
	SetLoggerTap(on bool)
	LoggerOutput() *audio.RingBuffer
}

// Command crea el proceso de ffmpeg (ffmpeg.Tools.Command; los tests usan un falso).
type Command func(ctx context.Context, args ...string) *exec.Cmd

// fileLayout es el nombre de los archivos (también el patrón strftime de ffmpeg).
const (
	fileLayout = "2006-01-02_15-04-05"
	strftime   = "%Y-%m-%d_%H-%M-%S"
)

// Service graba la emisión.
type Service struct {
	eng      Engine
	cmd      Command
	settings func() Settings
	log      *slog.Logger
	now      func() time.Time

	mu      sync.Mutex
	running bool
	lastErr string
	since   time.Time
}

// New crea el grabador.
func New(eng Engine, cmd Command, settings func() Settings, log *slog.Logger) *Service {
	return &Service{eng: eng, cmd: cmd, settings: settings, log: log, now: time.Now}
}

// Status es el estado del grabador para la interfaz.
type Status struct {
	Enabled   bool      `json:"enabled"`
	Recording bool      `json:"recording"`
	Since     time.Time `json:"since"`
	Error     string    `json:"error"`
	Dir       string    `json:"dir"`
	UsedMB    int64     `json:"used_mb"`
	MaxMB     int       `json:"max_mb"`
	MaxDays   int       `json:"max_days"`
}

// Status devuelve el estado.
func (s *Service) Status() Status {
	set := s.settings()
	s.mu.Lock()
	st := Status{Enabled: set.Enabled, Recording: s.running, Since: s.since, Error: s.lastErr, Dir: set.Dir,
		MaxMB: set.MaxMB, MaxDays: set.MaxDays}
	s.mu.Unlock()
	files, _ := s.Files()
	for _, f := range files {
		st.UsedMB += f.Size
	}
	st.UsedMB /= 1 << 20
	return st
}

// Run graba mientras ctx viva (y el grabador esté activado); se reinicia solo
// si ffmpeg se cae. Cada 10 minutos aplica la retención.
func (s *Service) Run(ctx context.Context) {
	prune := time.NewTicker(10 * time.Minute)
	defer prune.Stop()
	backoff := time.Second
	for ctx.Err() == nil {
		set := s.settings()
		if !set.Enabled || set.Dir == "" {
			s.eng.SetLoggerTap(false)
			select {
			case <-ctx.Done():
				return
			case <-time.After(15 * time.Second):
				continue
			}
		}
		started := s.now()
		err := s.session(ctx, set, prune.C)
		if ctx.Err() != nil {
			break
		}
		if err != nil {
			s.log.Error("grabador de la emisión", "err", err)
		}
		if time.Since(started) > time.Minute {
			backoff = time.Second
		}
		select {
		case <-ctx.Done():
		case <-time.After(backoff):
		}
		backoff = min(backoff*2, time.Minute)
	}
	s.eng.SetLoggerTap(false)
}

// session corre un ffmpeg que recibe PCM por stdin y escribe los MP3 por hora.
func (s *Service) session(ctx context.Context, set Settings, prune <-chan time.Time) error {
	if err := os.MkdirAll(set.Dir, 0o755); err != nil {
		s.setErr(err)
		return err
	}
	kbps := set.Kbps
	if kbps <= 0 {
		kbps = 64
	}
	// El proceso no se ata a ctx: al cerrar el programa se le cierra la entrada
	// para que termine de escribir el MP3 en curso (matarlo lo dejaría vacío).
	procCtx, kill := context.WithCancel(context.Background())
	defer kill()
	cmd := s.cmd(procCtx, "-hide_banner", "-loglevel", "error", "-nostdin",
		"-f", "f32le", "-ar", fmt.Sprint(audio.SampleRate), "-ac", fmt.Sprint(audio.Channels), "-i", "pipe:0",
		"-ac", "1", "-c:a", "libmp3lame", "-b:a", fmt.Sprintf("%dk", kbps),
		"-f", "segment", "-segment_time", "3600", "-segment_atclocktime", "1", "-strftime", "1",
		"-reset_timestamps", "1", filepath.Join(set.Dir, strftime+".mp3"))
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	var stderr strings.Builder
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		s.setErr(err)
		return fmt.Errorf("iniciar ffmpeg: %w", err)
	}
	ring := s.eng.LoggerOutput()
	ring.Discard(ring.Len())
	s.eng.SetLoggerTap(true)
	defer s.eng.SetLoggerTap(false)
	s.mu.Lock()
	s.running, s.since, s.lastErr = true, s.now(), ""
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		s.running = false
		s.mu.Unlock()
	}()
	s.Prune()

	w := bufio.NewWriterSize(stdin, 1<<16)
	samples := make([]float32, audio.SampleRate/5*audio.Channels)
	buf := make([]byte, len(samples)*4)
	tick := time.NewTicker(50 * time.Millisecond)
	defer tick.Stop()
	exited := make(chan error, 1)
	go func() { exited <- cmd.Wait() }()
	for {
		select {
		case <-ctx.Done():
			_ = w.Flush()
			_ = stdin.Close() // ffmpeg cierra el archivo en curso
			select {
			case <-exited:
			case <-time.After(5 * time.Second):
				kill()
				<-exited
			}
			return nil
		case err := <-exited:
			msg := strings.TrimSpace(stderr.String())
			if msg == "" && err != nil {
				msg = err.Error()
			}
			e := errors.New("ffmpeg se detuvo: " + msg)
			s.setErr(e)
			return e
		case <-prune:
			s.Prune()
			if set2 := s.settings(); !set2.Enabled || set2.Dir != set.Dir || set2.Kbps != set.Kbps {
				_ = w.Flush()
				_ = stdin.Close()
				<-exited
				return nil // ajustes cambiados: Run vuelve a empezar
			}
		case <-tick.C:
			for {
				n := ring.Read(samples)
				if n == 0 {
					break
				}
				for i, v := range samples[:n] {
					binary.LittleEndian.PutUint32(buf[i*4:], math.Float32bits(v))
				}
				if _, err := w.Write(buf[:n*4]); err != nil {
					s.setErr(err)
					return err
				}
			}
			_ = w.Flush()
		}
	}
}

func (s *Service) setErr(err error) {
	s.mu.Lock()
	s.lastErr = err.Error()
	s.mu.Unlock()
}

// File es un archivo de la grabación.
type File struct {
	Name  string    `json:"name"`
	Start time.Time `json:"start"`
	Size  int64     `json:"size"`
}

// Files devuelve los archivos de la grabación en orden cronológico.
func (s *Service) Files() ([]File, error) {
	dir := s.settings().Dir
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	var out []File
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".mp3") {
			continue
		}
		t, err := time.ParseInLocation(fileLayout, strings.TrimSuffix(name, ".mp3"), time.Local)
		if err != nil {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		out = append(out, File{Name: name, Start: t, Size: info.Size()})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Start.Before(out[j].Start) })
	return out, nil
}

// Prune borra lo que pasa los límites: primero lo más viejo que MaxDays y
// después, del más viejo al más nuevo, hasta quedar bajo MaxMB. Nunca borra el
// archivo que se está grabando (el último). Devuelve cuántos borró.
func (s *Service) Prune() int {
	set := s.settings()
	files, err := s.Files()
	if err != nil || len(files) <= 1 {
		return 0
	}
	keep := files[len(files)-1] // en curso
	files = files[:len(files)-1]
	var total int64 = keep.Size
	for _, f := range files {
		total += f.Size
	}
	deleted := 0
	limit := s.now().AddDate(0, 0, -set.MaxDays)
	for _, f := range files {
		tooOld := set.MaxDays > 0 && f.Start.Before(limit)
		tooBig := set.MaxMB > 0 && total > int64(set.MaxMB)<<20
		if !tooOld && !tooBig {
			break
		}
		if err := os.Remove(filepath.Join(set.Dir, f.Name)); err == nil {
			total -= f.Size
			deleted++
		}
	}
	if deleted > 0 {
		s.log.Info("grabador: retención", "borrados", deleted, "mb", total>>20)
	}
	return deleted
}

// FilePath devuelve la ruta de un archivo de la grabación (solo nombres válidos).
func (s *Service) FilePath(name string) (string, error) {
	if name != filepath.Base(name) || !strings.HasSuffix(name, ".mp3") {
		return "", errors.New("archivo inválido")
	}
	if _, err := time.ParseInLocation(fileLayout, strings.TrimSuffix(name, ".mp3"), time.Local); err != nil {
		return "", errors.New("archivo inválido")
	}
	p := filepath.Join(s.settings().Dir, name)
	if _, err := os.Stat(p); err != nil {
		return "", err
	}
	return p, nil
}

// Export copia el tramo [from, to) de la grabación a dst (MP3, sin recodificar)
// con el concatenador de ffmpeg. Error si no hay grabación de esa franja.
func (s *Service) Export(ctx context.Context, from, to time.Time, dst string) error {
	if !to.After(from) || to.Sub(from) > 12*time.Hour {
		return errors.New("tramo inválido (máximo 12 h)")
	}
	files, err := s.Files()
	if err != nil {
		return err
	}
	var list strings.Builder
	parts := 0
	for i, f := range files {
		end := s.now()
		if i+1 < len(files) {
			end = files[i+1].Start
		}
		if !end.After(from) || !f.Start.Before(to) {
			continue
		}
		p := filepath.Join(s.settings().Dir, f.Name)
		fmt.Fprintf(&list, "file '%s'\n", strings.ReplaceAll(filepath.ToSlash(p), "'", `'\''`))
		if from.After(f.Start) {
			fmt.Fprintf(&list, "inpoint %.3f\n", from.Sub(f.Start).Seconds())
		}
		if to.Before(end) {
			fmt.Fprintf(&list, "outpoint %.3f\n", to.Sub(f.Start).Seconds())
		}
		parts++
	}
	if parts == 0 {
		return errors.New("no hay grabación de esa franja")
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	lst, err := os.CreateTemp("", "airlog-*.txt")
	if err != nil {
		return err
	}
	defer os.Remove(lst.Name())
	if _, err := lst.WriteString(list.String()); err != nil {
		lst.Close()
		return err
	}
	lst.Close()
	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	cmd := s.cmd(ctx, "-hide_banner", "-loglevel", "error", "-nostdin", "-y", "-f", "concat", "-safe", "0",
		"-i", lst.Name(), "-c", "copy", dst)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("exportar: %v %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// ---------- Grabaciones programadas ----------

// Recording es una grabación programada: al terminar la franja se recorta de la
// grabación de la emisión y se guarda en <dir>/programas.
type Recording struct {
	Name    string `json:"name"`
	Days    int    `json:"days"`    // bit 0 = domingo … bit 6 = sábado
	Start   string `json:"start"`   // "HH:MM"
	Minutes int    `json:"minutes"` // duración
}

// RecordingsDir es la carpeta de las grabaciones programadas.
func (s *Service) RecordingsDir() string { return filepath.Join(s.settings().Dir, "programas") }

// RunScheduled revisa cada minuto si terminó alguna grabación programada (de
// las últimas 6 h) y la exporta si todavía no existe el archivo.
func (s *Service) RunScheduled(ctx context.Context, recordings func() []Recording) {
	t := time.NewTicker(time.Minute)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s.exportDue(ctx, recordings())
		}
	}
}

func (s *Service) exportDue(ctx context.Context, recs []Recording) {
	now := s.now()
	for _, r := range recs {
		for _, w := range windows(r, now) {
			dst := filepath.Join(s.RecordingsDir(), fileName(r.Name, w.from))
			if _, err := os.Stat(dst); err == nil {
				continue
			}
			if err := s.Export(ctx, w.from, w.to, dst); err != nil {
				s.log.Warn("grabación programada", "programa", r.Name, "err", err)
				continue
			}
			s.log.Info("grabación programada guardada", "archivo", dst)
		}
	}
}

type window struct{ from, to time.Time }

// windows devuelve las franjas de r que terminaron en las últimas 6 h (con 1 min de margen).
func windows(r Recording, now time.Time) []window {
	var h, m int
	if _, err := fmt.Sscanf(r.Start, "%d:%d", &h, &m); err != nil || r.Minutes <= 0 {
		return nil
	}
	var out []window
	for d := -1; d <= 0; d++ {
		day := now.AddDate(0, 0, d)
		from := time.Date(day.Year(), day.Month(), day.Day(), h, m, 0, 0, now.Location())
		to := from.Add(time.Duration(r.Minutes) * time.Minute)
		if r.Days != 0 && r.Days&(1<<int(from.Weekday())) == 0 {
			continue
		}
		if !now.Before(to.Add(time.Minute)) && now.Sub(to) < 6*time.Hour {
			out = append(out, window{from, to})
		}
	}
	return out
}

// fileName arma "Mañanas Club 2026-11-09 0600.mp3" sin caracteres prohibidos en Windows.
func fileName(name string, at time.Time) string {
	clean := strings.Map(func(r rune) rune {
		if strings.ContainsRune(`<>:"/\|?*`, r) || r < 32 {
			return '_'
		}
		return r
	}, strings.TrimSpace(name))
	if clean == "" {
		clean = "programa"
	}
	return clean + " " + at.Format("2006-01-02 1504") + ".mp3"
}
