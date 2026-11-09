package voicetrack

import (
	"context"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
	"time"

	"radioclub25/estudio/internal/audio"
	"radioclub25/estudio/internal/domain"
)

type fakeEngine struct {
	ring *audio.RingBuffer
	on   bool
}

func (f *fakeEngine) SetRecording(on bool)            { f.on = on }
func (f *fakeEngine) RecordOutput() *audio.RingBuffer { return f.ring }

type fakeLib struct {
	cat     int64
	tracks  map[int64]*domain.Track
	updated *domain.Track
}

func (l *fakeLib) EnsureCategory(context.Context, string, domain.CategoryKind) (int64, error) {
	return l.cat, nil
}
func (l *fakeLib) ImportFile(_ context.Context, path string, cat int64) (*domain.Track, error) {
	st, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	frames := (st.Size() - 44) / 4
	t := &domain.Track{ID: 77, Path: path, CategoryID: cat, Duration: audio.FramesToDuration(frames)}
	l.tracks = map[int64]*domain.Track{77: t}
	return t, nil
}
func (l *fakeLib) UpdateTrack(_ context.Context, t *domain.Track) error { l.updated = t; return nil }

type fakeSched struct{ after, track int64 }

func (s *fakeSched) InsertTrack(_ context.Context, after, track int64) error {
	s.after, s.track = after, track
	return nil
}

func TestRecordAndSave(t *testing.T) {
	dir := t.TempDir()
	eng := &fakeEngine{ring: audio.NewRingBuffer(audio.SampleRate * audio.Channels * 4)}
	lib, sched := &fakeLib{cat: 9}, &fakeSched{}
	svc := New(eng, lib, sched, dir, func() bool { return true })

	if err := svc.Start(); err != nil || !eng.on {
		t.Fatalf("Start = %v (grabando %v)", err, eng.on)
	}
	if err := svc.Start(); err == nil {
		t.Fatal("dos tomas a la vez no")
	}
	// 3 s de "voz" (seno) en bloques, como el motor
	sig := audio.Sine(300, 0.5, 3)
	for i := 0; i < len(sig); i += 4410 * 2 {
		eng.ring.Write(sig[i:min(i+4410*2, len(sig))])
		time.Sleep(25 * time.Millisecond)
	}
	take, err := svc.Stop()
	if err != nil || eng.on {
		t.Fatalf("Stop = %+v, %v", take, err)
	}
	if take.Duration < 2900*time.Millisecond || take.Duration > 3100*time.Millisecond {
		t.Fatalf("duración = %v", take.Duration)
	}
	// cabecera WAV con el tamaño real
	b, _ := os.ReadFile(filepath.Join(dir, take.File))
	if string(b[0:4]) != "RIFF" || string(b[8:12]) != "WAVE" || int(binary.LittleEndian.Uint32(b[40:])) != len(b)-44 {
		t.Fatalf("WAV inválido: %q %d/%d", b[:12], binary.LittleEndian.Uint32(b[40:]), len(b)-44)
	}
	if _, err := svc.TakePath("../secreto.wav"); err == nil {
		t.Fatal("no debe servir rutas fuera de la carpeta")
	}

	tr, err := svc.Save(context.Background(), take.File, 123, 1500*time.Millisecond, "")
	if err != nil {
		t.Fatal(err)
	}
	if sched.after != 123 || sched.track != 77 {
		t.Fatalf("insertada después de %d (pista %d)", sched.after, sched.track)
	}
	if lib.updated == nil || lib.updated.SegueAt != tr.Duration-1500*time.Millisecond || lib.updated.Title == "" {
		t.Fatalf("la siguiente debe entrar 1,5 s antes del final: %+v", lib.updated)
	}
	if _, err := os.Stat(filepath.Join(dir, take.File)); err == nil {
		t.Fatal("la toma se renombra al guardarla")
	}
}

func TestShortTakeAndNoMic(t *testing.T) {
	eng := &fakeEngine{ring: audio.NewRingBuffer(audio.SampleRate * audio.Channels)}
	svc := New(eng, &fakeLib{}, &fakeSched{}, t.TempDir(), func() bool { return false })
	if err := svc.Start(); err == nil {
		t.Fatal("sin micrófono no se puede grabar")
	}
	svc.micReady = func() bool { return true }
	_ = svc.Start()
	if _, err := svc.Stop(); err == nil {
		t.Fatal("una toma vacía debe rechazarse")
	}
}
