package main

import (
	"context"
	"encoding/binary"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"os/signal"
	"time"

	"radioclub25/estudio/internal/audio"
	"radioclub25/estudio/internal/platform/ffmpeg"
)

// sequencer reproduce una lista de archivos en los decks A/B con crossfade:
// cuando la pista actual llega a su punto de mezcla (duración − crossfade),
// arranca la siguiente y funde la actual. Es una versión mínima de lo que
// hará service/onair; sirve para probar el motor desde la línea de comandos.
type sequencer struct {
	e       *audio.Engine
	tools   ffmpeg.Tools
	files   []string
	xfade   time.Duration
	next    int // índice del próximo archivo a cargar
	current int // slot que suena (-1 = ninguno)
	loaded  int // slot cargado esperando turno (-1 = ninguno)
	out     io.Writer
	done    chan struct{}
	names   map[uint64]string // tag → "Artista - Título"
	// onStarted se llama cuando empieza a sonar una pista (p. ej. para la metadata de Icecast).
	onStarted func(name string)
}

func newSequencer(e *audio.Engine, tl ffmpeg.Tools, files []string, xfade time.Duration, out io.Writer) *sequencer {
	return &sequencer{e: e, tools: tl, files: files, xfade: xfade, current: -1, loaded: -1, out: out,
		done: make(chan struct{}), names: map[uint64]string{}}
}

// loadNext carga el siguiente archivo en el deck libre.
func (s *sequencer) loadNext() error {
	if s.next >= len(s.files) {
		return nil
	}
	path := s.files[s.next]
	info, err := s.tools.Probe(context.Background(), path)
	if err != nil {
		return err
	}
	r, err := s.tools.Decode(context.Background(), path, 0)
	if err != nil {
		return err
	}
	slot := audio.DeckA
	if s.current == audio.DeckA {
		slot = audio.DeckB
	}
	segue := info.Duration - s.xfade
	if segue < time.Second {
		segue = 0
	}
	if err := s.e.Load(slot, audio.NewPCMSource(r), audio.LoadOptions{Tag: uint64(s.next + 1), SegueAt: segue}); err != nil {
		return err
	}
	name := info.Title
	if info.Artist != "" {
		name = info.Artist + " - " + info.Title
	}
	if name == "" || name == " - " {
		name = path
	}
	s.names[uint64(s.next+1)] = name
	fmt.Fprintf(s.out, "[%d/%d] %s (%s)\n", s.next+1, len(s.files), name, info.Duration.Round(time.Second))
	s.loaded = slot
	s.next++
	return nil
}

// startLoaded hace sonar lo cargado y deja precargado lo siguiente.
func (s *sequencer) startLoaded() {
	if s.loaded < 0 {
		return
	}
	if s.current >= 0 {
		_ = s.e.Fade(s.current, 0, s.xfade, true)
	}
	_ = s.e.Play(s.loaded, 0)
	s.current, s.loaded = s.loaded, -1
	if err := s.loadNext(); err != nil {
		fmt.Fprintln(s.out, "error al cargar:", err)
	}
}

func (s *sequencer) handle(ev audio.Event) {
	switch ev.Type {
	case audio.EventStarted:
		if s.onStarted != nil && ev.Slot < audio.NumDecks {
			s.onStarted(s.names[ev.Tag])
		}
	case audio.EventSegue:
		if ev.Slot == s.current {
			s.startLoaded()
		}
	case audio.EventEnded:
		if ev.Slot == s.current && s.loaded >= 0 {
			s.startLoaded() // la pista terminó antes del punto de mezcla
		} else if ev.Slot == s.current && s.loaded < 0 {
			select {
			case <-s.done:
			default:
				close(s.done)
			}
		}
	}
}

// cmdPlay: estudio-cli play [-device X] [-xfade 3s] a.mp3 b.mp3 ...
func cmdPlay(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("play", flag.ContinueOnError)
	fs.SetOutput(stderr)
	device := fs.String("device", "", "tarjeta de salida (nombre o ID; vacío = predeterminada)")
	xfade := fs.Duration("xfade", 3*time.Second, "duración del crossfade")
	if err := fs.Parse(args); err != nil || fs.NArg() == 0 {
		fmt.Fprintln(stderr, "uso: estudio-cli play [-device NOMBRE] [-xfade 3s] archivo...")
		return 2
	}
	tl, err := ffmpeg.Locate()
	if err != nil {
		fmt.Fprintln(stderr, "error:", err)
		return 1
	}
	e := audio.NewEngine(audio.DefaultConfig())
	e.SetEncoderTap(false)
	out, err := audio.OpenOutput(*device, e.Render)
	if err != nil {
		fmt.Fprintln(stderr, "error:", err)
		return 1
	}
	defer out.Close()
	fmt.Fprintf(stdout, "Salida: %s · Ctrl+C para salir\n", out.Device.Name)

	seq := newSequencer(e, tl, fs.Args(), *xfade, stdout)
	if err := seq.loadNext(); err != nil {
		fmt.Fprintln(stderr, "error:", err)
		return 1
	}
	time.Sleep(300 * time.Millisecond) // dejar que el decodificador llene el buffer
	seq.startLoaded()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt)
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	for {
		select {
		case ev := <-e.Events():
			seq.handle(ev)
		case <-seq.done:
			fmt.Fprintln(stdout, "Fin de la lista.")
			return 0
		case <-sig:
			return 0
		case <-tick.C:
			st := e.Status()
			if st.Decks[seq.current&1].Playing {
				d := st.Decks[seq.current&1]
				fmt.Fprintf(stdout, "\r  %s  pico %5.1f dB  ", d.Position.Round(time.Second), audio.GainToDB(float64(max(st.Levels.PeakL, st.Levels.PeakR))))
			}
		}
	}
}

// cmdRender: estudio-cli render [-xfade 3s] [-max 10m] salida.wav a.mp3 b.mp3 ...
// Mezcla los archivos sin tarjeta de sonido (más rápido que tiempo real) y escribe un WAV.
func cmdRender(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("render", flag.ContinueOnError)
	fs.SetOutput(stderr)
	xfade := fs.Duration("xfade", 3*time.Second, "duración del crossfade")
	maxDur := fs.Duration("max", 30*time.Minute, "duración máxima")
	if err := fs.Parse(args); err != nil || fs.NArg() < 2 {
		fmt.Fprintln(stderr, "uso: estudio-cli render [-xfade 3s] [-max 30m] salida.wav archivo...")
		return 2
	}
	tl, err := ffmpeg.Locate()
	if err != nil {
		fmt.Fprintln(stderr, "error:", err)
		return 1
	}
	f, err := os.Create(fs.Arg(0))
	if err != nil {
		fmt.Fprintln(stderr, "error:", err)
		return 1
	}
	defer f.Close()
	w := newWAVWriter(f)

	e := audio.NewEngine(audio.Config{LimiterCeilingDB: -1})
	e.SetEncoderTap(false)
	seq := newSequencer(e, tl, fs.Args()[1:], *xfade, stdout)
	if err := seq.loadNext(); err != nil {
		fmt.Fprintln(stderr, "error:", err)
		return 1
	}
	seq.startLoaded()
	finished := false
	var written time.Duration
	for !finished && written < *maxDur {
		audio.RenderOfflineWaiting(e, time.Second, func(block []float32) {
			if !finished {
				w.write(block)
			}
			for {
				select {
				case ev := <-e.Events():
					seq.handle(ev)
					continue
				case <-seq.done:
					finished = true
				default:
				}
				break
			}
		})
		written += time.Second
	}
	if err := w.close(); err != nil {
		fmt.Fprintln(stderr, "error:", err)
		return 1
	}
	fmt.Fprintf(stdout, "Escrito %s (%s)\n", fs.Arg(0), audio.FramesToDuration(w.frames).Round(time.Millisecond))
	return 0
}

// wavWriter escribe WAV PCM de 16 bits estéreo 44,1 kHz.
type wavWriter struct {
	f      *os.File
	frames int64
	buf    []byte
	err    error
}

func newWAVWriter(f *os.File) *wavWriter {
	w := &wavWriter{f: f}
	_, w.err = f.Write(make([]byte, 44)) // cabecera provisional; se completa en close
	return w
}

func (w *wavWriter) write(samples []float32) {
	if w.err != nil {
		return
	}
	w.buf = w.buf[:0]
	for _, v := range samples {
		v = max(-1, min(1, v))
		w.buf = binary.LittleEndian.AppendUint16(w.buf, uint16(int16(math.Round(float64(v)*32767))))
	}
	_, w.err = w.f.Write(w.buf)
	w.frames += int64(len(samples) / audio.Channels)
}

func (w *wavWriter) close() error {
	if w.err != nil {
		return w.err
	}
	data := uint32(w.frames * audio.Channels * 2)
	h := make([]byte, 0, 44)
	h = append(h, "RIFF"...)
	h = binary.LittleEndian.AppendUint32(h, 36+data)
	h = append(h, "WAVEfmt "...)
	h = binary.LittleEndian.AppendUint32(h, 16)
	h = binary.LittleEndian.AppendUint16(h, 1) // PCM
	h = binary.LittleEndian.AppendUint16(h, audio.Channels)
	h = binary.LittleEndian.AppendUint32(h, audio.SampleRate)
	h = binary.LittleEndian.AppendUint32(h, audio.SampleRate*audio.Channels*2)
	h = binary.LittleEndian.AppendUint16(h, audio.Channels*2)
	h = binary.LittleEndian.AppendUint16(h, 16)
	h = append(h, "data"...)
	h = binary.LittleEndian.AppendUint32(h, data)
	if _, err := w.f.WriteAt(h, 0); err != nil {
		return err
	}
	return nil
}
