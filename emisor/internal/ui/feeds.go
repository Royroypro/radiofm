package ui

import (
	"fmt"
	"sync"

	"radioclub25/emisor/internal/config"
	"radioclub25/emisor/internal/feed"
	"radioclub25/emisor/internal/runner"
)

// feeds guarda el bus de audio de cada salida ("radio", "tv"). El ffmpeg que
// codifica lee siempre del bus; lo que lo alimenta (micrófono, línea, audio de
// la PC, tono, silencio) se cambia al aire sin tocar la conexión.
type feeds struct {
	mu   sync.Mutex
	open map[string]*feed.Feed
}

// feed devuelve el bus de which, abriéndolo si hace falta.
func (a *App) feed(which string) (*feed.Feed, error) {
	a.fd.mu.Lock()
	defer a.fd.mu.Unlock()
	if f, ok := a.fd.open[which]; ok {
		return f, nil
	}
	f, err := feed.New(feed.Options{Name: which, FFmpeg: a.ffmpeg, OnLog: a.addLog, Hide: runner.HideWindow})
	if err != nil {
		return nil, fmt.Errorf("No se pudo abrir el bus de audio: %w", err)
	}
	if a.fd.open == nil {
		a.fd.open = map[string]*feed.Feed{}
	}
	a.fd.open[which] = f
	return f, nil
}

// closeFeed cierra el bus de which (libera el micrófono o la captura).
func (a *App) closeFeed(which string) {
	a.fd.mu.Lock()
	f := a.fd.open[which]
	delete(a.fd.open, which)
	a.fd.mu.Unlock()
	if f != nil {
		f.Close()
	}
}

// busInput conecta la fuente src al bus de which y devuelve la entrada que debe
// leer el codificador. «El audio del video» no pasa por el bus: se devuelve igual.
func (a *App) busInput(which string, src config.Source) (config.Source, error) {
	if !feed.Supports(src) {
		a.closeFeed(which)
		return src, nil
	}
	f, err := a.feed(which)
	if err != nil {
		return src, err
	}
	if err := f.Switch(src); err != nil {
		return src, fmt.Errorf("Entrada de audio: %w", err)
	}
	return config.Source{Kind: config.SourceLoopback, Addr: f.Addr()}, nil
}

// inputOf es la entrada de audio de which en la configuración c.
func inputOf(c config.Config, which string) config.Source {
	if which == "radio" {
		return c.Radio.Input
	}
	return c.TV.Audio
}

// liveSwitchable dice si pasar de old a new se puede hacer al aire, sin reconectar.
func liveSwitchable(old, new config.Source) bool {
	return feed.Supports(old) && feed.Supports(new)
}
