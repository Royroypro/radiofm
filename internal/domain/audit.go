package domain

import (
	"context"
	"time"
)

// AuditEntry registra una acción relevante hecha en los paneles o por el sistema.
// UserID 0 significa "sistema" (por ejemplo, el bloqueo automático).
type AuditEntry struct {
	ID        int64
	UserID    int64
	UserEmail string
	Action    string // verbo corto: "create", "update", "delete", "login", "block"...
	Entity    string // "ad", "contract", "payment", "license", "settings"...
	EntityID  int64
	Details   string
	IP        string
	CreatedAt time.Time
}

// AuditRepository persiste y lista la auditoría.
type AuditRepository interface {
	Add(ctx context.Context, e *AuditEntry) error
	List(ctx context.Context, limit, offset int) ([]AuditEntry, error)
}

// Actor identifica quién ejecuta una acción, para la auditoría.
// El valor cero representa al sistema (tareas automáticas).
type Actor struct {
	UserID int64
	Email  string
	Role   Role
	IP     string
}

// SystemActor es el actor de las tareas automáticas.
var SystemActor = Actor{Email: "sistema"}
