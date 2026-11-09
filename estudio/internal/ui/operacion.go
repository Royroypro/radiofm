package ui

import (
	"context"
	"errors"
	"path/filepath"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"radioclub25/estudio/internal/service/airlog"
	"radioclub25/estudio/internal/service/rds"
)

// LoggerStatus devuelve el estado del grabador de la emisión.
func (a *App) LoggerStatus() airlog.Status { return a.core.AirLog.Status() }

// LoggerFiles devuelve los archivos de la grabación (más nuevos primero).
func (a *App) LoggerFiles() ([]airlog.File, error) {
	files, err := a.core.AirLog.Files()
	out := make([]airlog.File, 0, len(files))
	for i := len(files) - 1; i >= 0; i-- {
		out = append(out, files[i])
	}
	return out, err
}

// LoggerExport pide dónde guardar y exporta el tramo [from, to) (RFC 3339 o
// "2006-01-02T15:04" en hora local). Devuelve la ruta ("" si se canceló).
func (a *App) LoggerExport(from, to string) (string, error) {
	f, err := parseLocal(from)
	if err != nil {
		return "", err
	}
	t, err := parseLocal(to)
	if err != nil {
		return "", err
	}
	name := "emision_" + f.Format("2006-01-02_1504") + "-" + t.Format("1504") + ".mp3"
	path, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{Title: "Guardar tramo de la emisión",
		DefaultFilename: name, Filters: []runtime.FileFilter{{DisplayName: "MP3", Pattern: "*.mp3"}}})
	if err != nil || path == "" {
		return "", err
	}
	if filepath.Ext(path) == "" {
		path += ".mp3"
	}
	return path, a.core.AirLog.Export(context.Background(), f, t, path)
}

// OpenRecordings abre la carpeta de las grabaciones programadas en el Explorador.
func (a *App) OpenRecordings() {
	runtime.BrowserOpenURL(a.ctx, "file:///"+filepath.ToSlash(a.core.AirLog.RecordingsDir()))
}

func parseLocal(s string) (time.Time, error) {
	for _, layout := range []string{time.RFC3339, "2006-01-02T15:04", "2006-01-02T15:04:05"} {
		if t, err := time.ParseInLocation(layout, s, time.Local); err == nil {
			return t, nil
		}
	}
	return time.Time{}, errors.New("fecha y hora inválidas")
}

// RDSStatus devuelve el estado del envío al codificador RDS.
func (a *App) RDSStatus() rds.Status { return a.core.RDS.Status() }
