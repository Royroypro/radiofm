package ui

import (
	"errors"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"radioclub25/estudio/internal/platform/secrets"
	"radioclub25/estudio/internal/video"
)

// TVStatus devuelve el estado del modo TV.
func (a *App) TVStatus() video.Status { return a.core.TV.Status() }

// TVSetScene cambia la escena de fondo: visual (espectro) o image (placa).
func (a *App) TVSetScene(scene string) { a.core.TV.SetScene(video.Scene(scene)) }

// TVSetLayers enciende o apaga las capas al instante (sin guardar).
func (a *App) TVSetLayers(logo, now, clock, ticker bool) {
	a.core.TV.SetLayers(logo, now, clock, ticker)
}

// TVOnAir enciende o apaga el modo TV y lo guarda.
func (a *App) TVOnAir(on bool) error {
	s := a.core.Settings()
	s.TV.Enabled = on
	if err := a.core.SaveSettings(s); err != nil {
		return err
	}
	a.core.ApplyTV()
	return nil
}

// TVApply reinicia el modo TV con los ajustes guardados.
func (a *App) TVApply() { a.core.ApplyTV() }

// SetTVPassword guarda (cifrada) la clave de publicación de la TV.
func (a *App) SetTVPassword(p string) error {
	if p == "" {
		return errors.New("escriba la clave")
	}
	if err := a.core.Secrets.Set(secrets.TVPassword, p); err != nil {
		return err
	}
	a.core.ApplyTV()
	return nil
}

// HasTVPassword dice si ya hay clave guardada.
func (a *App) HasTVPassword() bool { return a.core.Secrets.Has(secrets.TVPassword) }

// PickImage abre el diálogo para elegir una imagen (logo, fondo, placa).
func (a *App) PickImage(title string) (string, error) {
	return runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{Title: title, Filters: []runtime.FileFilter{
		{DisplayName: "Imágenes", Pattern: "*.png;*.jpg;*.jpeg;*.gif"}}})
}
