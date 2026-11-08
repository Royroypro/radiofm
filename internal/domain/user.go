package domain

import (
	"context"
	"time"
)

// Role es el rol de un usuario del sistema.
type Role string

const (
	// RoleSuperAdmin es el proveedor del sistema: controla contratos, pagos,
	// licencia y todo lo demás. Solo se crea por línea de comandos.
	RoleSuperAdmin Role = "superadmin"
	// RoleAdmin es el administrador de la radio: marca, streams, publicidad,
	// programación y usuarios editores.
	RoleAdmin Role = "admin"
	// RoleEditor puede gestionar programación y publicidad, pero no ajustes ni usuarios.
	RoleEditor Role = "editor"
)

// Valid indica si el rol es uno de los conocidos.
func (r Role) Valid() bool {
	return r == RoleSuperAdmin || r == RoleAdmin || r == RoleEditor
}

// Label devuelve el nombre del rol para mostrar en la interfaz.
func (r Role) Label() string {
	switch r {
	case RoleSuperAdmin:
		return "Superadmin"
	case RoleAdmin:
		return "Administrador"
	case RoleEditor:
		return "Editor"
	}
	return string(r)
}

// rank ordena los roles de menor a mayor privilegio.
func (r Role) rank() int {
	switch r {
	case RoleSuperAdmin:
		return 3
	case RoleAdmin:
		return 2
	case RoleEditor:
		return 1
	}
	return 0
}

// AtLeast indica si r tiene igual o más privilegios que min.
func (r Role) AtLeast(min Role) bool { return r.rank() >= min.rank() }

// User es una persona que entra a los paneles.
type User struct {
	ID           int64
	Email        string
	Name         string
	PasswordHash string
	Role         Role
	Active       bool
	CreatedAt    time.Time
	LastLoginAt  *time.Time
}

// UserRepository persiste usuarios.
type UserRepository interface {
	Create(ctx context.Context, u *User) error
	GetByID(ctx context.Context, id int64) (*User, error)
	GetByEmail(ctx context.Context, email string) (*User, error)
	List(ctx context.Context) ([]User, error)
	Update(ctx context.Context, u *User) error
	UpdatePassword(ctx context.Context, id int64, hash string) error
	TouchLogin(ctx context.Context, id int64, at time.Time) error
	Delete(ctx context.Context, id int64) error
}

// Session es una sesión iniciada en el panel. El token en claro solo vive en
// la cookie del navegador; en la base se guarda su hash.
type Session struct {
	TokenHash string
	UserID    int64
	IP        string
	UserAgent string
	CreatedAt time.Time
	ExpiresAt time.Time
}

// SessionRepository persiste sesiones.
type SessionRepository interface {
	Create(ctx context.Context, s *Session) error
	Get(ctx context.Context, tokenHash string) (*Session, error)
	Delete(ctx context.Context, tokenHash string) error
	DeleteByUser(ctx context.Context, userID int64) error
	DeleteExpired(ctx context.Context, now time.Time) (int64, error)
}
