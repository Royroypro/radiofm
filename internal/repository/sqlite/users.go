package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"radioclub25/internal/domain"
)

// UserRepo implementa domain.UserRepository.
type UserRepo struct{ db *sql.DB }

// NewUserRepo crea el repositorio de usuarios.
func NewUserRepo(db *sql.DB) *UserRepo { return &UserRepo{db: db} }

const userCols = `id, email, name, password_hash, role, active, created_at, last_login_at`

func scanUser(r rowScanner) (*domain.User, error) {
	var u domain.User
	var role, created string
	var active int
	var last sql.NullString
	if err := r.Scan(&u.ID, &u.Email, &u.Name, &u.PasswordHash, &role, &active, &created, &last); err != nil {
		return nil, err
	}
	u.Role, u.Active, u.CreatedAt, u.LastLoginAt = domain.Role(role), active == 1, parseTime(created), parseTimePtr(last)
	return &u, nil
}

// Create inserta un usuario y rellena su ID.
func (r *UserRepo) Create(ctx context.Context, u *domain.User) error {
	if u.CreatedAt.IsZero() {
		u.CreatedAt = time.Now()
	}
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO users (email, name, password_hash, role, active, created_at) VALUES (?,?,?,?,?,?)`,
		u.Email, u.Name, u.PasswordHash, string(u.Role), boolInt(u.Active), formatTime(u.CreatedAt))
	if isUnique(err) {
		return domain.ErrConflict
	}
	if err != nil {
		return fmt.Errorf("users: create: %w", err)
	}
	u.ID, _ = res.LastInsertId()
	return nil
}

// GetByID busca por ID.
func (r *UserRepo) GetByID(ctx context.Context, id int64) (*domain.User, error) {
	u, err := scanUser(r.db.QueryRowContext(ctx, `SELECT `+userCols+` FROM users WHERE id = ?`, id))
	return u, notFound(err)
}

// GetByEmail busca por email (sin distinguir mayúsculas).
func (r *UserRepo) GetByEmail(ctx context.Context, email string) (*domain.User, error) {
	u, err := scanUser(r.db.QueryRowContext(ctx, `SELECT `+userCols+` FROM users WHERE email = ?`, email))
	return u, notFound(err)
}

// List devuelve todos los usuarios ordenados por rol y email.
func (r *UserRepo) List(ctx context.Context) ([]domain.User, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT `+userCols+` FROM users ORDER BY
		CASE role WHEN 'superadmin' THEN 0 WHEN 'admin' THEN 1 ELSE 2 END, email`)
	if err != nil {
		return nil, fmt.Errorf("users: list: %w", err)
	}
	defer rows.Close()
	var out []domain.User
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *u)
	}
	return out, rows.Err()
}

// Update modifica email, nombre, rol y estado (no la contraseña).
func (r *UserRepo) Update(ctx context.Context, u *domain.User) error {
	res, err := r.db.ExecContext(ctx, `UPDATE users SET email=?, name=?, role=?, active=? WHERE id=?`,
		u.Email, u.Name, string(u.Role), boolInt(u.Active), u.ID)
	if isUnique(err) {
		return domain.ErrConflict
	}
	return checkAffected(res, err)
}

// UpdatePassword cambia el hash de la contraseña.
func (r *UserRepo) UpdatePassword(ctx context.Context, id int64, hash string) error {
	return checkAffected(r.db.ExecContext(ctx, `UPDATE users SET password_hash=? WHERE id=?`, hash, id))
}

// TouchLogin guarda la fecha del último acceso.
func (r *UserRepo) TouchLogin(ctx context.Context, id int64, at time.Time) error {
	_, err := r.db.ExecContext(ctx, `UPDATE users SET last_login_at=? WHERE id=?`, formatTime(at), id)
	return err
}

// Delete borra un usuario (sus sesiones se borran en cascada).
func (r *UserRepo) Delete(ctx context.Context, id int64) error {
	return checkAffected(r.db.ExecContext(ctx, `DELETE FROM users WHERE id=?`, id))
}

// SessionRepo implementa domain.SessionRepository.
type SessionRepo struct{ db *sql.DB }

// NewSessionRepo crea el repositorio de sesiones.
func NewSessionRepo(db *sql.DB) *SessionRepo { return &SessionRepo{db: db} }

// Create guarda una sesión.
func (r *SessionRepo) Create(ctx context.Context, s *domain.Session) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO sessions (token_hash, user_id, ip, user_agent, created_at, expires_at) VALUES (?,?,?,?,?,?)`,
		s.TokenHash, s.UserID, s.IP, s.UserAgent, formatTime(s.CreatedAt), formatTime(s.ExpiresAt))
	if err != nil {
		return fmt.Errorf("sessions: create: %w", err)
	}
	return nil
}

// Get busca una sesión por el hash del token.
func (r *SessionRepo) Get(ctx context.Context, tokenHash string) (*domain.Session, error) {
	var s domain.Session
	var created, expires string
	err := r.db.QueryRowContext(ctx,
		`SELECT token_hash, user_id, ip, user_agent, created_at, expires_at FROM sessions WHERE token_hash=?`, tokenHash).
		Scan(&s.TokenHash, &s.UserID, &s.IP, &s.UserAgent, &created, &expires)
	if err != nil {
		return nil, notFound(err)
	}
	s.CreatedAt, s.ExpiresAt = parseTime(created), parseTime(expires)
	return &s, nil
}

// Delete borra una sesión.
func (r *SessionRepo) Delete(ctx context.Context, tokenHash string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM sessions WHERE token_hash=?`, tokenHash)
	return err
}

// DeleteByUser cierra todas las sesiones de un usuario.
func (r *SessionRepo) DeleteByUser(ctx context.Context, userID int64) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM sessions WHERE user_id=?`, userID)
	return err
}

// DeleteExpired borra las sesiones vencidas y devuelve cuántas borró.
func (r *SessionRepo) DeleteExpired(ctx context.Context, now time.Time) (int64, error) {
	res, err := r.db.ExecContext(ctx, `DELETE FROM sessions WHERE expires_at < ?`, formatTime(now))
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
