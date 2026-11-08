// Package billing gestiona los datos comerciales que controla el superadmin:
// cliente, planes (membresías), contratos y pagos.
//
// Los pagos se registran a mano. Para integrar una pasarela en el futuro se
// implementa PaymentGateway y se llama a RegisterGatewayPayment desde el
// webhook de la pasarela; el resto del sistema no cambia.
//
// Después de cada cambio se ejecuta onChange (license.Refresh) para que el
// estado de la licencia se actualice al momento.
package billing

import (
	"context"
	"errors"
	"strings"
	"time"

	"radioclub25/internal/domain"
	"radioclub25/internal/service/audit"
	"radioclub25/internal/service/license"
)

// PaymentGateway es el punto de extensión para cobros en línea (MercadoPago,
// Culqi, Niubiz...). Hoy no hay ninguna implementación.
type PaymentGateway interface {
	// Name devuelve el nombre de la pasarela.
	Name() string
	// CreateCheckout devuelve la URL donde el cliente paga el importe indicado.
	CreateCheckout(ctx context.Context, p domain.Payment) (checkoutURL string, err error)
}

// Service implementa los casos de uso de facturación.
type Service struct {
	repo     domain.BillingRepository
	audit    *audit.Service
	loc      *time.Location
	onChange func(ctx context.Context)
}

// New crea el servicio.
func New(repo domain.BillingRepository, au *audit.Service, loc *time.Location) *Service {
	return &Service{repo: repo, audit: au, loc: loc, onChange: func(context.Context) {}}
}

// OnChange registra la función a ejecutar tras cada cambio (normalmente license.Refresh).
func (s *Service) OnChange(fn func(ctx context.Context)) { s.onChange = fn }

// ---------- Cliente ----------

// Client devuelve los datos del cliente.
func (s *Service) Client(ctx context.Context) (*domain.Client, error) { return s.repo.GetClient(ctx) }

// SaveClient guarda los datos del cliente.
func (s *Service) SaveClient(ctx context.Context, actor domain.Actor, c *domain.Client) error {
	c.BusinessName = strings.TrimSpace(c.BusinessName)
	c.TaxID = strings.TrimSpace(c.TaxID)
	if c.TaxID != "" && (len(c.TaxID) != 11 || strings.Trim(c.TaxID, "0123456789") != "") {
		return domain.Invalid("tax_id", "El RUC debe tener 11 dígitos")
	}
	if err := s.repo.SaveClient(ctx, c); err != nil {
		return err
	}
	s.audit.Log(ctx, actor, "update", "client", 1, c.BusinessName)
	return nil
}

// ---------- Planes ----------

// Plans lista los planes.
func (s *Service) Plans(ctx context.Context) ([]domain.Plan, error) { return s.repo.ListPlans(ctx) }

// Plan devuelve un plan.
func (s *Service) Plan(ctx context.Context, id int64) (*domain.Plan, error) {
	return s.repo.GetPlan(ctx, id)
}

// SavePlan crea (ID 0) o modifica un plan.
func (s *Service) SavePlan(ctx context.Context, actor domain.Actor, p *domain.Plan) error {
	p.Name = strings.TrimSpace(p.Name)
	switch {
	case p.Name == "":
		return domain.Invalid("name", "El nombre es obligatorio")
	case p.Price < 0:
		return domain.Invalid("price", "El precio no puede ser negativo")
	case p.Period != domain.PeriodMonthly && p.Period != domain.PeriodYearly:
		return domain.Invalid("period", "Periodo inválido")
	case p.MaxActiveAds < 0 || p.MaxUploadMB < 0:
		return domain.Invalid("max_active_ads", "Los límites no pueden ser negativos")
	}
	if p.Currency == "" {
		p.Currency = "PEN"
	}
	var err error
	action := "update"
	if p.ID == 0 {
		action, err = "create", s.repo.CreatePlan(ctx, p)
	} else {
		err = s.repo.UpdatePlan(ctx, p)
	}
	if err != nil {
		return err
	}
	s.audit.Log(ctx, actor, action, "plan", p.ID, "%s %s %s", p.Name, p.Currency, p.Price)
	s.onChange(ctx)
	return nil
}

// DeletePlan borra un plan que no use ningún contrato.
func (s *Service) DeletePlan(ctx context.Context, actor domain.Actor, id int64) error {
	if err := s.repo.DeletePlan(ctx, id); err != nil {
		if errors.Is(err, domain.ErrConflict) {
			return domain.Invalid("id", "No se puede borrar: hay contratos con este plan. Desactívelo en su lugar.")
		}
		return err
	}
	s.audit.Log(ctx, actor, "delete", "plan", id, "")
	return nil
}

// ---------- Contratos ----------

// Contracts lista los contratos.
func (s *Service) Contracts(ctx context.Context) ([]domain.Contract, error) {
	return s.repo.ListContracts(ctx)
}

// Contract devuelve un contrato.
func (s *Service) Contract(ctx context.Context, id int64) (*domain.Contract, error) {
	return s.repo.GetContract(ctx, id)
}

// SaveContract crea (ID 0) o modifica un contrato.
func (s *Service) SaveContract(ctx context.Context, actor domain.Actor, c *domain.Contract) error {
	if _, err := s.repo.GetPlan(ctx, c.PlanID); err != nil {
		return domain.Invalid("plan_id", "Seleccione un plan válido")
	}
	if c.StartDate.IsZero() {
		return domain.Invalid("start_date", "La fecha de inicio es obligatoria")
	}
	if c.EndDate != nil && !c.EndDate.After(c.StartDate) {
		return domain.Invalid("end_date", "La fecha de fin debe ser posterior al inicio")
	}
	if c.Amount < 0 {
		return domain.Invalid("amount", "El importe no puede ser negativo")
	}
	switch c.Status {
	case domain.ContractActive, domain.ContractExpired, domain.ContractCancelled:
	default:
		return domain.Invalid("status", "Estado inválido")
	}
	if c.Currency == "" {
		c.Currency = "PEN"
	}
	var err error
	action := "update"
	if c.ID == 0 {
		action, err = "create", s.repo.CreateContract(ctx, c)
	} else {
		err = s.repo.UpdateContract(ctx, c)
	}
	if err != nil {
		return err
	}
	s.audit.Log(ctx, actor, action, "contract", c.ID, "plan=%d inicio=%s estado=%s", c.PlanID, c.StartDate.Format(domain.DateLayout), c.Status)
	s.onChange(ctx)
	return nil
}

// DeleteContract borra un contrato y sus pagos.
func (s *Service) DeleteContract(ctx context.Context, actor domain.Actor, id int64) error {
	if err := s.repo.DeleteContract(ctx, id); err != nil {
		return err
	}
	s.audit.Log(ctx, actor, "delete", "contract", id, "")
	s.onChange(ctx)
	return nil
}

// ---------- Pagos ----------

// Payments lista los pagos de un contrato (0 = todos).
func (s *Service) Payments(ctx context.Context, contractID int64) ([]domain.Payment, error) {
	return s.repo.ListPayments(ctx, contractID)
}

// Payment devuelve un pago.
func (s *Service) Payment(ctx context.Context, id int64) (*domain.Payment, error) {
	return s.repo.GetPayment(ctx, id)
}

// SavePayment crea (ID 0) o modifica un pago.
func (s *Service) SavePayment(ctx context.Context, actor domain.Actor, p *domain.Payment) error {
	if _, err := s.repo.GetContract(ctx, p.ContractID); err != nil {
		return domain.Invalid("contract_id", "Seleccione un contrato válido")
	}
	if p.PeriodStart.IsZero() || p.PeriodEnd.IsZero() || p.PeriodEnd.Before(p.PeriodStart) {
		return domain.Invalid("period_end", "El periodo cubierto es inválido")
	}
	if p.Amount <= 0 {
		return domain.Invalid("amount", "El importe debe ser mayor que 0")
	}
	switch p.Status {
	case domain.PaymentPending, domain.PaymentVoid:
	case domain.PaymentPaid:
		if !p.Method.Valid() {
			return domain.Invalid("method", "Seleccione el método de pago")
		}
		if p.PaidAt == nil {
			now := time.Now()
			p.PaidAt = &now
		}
	default:
		return domain.Invalid("status", "Estado inválido")
	}
	if p.Currency == "" {
		p.Currency = "PEN"
	}
	var err error
	action := "update"
	if p.ID == 0 {
		action, err = "create", s.repo.CreatePayment(ctx, p)
	} else {
		err = s.repo.UpdatePayment(ctx, p)
	}
	if err != nil {
		return err
	}
	s.audit.Log(ctx, actor, action, "payment", p.ID, "contrato=%d %s %s %s→%s %s", p.ContractID, p.Currency, p.Amount,
		p.PeriodStart.Format(domain.DateLayout), p.PeriodEnd.Format(domain.DateLayout), p.Status)
	s.onChange(ctx)
	return nil
}

// DeletePayment borra un pago.
func (s *Service) DeletePayment(ctx context.Context, actor domain.Actor, id int64) error {
	if err := s.repo.DeletePayment(ctx, id); err != nil {
		return err
	}
	s.audit.Log(ctx, actor, "delete", "payment", id, "")
	s.onChange(ctx)
	return nil
}

// PaidUntil devuelve el último día cubierto por pagos del contrato (nil si no hay pagos).
func (s *Service) PaidUntil(ctx context.Context, contractID int64) (*time.Time, error) {
	return license.PaidUntil(ctx, s.repo, contractID)
}

// NextPeriod propone el siguiente periodo a cobrar de un contrato: empieza el
// día siguiente al último pagado (o al inicio del contrato) y dura un mes o
// un año según el plan. Sirve para rellenar el formulario de "nuevo pago".
func (s *Service) NextPeriod(ctx context.Context, c *domain.Contract) (start, end time.Time, err error) {
	paid, err := license.PaidUntil(ctx, s.repo, c.ID)
	if err != nil {
		return
	}
	start = c.StartDate
	if paid != nil {
		start = paid.AddDate(0, 0, 1)
	}
	months := 1
	if plan, perr := s.repo.GetPlan(ctx, c.PlanID); perr == nil && plan.Period == domain.PeriodYearly {
		months = 12
	}
	end = start.AddDate(0, months, -1)
	return start, end, nil
}

// Summary es el resumen comercial para el panel superadmin.
type Summary struct {
	Contract     *domain.Contract
	Plan         *domain.Plan
	PaidUntil    *time.Time
	PendingTotal domain.Money
	PendingCount int
	LastPayment  *domain.Payment
	DaysToDue    int  // días hasta el fin de lo pagado (negativo = vencido)
	HasDueDate   bool // false si no hay contrato
}

// Summary calcula el resumen del contrato vigente.
func (s *Service) Summary(ctx context.Context) (Summary, error) {
	var sum Summary
	c, err := s.repo.CurrentContract(ctx)
	if errors.Is(err, domain.ErrNotFound) {
		return sum, nil
	}
	if err != nil {
		return sum, err
	}
	sum.Contract = c
	sum.Plan, _ = s.repo.GetPlan(ctx, c.PlanID)
	pays, err := s.repo.ListPayments(ctx, c.ID)
	if err != nil {
		return sum, err
	}
	for i := range pays {
		p := pays[i]
		switch p.Status {
		case domain.PaymentPending:
			sum.PendingTotal += p.Amount
			sum.PendingCount++
		case domain.PaymentPaid:
			if sum.PaidUntil == nil || p.PeriodEnd.After(*sum.PaidUntil) {
				t := p.PeriodEnd
				sum.PaidUntil = &t
			}
			if sum.LastPayment == nil || (p.PaidAt != nil && sum.LastPayment.PaidAt != nil && p.PaidAt.After(*sum.LastPayment.PaidAt)) {
				sum.LastPayment = &p
			}
		}
	}
	due := c.StartDate.AddDate(0, 0, -1)
	if sum.PaidUntil != nil {
		due = *sum.PaidUntil
	}
	y, m, d := time.Now().In(s.loc).Date()
	today := time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
	sum.DaysToDue = int(due.Sub(today).Hours() / 24)
	sum.HasDueDate = true
	return sum, nil
}
