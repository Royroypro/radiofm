// Package superadmin contiene los handlers del panel del proveedor
// (/superadmin): cliente, planes, contratos, pagos, licencia y auditoría.
// Todas sus rutas exigen el rol superadmin (ver router.go).
package superadmin

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"radioclub25/internal/domain"
	"radioclub25/internal/platform/config"
	"radioclub25/internal/platform/storage"
	"radioclub25/internal/service/audit"
	"radioclub25/internal/service/billing"
	"radioclub25/internal/service/license"
	"radioclub25/internal/service/media"
	"radioclub25/internal/transport/httpserver/admin"
	"radioclub25/internal/transport/httpserver/form"
	"radioclub25/internal/transport/httpserver/middleware"
	"radioclub25/internal/transport/httpserver/views"
)

// Handler agrupa las dependencias del panel superadmin.
type Handler struct {
	Views   *views.Renderer
	Billing *billing.Service
	License *license.Service
	Audit   *audit.Service
	Media   *media.Service
	Store   *storage.Store
	Cfg     config.Config
	Log     *slog.Logger
}

// Dashboard muestra el estado comercial y de la licencia: GET /superadmin
func (h *Handler) Dashboard(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sum, err := h.Billing.Summary(ctx)
	if err != nil {
		h.Views.Error(w, r, err)
		return
	}
	client, _ := h.Billing.Client(ctx)
	recent, _ := h.Audit.List(ctx, 8, 0)
	h.Views.Render(w, r, http.StatusOK, "superadmin/dashboard", "Superadmin", map[string]any{
		"Summary": sum, "Client": client, "Recent": recent, "Status": h.Media.Status(ctx),
	})
}

// formError: igual que en admin, vuelve a dibujar el formulario con el error.
func (h *Handler) formError(w http.ResponseWriter, r *http.Request, page, title string, data any, err error) {
	p := &views.Page{Title: title, Data: data}
	if ve := views.AsValidation(err); ve != nil {
		p.Error, p.FieldErrors = ve.Msg, map[string]string{ve.Field: ve.Msg}
		h.Views.RenderPage(w, r, http.StatusUnprocessableEntity, page, p)
		return
	}
	h.Views.Error(w, r, err)
}

// ---------- Cliente ----------

// ClientForm muestra los datos del cliente: GET /superadmin/client
func (h *Handler) ClientForm(w http.ResponseWriter, r *http.Request) {
	c, err := h.Billing.Client(r.Context())
	if err != nil {
		h.Views.Error(w, r, err)
		return
	}
	h.Views.Render(w, r, http.StatusOK, "superadmin/client", "Cliente", c)
}

// ClientSave guarda los datos del cliente: POST /superadmin/client
func (h *Handler) ClientSave(w http.ResponseWriter, r *http.Request) {
	c := &domain.Client{
		BusinessName: form.String(r, "business_name"), TaxID: form.String(r, "tax_id"),
		ContactName: form.String(r, "contact_name"), Phone: form.String(r, "phone"), Email: form.String(r, "email"),
		Address: form.String(r, "address"), Notes: form.String(r, "notes"),
	}
	if err := h.Billing.SaveClient(r.Context(), middleware.ActorFrom(r), c); err != nil {
		h.formError(w, r, "superadmin/client", "Cliente", c, err)
		return
	}
	views.Redirect(w, r, "/superadmin/client", "ok", "Datos del cliente guardados.")
}

// ---------- Planes ----------

// Plans lista los planes: GET /superadmin/plans
func (h *Handler) Plans(w http.ResponseWriter, r *http.Request) {
	list, err := h.Billing.Plans(r.Context())
	if err != nil {
		h.Views.Error(w, r, err)
		return
	}
	h.Views.Render(w, r, http.StatusOK, "superadmin/plans", "Planes", list)
}

// PlanForm muestra el formulario de plan: GET /superadmin/plans/new y /superadmin/plans/{id}
func (h *Handler) PlanForm(w http.ResponseWriter, r *http.Request) {
	p := &domain.Plan{Currency: "PEN", Period: domain.PeriodMonthly, TVEnabled: true, StatsEnabled: true, Active: true}
	if id := form.ID(chi.URLParam(r, "id")); id > 0 {
		var err error
		if p, err = h.Billing.Plan(r.Context(), id); err != nil {
			h.Views.Error(w, r, err)
			return
		}
	}
	h.Views.Render(w, r, http.StatusOK, "superadmin/plan_form", "Plan", p)
}

// PlanSave guarda un plan: POST /superadmin/plans y /superadmin/plans/{id}
func (h *Handler) PlanSave(w http.ResponseWriter, r *http.Request) {
	p := &domain.Plan{
		ID: form.ID(chi.URLParam(r, "id")), Name: form.String(r, "name"), Price: form.Money(r, "price"),
		Currency: form.String(r, "currency"), Period: domain.Period(form.String(r, "period")),
		MaxActiveAds: form.Int(r, "max_active_ads"), MaxUploadMB: form.Int(r, "max_upload_mb"),
		TVEnabled: form.Bool(r, "tv_enabled"), StatsEnabled: form.Bool(r, "stats_enabled"), Active: form.Bool(r, "active"),
	}
	if err := h.Billing.SavePlan(r.Context(), middleware.ActorFrom(r), p); err != nil {
		h.formError(w, r, "superadmin/plan_form", "Plan", p, err)
		return
	}
	views.Redirect(w, r, "/superadmin/plans", "ok", "Plan guardado.")
}

// PlanDelete borra un plan: POST /superadmin/plans/{id}/delete
func (h *Handler) PlanDelete(w http.ResponseWriter, r *http.Request) {
	err := h.Billing.DeletePlan(r.Context(), middleware.ActorFrom(r), form.ID(chi.URLParam(r, "id")))
	h.redirectResult(w, r, "/superadmin/plans", "Plan eliminado.", err)
}

// ---------- Contratos ----------

// Contracts lista los contratos: GET /superadmin/contracts
func (h *Handler) Contracts(w http.ResponseWriter, r *http.Request) {
	list, err := h.Billing.Contracts(r.Context())
	if err != nil {
		h.Views.Error(w, r, err)
		return
	}
	h.Views.Render(w, r, http.StatusOK, "superadmin/contracts", "Contratos", list)
}

// Contract muestra un contrato con sus pagos: GET /superadmin/contracts/{id}
func (h *Handler) Contract(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	c, err := h.Billing.Contract(ctx, form.ID(chi.URLParam(r, "id")))
	if err != nil {
		h.Views.Error(w, r, err)
		return
	}
	pays, err := h.Billing.Payments(ctx, c.ID)
	if err != nil {
		h.Views.Error(w, r, err)
		return
	}
	paidUntil, _ := h.Billing.PaidUntil(ctx, c.ID)
	h.Views.Render(w, r, http.StatusOK, "superadmin/contract", "Contrato #"+strconv.FormatInt(c.ID, 10), map[string]any{
		"Contract": c, "Payments": pays, "PaidUntil": paidUntil,
	})
}

func (h *Handler) contractFormData(r *http.Request, c *domain.Contract) map[string]any {
	plans, _ := h.Billing.Plans(r.Context())
	return map[string]any{"Contract": c, "Plans": plans}
}

// ContractForm muestra el formulario de contrato: GET /superadmin/contracts/new y /superadmin/contracts/{id}/edit
func (h *Handler) ContractForm(w http.ResponseWriter, r *http.Request) {
	c := &domain.Contract{Currency: "PEN", AutoRenew: true, Status: domain.ContractActive}
	if id := form.ID(chi.URLParam(r, "id")); id > 0 {
		var err error
		if c, err = h.Billing.Contract(r.Context(), id); err != nil {
			h.Views.Error(w, r, err)
			return
		}
	}
	h.Views.Render(w, r, http.StatusOK, "superadmin/contract_form", "Contrato", h.contractFormData(r, c))
}

// ContractSave guarda un contrato: POST /superadmin/contracts y /superadmin/contracts/{id}/edit
func (h *Handler) ContractSave(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	c := &domain.Contract{}
	if id := form.ID(chi.URLParam(r, "id")); id > 0 {
		var err error
		if c, err = h.Billing.Contract(ctx, id); err != nil {
			h.Views.Error(w, r, err)
			return
		}
	}
	c.PlanID = form.Int64(r, "plan_id")
	c.StartDate = form.Date(r, "start_date")
	c.EndDate = form.DatePtr(r, "end_date")
	c.Amount = form.Money(r, "amount")
	c.Currency = form.String(r, "currency")
	c.AutoRenew = form.Bool(r, "auto_renew")
	c.Status = domain.ContractStatus(form.String(r, "status"))
	c.Notes = form.String(r, "notes")
	url, err := admin.Upload(r, "document", storage.KindDocument, "billing", h.Store, h.Cfg.MaxUploadMB)
	if err != nil {
		h.formError(w, r, "superadmin/contract_form", "Contrato", h.contractFormData(r, c), err)
		return
	}
	if url != "" {
		_ = h.Store.Delete(c.DocumentURL)
		c.DocumentURL = url
	}
	if err := h.Billing.SaveContract(ctx, middleware.ActorFrom(r), c); err != nil {
		h.formError(w, r, "superadmin/contract_form", "Contrato", h.contractFormData(r, c), err)
		return
	}
	views.Redirect(w, r, "/superadmin/contracts/"+strconv.FormatInt(c.ID, 10), "ok", "Contrato guardado.")
}

// ContractDelete borra un contrato y sus pagos: POST /superadmin/contracts/{id}/delete
func (h *Handler) ContractDelete(w http.ResponseWriter, r *http.Request) {
	err := h.Billing.DeleteContract(r.Context(), middleware.ActorFrom(r), form.ID(chi.URLParam(r, "id")))
	h.redirectResult(w, r, "/superadmin/contracts", "Contrato eliminado.", err)
}

// ---------- Pagos ----------

// Payments lista todos los pagos: GET /superadmin/payments
func (h *Handler) Payments(w http.ResponseWriter, r *http.Request) {
	list, err := h.Billing.Payments(r.Context(), 0)
	if err != nil {
		h.Views.Error(w, r, err)
		return
	}
	h.Views.Render(w, r, http.StatusOK, "superadmin/payments", "Pagos", list)
}

func (h *Handler) paymentFormData(r *http.Request, p *domain.Payment) map[string]any {
	contracts, _ := h.Billing.Contracts(r.Context())
	return map[string]any{"Payment": p, "Contracts": contracts}
}

// PaymentForm muestra el formulario de pago:
// GET /superadmin/payments/new?contract=ID (propone el siguiente periodo) y /superadmin/payments/{id}
func (h *Handler) PaymentForm(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	p := &domain.Payment{Currency: "PEN", Status: domain.PaymentPaid, Method: "transfer"}
	if id := form.ID(chi.URLParam(r, "id")); id > 0 {
		var err error
		if p, err = h.Billing.Payment(ctx, id); err != nil {
			h.Views.Error(w, r, err)
			return
		}
	} else if cid := form.ID(r.URL.Query().Get("contract")); cid > 0 {
		if c, err := h.Billing.Contract(ctx, cid); err == nil {
			p.ContractID, p.Amount, p.Currency = c.ID, c.Amount, c.Currency
			p.PeriodStart, p.PeriodEnd, _ = h.Billing.NextPeriod(ctx, c)
		}
	}
	h.Views.Render(w, r, http.StatusOK, "superadmin/payment_form", "Pago", h.paymentFormData(r, p))
}

// PaymentSave guarda un pago: POST /superadmin/payments y /superadmin/payments/{id}
func (h *Handler) PaymentSave(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	p := &domain.Payment{}
	if id := form.ID(chi.URLParam(r, "id")); id > 0 {
		var err error
		if p, err = h.Billing.Payment(ctx, id); err != nil {
			h.Views.Error(w, r, err)
			return
		}
	}
	p.ContractID = form.Int64(r, "contract_id")
	p.PeriodStart = form.Date(r, "period_start")
	p.PeriodEnd = form.Date(r, "period_end")
	p.Amount = form.Money(r, "amount")
	p.Currency = form.String(r, "currency")
	p.Status = domain.PaymentStatus(form.String(r, "status"))
	p.Method = domain.PaymentMethod(form.String(r, "method"))
	p.Reference = form.String(r, "reference")
	p.PaidAt = form.DateTimePtr(r, "paid_at", h.Cfg.Location)
	p.Notes = form.String(r, "notes")
	url, err := admin.Upload(r, "receipt", storage.KindDocument, "billing", h.Store, h.Cfg.MaxUploadMB)
	if err != nil {
		h.formError(w, r, "superadmin/payment_form", "Pago", h.paymentFormData(r, p), err)
		return
	}
	if url != "" {
		_ = h.Store.Delete(p.ReceiptURL)
		p.ReceiptURL = url
	}
	if err := h.Billing.SavePayment(ctx, middleware.ActorFrom(r), p); err != nil {
		h.formError(w, r, "superadmin/payment_form", "Pago", h.paymentFormData(r, p), err)
		return
	}
	views.Redirect(w, r, "/superadmin/contracts/"+strconv.FormatInt(p.ContractID, 10), "ok", "Pago guardado.")
}

// PaymentDelete borra un pago: POST /superadmin/payments/{id}/delete
func (h *Handler) PaymentDelete(w http.ResponseWriter, r *http.Request) {
	err := h.Billing.DeletePayment(r.Context(), middleware.ActorFrom(r), form.ID(chi.URLParam(r, "id")))
	h.redirectResult(w, r, "/superadmin/payments", "Pago eliminado.", err)
}

// ---------- Licencia ----------

// LicensePage muestra el estado y la política de bloqueo: GET /superadmin/license
func (h *Handler) LicensePage(w http.ResponseWriter, r *http.Request) {
	pol, err := h.License.Policy(r.Context())
	if err != nil {
		h.Views.Error(w, r, err)
		return
	}
	h.Views.Render(w, r, http.StatusOK, "superadmin/license", "Licencia y bloqueos", map[string]any{
		"Policy": pol, "State": h.License.Current(),
	})
}

// LicensePolicy guarda la política: POST /superadmin/license/policy
func (h *Handler) LicensePolicy(w http.ResponseWriter, r *http.Request) {
	err := h.License.SavePolicy(r.Context(), middleware.ActorFrom(r), form.Int(r, "grace_days"),
		domain.LicenseLevel(form.String(r, "auto_block_level")), form.String(r, "suspended_message"))
	h.redirectResult(w, r, "/superadmin/license", "Política guardada.", err)
}

// LicenseBlock aplica un bloqueo manual: POST /superadmin/license/block
func (h *Handler) LicenseBlock(w http.ResponseWriter, r *http.Request) {
	err := h.License.Block(r.Context(), middleware.ActorFrom(r), domain.LicenseLevel(form.String(r, "level")), form.String(r, "reason"))
	h.redirectResult(w, r, "/superadmin/license", "Bloqueo aplicado.", err)
}

// LicenseUnblock quita el bloqueo manual: POST /superadmin/license/unblock
func (h *Handler) LicenseUnblock(w http.ResponseWriter, r *http.Request) {
	err := h.License.Unblock(r.Context(), middleware.ActorFrom(r))
	h.redirectResult(w, r, "/superadmin/license", "Bloqueo manual retirado.", err)
}

// LicenseRefresh recalcula el estado ahora: POST /superadmin/license/refresh
func (h *Handler) LicenseRefresh(w http.ResponseWriter, r *http.Request) {
	_, err := h.License.Refresh(r.Context())
	h.redirectResult(w, r, "/superadmin/license", "Estado recalculado.", err)
}

// ---------- Auditoría ----------

// AuditLog lista la auditoría paginada: GET /superadmin/audit?page=N
func (h *Handler) AuditLog(w http.ResponseWriter, r *http.Request) {
	const perPage = 50
	page := form.Int(r, "page")
	if page < 1 {
		page = 1
	}
	list, err := h.Audit.List(r.Context(), perPage+1, (page-1)*perPage)
	if err != nil {
		h.Views.Error(w, r, err)
		return
	}
	hasNext := len(list) > perPage
	if hasNext {
		list = list[:perPage]
	}
	h.Views.Render(w, r, http.StatusOK, "superadmin/audit", "Auditoría", map[string]any{
		"Entries": list, "Page": page, "HasNext": hasNext,
	})
}

// redirectResult redirige con un flash de éxito o con el mensaje del error de validación.
func (h *Handler) redirectResult(w http.ResponseWriter, r *http.Request, to, okMsg string, err error) {
	if err == nil {
		views.Redirect(w, r, to, "ok", okMsg)
		return
	}
	if ve := views.AsValidation(err); ve != nil {
		views.Redirect(w, r, to, "error", ve.Msg)
		return
	}
	if errors.Is(err, domain.ErrNotFound) {
		views.Redirect(w, r, to, "error", "No encontrado.")
		return
	}
	h.Views.Error(w, r, err)
}
