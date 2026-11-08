//go:build !windows

package winsys

import "errors"

// ErrAlreadyRunning indica que ya hay otra instancia abierta.
var ErrAlreadyRunning = errors.New("Club25 Estudio ya está abierto")

// SingleInstance no hace nada fuera de Windows.
func SingleInstance(string) (func(), error) { return func() {}, nil }

// KeepAwake no hace nada fuera de Windows.
func KeepAwake() {}

// AllowSleep no hace nada fuera de Windows.
func AllowSleep() {}

// Alert no hace nada fuera de Windows.
func Alert(string, string) {}

// SetAutoStart no está disponible fuera de Windows.
func SetAutoStart(string, bool) error { return errors.New("solo en Windows") }

// AutoStartEnabled siempre es false fuera de Windows.
func AutoStartEnabled(string) bool { return false }
