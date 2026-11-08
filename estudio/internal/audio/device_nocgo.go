//go:build !cgo

package audio

import "errors"

// ErrNoCGO indica que el binario se compiló sin CGO: miniaudio (malgo) lo necesita.
var ErrNoCGO = errors.New("audio: compilado sin CGO; instale gcc (llvm-mingw) y compile con CGO_ENABLED=1 (ver docs/setup-windows.md)")

// ListDevices sin CGO no puede acceder a la tarjeta de sonido.
func ListDevices() ([]Device, error) { return nil, ErrNoCGO }

// Stream es un dispositivo de audio abierto (sin CGO nunca se abre).
type Stream struct{ Device Device }

// Close no hace nada sin CGO.
func (s *Stream) Close() error { return nil }

// OpenOutput sin CGO devuelve ErrNoCGO (use RunClock).
func OpenOutput(string, func([]float32)) (*Stream, error) { return nil, ErrNoCGO }

// OpenCapture sin CGO devuelve ErrNoCGO.
func OpenCapture(string, *RingBuffer) (*Stream, error) { return nil, ErrNoCGO }
