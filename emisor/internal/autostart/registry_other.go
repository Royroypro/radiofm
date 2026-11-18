//go:build !windows

package autostart

import "errors"

var errUnsupported = errors.New("el arranque automático solo existe en Windows")

// Get no hace nada fuera de Windows.
func Get(string) (string, bool, error) { return "", false, nil }

// Set no hace nada fuera de Windows.
func Set(_ string, enabled bool, _ string) error {
	if enabled {
		return errUnsupported
	}
	return nil
}
