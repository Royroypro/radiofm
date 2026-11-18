// Package icefake es un servidor Icecast mínimo para pruebas: acepta fuentes
// por PUT (como Icecast 2.4+ y ffmpeg), exige usuario y clave (Basic), rechaza
// un segundo emisor en el mismo montaje y guarda lo recibido en memoria.
//
// No sirve oyentes ni reemplaza a Icecast: solo comprueba que el emisor se
// conecta como debe y que lo que envía es audio válido. Lo usan los tests y
// cmd/icecast-prueba.
package icefake

import (
	"bufio"
	"bytes"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
)

// Source es lo que llegó a un montaje.
type Source struct {
	ContentType string
	Headers     http.Header
	Data        bytes.Buffer
	Connected   bool
}

// Server guarda las fuentes por montaje.
type Server struct {
	User, Password string
	// OnChunk (opcional) se llama al recibir datos: montaje y bytes totales.
	OnChunk func(mount string, total int)
	// OnMeta (opcional) se llama al recibir un título (/admin/metadata).
	OnMeta func(mount, song string)

	mu      sync.Mutex
	sources map[string]*Source
	rejects int
	meta    map[string][]string // títulos recibidos por montaje, en orden
}

// New crea un servidor con las credenciales de fuente.
func New(user, password string) *Server {
	return &Server{User: user, Password: password, sources: map[string]*Source{}}
}

// Serve acepta conexiones hasta que ln se cierre.
//
// Se trabaja sobre TCP crudo y no con net/http: la fuente envía un PUT sin
// Content-Length ni chunked (el cuerpo es el resto de la conexión), y net/http
// lo leería como cuerpo vacío. Igual que Icecast: si viene «Expect:
// 100-continue» responde 100, si no 200, y lee hasta que se corte.
func (s *Server) Serve(ln net.Listener) error {
	for {
		c, err := ln.Accept()
		if err != nil {
			return err
		}
		go s.handle(c)
	}
}

const crlf = "\r\n"

func (s *Server) handle(c net.Conn) {
	defer c.Close()
	br := bufio.NewReader(c)
	r, err := http.ReadRequest(br)
	if err != nil {
		return
	}
	reply := func(status string, headers ...string) {
		msg := "HTTP/1.0 " + status + crlf + "Server: icefake" + crlf
		for _, h := range headers {
			msg += h + crlf
		}
		_, _ = io.WriteString(c, msg+crlf)
	}
	if r.Method == http.MethodGet && r.URL.Path == "/admin/metadata" {
		s.metadata(r, reply)
		return
	}
	if r.Method != http.MethodPut && r.Method != "SOURCE" {
		reply("405 Method Not Allowed")
		return
	}
	u, p, ok := r.BasicAuth()
	if !ok || u != s.User || p != s.Password {
		s.mu.Lock()
		s.rejects++
		s.mu.Unlock()
		reply("401 Unauthorized", `WWW-Authenticate: Basic realm="Icecast2 Server"`)
		return
	}
	mount := r.URL.Path
	s.mu.Lock()
	if src, busy := s.sources[mount]; busy && src.Connected {
		s.mu.Unlock()
		reply("403 Forbidden")
		return
	}
	src := &Source{ContentType: r.Header.Get("Content-Type"), Headers: r.Header.Clone(), Connected: true}
	s.sources[mount] = src
	s.mu.Unlock()

	if strings.EqualFold(r.Header.Get("Expect"), "100-continue") {
		_, _ = io.WriteString(c, "HTTP/1.1 100 Continue"+crlf+crlf)
	} else {
		reply("200 OK")
	}
	buf := make([]byte, 32*1024)
	for {
		n, err := br.Read(buf)
		if n > 0 {
			s.mu.Lock()
			src.Data.Write(buf[:n])
			total := src.Data.Len()
			s.mu.Unlock()
			if s.OnChunk != nil {
				s.OnChunk(mount, total)
			}
		}
		if err != nil { // la fuente se desconectó
			break
		}
	}
	s.mu.Lock()
	src.Connected = false
	s.mu.Unlock()
}

// Get devuelve una copia de lo recibido en un montaje.
func (s *Server) Get(mount string) (contentType string, data []byte, headers http.Header, ok bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	src, ok := s.sources[mount]
	if !ok {
		return "", nil, nil, false
	}
	return src.ContentType, append([]byte(nil), src.Data.Bytes()...), src.Headers, true
}

// Rejected cuenta los intentos con usuario o clave incorrectos.
func (s *Server) Rejected() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.rejects
}

// metadata atiende GET /admin/metadata?mount=/radio&mode=updinfo&song=… (título
// de la canción), con el usuario y la clave de fuente, como Icecast.
func (s *Server) metadata(r *http.Request, reply func(string, ...string)) {
	u, p, ok := r.BasicAuth()
	if !ok || u != s.User || p != s.Password {
		reply("401 Unauthorized", `WWW-Authenticate: Basic realm="Icecast2 Server"`)
		return
	}
	q := r.URL.Query()
	mount, song := q.Get("mount"), q.Get("song")
	if q.Get("mode") != "updinfo" || mount == "" {
		reply("400 Bad Request")
		return
	}
	s.mu.Lock()
	if s.meta == nil {
		s.meta = map[string][]string{}
	}
	s.meta[mount] = append(s.meta[mount], song)
	s.mu.Unlock()
	if s.OnMeta != nil {
		s.OnMeta(mount, song)
	}
	reply("200 OK", "Content-Type: text/xml")
}

// Titles devuelve los títulos recibidos en un montaje, en orden.
func (s *Server) Titles(mount string) []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.meta[mount]...)
}
