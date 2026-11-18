package ui

import (
	"fmt"
	"sync"

	"radioclub25/emisor/internal/config"
	"radioclub25/emisor/internal/loopback"
)

// capture es una captura de altavoces abierta, servida a ffmpeg por TCP local.
// Si la radio y la TV usan el mismo adaptador, comparten la misma captura.
type capture struct {
	srv  *loopback.Server
	stop func()
}

// captures guarda las capturas abiertas por nombre de adaptador ("" = predeterminado).
type captures struct {
	mu   sync.Mutex
	open map[string]*capture
}

// addr devuelve la dirección para ffmpeg de la captura del adaptador name, abriéndola si hace falta.
func (cs *captures) addr(name string) (string, error) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	if c, ok := cs.open[name]; ok {
		return c.srv.Addr(), nil
	}
	srv, err := loopback.NewServer()
	if err != nil {
		return "", err
	}
	stop, err := loopback.Capture(name, srv.Write)
	if err != nil {
		srv.Close()
		return "", fmt.Errorf("Audio de la PC: %w", err)
	}
	if cs.open == nil {
		cs.open = map[string]*capture{}
	}
	cs.open[name] = &capture{srv: srv, stop: stop}
	return srv.Addr(), nil
}

// keep cierra las capturas que ya no usa ninguna salida (por ejemplo, tras cambiar de adaptador).
func (cs *captures) keep(names map[string]bool) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	for n, c := range cs.open {
		if !names[n] {
			c.stop()
			c.srv.Close()
			delete(cs.open, n)
		}
	}
}

// closeAll cierra todas las capturas.
func (cs *captures) closeAll() { cs.keep(nil) }

// loopbackInUse devuelve los adaptadores de reproducción que usa la configuración de las salidas activas.
func loopbackInUse(c config.Config, radio, tv bool) map[string]bool {
	in := map[string]bool{}
	if radio && c.Radio.Input.Kind == config.SourceLoopback {
		in[c.Radio.Input.Name] = true
	}
	if tv && c.TV.Audio.Kind == config.SourceLoopback {
		in[c.TV.Audio.Name] = true
	}
	return in
}

// fillLoopback pone en s la dirección de la captura si la fuente es de altavoces.
func (a *App) fillLoopback(s *config.Source) error {
	if s.Kind != config.SourceLoopback {
		return nil
	}
	addr, err := a.caps.addr(s.Name)
	if err != nil {
		return err
	}
	s.Addr = addr
	return nil
}
