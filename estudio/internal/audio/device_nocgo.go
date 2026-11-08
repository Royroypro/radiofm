//go:build !cgo

package audio

import "errors"

// ErrNoCGO indica que el binario se compiló sin CGO: miniaudio (malgo) lo necesita.
var ErrNoCGO = errors.New("audio: compilado sin CGO; instale gcc (llvm-mingw) y compile con CGO_ENABLED=1 (ver docs/setup-windows.md)")

// ListDevices sin CGO no puede acceder a la tarjeta de sonido.
func ListDevices() ([]Device, error) { return nil, ErrNoCGO }
