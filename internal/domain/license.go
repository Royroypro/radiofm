package domain

import (
	"context"
	"time"
)

// LicenseLevel es el estado de servicio del cliente. Los niveles están
// ordenados de menos a más restrictivo.
type LicenseLevel string

const (
	// LicenseActive: todo funciona.
	LicenseActive LicenseLevel = "active"
	// LicenseOverdue: todo funciona, pero el panel admin muestra aviso de mora.
	LicenseOverdue LicenseLevel = "overdue"
	// LicensePanelOnly: la web pública sigue; el panel admin queda en solo lectura.
	LicensePanelOnly LicenseLevel = "panel_only"
	// LicenseNoAds: como PanelOnly y además no se sirve publicidad.
	LicenseNoAds LicenseLevel = "no_ads"
	// LicenseFull: la web pública muestra "servicio suspendido" y se cortan los streams.
	LicenseFull LicenseLevel = "full"
)

// BlockLevels son los niveles que se pueden elegir como bloqueo.
var BlockLevels = []LicenseLevel{LicensePanelOnly, LicenseNoAds, LicenseFull}

func (l LicenseLevel) rank() int {
	switch l {
	case LicenseActive:
		return 0
	case LicenseOverdue:
		return 1
	case LicensePanelOnly:
		return 2
	case LicenseNoAds:
		return 3
	case LicenseFull:
		return 4
	}
	return 0
}

// Valid indica si el nivel es conocido.
func (l LicenseLevel) Valid() bool {
	switch l {
	case LicenseActive, LicenseOverdue, LicensePanelOnly, LicenseNoAds, LicenseFull:
		return true
	}
	return false
}

// IsBlock indica si el nivel es un bloqueo (no solo un aviso).
func (l LicenseLevel) IsBlock() bool { return l.rank() >= LicensePanelOnly.rank() }

// AdminReadOnly indica si el panel admin debe impedir cambios.
func (l LicenseLevel) AdminReadOnly() bool { return l.rank() >= LicensePanelOnly.rank() }

// AdsDisabled indica si no se debe servir publicidad.
func (l LicenseLevel) AdsDisabled() bool { return l.rank() >= LicenseNoAds.rank() }

// PublicBlocked indica si la web pública y los streams deben cortarse.
func (l LicenseLevel) PublicBlocked() bool { return l == LicenseFull }

// Label devuelve el nombre del nivel para la interfaz.
func (l LicenseLevel) Label() string {
	switch l {
	case LicenseActive:
		return "Activo"
	case LicenseOverdue:
		return "En mora (periodo de gracia)"
	case LicensePanelOnly:
		return "Bloqueo: panel en solo lectura"
	case LicenseNoAds:
		return "Bloqueo: panel y publicidad"
	case LicenseFull:
		return "Bloqueo total"
	}
	return string(l)
}

// LicensePolicy es la configuración de bloqueo que decide el superadmin.
type LicensePolicy struct {
	GraceDays        int          // días de mora antes del bloqueo automático
	AutoBlockLevel   LicenseLevel // nivel aplicado al terminar la gracia
	ManualLevel      LicenseLevel // "" = sin bloqueo manual; si no, se fuerza este nivel
	ManualReason     string
	SuspendedMessage string // mensaje público en bloqueo total
}

// DefaultLicensePolicy devuelve la política inicial.
func DefaultLicensePolicy() LicensePolicy {
	return LicensePolicy{
		GraceDays:        5,
		AutoBlockLevel:   LicensePanelOnly,
		SuspendedMessage: "Servicio temporalmente suspendido. Volveremos pronto.",
	}
}

// LicenseState es el resultado de evaluar la licencia en un instante.
type LicenseState struct {
	Level       LicenseLevel
	Reason      string     // explicación legible del nivel
	PaidUntil   *time.Time // último día cubierto por pagos (nil = sin pagos)
	GraceUntil  *time.Time // último día de gracia
	Enforced    bool       // false si no hay contrato vigente: no se controla
	Manual      bool       // true si el nivel viene de un bloqueo manual
	EvaluatedAt time.Time
	Limits      PlanLimits
}

// PlanLimits son los límites efectivos según el plan del contrato vigente.
// Un valor 0 significa sin límite.
type PlanLimits struct {
	PlanName     string
	MaxActiveAds int
	MaxUploadMB  int
	TVEnabled    bool
	StatsEnabled bool
}

// UnlimitedLimits son los límites cuando no hay contrato que aplicar.
func UnlimitedLimits() PlanLimits {
	return PlanLimits{PlanName: "Sin plan", TVEnabled: true, StatsEnabled: true}
}

// EvaluateLicense calcula el nivel de licencia. Es una función pura para poder
// probarla sin base de datos.
//
//   - Si hay bloqueo manual, manda el bloqueo manual.
//   - Si no hay contrato vigente, no se controla (activo).
//   - Si hoy <= PaidUntil: activo.
//   - Si hoy <= PaidUntil + GraceDays: en mora.
//   - Si no: se aplica AutoBlockLevel.
//
// Sin ningún pago, PaidUntil se toma como el día anterior al inicio del contrato.
func EvaluateLicense(now time.Time, pol LicensePolicy, contract *Contract, paidUntil *time.Time, limits PlanLimits) LicenseState {
	st := LicenseState{Level: LicenseActive, EvaluatedAt: now, PaidUntil: paidUntil, Limits: limits}
	today := truncateDay(now)

	if pol.ManualLevel != "" && pol.ManualLevel != LicenseActive {
		st.Level, st.Manual = pol.ManualLevel, true
		st.Reason = "Bloqueo manual: " + pol.ManualReason
		st.Enforced = contract != nil
		return st
	}
	if contract == nil {
		st.Reason = "Sin contrato vigente: el servicio no está controlado"
		st.Limits = UnlimitedLimits()
		return st
	}
	st.Enforced = true

	covered := truncateDay(contract.StartDate).AddDate(0, 0, -1)
	if paidUntil != nil {
		covered = truncateDay(*paidUntil)
	}
	grace := covered.AddDate(0, 0, pol.GraceDays)
	st.GraceUntil = &grace

	switch {
	case !today.After(covered):
		st.Reason = "Pagos al día"
	case !today.After(grace):
		st.Level = LicenseOverdue
		st.Reason = "Pago vencido, en periodo de gracia hasta el " + grace.Format("02/01/2006")
	default:
		st.Level = pol.AutoBlockLevel
		if !st.Level.IsBlock() {
			st.Level = LicensePanelOnly
		}
		st.Reason = "Bloqueo automático por falta de pago (gracia vencida el " + grace.Format("02/01/2006") + ")"
	}
	return st
}

// truncateDay devuelve la fecha de calendario de t (en su propia zona) como
// medianoche UTC, para comparar fechas sin que influya la zona horaria.
func truncateDay(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

// LicenseRepository persiste la política y el último nivel aplicado.
type LicenseRepository interface {
	GetPolicy(ctx context.Context) (LicensePolicy, error)
	SavePolicy(ctx context.Context, p LicensePolicy) error
	// LastLevel devuelve el último nivel aplicado (para detectar cambios entre evaluaciones).
	LastLevel(ctx context.Context) (LicenseLevel, error)
	SaveLastLevel(ctx context.Context, l LicenseLevel) error
}
