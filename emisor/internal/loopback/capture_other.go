//go:build !windows || !cgo

package loopback

import "errors"

// Available es false: esta compilación no captura altavoces (fuera de Windows
// o sin CGO). Las pruebas de la lógica no lo necesitan.
const Available = false

var errUnavailable = errors.New("esta versión del emisor no puede capturar la salida de audio (compilada sin CGO)")

// ListPlayback no devuelve nada en esta compilación.
func ListPlayback() ([]string, error) { return nil, errUnavailable }

// Capture no está disponible en esta compilación.
func Capture(string, func([]byte)) (func(), error) { return nil, errUnavailable }
