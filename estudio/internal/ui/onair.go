package ui

import (
	"context"

	"radioclub25/estudio/internal/app"
	"radioclub25/estudio/internal/domain"
	"radioclub25/estudio/internal/platform/secrets"
	"radioclub25/estudio/internal/service/onair"
	"radioclub25/estudio/internal/service/streaming"
	syncsvc "radioclub25/estudio/internal/service/sync"
)

// OnAirView es todo lo que dibuja la pantalla Al aire (se consulta varias veces por segundo).
type OnAirView struct {
	Air    onair.Snapshot   `json:"air"`
	Stream streaming.Status `json:"stream"`
	Audio  app.AudioState   `json:"audio"`
	Sync   syncsvc.Status   `json:"sync"`
	// TimeAnnounce es el interruptor «Hora locutada».
	TimeAnnounce bool `json:"time_announce"`
}

// OnAirStatus devuelve la foto del aire, la emisión y los dispositivos.
func (a *App) OnAirStatus() OnAirView {
	return OnAirView{Air: a.core.OnAir.Snapshot(), Stream: a.core.Streaming.Status(), Audio: a.core.Audio(),
		Sync: a.core.Sync.Status(), TimeAnnounce: !a.core.Settings().Locutions.TimeOff}
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

// SetServerConnection guarda la URL del panel y el token (cifrado) y sincroniza ya.
// token vacío conserva el guardado.
func (a *App) SetServerConnection(url, token string) error {
	st := a.core.Settings()
	st.Server.URL = url
	if err := a.core.SaveSettings(st); err != nil {
		return err
	}
	if token != "" {
		if err := a.core.Secrets.Set(secrets.ServerToken, token); err != nil {
			return err
		}
	}
	a.core.Sync.SyncNow()
	return nil
}

// SyncNow sincroniza con el panel ahora.
func (a *App) SyncNow() { a.core.Sync.SyncNow() }

// SyncStatus devuelve el estado de la sincronización.
func (a *App) SyncStatus() syncsvc.Status { return a.core.Sync.Status() }

// SetProcessor cambia el procesador de audio (preset: natural, fm, voice, off;
// scope: both o stream) y lo aplica al instante.
func (a *App) SetProcessor(preset, scope string) error {
	s := a.core.Settings()
	s.Audio.ProcPreset, s.Audio.ProcScope = preset, scope
	if err := a.core.SaveSettings(s); err != nil {
		return err
	}
	a.core.ApplyProcessor()
	return nil
}

// SetTimeAnnounce enciende o apaga la hora locutada (interruptor de Al aire).
// Apagada, los elementos de hora locutada de la playlist se saltan.
func (a *App) SetTimeAnnounce(on bool) error {
	s := a.core.Settings()
	s.Locutions.TimeOff = !on
	return a.core.SaveSettings(s)
}

// OnAirStopAfter alterna "parar al terminar lo que suena".
func (a *App) OnAirStopAfter() error { return a.core.OnAir.StopAfterCurrent() }

// History devuelve las últimas emisiones (la más reciente primero).
func (a *App) History(limit int) ([]domain.Play, error) {
	plays, err := a.core.Reports.Recent(context.Background(), limit)
	if plays == nil {
		plays = []domain.Play{}
	}
	return plays, err
}
