package ui

import (
	"errors"
	"fmt"
	"sync"

	"radioclub25/emisor/internal/config"
	"radioclub25/emisor/internal/ffmpeg"
	"radioclub25/emisor/internal/runner"
)

// eqState recuerda qué ganancias tiene cada ffmpeg corriendo, para mandar al
// aire solo las bandas que cambian.
type eqState struct {
	mu   sync.Mutex
	sent map[string][10]float64 // "radio" / "tv" → ganancias aplicadas en el proceso actual
}

func (e *eqState) set(which string, g [10]float64) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.sent == nil {
		e.sent = map[string][10]float64{}
	}
	e.sent[which] = g
}

func (e *eqState) get(which string) [10]float64 {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.sent[which]
}

// EQProfiles devuelve los perfiles de fábrica y los propios.
func (a *App) EQProfiles() []config.EQProfile {
	a.mu.Lock()
	own := append([]config.EQProfile(nil), a.cfg.EQProfiles...)
	a.mu.Unlock()
	return append(append([]config.EQProfile(nil), config.EQPresets...), own...)
}

// SetEQ cambia el ecualizador de "radio" o "tv". Al aire se aplica en caliente,
// banda por banda, sin cortar; si no, queda guardado para el inicio.
func (a *App) SetEQ(which string, eq config.EQ) error {
	r, err := a.runnerFor(which)
	if err != nil {
		return err
	}
	if err := config.ValidateEQ(which+".eq", eq); err != nil {
		return err
	}
	a.mu.Lock()
	c := a.cfg
	a.mu.Unlock()
	if which == "radio" {
		c.Radio.EQ = eq
	} else {
		c.TV.EQ = eq
	}
	if err := a.SaveConfig(c); err != nil {
		return err
	}
	to := eq.Effective()
	cmds := ffmpeg.EQCommands(a.eq.get(which), to)
	if len(cmds) == 0 {
		return nil
	}
	if err := r.Send(cmds); err != nil {
		if errors.Is(err, runner.ErrNotRunning) {
			return nil // se usará al iniciar
		}
		return fmt.Errorf("No se pudo cambiar el ecualizador al aire: %w", err)
	}
	a.eq.set(which, to)
	return nil
}

// SaveEQProfile guarda (o reemplaza) un perfil propio con esas ganancias.
func (a *App) SaveEQProfile(name string, gains [10]float64) ([]config.EQProfile, error) {
	name, err := config.CleanProfileName(name)
	if err != nil {
		return nil, err
	}
	if err := config.ValidateEQ("eq", config.EQ{Gains: gains}); err != nil {
		return nil, err
	}
	a.mu.Lock()
	c := a.cfg
	a.mu.Unlock()
	c.EQProfiles = config.UpsertProfile(append([]config.EQProfile(nil), c.EQProfiles...), config.EQProfile{Name: name, Gains: gains})
	if err := a.SaveConfig(c); err != nil {
		return nil, err
	}
	a.log("Perfil de ecualización guardado: %s", name)
	return a.EQProfiles(), nil
}

// DeleteEQProfile borra un perfil propio (los de fábrica no se borran).
func (a *App) DeleteEQProfile(name string) ([]config.EQProfile, error) {
	for _, p := range config.EQPresets {
		if p.Name == name {
			return nil, errors.New("Los perfiles de fábrica no se pueden borrar.")
		}
	}
	a.mu.Lock()
	c := a.cfg
	a.mu.Unlock()
	c.EQProfiles = config.RemoveProfile(append([]config.EQProfile(nil), c.EQProfiles...), name)
	if err := a.SaveConfig(c); err != nil {
		return nil, err
	}
	a.log("Perfil de ecualización borrado: %s", name)
	return a.EQProfiles(), nil
}
