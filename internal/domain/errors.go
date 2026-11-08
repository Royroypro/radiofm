package domain

import "errors"

// Errores de dominio. Las capas superiores los comparan con errors.Is para
// decidir el código HTTP o el mensaje a mostrar.
var (
	// ErrNotFound indica que la entidad solicitada no existe.
	ErrNotFound = errors.New("no encontrado")
	// ErrConflict indica que la operación choca con datos existentes (p. ej. email duplicado).
	ErrConflict = errors.New("conflicto con datos existentes")
	// ErrInvalidCredentials indica usuario o contraseña incorrectos.
	ErrInvalidCredentials = errors.New("credenciales inválidas")
	// ErrUnauthorized indica que no hay sesión válida.
	ErrUnauthorized = errors.New("no autenticado")
	// ErrForbidden indica que el usuario no tiene permiso para la acción.
	ErrForbidden = errors.New("permiso denegado")
	// ErrLicenseBlocked indica que la licencia del cliente impide la acción.
	ErrLicenseBlocked = errors.New("servicio bloqueado por licencia")
	// ErrPlanLimit indica que la acción supera un límite del plan contratado.
	ErrPlanLimit = errors.New("límite del plan alcanzado")
	// ErrTooManyAttempts indica demasiados intentos (p. ej. login).
	ErrTooManyAttempts = errors.New("demasiados intentos, espere un momento")
)

// ValidationError describe un error de validación de datos de entrada.
// Field es el nombre del campo del formulario y Msg el mensaje para el usuario.
type ValidationError struct {
	Field string
	Msg   string
}

func (e *ValidationError) Error() string { return e.Field + ": " + e.Msg }

// Invalid crea un *ValidationError.
func Invalid(field, msg string) error { return &ValidationError{Field: field, Msg: msg} }
