// Package rds envía al codificador RDS del transmisor FM el nombre de la radio
// (PS, 8 caracteres) y lo que suena (RadioText, hasta 64): es lo que muestra
// la pantalla del radio del oyente y del auto.
//
// Es genérico: cada marca de codificador usa su propio comando, así que el
// comando se arma con una plantilla editable (Ajustes → RDS). Muchos aceptan
// texto ASCII por TCP, UDP o puerto serie del estilo «PS=…» / «RT=…»; si el
// equipo usa otra sintaxis, se cambia la plantilla. {ps} y {text} se reemplazan.
package rds

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"strings"
	"sync"
	"time"
	"unicode"

	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
)

// Settings son los ajustes del envío.
type Settings struct {
	Enabled    bool
	Transport  string // tcp | udp | serial
	Address    string // host:puerto o COM3
	Baud       int
	PS         string
	PSTemplate string // "PS={ps}\r\n"
	RTTemplate string // "RT={text}\r\n"
	RTFormat   string // "{artist} - {title}"
	SkipSpots  bool
	MinSeconds int
	Station    string // nombre de la radio (texto en cuñas/jingles si SkipSpots)
}

// Now es lo que suena (lo informa el aire).
type Now struct {
	Artist, Title string
	Music         bool // canción (no cuña, jingle ni locución)
}

// Service mantiene la conexión con el codificador y envía los cambios.
type Service struct {
	settings func() Settings
	log      *slog.Logger
	dial     func(s Settings) (io.WriteCloser, error)
	now      func() time.Time

	in chan Now

	mu       sync.Mutex
	lastText string
	lastSent time.Time
	lastErr  string
	conn     bool
}

// New crea el servicio.
func New(settings func() Settings, log *slog.Logger) *Service {
	return &Service{settings: settings, log: log, dial: dial, now: time.Now, in: make(chan Now, 1)}
}

// Status es el estado para la interfaz.
type Status struct {
	Enabled   bool      `json:"enabled"`
	Connected bool      `json:"connected"`
	LastText  string    `json:"last_text"`
	LastSent  time.Time `json:"last_sent"`
	Error     string    `json:"error"`
}

// Status devuelve el estado.
func (s *Service) Status() Status {
	s.mu.Lock()
	defer s.mu.Unlock()
	return Status{Enabled: s.settings().Enabled, Connected: s.conn, LastText: s.lastText, LastSent: s.lastSent, Error: s.lastErr}
}

// Update informa lo que empezó a sonar (no bloquea: si hay uno pendiente, gana el último).
func (s *Service) Update(n Now) {
	select {
	case s.in <- n:
	default:
		select {
		case <-s.in:
		default:
		}
		select {
		case s.in <- n:
		default:
		}
	}
}

// Run envía los cambios mientras ctx viva; reconecta solo.
func (s *Service) Run(ctx context.Context) {
	var w io.WriteCloser
	var pending *string
	closeConn := func() {
		if w != nil {
			_ = w.Close()
			w = nil
		}
		s.mu.Lock()
		s.conn = false
		s.mu.Unlock()
	}
	defer closeConn()
	retry := time.NewTicker(2 * time.Second)
	defer retry.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case n := <-s.in:
			t := Text(n, s.settings())
			pending = &t
		case <-retry.C:
		}
		set := s.settings()
		if !set.Enabled || set.Address == "" {
			closeConn()
			continue
		}
		if w == nil {
			c, err := s.dial(set)
			if err != nil {
				s.fail(err)
				continue
			}
			w = c
			s.mu.Lock()
			s.conn, s.lastErr = true, ""
			s.mu.Unlock()
			// al conectar: el nombre y el último texto
			if err := send(w, set.PSTemplate, "{ps}", PS(set.PS)); err != nil {
				s.fail(err)
				closeConn()
				continue
			}
			s.mu.Lock()
			last := s.lastText
			s.lastSent = time.Time{} // reconectado: reenviar aunque sea el mismo texto
			s.mu.Unlock()
			if pending == nil && last != "" {
				pending = &last
			}
		}
		if pending == nil {
			continue
		}
		s.mu.Lock()
		wait := time.Duration(set.MinSeconds)*time.Second - s.now().Sub(s.lastSent)
		same := *pending == s.lastText && !s.lastSent.IsZero()
		s.mu.Unlock()
		if same {
			pending = nil
			continue
		}
		if wait > 0 {
			continue // se reintenta en el próximo tic (gana el último texto)
		}
		if err := send(w, set.RTTemplate, "{text}", *pending); err != nil {
			s.fail(err)
			closeConn()
			continue
		}
		s.mu.Lock()
		s.lastText, s.lastSent = *pending, s.now()
		s.mu.Unlock()
		pending = nil
	}
}

func (s *Service) fail(err error) {
	s.mu.Lock()
	changed := s.lastErr != err.Error()
	s.lastErr = err.Error()
	s.mu.Unlock()
	if changed {
		s.log.Warn("RDS", "err", err)
	}
}

// Text arma el RadioText de lo que suena (ASCII, máximo 64).
func Text(n Now, set Settings) string {
	if !n.Music && set.SkipSpots {
		return Clean(set.Station, 64)
	}
	f := set.RTFormat
	if f == "" {
		f = "{artist} - {title}"
	}
	t := strings.NewReplacer("{artist}", n.Artist, "{title}", n.Title, "{station}", set.Station).Replace(f)
	t = strings.Trim(strings.TrimSpace(t), "- ")
	if t == "" {
		t = set.Station
	}
	return Clean(t, 64)
}

// PS deja el nombre en 8 caracteres ASCII (relleno con espacios).
func PS(s string) string {
	s = Clean(s, 8)
	return s + strings.Repeat(" ", 8-len(s))
}

var stripMarks = transform.Chain(norm.NFD, runes.Remove(runes.In(unicode.Mn)), norm.NFC)

// Clean quita tildes y caracteres fuera de ASCII imprimible (el juego de
// caracteres del RDS no los admite igual en todos los equipos) y corta a max.
func Clean(s string, max int) string {
	out, _, err := transform.String(stripMarks, s)
	if err != nil {
		out = s
	}
	var b strings.Builder
	for _, r := range out {
		switch {
		case r == 'ñ':
			b.WriteByte('n')
		case r == 'Ñ':
			b.WriteByte('N')
		case r >= 32 && r < 127 && r != '\r' && r != '\n':
			b.WriteRune(r)
		}
	}
	t := strings.Join(strings.Fields(b.String()), " ")
	if len(t) > max {
		t = strings.TrimSpace(t[:max])
	}
	return t
}

// send arma el comando con la plantilla (admite \r, \n y \t escritos así) y lo escribe.
func send(w io.Writer, tmpl, key, value string) error {
	if tmpl == "" {
		return nil
	}
	cmd := strings.NewReplacer(`\r`, "\r", `\n`, "\n", `\t`, "\t").Replace(tmpl)
	cmd = strings.ReplaceAll(cmd, key, value)
	if c, ok := w.(net.Conn); ok {
		_ = c.SetWriteDeadline(time.Now().Add(5 * time.Second))
	}
	_, err := io.WriteString(w, cmd)
	return err
}

func dial(s Settings) (io.WriteCloser, error) {
	switch s.Transport {
	case "", "tcp":
		return net.DialTimeout("tcp", s.Address, 5*time.Second)
	case "udp":
		return net.DialTimeout("udp", s.Address, 5*time.Second)
	case "serial":
		return openSerial(s.Address, s.Baud)
	}
	return nil, fmt.Errorf("transporte %q desconocido", s.Transport)
}

// errNoSerial: el puerto serie solo está en Windows.
var errNoSerial = errors.New("puerto serie no disponible en este sistema")
