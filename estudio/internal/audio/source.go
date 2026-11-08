package audio

import (
	"encoding/binary"
	"errors"
	"io"
	"math"
)

// Source entrega muestras en el formato interno (float32 estéreo 44,1 kHz).
// Read devuelve io.EOF cuando no hay más. Close libera recursos (p. ej. mata
// el proceso de ffmpeg). Nunca se llama desde el callback de audio: lo usa
// la goroutine que llena el ring buffer del reproductor.
type Source interface {
	Read(dst []float32) (int, error)
	Close() error
}

// PCMSource adapta un io.Reader de bytes float32 little-endian entrelazados
// (la salida de `ffmpeg -f f32le -ac 2 -ar 44100`) a Source.
type PCMSource struct {
	r    io.ReadCloser
	buf  []byte
	tail []byte // bytes sobrantes que no completaron una muestra
}

// NewPCMSource envuelve r. Close cierra r.
func NewPCMSource(r io.ReadCloser) *PCMSource { return &PCMSource{r: r} }

// Read implementa Source.
func (s *PCMSource) Read(dst []float32) (int, error) {
	need := len(dst) * 4
	if cap(s.buf) < need {
		s.buf = make([]byte, need)
	}
	buf := s.buf[:need]
	k := copy(buf, s.tail)
	s.tail = s.tail[:0]
	n, err := io.ReadAtLeast(s.r, buf[k:], 4-min(k, 4))
	n += k
	if errors.Is(err, io.ErrUnexpectedEOF) {
		err = io.EOF
	}
	samples := n / 4
	for i := 0; i < samples; i++ {
		dst[i] = math.Float32frombits(binary.LittleEndian.Uint32(buf[i*4:]))
	}
	if rest := n - samples*4; rest > 0 {
		s.tail = append(s.tail, buf[samples*4:n]...)
	}
	if samples > 0 && err == io.EOF {
		err = nil // entregar primero lo leído; el próximo Read dará EOF
	}
	return samples, err
}

// Close implementa Source.
func (s *PCMSource) Close() error { return s.r.Close() }

// MemorySource reproduce muestras ya decodificadas (carts cortos, tests).
type MemorySource struct {
	samples []float32
	pos     int
}

// NewMemorySource crea una fuente desde muestras entrelazadas. No copia el slice.
func NewMemorySource(samples []float32) *MemorySource { return &MemorySource{samples: samples} }

// Read implementa Source.
func (m *MemorySource) Read(dst []float32) (int, error) {
	if m.pos >= len(m.samples) {
		return 0, io.EOF
	}
	n := copy(dst, m.samples[m.pos:])
	m.pos += n
	return n, nil
}

// Close implementa Source.
func (m *MemorySource) Close() error { return nil }

// ReadAll decodifica una fuente completa a memoria (máximo maxFrames frames).
func ReadAll(src Source, maxFrames int64) ([]float32, error) {
	defer src.Close()
	var out []float32
	chunk := make([]float32, 8192)
	for int64(len(out)/Channels) < maxFrames {
		n, err := src.Read(chunk)
		out = append(out, chunk[:n]...)
		if err == io.EOF {
			break
		}
		if err != nil {
			return out, err
		}
	}
	if lim := int(maxFrames) * Channels; len(out) > lim {
		out = out[:lim]
	}
	return out, nil
}

// Sine genera un seno estéreo de la frecuencia y amplitud dadas (para tests y pruebas de salida).
func Sine(freq float64, amp float32, d float64) []float32 {
	frames := int(d * SampleRate)
	out := make([]float32, frames*Channels)
	for i := 0; i < frames; i++ {
		v := amp * float32(math.Sin(2*math.Pi*freq*float64(i)/SampleRate))
		out[i*2], out[i*2+1] = v, v
	}
	return out
}
