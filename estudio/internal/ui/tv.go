package ui

import (
	"errors"
	"strings"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"radioclub25/estudio/internal/platform/config"
	"radioclub25/estudio/internal/platform/secrets"
	"radioclub25/estudio/internal/service/updater"
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

// PickVideo abre el diálogo para elegir un video (videoclip o promo de una pista).
func (a *App) PickVideo(title string) (string, error) {
	return runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{Title: title, Filters: []runtime.FileFilter{
		{DisplayName: "Videos", Pattern: "*.mp4;*.mkv;*.webm;*.mov;*.avi;*.mpg;*.mpeg;*.m4v;*.ts"}}})
}

// PickBackground abre el diálogo para el fondo de la TV: imagen o video en bucle.
func (a *App) PickBackground(title string) (string, error) {
	return runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{Title: title, Filters: []runtime.FileFilter{
		{DisplayName: "Imágenes y videos", Pattern: "*.png;*.jpg;*.jpeg;*.gif;*.mp4;*.mkv;*.webm;*.mov;*.avi;*.m4v"}}})
}

// TVSetSource pone al aire la fuente id de Ajustes de TV ("" = volver a automático).
func (a *App) TVSetSource(id string) error {
	if id == "" {
		a.core.TV.SetSource(nil)
		return nil
	}
	for _, src := range a.core.Settings().TV.Sources {
		if src.ID == id {
			audio := src.Audio
			if audio == "" {
				audio = "radio"
			}
			a.core.TV.SetSource(&video.SourceSpec{ID: src.ID, Name: src.Name, Path: strings.TrimSpace(src.Path),
				Loop: src.Loop, Audio: audio})
			return nil
		}
	}
	return errors.New("la fuente no existe (guarde los cambios primero)")
}

// SaveTVSources guarda la lista de fuentes sin reiniciar la TV.
func (a *App) SaveTVSources(sources []config.TVSource) error {
	s := a.core.Settings()
	s.TV.Sources = sources
	return a.core.SaveSettings(s)
}

// UpdateStatus devuelve si hay una versión nueva y si ya está descargada.
func (a *App) UpdateStatus() updater.Status { return a.core.Updater.Status() }

// UpdateCheck busca una versión nueva ahora.
func (a *App) UpdateCheck() { a.core.Updater.Check() }

// UpdateInstall instala la versión descargada: el programa se cierra, se instala
// (Windows pide confirmar) y se vuelve a abrir solo.
func (a *App) UpdateInstall() error {
	if err := a.core.Updater.Install(); err != nil {
		return err
	}
	go func() {
		time.Sleep(300 * time.Millisecond)
		runtime.Quit(a.ctx)
	}()
	return nil
}
