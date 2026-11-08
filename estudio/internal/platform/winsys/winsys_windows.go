//go:build windows

package winsys

import (
	"errors"
	"fmt"
	"os"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

// ErrAlreadyRunning indica que ya hay otra instancia abierta.
var ErrAlreadyRunning = errors.New("Club25 Estudio ya está abierto en esta PC")

// SingleInstance crea un mutex con nombre global. Si ya existe, otra instancia
// está corriendo (dos instancias sonarían a la vez). Devuelve una función para liberarlo.
func SingleInstance(name string) (func(), error) {
	p, err := windows.UTF16PtrFromString(`Global\` + name)
	if err != nil {
		return nil, err
	}
	h, err := windows.CreateMutex(nil, false, p)
	if errors.Is(err, windows.ERROR_ALREADY_EXISTS) {
		if h != 0 {
			windows.CloseHandle(h)
		}
		return nil, ErrAlreadyRunning
	}
	if err != nil {
		return nil, fmt.Errorf("instancia única: %w", err)
	}
	return func() { windows.CloseHandle(h) }, nil
}

var (
	kernel32                    = windows.NewLazySystemDLL("kernel32.dll")
	procSetThreadExecutionState = kernel32.NewProc("SetThreadExecutionState")
	user32                      = windows.NewLazySystemDLL("user32.dll")
	procMessageBoxW             = user32.NewProc("MessageBoxW")
)

const (
	esContinuous     = 0x80000000
	esSystemRequired = 0x00000001
)

// KeepAwake impide que Windows suspenda la PC mientras el programa está abierto
// (la pantalla sí puede apagarse). Debe llamarse desde un hilo que viva todo el programa.
func KeepAwake() {
	_, _, _ = procSetThreadExecutionState.Call(uintptr(esContinuous | esSystemRequired))
}

// AllowSleep revierte KeepAwake.
func AllowSleep() { _, _, _ = procSetThreadExecutionState.Call(uintptr(esContinuous)) }

// Alert muestra un cuadro de mensaje nativo (para errores antes de abrir la ventana).
func Alert(title, msg string) {
	t, _ := windows.UTF16PtrFromString(title)
	m, _ := windows.UTF16PtrFromString(msg)
	_, _, _ = procMessageBoxW.Call(0, uintptr(unsafe.Pointer(m)), uintptr(unsafe.Pointer(t)), 0x30) // MB_ICONWARNING
}

const runKey = `Software\Microsoft\Windows\CurrentVersion\Run`

// SetAutoStart activa o desactiva el arranque al iniciar sesión (clave Run del
// usuario: no necesita permisos de administrador).
func SetAutoStart(name string, on bool) error {
	k, _, err := registry.CreateKey(registry.CURRENT_USER, runKey, registry.SET_VALUE|registry.QUERY_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	if !on {
		if err := k.DeleteValue(name); err != nil && !errors.Is(err, registry.ErrNotExist) {
			return err
		}
		return nil
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	return k.SetStringValue(name, `"`+exe+`" --autostart`)
}

// AutoStartEnabled indica si el arranque al iniciar sesión está activado.
func AutoStartEnabled(name string) bool {
	k, err := registry.OpenKey(registry.CURRENT_USER, runKey, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer k.Close()
	_, _, err = k.GetStringValue(name)
	return err == nil
}
