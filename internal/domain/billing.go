package domain

import (
	"context"
	"fmt"
	"time"
)

// DateLayout es el formato de las fechas sin hora (contratos y periodos de pago).
const DateLayout = "2006-01-02"

// Money es un importe en céntimos para evitar errores de redondeo.
type Money int64

// String formatea el importe como "1,234.50".
func (m Money) String() string {
	neg := m < 0
	if neg {
		m = -m
	}
	units, cents := int64(m)/100, int64(m)%100
	s := fmt.Sprintf("%d", units)
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	if neg {
		s = "-" + s
	}
	return fmt.Sprintf("%s.%02d", s, cents)
}

// CurrencySymbol devuelve el símbolo de una moneda ISO 4217.
func CurrencySymbol(code string) string {
	switch code {
	case "PEN":
		return "S/"
	case "USD":
		return "US$"
	}
	return code
}

// Client son los datos del cliente (la radio) que contrata el servicio.
// Hay un único cliente por instalación.
type Client struct {
	BusinessName string // razón social
	TaxID        string // RUC
	ContactName  string
	Phone        string
	Email        string
	Address      string
	Notes        string
}

// Period es la periodicidad de cobro de un plan.
type Period string

const (
	PeriodMonthly Period = "monthly"
	PeriodYearly  Period = "yearly"
)

// Label devuelve el nombre del periodo para la interfaz.
func (p Period) Label() string {
	if p == PeriodYearly {
		return "Anual"
	}
	return "Mensual"
}

// Plan es una membresía con su precio y límites.
// Un límite en 0 significa "sin límite".
type Plan struct {
	ID           int64
	Name         string
	Price        Money
	Currency     string
	Period       Period
	MaxActiveAds int
	MaxUploadMB  int
	TVEnabled    bool
	StatsEnabled bool
	Active       bool
}

// ContractStatus es el estado administrativo de un contrato.
type ContractStatus string

const (
	ContractActive    ContractStatus = "active"
	ContractExpired   ContractStatus = "expired"
	ContractCancelled ContractStatus = "cancelled"
)

// Label devuelve el nombre del estado para la interfaz.
func (s ContractStatus) Label() string {
	switch s {
	case ContractActive:
		return "Vigente"
	case ContractExpired:
		return "Vencido"
	case ContractCancelled:
		return "Cancelado"
	}
	return string(s)
}

// Contract es el contrato del cliente con un plan.
type Contract struct {
	ID          int64
	PlanID      int64
	PlanName    string // solo lectura, se rellena al listar
	StartDate   time.Time
	EndDate     *time.Time // nil = indefinido
	Amount      Money      // importe por periodo
	Currency    string
	AutoRenew   bool
	Status      ContractStatus
	DocumentURL string
	Notes       string
	CreatedAt   time.Time
}

// PaymentMethod es la forma en que se recibió un pago.
type PaymentMethod string

// PaymentMethods lista los métodos aceptados con su etiqueta.
var PaymentMethods = []struct {
	Value PaymentMethod
	Label string
}{
	{"cash", "Efectivo"},
	{"transfer", "Transferencia"},
	{"yape", "Yape"},
	{"plin", "Plin"},
	{"deposit", "Depósito"},
	{"card", "Tarjeta"},
	{"other", "Otro"},
}

// Label devuelve el nombre del método para la interfaz.
func (m PaymentMethod) Label() string {
	for _, x := range PaymentMethods {
		if x.Value == m {
			return x.Label
		}
	}
	return string(m)
}

// Valid indica si el método es conocido.
func (m PaymentMethod) Valid() bool { return m.Label() != string(m) }

// PaymentStatus es el estado de un pago.
type PaymentStatus string

const (
	PaymentPending PaymentStatus = "pending"
	PaymentPaid    PaymentStatus = "paid"
	PaymentVoid    PaymentStatus = "void"
)

// Label devuelve el nombre del estado para la interfaz.
func (s PaymentStatus) Label() string {
	switch s {
	case PaymentPending:
		return "Pendiente"
	case PaymentPaid:
		return "Pagado"
	case PaymentVoid:
		return "Anulado"
	}
	return string(s)
}

// Payment es un pago (o una cuota pendiente) de un periodo de un contrato.
// PeriodEnd es el último día cubierto por el pago (inclusive).
type Payment struct {
	ID          int64
	ContractID  int64
	PeriodStart time.Time
	PeriodEnd   time.Time
	Amount      Money
	Currency    string
	Status      PaymentStatus
	Method      PaymentMethod
	Reference   string // nº de operación, voucher, etc.
	PaidAt      *time.Time
	ReceiptURL  string
	Notes       string
	CreatedAt   time.Time
}

// BillingRepository persiste cliente, planes, contratos y pagos.
type BillingRepository interface {
	GetClient(ctx context.Context) (*Client, error)
	SaveClient(ctx context.Context, c *Client) error

	CreatePlan(ctx context.Context, p *Plan) error
	GetPlan(ctx context.Context, id int64) (*Plan, error)
	ListPlans(ctx context.Context) ([]Plan, error)
	UpdatePlan(ctx context.Context, p *Plan) error
	DeletePlan(ctx context.Context, id int64) error

	CreateContract(ctx context.Context, c *Contract) error
	GetContract(ctx context.Context, id int64) (*Contract, error)
	ListContracts(ctx context.Context) ([]Contract, error)
	UpdateContract(ctx context.Context, c *Contract) error
	DeleteContract(ctx context.Context, id int64) error
	// CurrentContract devuelve el contrato vigente más reciente o ErrNotFound.
	CurrentContract(ctx context.Context) (*Contract, error)

	CreatePayment(ctx context.Context, p *Payment) error
	GetPayment(ctx context.Context, id int64) (*Payment, error)
	ListPayments(ctx context.Context, contractID int64) ([]Payment, error) // contractID 0 = todos
	UpdatePayment(ctx context.Context, p *Payment) error
	DeletePayment(ctx context.Context, id int64) error
}
