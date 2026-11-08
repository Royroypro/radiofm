package ui

import (
	"context"

	"radioclub25/estudio/internal/app"
	"radioclub25/estudio/internal/domain"
	"radioclub25/estudio/internal/platform/secrets"
	"radioclub25/estudio/internal/service/onair"
	"radioclub25/estudio/internal/service/streaming"
)

// OnAirView es todo lo que dibuja la pantalla Al aire (se consulta varias veces por segundo).
type OnAirView struct {
	Air    onair.Snapshot   `json:"air"`
	Stream streaming.Status `json:"stream"`
	Audio  app.AudioState   `json:"audio"`
}

// OnAirStatus devuelve la foto del aire, la emisión y los dispositivos.
func (a *App) OnAirStatus() OnAirView {
	return OnAirView{Air: a.core.OnAir.Snapshot(), Stream: a.core.Streaming.Status(), Audio: a.core.Audio()}
}

// OnAirStart arranca la automatización.
func (a *App) OnAirStart() error { return a.core.OnAir.Start() }

// OnAirStop detiene la automatización.
func (a *App) OnAirStop() error { return a.core.OnAir.Stop() }

// OnAirNext pasa al siguiente elemento (Espacio).
func (a *App) OnAirNext() error { return a.core.OnAir.Next() }

// OnAirMode cambia el modo (auto, assisted, live).
func (a *App) OnAirMode(mode string) error { return a.core.OnAir.SetMode(domain.StudioMode(mode)) }

// SetMic abre o cierra el micrófono.
func (a *App) SetMic(on bool) { a.core.Engine.SetMic(on) }

// SetDuck fuerza el ducking de la música.
func (a *App) SetDuck(on bool) { a.core.Engine.SetDuck(on) }

// TriggerCart dispara un botón de la botonera.
func (a *App) TriggerCart(page, slot int) error { return a.core.OnAir.TriggerCart(page, slot) }

// StopCarts detiene todos los botones.
func (a *App) StopCarts() error { return a.core.OnAir.StopCarts() }

// Carts devuelve los botones de una página.
func (a *App) Carts(page int) ([]domain.Cart, error) {
	list, err := a.core.OnAir.Carts(context.Background(), page)
	if list == nil {
		list = []domain.Cart{}
	}
	return list, err
}

// SaveCart guarda un botón (path vacío lo borra).
func (a *App) SaveCart(c domain.Cart) error { return a.core.OnAir.SaveCart(context.Background(), c) }

// SetIcecastPassword guarda la contraseña de fuente (cifrada) y reinicia la emisión.
func (a *App) SetIcecastPassword(pw string) error {
	if err := a.core.Secrets.Set(secrets.IcecastPassword, pw); err != nil {
		return err
	}
	a.core.Streaming.Restart(a.ctx)
	return nil
}

// RestartStreaming reinicia la emisión a Icecast con los ajustes actuales.
func (a *App) RestartStreaming() { a.core.Streaming.Restart(a.ctx) }

// RestartAudio reabre la tarjeta de salida y el micrófono.
func (a *App) RestartAudio() app.AudioState {
	a.core.RestartAudio()
	return a.core.Audio()
}
