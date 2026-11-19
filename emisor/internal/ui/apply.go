package ui

import (
	"errors"
	"fmt"
	"reflect"

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
	a.mu.Lock()
	before := a.cfg
	a.mu.Unlock()
	if err := a.SaveConfig(c); err != nil {
		return false, err
	}
	if which == "radio" {
		a.followRadio(c) // la TV con «audio de la radio» sigue a la entrada nueva
	} else {
		a.forgetProbe() // al reconectar se vuelve a mirar si el video trae audio
	}
	if !r.Running() {
		return false, nil
	}
	// Si solo cambió la entrada de audio, se cambia al aire: los oyentes no se cortan.
	if onlyInputChanged(before, c, which) {
		if err := a.switchLive(which, liveSource(c, which)); err != nil {
			return false, err
		}
		a.markDirty()
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
	}
	if err := a.SaveConfig(c); err != nil {
		return err
	}
	if err := r.Send(ffmpeg.GainCommand(gainDB)); err != nil && !errors.Is(err, runner.ErrNotRunning) {
		return fmt.Errorf("No se pudo cambiar la ganancia al aire: %w", err)
	}
	return nil
}

// SetInput cambia la entrada de audio de "radio" o "tv" y la guarda. Si esa
// salida está al aire, el cambio se hace sin reconectar; solo pasar a o desde
// «el audio del video» (TV) exige reconexión, que se hace en seguida.
// Responde si hubo reconexión.
func (a *App) SetInput(which string, src config.Source) (bool, error) {
	r, err := a.runnerFor(which)
	if err != nil {
		return false, err
	}
	if err := a.requireAuth(); err != nil {
		return false, err
	}
	a.mu.Lock()
	c := a.cfg
	a.mu.Unlock()
	old := inputOf(c, which)
	if which == "radio" {
		c.Radio.Input = src
		err = c.ValidateRadio()
	} else {
		c.TV.Audio = src
		err = c.ValidateTV()
	}
	if err != nil {
		return false, err
	}
	if err := a.SaveConfig(c); err != nil {
		return false, err
	}
	if which == "radio" {
		a.followRadio(c) // la TV con «audio de la radio» cambia junto con la radio
	}
	if !r.Running() {
		return false, nil
	}
	if !liveSwitchable(old, src) {
		a.log("[%s] «el audio del video» cambia la forma de emitir: reconexión inmediata", which)
		r.Restart()
		a.markDirty()
		return true, nil
	}
	if which == "tv" && !a.tvUsesBus() {
		// Al aire va solo el audio del video: la fuente nueva queda guardada y se
		// usará si el video deja de traer audio o al cambiar el modo.
		return false, nil
	}
	return false, a.switchLive(which, liveSource(c, which))
}

// switchLive cambia la entrada del bus de which sin tocar la conexión.
func (a *App) switchLive(which string, src config.Source) error {
	f, err := a.feed(which)
	if err != nil {
		return err
	}
	if err := f.Switch(src); err != nil {
		a.log("[%s] no se pudo abrir la entrada nueva (%v); sigue al aire con silencio", which, err)
		return fmt.Errorf("Entrada de audio: %w", err)
	}
	return nil
}

// onlyInputChanged dice si entre old y new solo cambió la entrada de audio de
// which, y ese cambio se puede hacer al aire.
func onlyInputChanged(old, new config.Config, which string) bool {
	if which == "radio" {
		o, n := old.Radio, new.Radio
		if !liveSwitchable(o.Input, n.Input) {
			return false
		}
		o.Input, n.Input = config.Source{}, config.Source{}
		return reflect.DeepEqual(o, n)
	}
	o, n := old.TV, new.TV
	if !liveSwitchable(o.Audio, n.Audio) {
		return false
	}
	o.Audio, n.Audio = config.Source{}, config.Source{}
	return reflect.DeepEqual(o, n)
}
