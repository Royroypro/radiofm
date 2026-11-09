//go:build windows

package rds

import (
	"fmt"
	"io"
	"os"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

// openSerial abre COMn a baud 8N1 para escribir.
func openSerial(port string, baud int) (io.WriteCloser, error) {
	if baud <= 0 {
		baud = 9600
	}
	name := strings.TrimSpace(port)
	if !strings.HasPrefix(name, `\\.\`) {
		name = `\\.\` + name // necesario para COM10 en adelante
	}
	p, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return nil, err
	}
	h, err := windows.CreateFile(p, windows.GENERIC_READ|windows.GENERIC_WRITE, 0, nil, windows.OPEN_EXISTING, 0, 0)
	if err != nil {
		return nil, fmt.Errorf("abrir %s: %w", port, err)
	}
	var dcb windows.DCB
	dcb.DCBlength = uint32(unsafe.Sizeof(dcb))
	if err := windows.GetCommState(h, &dcb); err != nil {
		windows.CloseHandle(h)
		return nil, fmt.Errorf("leer %s: %w", port, err)
	}
	dcb.BaudRate = uint32(baud)
	dcb.ByteSize = 8
	dcb.Parity = windows.NOPARITY
	dcb.StopBits = windows.ONESTOPBIT
	if err := windows.SetCommState(h, &dcb); err != nil {
		windows.CloseHandle(h)
		return nil, fmt.Errorf("configurar %s: %w", port, err)
	}
	timeouts := windows.CommTimeouts{WriteTotalTimeoutConstant: 2000}
	_ = windows.SetCommTimeouts(h, &timeouts)
	return os.NewFile(uintptr(h), port), nil
}
