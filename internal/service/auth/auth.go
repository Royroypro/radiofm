// Package auth gestiona el inicio de sesión, las sesiones y los usuarios de los paneles.
//
// Sesiones: al iniciar sesión se genera un token aleatorio de 32 bytes que va
// en una cookie HttpOnly. En la base solo se guarda su SHA-256, así que un
// robo de la base no permite suplantar sesiones.
//
// Permisos sobre usuarios (canManage):
//   - Nadie puede crear, editar ni borrar superadmins desde el panel
//     (solo con el comando `radioweb create-superadmin`).
//   - Superadmin y admin gestionan admins y editores.
//   - Un editor no gestiona usuarios.
package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/mail"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"

	"radioclub25/internal/domain"
	"radioclub25/internal/service/audit"
)

// MinPasswordLen es la longitud mínima de contraseña.
const MinPasswordLen = 8

const bcryptCost = 12

// Service implementa los casos de uso de autenticación.
type Service struct {
	users    domain.UserRepository
	sessions domain.SessionRepository
	audit    *audit.Service
	ttl      time.Duration
	limiter  *limiter
	now      func() time.Time
}

// New crea el servicio. ttl es la duración de una sesión.
func New(users domain.UserRepository, sessions domain.SessionRepository, au *audit.Service, ttl time.Duration) *Service {
	return &Service{users: users, sessions: sessions, audit: au, ttl: ttl,
		limiter: newLimiter(10, 15*time.Minute), now: time.Now}
}

// Login valida credenciales y crea una sesión. Devuelve el token para la cookie.
func (s *Service) Login(ctx context.Context, email, password, ip, userAgent string) (string, *domain.User, error) {
	email = normalizeEmail(email)
	key := ip + "|" + email
	if !s.limiter.allow(key, s.now()) {
		return "", nil, domain.ErrTooManyAttempts
	}
	u, err := s.users.GetByEmail(ctx, email)
	if err != nil || !u.Active || bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(password)) != nil {
		if err != nil && !errors.Is(err, domain.ErrNotFound) {
			return "", nil, fmt.Errorf("auth: login: %w", err)
		}
		s.limiter.fail(key, s.now())
		s.audit.Log(ctx, domain.Actor{Email: email, IP: ip}, "login_failed", "user", 0, "")
		return "", nil, domain.ErrInvalidCredentials
	}
	s.limiter.reset(key)

	token, hash := newToken()
	now := s.now()
	if len(userAgent) > 255 {
		userAgent = userAgent[:255]
	}
	if err := s.sessions.Create(ctx, &domain.Session{TokenHash: hash, UserID: u.ID, IP: ip, UserAgent: userAgent,
		CreatedAt: now, ExpiresAt: now.Add(s.ttl)}); err != nil {
		return "", nil, fmt.Errorf("auth: crear sesión: %w", err)
	}
	_ = s.users.TouchLogin(ctx, u.ID, now)
	s.audit.Log(ctx, domain.Actor{UserID: u.ID, Email: u.Email, IP: ip}, "login", "user", u.ID, "")
	return token, u, nil
}

// Authenticate devuelve el usuario dueño del token o ErrUnauthorized.
func (s *Service) Authenticate(ctx context.Context, token string) (*domain.User, error) {
	if token == "" {
		return nil, domain.ErrUnauthorized
	}
	sess, err := s.sessions.Get(ctx, hashToken(token))
	if err != nil {
		return nil, domain.ErrUnauthorized
	}
	if s.now().After(sess.ExpiresAt) {
		_ = s.sessions.Delete(ctx, sess.TokenHash)
		return nil, domain.ErrUnauthorized
	}
	u, err := s.users.GetByID(ctx, sess.UserID)
	if err != nil || !u.Active {
		return nil, domain.ErrUnauthorized
	}
	return u, nil
}

// Logout borra la sesión del token.
func (s *Service) Logout(ctx context.Context, token string) error {
	if token == "" {
		return nil
	}
	return s.sessions.Delete(ctx, hashToken(token))
}

// ChangePassword cambia la contraseña propia verificando la actual.
func (s *Service) ChangePassword(ctx context.Context, actor domain.Actor, current, next string) error {
	u, err := s.users.GetByID(ctx, actor.UserID)
	if err != nil {
		return err
	}
	if bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(current)) != nil {
		return domain.Invalid("current_password", "La contraseña actual no es correcta")
	}
	if err := validatePassword(next); err != nil {
		return err
	}
	hash, err := hashPassword(next)
	if err != nil {
		return err
	}
	if err := s.users.UpdatePassword(ctx, u.ID, hash); err != nil {
		return err
	}
	s.audit.Log(ctx, actor, "change_password", "user", u.ID, "")
	return nil
}

// ListUsers devuelve los usuarios visibles para el actor (un admin no ve superadmins).
func (s *Service) ListUsers(ctx context.Context, actor domain.Actor) ([]domain.User, error) {
	all, err := s.users.List(ctx)
	if err != nil {
		return nil, err
	}
	if actor.Role == domain.RoleSuperAdmin {
		return all, nil
	}
	out := all[:0]
	for _, u := range all {
		if u.Role != domain.RoleSuperAdmin {
			out = append(out, u)
		}
	}
	return out, nil
}

// GetUser devuelve un usuario si el actor puede gestionarlo.
func (s *Service) GetUser(ctx context.Context, actor domain.Actor, id int64) (*domain.User, error) {
	u, err := s.users.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if !canManage(actor.Role, u.Role) {
		return nil, domain.ErrForbidden
	}
	return u, nil
}

// UserInput son los datos de alta/edición de un usuario.
type UserInput struct {
	Email    string
	Name     string
	Role     domain.Role
	Active   bool
	Password string // obligatorio al crear; al editar, vacío = no cambiar
}

// CreateUser crea un admin o editor.
func (s *Service) CreateUser(ctx context.Context, actor domain.Actor, in UserInput) (*domain.User, error) {
	if !canManage(actor.Role, in.Role) {
		return nil, domain.ErrForbidden
	}
	if err := validateUser(in, true); err != nil {
		return nil, err
	}
	hash, err := hashPassword(in.Password)
	if err != nil {
		return nil, err
	}
	u := &domain.User{Email: normalizeEmail(in.Email), Name: strings.TrimSpace(in.Name), Role: in.Role,
		Active: in.Active, PasswordHash: hash}
	if err := s.users.Create(ctx, u); err != nil {
		if errors.Is(err, domain.ErrConflict) {
			return nil, domain.Invalid("email", "Ya existe un usuario con ese email")
		}
		return nil, err
	}
	s.audit.Log(ctx, actor, "create", "user", u.ID, "%s (%s)", u.Email, u.Role)
	return u, nil
}

// UpdateUser edita un usuario existente.
func (s *Service) UpdateUser(ctx context.Context, actor domain.Actor, id int64, in UserInput) error {
	u, err := s.GetUser(ctx, actor, id)
	if err != nil {
		return err
	}
	if !canManage(actor.Role, in.Role) {
		return domain.ErrForbidden
	}
	if id == actor.UserID && (!in.Active || in.Role != u.Role) {
		return domain.Invalid("role", "No puede desactivarse ni cambiarse el rol a sí mismo")
	}
	if err := validateUser(in, false); err != nil {
		return err
	}
	u.Email, u.Name, u.Role, u.Active = normalizeEmail(in.Email), strings.TrimSpace(in.Name), in.Role, in.Active
	if err := s.users.Update(ctx, u); err != nil {
		if errors.Is(err, domain.ErrConflict) {
			return domain.Invalid("email", "Ya existe un usuario con ese email")
		}
		return err
	}
	if in.Password != "" {
		hash, err := hashPassword(in.Password)
		if err != nil {
			return err
		}
		if err := s.users.UpdatePassword(ctx, id, hash); err != nil {
			return err
		}
		_ = s.sessions.DeleteByUser(ctx, id)
	}
	if !u.Active {
		_ = s.sessions.DeleteByUser(ctx, id)
	}
	s.audit.Log(ctx, actor, "update", "user", id, "%s (%s, activo=%t)", u.Email, u.Role, u.Active)
	return nil
}

// DeleteUser borra un usuario (no a sí mismo).
func (s *Service) DeleteUser(ctx context.Context, actor domain.Actor, id int64) error {
	if id == actor.UserID {
		return domain.Invalid("id", "No puede borrarse a sí mismo")
	}
	u, err := s.GetUser(ctx, actor, id)
	if err != nil {
		return err
	}
	if err := s.users.Delete(ctx, id); err != nil {
		return err
	}
	s.audit.Log(ctx, actor, "delete", "user", id, u.Email)
	return nil
}

// EnsureSuperAdmin crea un superadmin con ese email o le cambia la contraseña
// si ya existe. Solo se usa desde la línea de comandos o el arranque.
func (s *Service) EnsureSuperAdmin(ctx context.Context, email, password string) (*domain.User, bool, error) {
	email = normalizeEmail(email)
	if _, err := mail.ParseAddress(email); err != nil {
		return nil, false, domain.Invalid("email", "Email inválido")
	}
	if err := validatePassword(password); err != nil {
		return nil, false, err
	}
	hash, err := hashPassword(password)
	if err != nil {
		return nil, false, err
	}
	if u, err := s.users.GetByEmail(ctx, email); err == nil {
		u.Role, u.Active = domain.RoleSuperAdmin, true
		if err := s.users.Update(ctx, u); err != nil {
			return nil, false, err
		}
		return u, false, s.users.UpdatePassword(ctx, u.ID, hash)
	}
	u := &domain.User{Email: email, Name: "Superadmin", Role: domain.RoleSuperAdmin, Active: true, PasswordHash: hash}
	if err := s.users.Create(ctx, u); err != nil {
		return nil, false, err
	}
	s.audit.Log(ctx, domain.SystemActor, "create", "user", u.ID, "superadmin %s", email)
	return u, true, nil
}

// HasSuperAdmin indica si existe al menos un superadmin.
func (s *Service) HasSuperAdmin(ctx context.Context) (bool, error) {
	all, err := s.users.List(ctx)
	if err != nil {
		return false, err
	}
	for _, u := range all {
		if u.Role == domain.RoleSuperAdmin {
			return true, nil
		}
	}
	return false, nil
}

// CleanupSessions borra las sesiones vencidas (tarea periódica).
func (s *Service) CleanupSessions(ctx context.Context) error {
	_, err := s.sessions.DeleteExpired(ctx, s.now())
	return err
}

// ---------- utilidades ----------

func canManage(actor, target domain.Role) bool {
	return target != domain.RoleSuperAdmin && target.Valid() && actor.AtLeast(domain.RoleAdmin)
}

func validateUser(in UserInput, creating bool) error {
	if _, err := mail.ParseAddress(strings.TrimSpace(in.Email)); err != nil {
		return domain.Invalid("email", "Email inválido")
	}
	if strings.TrimSpace(in.Name) == "" {
		return domain.Invalid("name", "El nombre es obligatorio")
	}
	if creating || in.Password != "" {
		return validatePassword(in.Password)
	}
	return nil
}

func validatePassword(p string) error {
	if len(p) < MinPasswordLen {
		return domain.Invalid("password", fmt.Sprintf("La contraseña debe tener al menos %d caracteres", MinPasswordLen))
	}
	if len(p) > 72 { // límite de bcrypt
		return domain.Invalid("password", "La contraseña no puede superar 72 caracteres")
	}
	return nil
}

func hashPassword(p string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(p), bcryptCost)
	if err != nil {
		return "", fmt.Errorf("auth: hash: %w", err)
	}
	return string(b), nil
}

func normalizeEmail(e string) string { return strings.ToLower(strings.TrimSpace(e)) }

func newToken() (token, hash string) {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	token = hex.EncodeToString(b)
	return token, hashToken(token)
}

func hashToken(t string) string {
	sum := sha256.Sum256([]byte(t))
	return hex.EncodeToString(sum[:])
}

// limiter limita intentos fallidos de login por clave (IP+email) en una ventana.
type limiter struct {
	mu     sync.Mutex
	max    int
	window time.Duration
	hits   map[string][]time.Time
}

func newLimiter(max int, window time.Duration) *limiter {
	return &limiter{max: max, window: window, hits: map[string][]time.Time{}}
}

func (l *limiter) allow(key string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	recent := l.prune(key, now)
	return len(recent) < l.max
}

func (l *limiter) fail(key string, now time.Time) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.hits[key] = append(l.prune(key, now), now)
}

func (l *limiter) reset(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.hits, key)
}

func (l *limiter) prune(key string, now time.Time) []time.Time {
	var keep []time.Time
	for _, t := range l.hits[key] {
		if now.Sub(t) < l.window {
			keep = append(keep, t)
		}
	}
	if len(keep) == 0 {
		delete(l.hits, key)
	} else {
		l.hits[key] = keep
	}
	return keep
}
