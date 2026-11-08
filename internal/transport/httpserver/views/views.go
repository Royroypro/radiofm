// Package views dibuja las páginas HTML con html/template.
//
// Organización de web/templates:
//
//	layouts/public.html   esqueleto de la web pública      (define "layout")
//	layouts/panel.html    esqueleto de admin y superadmin  (define "layout")
//	layouts/bare.html     esqueleto mínimo: login, errores (define "layout")
//	partials/*.html       bloques reutilizables (flash, csrf, banner...)
//	<área>/<página>.html  cada página define "title" y "content"
//
// El layout se elige por la carpeta de la página: public → public.html;
// admin y superadmin → panel.html; auth y errors → bare.html.
//
// Todas las páginas reciben un *Page con los datos comunes (ajustes del
// sitio, usuario, licencia, token CSRF, mensaje flash) y en .Data los datos
// propios de la página.
package views

import (
	"bytes"
	"context"
	"fmt"
	"html/template"
	"io/fs"
	"log/slog"
	"net/http"
	"path"
	"strings"
	"sync"
	"time"

	"radioclub25/internal/domain"
)

// Page son los datos que recibe cada plantilla.
type Page struct {
	Title       string
	Site        domain.SiteSettings
	User        *domain.User
	License     domain.LicenseState
	CSRF        string
	Flash       *Flash
	Path        string
	Error       string            // error general del formulario
	FieldErrors map[string]string // errores por campo
	Banners     map[domain.AdPosition]*domain.Ad
	Data        any
	Now         time.Time
	BaseURL     string
}

// FieldError devuelve el error del campo (para usar en plantillas).
func (p *Page) FieldError(name string) string { return p.FieldErrors[name] }

// Banner devuelve el banner de una posición o nil.
func (p *Page) Banner(pos string) *domain.Ad { return p.Banners[domain.AdPosition(pos)] }

// CommonFunc rellena los datos comunes de Page a partir de la petición.
type CommonFunc func(w http.ResponseWriter, r *http.Request, p *Page)

// Renderer compila y ejecuta las plantillas.
type Renderer struct {
	fsys   fs.FS
	reload bool // true en desarrollo: recompila en cada petición
	common CommonFunc
	log    *slog.Logger
	loc    *time.Location

	mu    sync.RWMutex
	pages map[string]*template.Template
}

// New crea el renderer. fsys debe contener la carpeta templates/.
func New(fsys fs.FS, reload bool, loc *time.Location, log *slog.Logger) (*Renderer, error) {
	r := &Renderer{fsys: fsys, reload: reload, log: log, loc: loc}
	if err := r.load(); err != nil {
		return nil, err
	}
	return r, nil
}

// SetCommon registra la función que rellena los datos comunes.
func (rd *Renderer) SetCommon(fn CommonFunc) { rd.common = fn }

func layoutFor(page string) string {
	switch strings.SplitN(page, "/", 2)[0] {
	case "public":
		return "templates/layouts/public.html"
	case "admin", "superadmin":
		return "templates/layouts/panel.html"
	default:
		return "templates/layouts/bare.html"
	}
}

func (rd *Renderer) load() error {
	partials, err := fs.Glob(rd.fsys, "templates/partials/*.html")
	if err != nil {
		return err
	}
	pages := map[string]*template.Template{}
	for _, dir := range []string{"public", "admin", "superadmin", "auth", "errors"} {
		files, err := fs.Glob(rd.fsys, "templates/"+dir+"/*.html")
		if err != nil {
			return err
		}
		for _, f := range files {
			name := dir + "/" + strings.TrimSuffix(path.Base(f), ".html")
			all := append([]string{layoutFor(name)}, partials...)
			all = append(all, f)
			t, err := template.New(path.Base(layoutFor(name))).Funcs(rd.funcs()).ParseFS(rd.fsys, all...)
			if err != nil {
				return fmt.Errorf("views: compilar %s: %w", name, err)
			}
			pages[name] = t
		}
	}
	rd.mu.Lock()
	rd.pages = pages
	rd.mu.Unlock()
	return nil
}

// Render dibuja la página name (p. ej. "admin/ads") con el título y los datos indicados.
func (rd *Renderer) Render(w http.ResponseWriter, r *http.Request, status int, name, title string, data any) {
	rd.RenderPage(w, r, status, name, &Page{Title: title, Data: data})
}

// RenderPage dibuja una página con un *Page ya preparado (para pasar errores de formulario).
func (rd *Renderer) RenderPage(w http.ResponseWriter, r *http.Request, status int, name string, p *Page) {
	if rd.reload {
		if err := rd.load(); err != nil {
			rd.log.Error("recompilar plantillas", "err", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
	rd.mu.RLock()
	t, ok := rd.pages[name]
	rd.mu.RUnlock()
	if !ok {
		rd.log.Error("plantilla inexistente", "name", name)
		http.Error(w, "Plantilla no encontrada: "+name, http.StatusInternalServerError)
		return
	}
	if p.Now.IsZero() {
		p.Now = time.Now().In(rd.loc)
	}
	p.Path = r.URL.Path
	if rd.common != nil {
		rd.common(w, r, p)
	}
	var buf bytes.Buffer
	if err := t.ExecuteTemplate(&buf, "layout", p); err != nil {
		rd.log.Error("ejecutar plantilla", "name", name, "err", err)
		http.Error(w, "Error al mostrar la página", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_, _ = buf.WriteTo(w)
}

// ---------- mensajes flash ----------

// Flash es un mensaje que se muestra una sola vez tras una redirección.
type Flash struct {
	Kind    string // "ok" o "error"
	Message string
}

const flashCookie = "rc_flash"

// SetFlash guarda un mensaje para la próxima página.
func SetFlash(w http.ResponseWriter, kind, msg string) {
	http.SetCookie(w, &http.Cookie{Name: flashCookie, Value: kind + "|" + urlEncode(msg), Path: "/",
		HttpOnly: true, SameSite: http.SameSiteLaxMode, MaxAge: 60})
}

// PopFlash lee y borra el mensaje flash.
func PopFlash(w http.ResponseWriter, r *http.Request) *Flash {
	c, err := r.Cookie(flashCookie)
	if err != nil || c.Value == "" {
		return nil
	}
	http.SetCookie(w, &http.Cookie{Name: flashCookie, Value: "", Path: "/", MaxAge: -1})
	kind, msg, _ := strings.Cut(c.Value, "|")
	return &Flash{Kind: kind, Message: urlDecode(msg)}
}

// Redirect guarda un flash y redirige (patrón POST → redirect → GET).
func Redirect(w http.ResponseWriter, r *http.Request, to, kind, msg string) {
	if msg != "" {
		SetFlash(w, kind, msg)
	}
	http.Redirect(w, r, to, http.StatusSeeOther)
}

// ---------- errores ----------

// ErrorMessage traduce un error de dominio a código HTTP y mensaje para el usuario.
func ErrorMessage(err error) (int, string) {
	switch {
	case err == nil:
		return http.StatusOK, ""
	case isErr(err, domain.ErrNotFound):
		return http.StatusNotFound, "No encontrado."
	case isErr(err, domain.ErrForbidden):
		return http.StatusForbidden, "No tiene permiso para esta acción."
	case isErr(err, domain.ErrPlanLimit):
		return http.StatusForbidden, "Su plan no permite esta acción o alcanzó su límite. Contacte al proveedor."
	case isErr(err, domain.ErrLicenseBlocked):
		return http.StatusForbidden, "El servicio está bloqueado. Contacte al proveedor."
	case isErr(err, domain.ErrTooManyAttempts):
		return http.StatusTooManyRequests, domain.ErrTooManyAttempts.Error()
	case isErr(err, context.Canceled):
		return 499, "Petición cancelada."
	}
	if ve := AsValidation(err); ve != nil {
		return http.StatusUnprocessableEntity, ve.Msg
	}
	return http.StatusInternalServerError, "Ocurrió un error inesperado. Inténtelo de nuevo."
}

// Error dibuja la página de error adecuada para err.
func (rd *Renderer) Error(w http.ResponseWriter, r *http.Request, err error) {
	status, msg := ErrorMessage(err)
	if status >= 500 {
		rd.log.Error("error en handler", "path", r.URL.Path, "err", err)
	}
	rd.Render(w, r, status, "errors/error", "Error", map[string]any{"Status": status, "Message": msg})
}
