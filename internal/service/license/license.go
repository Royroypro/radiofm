// Package license decide el nivel de servicio del cliente (activo, en mora o
// bloqueado) y los límites del plan contratado.
//
// Cómo funciona:
//   - Refresh lee la política, el contrato vigente y sus pagos, y calcula el
//     estado con domain.EvaluateLicense. El resultado queda en memoria y
//     Current() lo devuelve sin tocar la base (se consulta en cada petición).
//   - Refresh se ejecuta al arrancar, cada hora (scheduler) y después de
//     cualquier cambio en contratos, pagos o política.
//   - Si el nivel cambia se registra en auditoría; si pasa a bloqueo total se
//     cortan los streams mediante el StreamCutter (servicio media).
package license

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"radioclub25/internal/domain"
	"radioclub25/internal/service/audit"
)

// StreamCutter corta las emisiones en curso (lo implementa service/media).
type StreamCutter interface {
	CutStreams(ctx context.Context) error
}

// Service mantiene el estado de la licencia.
type Service struct {
	repo    domain.LicenseRepository
	billing domain.BillingRepository
	cutter  StreamCutter
	audit   *audit.Service
	log     *slog.Logger
	loc     *time.Location
	now     func() time.Time

	refreshMu sync.Mutex // serializa Refresh
	mu        sync.RWMutex
	state     domain.LicenseState
	policy    domain.LicensePolicy
}

// New crea el servicio. Hasta el primer Refresh el estado es "activo sin límites".
func New(repo domain.LicenseRepository, billing domain.BillingRepository, cutter StreamCutter,
	au *audit.Service, log *slog.Logger, loc *time.Location) *Service {
	return &Service{repo: repo, billing: billing, cutter: cutter, audit: au, log: log, loc: loc, now: time.Now,
		state:  domain.LicenseState{Level: domain.LicenseActive, Limits: domain.UnlimitedLimits()},
		policy: domain.DefaultLicensePolicy()}
}

// Current devuelve el último estado calculado (no consulta la base).
func (s *Service) Current() domain.LicenseState {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.state
}

// SuspendedMessage devuelve el mensaje público para el bloqueo total.
func (s *Service) SuspendedMessage() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.policy.SuspendedMessage
}

// Policy devuelve la política guardada.
func (s *Service) Policy(ctx context.Context) (domain.LicensePolicy, error) {
	return s.repo.GetPolicy(ctx)
}

// Refresh recalcula el estado y aplica las consecuencias de un cambio de nivel.
func (s *Service) Refresh(ctx context.Context) (domain.LicenseState, error) {
	s.refreshMu.Lock()
	defer s.refreshMu.Unlock()

	pol, err := s.repo.GetPolicy(ctx)
	if err != nil {
		return s.Current(), fmt.Errorf("license: política: %w", err)
	}
	contract, err := s.billing.CurrentContract(ctx)
	if err != nil && !errors.Is(err, domain.ErrNotFound) {
		return s.Current(), fmt.Errorf("license: contrato: %w", err)
	}
	if errors.Is(err, domain.ErrNotFound) {
		contract = nil
	}

	var paidUntil *time.Time
	limits := domain.UnlimitedLimits()
	if contract != nil {
		if paidUntil, err = PaidUntil(ctx, s.billing, contract.ID); err != nil {
			return s.Current(), err
		}
		if plan, err := s.billing.GetPlan(ctx, contract.PlanID); err == nil {
			limits = domain.PlanLimits{PlanName: plan.Name, MaxActiveAds: plan.MaxActiveAds, MaxUploadMB: plan.MaxUploadMB,
				TVEnabled: plan.TVEnabled, StatsEnabled: plan.StatsEnabled}
		}
	}

	st := domain.EvaluateLicense(s.now().In(s.loc), pol, contract, paidUntil, limits)
	s.mu.Lock()
	s.state, s.policy = st, pol
	s.mu.Unlock()

	last, err := s.repo.LastLevel(ctx)
	if err != nil {
		return st, fmt.Errorf("license: último nivel: %w", err)
	}
	if last != st.Level {
		if err := s.repo.SaveLastLevel(ctx, st.Level); err != nil {
			return st, err
		}
		s.log.Warn("cambio de nivel de licencia", "de", last, "a", st.Level, "motivo", st.Reason)
		s.audit.Log(ctx, domain.SystemActor, "level_change", "license", 0, "%s → %s: %s", last, st.Level, st.Reason)
		if st.Level.PublicBlocked() && s.cutter != nil {
			if err := s.cutter.CutStreams(ctx); err != nil {
				s.log.Error("no se pudieron cortar los streams", "err", err)
			}
		}
	}
	return st, nil
}

// SavePolicy guarda los días de gracia, el nivel de bloqueo automático y el mensaje público.
// No toca el bloqueo manual.
func (s *Service) SavePolicy(ctx context.Context, actor domain.Actor, graceDays int, autoLevel domain.LicenseLevel, message string) error {
	if graceDays < 0 || graceDays > 90 {
		return domain.Invalid("grace_days", "Los días de gracia deben estar entre 0 y 90")
	}
	if !autoLevel.IsBlock() {
		return domain.Invalid("auto_block_level", "Nivel de bloqueo inválido")
	}
	pol, err := s.repo.GetPolicy(ctx)
	if err != nil {
		return err
	}
	pol.GraceDays, pol.AutoBlockLevel, pol.SuspendedMessage = graceDays, autoLevel, strings.TrimSpace(message)
	if err := s.repo.SavePolicy(ctx, pol); err != nil {
		return err
	}
	s.audit.Log(ctx, actor, "update", "license", 0, "gracia=%d días, bloqueo automático=%s", graceDays, autoLevel)
	_, err = s.Refresh(ctx)
	return err
}

// Block aplica un bloqueo manual de nivel level con un motivo.
func (s *Service) Block(ctx context.Context, actor domain.Actor, level domain.LicenseLevel, reason string) error {
	if !level.IsBlock() {
		return domain.Invalid("level", "Nivel de bloqueo inválido")
	}
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return domain.Invalid("reason", "Indique el motivo del bloqueo")
	}
	pol, err := s.repo.GetPolicy(ctx)
	if err != nil {
		return err
	}
	pol.ManualLevel, pol.ManualReason = level, reason
	if err := s.repo.SavePolicy(ctx, pol); err != nil {
		return err
	}
	s.audit.Log(ctx, actor, "block", "license", 0, "%s: %s", level, reason)
	_, err = s.Refresh(ctx)
	return err
}

// Unblock quita el bloqueo manual; vuelve a mandar la evaluación automática.
func (s *Service) Unblock(ctx context.Context, actor domain.Actor) error {
	pol, err := s.repo.GetPolicy(ctx)
	if err != nil {
		return err
	}
	pol.ManualLevel, pol.ManualReason = "", ""
	if err := s.repo.SavePolicy(ctx, pol); err != nil {
		return err
	}
	s.audit.Log(ctx, actor, "unblock", "license", 0, "")
	_, err = s.Refresh(ctx)
	return err
}

// PaidUntil devuelve el último día cubierto por pagos "pagados" del contrato, o nil.
func PaidUntil(ctx context.Context, repo domain.BillingRepository, contractID int64) (*time.Time, error) {
	pays, err := repo.ListPayments(ctx, contractID)
	if err != nil {
		return nil, fmt.Errorf("license: pagos: %w", err)
	}
	var max *time.Time
	for i := range pays {
		if pays[i].Status == domain.PaymentPaid && (max == nil || pays[i].PeriodEnd.After(*max)) {
			t := pays[i].PeriodEnd
			max = &t
		}
	}
	return max, nil
}
