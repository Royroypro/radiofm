// Package voicetrack graba las intervenciones del locutor entre dos canciones
// (voice tracking, idea del Voicetracker de Rivendell; escrito desde cero).
//
// Flujo: el operador elige en la playlist la canción A (la que suena antes) →
// Start graba el micrófono configurado (sin sacarlo al aire) → Stop entrega la
// toma (WAV) para escucharla → Save la guarda en la biblioteca, categoría
// «Voice tracking» (tipo locución), con el punto de mezcla «solape» segundos
// antes del final, y la inserta bloqueada después de A. Al aire suena como una
// locución: entra sobre el final de A (que se funde) y B entra durante sus
// últimos segundos (transition.go). Regenerar la playlist la conserva (bloqueada).
package voicetrack

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"radioclub25/estudio/internal/audio"
	"radioclub25/estudio/internal/domain"
)

// CategoryName es la categoría de la biblioteca donde quedan los voice tracks.
const CategoryName = "Voice tracking"

// MaxTake es la duración máxima de una toma.
const MaxTake = 5 * time.Minute

// Engine es lo que el servicio usa del motor de audio.
type Engine interface {
	SetRecording(on bool)
	RecordOutput() *audio.RingBuffer
}

// Library es lo que usa de la biblioteca.
type Library interface {
	EnsureCategory(ctx context.Context, name string, kind domain.CategoryKind) (int64, error)
	ImportFile(ctx context.Context, path string, categoryID int64) (*domain.Track, error)
	UpdateTrack(ctx context.Context, t *domain.Track) error
}

// Scheduler es lo que usa de la playlist.
type Scheduler interface {
	InsertTrack(ctx context.Context, afterID, trackID int64) error
}

// Take es una toma grabada.
type Take struct {
	File     string        `json:"file"` // nombre del archivo en la carpeta de voice tracks
	Duration time.Duration `json:"duration"`
}

// Service graba y guarda voice tracks. Una toma a la vez.
type Service struct {
	eng      Engine
	lib      Library
	sched    Scheduler
	dir      string
	micReady func() bool

	mu  sync.Mutex
	rec *recording
}

type recording struct {
	f       *os.File
	path    string
	frames  int64
	stop    chan struct{}
	done    chan error
	started time.Time
}

// New crea el servicio. dir es la carpeta de los voice tracks; micReady dice si
// hay un micrófono abierto (Ajustes → Audio).
func New(eng Engine, lib Library, sched Scheduler, dir string, micReady func() bool) *Service {
	return &Service{eng: eng, lib: lib, sched: sched, dir: dir, micReady: micReady}
}

// Dir es la carpeta de las tomas (la pre-escucha sirve archivos solo de aquí).
func (s *Service) Dir() string { return s.dir }

// Recording indica si hay una toma en curso y desde cuándo.
func (s *Service) Recording() (bool, time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.rec == nil {
		return false, 0
	}
	return true, time.Since(s.rec.started)
}

// Start empieza a grabar el micrófono.
func (s *Service) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.rec != nil {
		return errors.New("ya se está grabando")
	}
	if s.micReady != nil && !s.micReady() {
		return domain.Invalid("mic", "Configure el micrófono en Ajustes → Audio")
	}
	if err := os.MkdirAll(s.dir, 0o755); err != nil {
		return err
	}
	path := filepath.Join(s.dir, "toma-"+time.Now().Format("20060102-150405")+".wav")
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	if err := writeWAVHeader(f, 0); err != nil {
		f.Close()
		return err
	}
	r := &recording{f: f, path: path, stop: make(chan struct{}), done: make(chan error, 1), started: time.Now()}
	s.rec = r
	s.eng.SetRecording(true)
	go s.drain(r)
	return nil
}

// drain pasa lo grabado del motor al archivo (PCM 16 bits) cada 20 ms.
func (s *Service) drain(r *recording) {
	ring := s.eng.RecordOutput()
	buf := make([]float32, audio.SampleRate/10*audio.Channels)
	pcm := make([]byte, len(buf)*2)
	flush := func() error {
		for {
			n := ring.Read(buf)
			if n == 0 {
				return nil
			}
			for i, v := range buf[:n] {
				x := math.Max(-1, math.Min(1, float64(v)))
				binary.LittleEndian.PutUint16(pcm[i*2:], uint16(int16(x*32767)))
			}
			if _, err := r.f.Write(pcm[:n*2]); err != nil {
				return err
			}
			r.frames += int64(n / audio.Channels)
		}
	}
	t := time.NewTicker(20 * time.Millisecond)
	defer t.Stop()
	limit := time.After(MaxTake)
	for {
		select {
		case <-r.stop:
			r.done <- flush()
			return
		case <-limit:
			s.eng.SetRecording(false)
			<-r.stop
			r.done <- flush()
			return
		case <-t.C:
			if err := flush(); err != nil {
				s.eng.SetRecording(false)
				<-r.stop
				r.done <- err
				return
			}
		}
	}
}

// Stop termina la toma y la deja lista para escuchar o guardar.
func (s *Service) Stop() (Take, error) {
	s.mu.Lock()
	r := s.rec
	s.rec = nil
	s.mu.Unlock()
	if r == nil {
		return Take{}, errors.New("no se está grabando")
	}
	s.eng.SetRecording(false)
	time.Sleep(30 * time.Millisecond) // el último bloque del motor
	close(r.stop)
	err := <-r.done
	if err == nil {
		err = finishWAV(r.f, r.frames)
	}
	if cerr := r.f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		_ = os.Remove(r.path)
		return Take{}, fmt.Errorf("grabar: %w", err)
	}
	d := audio.FramesToDuration(r.frames)
	if d < time.Second {
		_ = os.Remove(r.path)
		return Take{}, domain.Invalid("take", "La toma es demasiado corta (¿llega audio del micrófono?)")
	}
	return Take{File: filepath.Base(r.path), Duration: d}, nil
}

// Discard borra una toma que no se va a usar.
func (s *Service) Discard(file string) error {
	p, err := s.takePath(file)
	if err != nil {
		return err
	}
	return os.Remove(p)
}

// takePath valida que file sea una toma de la carpeta (sin rutas).
func (s *Service) takePath(file string) (string, error) {
	if file == "" || file != filepath.Base(file) || !strings.HasSuffix(strings.ToLower(file), ".wav") {
		return "", domain.Invalid("file", "Toma inválida")
	}
	p := filepath.Join(s.dir, file)
	if _, err := os.Stat(p); err != nil {
		return "", domain.Invalid("file", "La toma ya no existe")
	}
	return p, nil
}

// TakePath devuelve la ruta de una toma (para la pre-escucha).
func (s *Service) TakePath(file string) (string, error) { return s.takePath(file) }

// Save guarda la toma en la biblioteca y la inserta después del elemento
// afterID. overlap es cuánto antes del final de la locución entra la canción
// siguiente (0 = al terminar).
func (s *Service) Save(ctx context.Context, file string, afterID int64, overlap time.Duration, title string) (*domain.Track, error) {
	p, err := s.takePath(file)
	if err != nil {
		return nil, err
	}
	if overlap < 0 || overlap > 10*time.Second {
		return nil, domain.Invalid("overlap", "El solape debe estar entre 0 y 10 s")
	}
	cat, err := s.lib.EnsureCategory(ctx, CategoryName, domain.KindVoice)
	if err != nil {
		return nil, err
	}
	final := filepath.Join(s.dir, "vt-"+strings.TrimPrefix(file, "toma-"))
	if err := os.Rename(p, final); err != nil {
		return nil, err
	}
	t, err := s.lib.ImportFile(ctx, final, cat)
	if err != nil {
		return nil, err
	}
	t.Title = strings.TrimSpace(title)
	if t.Title == "" {
		t.Title = "Voice track " + time.Now().Format("02/01 15:04")
	}
	t.Artist = "Locutor"
	if overlap > 0 && t.Duration > overlap+time.Second {
		t.SegueAt = t.Duration - overlap
	}
	if err := s.lib.UpdateTrack(ctx, t); err != nil { // marca las marcas como manuales
		return nil, err
	}
	if err := s.sched.InsertTrack(ctx, afterID, t.ID); err != nil {
		return nil, err
	}
	return t, nil
}

// --- WAV (PCM 16 bits, 44,1 kHz estéreo) ---

func writeWAVHeader(w io.Writer, frames int64) error {
	dataLen := uint32(frames * audio.Channels * 2)
	h := make([]byte, 44)
	copy(h[0:], "RIFF")
	binary.LittleEndian.PutUint32(h[4:], 36+dataLen)
	copy(h[8:], "WAVEfmt ")
	binary.LittleEndian.PutUint32(h[16:], 16)
	binary.LittleEndian.PutUint16(h[20:], 1) // PCM
	binary.LittleEndian.PutUint16(h[22:], audio.Channels)
	binary.LittleEndian.PutUint32(h[24:], audio.SampleRate)
	binary.LittleEndian.PutUint32(h[28:], audio.SampleRate*audio.Channels*2)
	binary.LittleEndian.PutUint16(h[32:], audio.Channels*2)
	binary.LittleEndian.PutUint16(h[34:], 16)
	copy(h[36:], "data")
	binary.LittleEndian.PutUint32(h[40:], dataLen)
	_, err := w.Write(h)
	return err
}

// finishWAV reescribe la cabecera con el tamaño real.
func finishWAV(f *os.File, frames int64) error {
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return err
	}
	return writeWAVHeader(f, frames)
}
