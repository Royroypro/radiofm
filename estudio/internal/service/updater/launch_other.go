//go:build !windows

package updater

import "errors"

func launchDetached(string, ...string) error {
	return errors.New("la actualización automática solo funciona en Windows")
}
