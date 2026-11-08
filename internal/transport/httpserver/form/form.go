// Package form ayuda a leer valores de formularios HTML de forma tolerante:
// los campos vacíos o mal escritos devuelven el valor cero en lugar de error,
// y la validación de negocio la hace el servicio correspondiente.
package form

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"radioclub25/internal/domain"
)

// String devuelve el valor sin espacios a los lados.
func String(r *http.Request, name string) string { return strings.TrimSpace(r.FormValue(name)) }

// Int devuelve el valor como int (0 si está vacío o es inválido).
func Int(r *http.Request, name string) int {
	n, _ := strconv.Atoi(String(r, name))
	return n
}

// Int64 devuelve el valor como int64 (0 si está vacío o es inválido).
func Int64(r *http.Request, name string) int64 {
	n, _ := strconv.ParseInt(String(r, name), 10, 64)
	return n
}

// Bool devuelve true si el checkbox está marcado ("on", "1", "true").
func Bool(r *http.Request, name string) bool {
	switch strings.ToLower(String(r, name)) {
	case "on", "1", "true", "yes", "si", "sí":
		return true
	}
	return false
}

// Date lee un <input type="date"> (YYYY-MM-DD) como medianoche UTC. Cero si está vacío.
func Date(r *http.Request, name string) time.Time {
	t, _ := time.Parse(domain.DateLayout, String(r, name))
	return t
}

// DatePtr es como Date pero devuelve nil si el campo está vacío.
func DatePtr(r *http.Request, name string) *time.Time {
	t := Date(r, name)
	if t.IsZero() {
		return nil
	}
	return &t
}

// DateTimePtr lee un <input type="datetime-local"> en la zona loc. nil si está vacío.
func DateTimePtr(r *http.Request, name string, loc *time.Location) *time.Time {
	t, err := time.ParseInLocation("2006-01-02T15:04", String(r, name), loc)
	if err != nil {
		return nil
	}
	return &t
}

// Money lee un importe "1234.50" o "1,234.50" y lo devuelve en céntimos.
func Money(r *http.Request, name string) domain.Money {
	s := strings.ReplaceAll(String(r, name), ",", "")
	if s == "" {
		return 0
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return -1 // inválido: el servicio lo rechazará
	}
	if f < 0 {
		return domain.Money(f*100 - 0.5)
	}
	return domain.Money(f*100 + 0.5)
}

// ID lee un parámetro numérico de la ruta (chi) o del formulario.
func ID(s string) int64 {
	n, _ := strconv.ParseInt(s, 10, 64)
	return n
}
