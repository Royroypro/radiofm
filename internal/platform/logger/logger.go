// Package logger crea el logger estructurado (log/slog) de la aplicación:
// texto legible en desarrollo y JSON en producción.
package logger

import (
	"log/slog"
	"os"
)

// New devuelve un *slog.Logger y lo deja como logger por defecto.
func New(production bool) *slog.Logger {
	var h slog.Handler
	if production {
		h = slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})
	} else {
		h = slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug})
	}
	l := slog.New(h)
	slog.SetDefault(l)
	return l
}
