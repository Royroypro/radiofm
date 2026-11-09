package views

import (
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"net/url"
	"regexp"
	"strings"
	"time"

	"radioclub25/internal/domain"
)

// funcs son las funciones disponibles en todas las plantillas.
func (rd *Renderer) funcs() template.FuncMap {
	loc := rd.loc
	return template.FuncMap{
		// fechas (siempre en la zona horaria de la radio)
		"date": func(t any) string { return fmtTime(t, loc, "02/01/2006") },
		"datetime": func(t any) string {
			return fmtTime(t, loc, "02/01/2006 15:04")
		},
		// fechas sin hora guardadas como medianoche UTC (contratos, pagos)
		"day": func(t any) string { return fmtTime(t, time.UTC, "02/01/2006") },
		// valores para <input type="date"> y <input type="datetime-local">
		"dateInput":     func(t any) string { return fmtTime(t, time.UTC, "2006-01-02") },
		"datetimeInput": func(t any) string { return fmtTime(t, loc, "2006-01-02T15:04") },
		"money": func(m domain.Money, currency string) string {
			return domain.CurrencySymbol(currency) + " " + m.String()
		},
		"moneyInput": func(m domain.Money) string { return fmt.Sprintf("%d.%02d", int64(m)/100, int64(m)%100) },
		"pct":        func(f float64) string { return fmt.Sprintf("%.1f %%", f) },
		"json": func(v any) template.JS {
			b, _ := json.Marshal(v)
			return template.JS(b)
		},
		"split": func(s, sep string) []string {
			var out []string
			for _, p := range strings.Split(s, sep) {
				if p = strings.TrimSpace(p); p != "" {
					out = append(out, p)
				}
			}
			return out
		},
		"hasPrefix": strings.HasPrefix,
		"lower":     strings.ToLower,
		"add":       func(a, b int) int { return a + b },
		"seq": func(n int) []int {
			s := make([]int, n)
			for i := range s {
				s[i] = i
			}
			return s
		},
		"weekdays": func() [7]string { return domain.WeekdayNames },
		"adTypes": func() []domain.AdType {
			return []domain.AdType{domain.AdBanner, domain.AdPrerollVideo, domain.AdAudioSpot}
		},
		"adPositions":  func() []domain.AdPosition { return domain.AdPositions },
		"payMethods":   func() any { return domain.PaymentMethods },
		"blockLevels":  func() []domain.LicenseLevel { return domain.BlockLevels },
		"roles":        func() []domain.Role { return []domain.Role{domain.RoleAdmin, domain.RoleEditor} },
		"isSuperAdmin": func(u *domain.User) bool { return u != nil && u.Role == domain.RoleSuperAdmin },
		"isAdmin":      func(u *domain.User) bool { return u != nil && u.Role.AtLeast(domain.RoleAdmin) },
		// whatsappURL convierte "+51 999 999 999" en https://wa.me/51999999999
		"whatsappURL": func(n string) string {
			digits := strings.Map(func(r rune) rune {
				if r >= '0' && r <= '9' {
					return r
				}
				return -1
			}, n)
			return "https://wa.me/" + digits
		},
		// playerConfig es la configuración que recibe site.js en window.RC.
		"playerConfig": func(s domain.SiteSettings) map[string]any {
			return map[string]any{
				"radioURL": s.RadioStreamURL, "tvURL": s.TVStreamURL, "radioEnabled": s.RadioEnabled,
				"tvEnabled": s.TVEnabled, "slogan": s.Slogan, "prerollIntervalMin": s.PrerollIntervalMin,
			}
		},
		// safeColor deja pasar solo colores #RRGGBB al CSS (si no, usa el azul de la marca).
		"safeColor": func(c string) template.CSS {
			if hexColor.MatchString(c) {
				return template.CSS(c)
			}
			return template.CSS("#0A1A6B")
		},
		// freqArgs prepara el partial "freq_select" (frecuencia de un programa o cuña; 0 = todas).
		"freqArgs": func(freqs []domain.Frequency, selected int64, hint string) map[string]any {
			return map[string]any{"Freqs": freqs, "Selected": selected, "Hint": hint}
		},
		"colorArgs": func(name, label, value, def, err string) map[string]string {
			return map[string]string{"Name": name, "Label": label, "Value": value, "Default": def, "Err": err}
		},
		"int": func(v any) int {
			switch x := v.(type) {
			case time.Weekday:
				return int(x)
			case int64:
				return int(x)
			case int:
				return x
			}
			return 0
		},
		"neg":    func(n int) int { return -n },
		"telURL": func(n string) string { return "tel:" + strings.ReplaceAll(n, " ", "") },
		"levelClass": func(l domain.LicenseLevel) string {
			switch {
			case l == domain.LicenseActive:
				return "ok"
			case l == domain.LicenseOverdue:
				return "warn"
			default:
				return "danger"
			}
		},
	}
}

var hexColor = regexp.MustCompile(`^#[0-9A-Fa-f]{6}$`)

func fmtTime(v any, loc *time.Location, layout string) string {
	switch t := v.(type) {
	case time.Time:
		if t.IsZero() {
			return ""
		}
		return t.In(loc).Format(layout)
	case *time.Time:
		if t == nil || t.IsZero() {
			return ""
		}
		return t.In(loc).Format(layout)
	}
	return ""
}

func urlEncode(s string) string { return url.QueryEscape(s) }

func urlDecode(s string) string {
	v, err := url.QueryUnescape(s)
	if err != nil {
		return ""
	}
	return v
}

func isErr(err, target error) bool { return errors.Is(err, target) }

// AsValidation devuelve el *domain.ValidationError contenido en err, o nil.
func AsValidation(err error) *domain.ValidationError {
	var ve *domain.ValidationError
	if errors.As(err, &ve) {
		return ve
	}
	return nil
}
