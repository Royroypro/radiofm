// Package audit registra las acciones relevantes (crear, editar, borrar,
// bloquear, iniciar sesión...) para que el superadmin pueda revisarlas.
// Un fallo al registrar nunca interrumpe la acción principal: solo se loguea.
package audit

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"radioclub25/internal/domain"
)

// Service escribe y lee la auditoría.
type Service struct {
	repo domain.AuditRepository
	log  *slog.Logger
}

// New crea el servicio.
func New(repo domain.AuditRepository, log *slog.Logger) *Service {
	return &Service{repo: repo, log: log}
}

// Log registra una acción. details admite formato estilo fmt.Sprintf.
func (s *Service) Log(ctx context.Context, a domain.Actor, action, entity string, entityID int64, details string, args ...any) {
	if len(args) > 0 {
		details = fmt.Sprintf(details, args...)
	}
	e := &domain.AuditEntry{
		UserID: a.UserID, UserEmail: a.Email, IP: a.IP,
		Action: action, Entity: entity, EntityID: entityID, Details: details, CreatedAt: time.Now(),
	}
	if err := s.repo.Add(context.WithoutCancel(ctx), e); err != nil {
		s.log.Error("no se pudo registrar auditoría", "err", err, "action", action, "entity", entity)
	}
}

// List devuelve hasta limit entradas saltando offset (más reciente primero).
func (s *Service) List(ctx context.Context, limit, offset int) ([]domain.AuditEntry, error) {
	return s.repo.List(ctx, limit, offset)
}
