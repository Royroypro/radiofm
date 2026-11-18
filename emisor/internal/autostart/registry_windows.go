//go:build windows

package autostart

import (
	"errors"
	"fmt"

	"golang.org/x/sys/windows/registry"
)

// Get devuelve la línea guardada en key (bajo HKCU) para el emisor, si existe.
func Get(key string) (string, bool, error) {
	k, err := registry.OpenKey(registry.CURRENT_USER, key, registry.QUERY_VALUE)
	if errors.Is(err, registry.ErrNotExist) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	defer k.Close()
	v, _, err := k.GetStringValue(ValueName)
	if errors.Is(err, registry.ErrNotExist) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return v, true, nil
}

// Set pone (enabled) o quita la entrada del emisor en key (bajo HKCU).
// Solo escribe si cambia algo.
func Set(key string, enabled bool, command string) error {
	cur, ok, err := Get(key)
	if err != nil {
		return err
	}
	if enabled && ok && cur == command || !enabled && !ok {
		return nil
	}
	k, _, err := registry.CreateKey(registry.CURRENT_USER, key, registry.QUERY_VALUE|registry.SET_VALUE)
	if err != nil {
		return fmt.Errorf("arranque con Windows: %w", err)
	}
	defer k.Close()
	if enabled {
		return k.SetStringValue(ValueName, command)
	}
	if err := k.DeleteValue(ValueName); err != nil && !errors.Is(err, registry.ErrNotExist) {
		return fmt.Errorf("arranque con Windows: %w", err)
	}
	return nil
}
