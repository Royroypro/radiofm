package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"radioclub25/internal/domain"
)

// BillingRepo implementa domain.BillingRepository (cliente, planes, contratos y pagos).
type BillingRepo struct{ db *sql.DB }

// NewBillingRepo crea el repositorio de facturación.
func NewBillingRepo(db *sql.DB) *BillingRepo { return &BillingRepo{db: db} }

// ---------- Cliente ----------

// GetClient devuelve los datos del cliente único.
func (r *BillingRepo) GetClient(ctx context.Context) (*domain.Client, error) {
	var c domain.Client
	err := r.db.QueryRowContext(ctx, `SELECT business_name, tax_id, contact_name, phone, email, address, notes FROM client WHERE id=1`).
		Scan(&c.BusinessName, &c.TaxID, &c.ContactName, &c.Phone, &c.Email, &c.Address, &c.Notes)
	return &c, notFound(err)
}

// SaveClient actualiza los datos del cliente.
func (r *BillingRepo) SaveClient(ctx context.Context, c *domain.Client) error {
	_, err := r.db.ExecContext(ctx, `UPDATE client SET business_name=?, tax_id=?, contact_name=?, phone=?, email=?, address=?, notes=? WHERE id=1`,
		c.BusinessName, c.TaxID, c.ContactName, c.Phone, c.Email, c.Address, c.Notes)
	return err
}

// ---------- Planes ----------

const planCols = `id, name, price_cents, currency, period, max_active_ads, max_upload_mb, tv_enabled, stats_enabled, active`

func scanPlan(r rowScanner) (*domain.Plan, error) {
	var p domain.Plan
	var price int64
	var period string
	var tv, stats, active int
	if err := r.Scan(&p.ID, &p.Name, &price, &p.Currency, &period, &p.MaxActiveAds, &p.MaxUploadMB, &tv, &stats, &active); err != nil {
		return nil, err
	}
	p.Price, p.Period = domain.Money(price), domain.Period(period)
	p.TVEnabled, p.StatsEnabled, p.Active = tv == 1, stats == 1, active == 1
	return &p, nil
}

// CreatePlan inserta un plan.
func (r *BillingRepo) CreatePlan(ctx context.Context, p *domain.Plan) error {
	res, err := r.db.ExecContext(ctx, `INSERT INTO plans (name, price_cents, currency, period, max_active_ads, max_upload_mb,
		tv_enabled, stats_enabled, active) VALUES (?,?,?,?,?,?,?,?,?)`,
		p.Name, int64(p.Price), p.Currency, string(p.Period), p.MaxActiveAds, p.MaxUploadMB,
		boolInt(p.TVEnabled), boolInt(p.StatsEnabled), boolInt(p.Active))
	if err != nil {
		return fmt.Errorf("plans: create: %w", err)
	}
	p.ID, _ = res.LastInsertId()
	return nil
}

// GetPlan busca un plan por ID.
func (r *BillingRepo) GetPlan(ctx context.Context, id int64) (*domain.Plan, error) {
	p, err := scanPlan(r.db.QueryRowContext(ctx, `SELECT `+planCols+` FROM plans WHERE id=?`, id))
	return p, notFound(err)
}

// ListPlans devuelve todos los planes.
func (r *BillingRepo) ListPlans(ctx context.Context) ([]domain.Plan, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT `+planCols+` FROM plans ORDER BY active DESC, price_cents`)
	if err != nil {
		return nil, fmt.Errorf("plans: list: %w", err)
	}
	defer rows.Close()
	var out []domain.Plan
	for rows.Next() {
		p, err := scanPlan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *p)
	}
	return out, rows.Err()
}

// UpdatePlan modifica un plan.
func (r *BillingRepo) UpdatePlan(ctx context.Context, p *domain.Plan) error {
	return checkAffected(r.db.ExecContext(ctx, `UPDATE plans SET name=?, price_cents=?, currency=?, period=?, max_active_ads=?,
		max_upload_mb=?, tv_enabled=?, stats_enabled=?, active=? WHERE id=?`,
		p.Name, int64(p.Price), p.Currency, string(p.Period), p.MaxActiveAds, p.MaxUploadMB,
		boolInt(p.TVEnabled), boolInt(p.StatsEnabled), boolInt(p.Active), p.ID))
}

// DeletePlan borra un plan. Falla si algún contrato lo usa (ON DELETE RESTRICT).
func (r *BillingRepo) DeletePlan(ctx context.Context, id int64) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM plans WHERE id=?`, id)
	if err != nil {
		return domain.ErrConflict
	}
	return checkAffected(res, nil)
}

// ---------- Contratos ----------

const contractSelect = `SELECT c.id, c.plan_id, COALESCE(p.name, ''), c.start_date, c.end_date, c.amount_cents, c.currency,
	c.auto_renew, c.status, c.document_url, c.notes, c.created_at
	FROM contracts c LEFT JOIN plans p ON p.id = c.plan_id`

func scanContract(r rowScanner) (*domain.Contract, error) {
	var c domain.Contract
	var start, status, created string
	var end sql.NullString
	var amount int64
	var renew int
	if err := r.Scan(&c.ID, &c.PlanID, &c.PlanName, &start, &end, &amount, &c.Currency, &renew, &status,
		&c.DocumentURL, &c.Notes, &created); err != nil {
		return nil, err
	}
	c.StartDate, c.EndDate = parseDate(start), parseDatePtr(end)
	c.Amount, c.AutoRenew, c.Status, c.CreatedAt = domain.Money(amount), renew == 1, domain.ContractStatus(status), parseTime(created)
	return &c, nil
}

// CreateContract inserta un contrato.
func (r *BillingRepo) CreateContract(ctx context.Context, c *domain.Contract) error {
	c.CreatedAt = time.Now()
	res, err := r.db.ExecContext(ctx, `INSERT INTO contracts (plan_id, start_date, end_date, amount_cents, currency, auto_renew,
		status, document_url, notes, created_at) VALUES (?,?,?,?,?,?,?,?,?,?)`,
		c.PlanID, formatDate(c.StartDate), formatDatePtr(c.EndDate), int64(c.Amount), c.Currency, boolInt(c.AutoRenew),
		string(c.Status), c.DocumentURL, c.Notes, formatTime(c.CreatedAt))
	if err != nil {
		return fmt.Errorf("contracts: create: %w", err)
	}
	c.ID, _ = res.LastInsertId()
	return nil
}

// GetContract busca un contrato por ID.
func (r *BillingRepo) GetContract(ctx context.Context, id int64) (*domain.Contract, error) {
	c, err := scanContract(r.db.QueryRowContext(ctx, contractSelect+` WHERE c.id=?`, id))
	return c, notFound(err)
}

// ListContracts devuelve todos los contratos, los más recientes primero.
func (r *BillingRepo) ListContracts(ctx context.Context) ([]domain.Contract, error) {
	rows, err := r.db.QueryContext(ctx, contractSelect+` ORDER BY c.start_date DESC, c.id DESC`)
	if err != nil {
		return nil, fmt.Errorf("contracts: list: %w", err)
	}
	defer rows.Close()
	var out []domain.Contract
	for rows.Next() {
		c, err := scanContract(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *c)
	}
	return out, rows.Err()
}

// UpdateContract modifica un contrato.
func (r *BillingRepo) UpdateContract(ctx context.Context, c *domain.Contract) error {
	return checkAffected(r.db.ExecContext(ctx, `UPDATE contracts SET plan_id=?, start_date=?, end_date=?, amount_cents=?,
		currency=?, auto_renew=?, status=?, document_url=?, notes=? WHERE id=?`,
		c.PlanID, formatDate(c.StartDate), formatDatePtr(c.EndDate), int64(c.Amount), c.Currency, boolInt(c.AutoRenew),
		string(c.Status), c.DocumentURL, c.Notes, c.ID))
}

// DeleteContract borra un contrato y sus pagos.
func (r *BillingRepo) DeleteContract(ctx context.Context, id int64) error {
	return checkAffected(r.db.ExecContext(ctx, `DELETE FROM contracts WHERE id=?`, id))
}

// CurrentContract devuelve el contrato vigente con la fecha de inicio más reciente.
func (r *BillingRepo) CurrentContract(ctx context.Context) (*domain.Contract, error) {
	c, err := scanContract(r.db.QueryRowContext(ctx, contractSelect+` WHERE c.status='active' ORDER BY c.start_date DESC, c.id DESC LIMIT 1`))
	return c, notFound(err)
}

// ---------- Pagos ----------

const paymentCols = `id, contract_id, period_start, period_end, amount_cents, currency, status, method, reference, paid_at,
	receipt_url, notes, created_at`

func scanPayment(r rowScanner) (*domain.Payment, error) {
	var p domain.Payment
	var start, end, status, method, created string
	var paid sql.NullString
	var amount int64
	if err := r.Scan(&p.ID, &p.ContractID, &start, &end, &amount, &p.Currency, &status, &method, &p.Reference, &paid,
		&p.ReceiptURL, &p.Notes, &created); err != nil {
		return nil, err
	}
	p.PeriodStart, p.PeriodEnd, p.Amount = parseDate(start), parseDate(end), domain.Money(amount)
	p.Status, p.Method, p.PaidAt, p.CreatedAt = domain.PaymentStatus(status), domain.PaymentMethod(method), parseTimePtr(paid), parseTime(created)
	return &p, nil
}

// CreatePayment inserta un pago.
func (r *BillingRepo) CreatePayment(ctx context.Context, p *domain.Payment) error {
	p.CreatedAt = time.Now()
	res, err := r.db.ExecContext(ctx, `INSERT INTO payments (contract_id, period_start, period_end, amount_cents, currency,
		status, method, reference, paid_at, receipt_url, notes, created_at) VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`,
		p.ContractID, formatDate(p.PeriodStart), formatDate(p.PeriodEnd), int64(p.Amount), p.Currency, string(p.Status),
		string(p.Method), p.Reference, formatTimePtr(p.PaidAt), p.ReceiptURL, p.Notes, formatTime(p.CreatedAt))
	if err != nil {
		return fmt.Errorf("payments: create: %w", err)
	}
	p.ID, _ = res.LastInsertId()
	return nil
}

// GetPayment busca un pago por ID.
func (r *BillingRepo) GetPayment(ctx context.Context, id int64) (*domain.Payment, error) {
	p, err := scanPayment(r.db.QueryRowContext(ctx, `SELECT `+paymentCols+` FROM payments WHERE id=?`, id))
	return p, notFound(err)
}

// ListPayments devuelve los pagos de un contrato (o todos si contractID es 0), por periodo descendente.
func (r *BillingRepo) ListPayments(ctx context.Context, contractID int64) ([]domain.Payment, error) {
	q, args := `SELECT `+paymentCols+` FROM payments`, []any{}
	if contractID > 0 {
		q, args = q+` WHERE contract_id=?`, append(args, contractID)
	}
	rows, err := r.db.QueryContext(ctx, q+` ORDER BY period_end DESC, id DESC`, args...)
	if err != nil {
		return nil, fmt.Errorf("payments: list: %w", err)
	}
	defer rows.Close()
	var out []domain.Payment
	for rows.Next() {
		p, err := scanPayment(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *p)
	}
	return out, rows.Err()
}

// UpdatePayment modifica un pago.
func (r *BillingRepo) UpdatePayment(ctx context.Context, p *domain.Payment) error {
	return checkAffected(r.db.ExecContext(ctx, `UPDATE payments SET contract_id=?, period_start=?, period_end=?, amount_cents=?,
		currency=?, status=?, method=?, reference=?, paid_at=?, receipt_url=?, notes=? WHERE id=?`,
		p.ContractID, formatDate(p.PeriodStart), formatDate(p.PeriodEnd), int64(p.Amount), p.Currency, string(p.Status),
		string(p.Method), p.Reference, formatTimePtr(p.PaidAt), p.ReceiptURL, p.Notes, p.ID))
}

// DeletePayment borra un pago.
func (r *BillingRepo) DeletePayment(ctx context.Context, id int64) error {
	return checkAffected(r.db.ExecContext(ctx, `DELETE FROM payments WHERE id=?`, id))
}
