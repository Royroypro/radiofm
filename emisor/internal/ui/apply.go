package ui

import (
	"errors"
	"fmt"

	"radioclub25/emisor/internal/config"
	"radioclub25/emisor/internal/ffmpeg"
	"radioclub25/emisor/internal/runner"
)

// runnerFor devuelve el supervisor de "radio" o "tv".
func (a *App) runnerFor(which string) (*runner.Runner, error) {
	switch which {
	case "radio":
		return a.radio, nil
	case "tv":
		return a.tv, nil
	}
	return nil, fmt.Errorf("salida desconocida: %q", which)
}

// SetEnhance enciende o apaga MEJORA en "radio" o "tv". Si esa salida está al
// aire se aplica en caliente, sin cortar; si no, queda guardado para el inicio.
func (a *App) SetEnhance(which string, on bool) error {
	r, err := a.runnerFor(which)
	if err != nil {
		return err
	}
	a.mu.Lock()
	c := a.cfg
	a.mu.Unlock()
	if which == "radio" {
		c.Radio.Enhance = on
	} else {
		c.TV.Enhance = on
	}
	if err := a.SaveConfig(c); err != nil {
		return err
	}
	state := map[bool]string{true: "encendida", false: "apagada"}[on]
	if err := r.Send(ffmpeg.EnhanceCommands(on)); err != nil {
		if errors.Is(err, runner.ErrNotRunning) {
			a.log("[%s] MEJORA %s (se usará al iniciar)", which, state)
			return nil
		}
		return fmt.Errorf("No se pudo cambiar MEJORA al aire: %w", err)
	}
	a.log("[%s] MEJORA %s al aire", which, state)
	return nil
}

// Apply guarda la configuración y, si la salida ("radio" o "tv") está al aire,
// la reconecta en seguida con los cambios (formato, bitrate, dispositivo,
// servidor…). Responde si hubo reinicio. La otra salida no se toca.
func (a *App) Apply(which string, c config.Config) (bool, error) {
	r, err := a.runnerFor(which)
	if err != nil {
		return false, err
	}
	if err := a.requireAuth(); err != nil {
		return false, err
	}
	// Antes de cortar, se comprueba que la configuración nueva sirva: si no, se
	// sigue al aire con la anterior y se avisa qué falta.
	if which == "radio" {
		err = c.ValidateRadio()
	} else {
		err = c.ValidateTV()
	}
	if err != nil {
		_ = a.SaveConfig(c)
		return false, err
	}
	if err := a.SaveConfig(c); err != nil {
		return false, err
	}
	if !r.Running() {
		return false, nil
	}
	a.log("[%s] aplicando cambios: reconexión inmediata", which)
	r.Restart()
	a.markDirty()
	return true, nil
}

// SetGain cambia la ganancia (dB) de "radio" o "tv". Al aire se aplica sin cortar.
func (a *App) SetGain(which string, gainDB float64) error {
	r, err := a.runnerFor(which)
	if err != nil {
		return err
	}
	if gainDB < -30 || gainDB > 30 {
		return errors.New("La ganancia debe estar entre -30 y +30 dB.")
	}
	a.mu.Lock()
	c := a.cfg
	a.mu.Unlock()
	if which == "radio" {
		c.Radio.GainDB = gainDB
	} else {
		c.TV.GainDB = gainDB
		if c.TV.Audio.Kind == config.SourceNone {
			gainDB = 0 // el silencio no lleva ganancia
		}
	}
	if err := a.SaveConfig(c); err != nil {
		return err
	}
	if err := r.Send(ffmpeg.GainCommand(gainDB)); err != nil && !errors.Is(err, runner.ErrNotRunning) {
		return fmt.Errorf("No se pudo cambiar la ganancia al aire: %w", err)
	}
	return nil
}
