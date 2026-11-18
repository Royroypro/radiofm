// Package loopback captura lo que suena en los altavoces de la PC (o en otro
// adaptador de reproducción) y se lo pasa a ffmpeg.
//
// ffmpeg en Windows no sabe capturar la salida de audio (DirectShow solo ve
// entradas), así que la captura la hace el emisor con WASAPI en modo loopback
// (miniaudio, por malgo; ver capture_windows.go) y la sirve como PCM crudo
// (s16le, estéreo, 48 kHz) por TCP en 127.0.0.1. ffmpeg la lee con
// `-f s16le -ar 48000 -ac 2 -i tcp://127.0.0.1:PUERTO`.
//
// WASAPI no entrega datos mientras no suena nada, y ffmpeg necesita un flujo
// continuo (si no, Icecast corta y el supervisor lo da por colgado). Por eso
// Server envía a ritmo de reloj y rellena con silencio lo que falte. Este
// archivo no depende del hardware y se prueba con datos falsos.
package loopback

import (
	"net"
	"sync"
	"time"
)

// Formato que se entrega a ffmpeg.
const (
	SampleRate = 48000
	Channels   = 2
	frameBytes = Channels * 2 // s16le
)

// Tiempos del ritmo de envío.
const (
	tick      = 20 * time.Millisecond  // cada cuánto se envía
	maxBuffer = 300 * time.Millisecond // más audio guardado que esto: se descarta lo viejo
)

// Server reparte el audio capturado a cada ffmpeg conectado.
type Server struct {
	ln net.Listener

	mu      sync.Mutex
	buf     []byte // audio capturado aún no enviado
	clients map[net.Conn]chan []byte
	closed  bool
	done    chan struct{}
	now     func() time.Time // reloj (reemplazable en tests)
}

// NewServer escucha en 127.0.0.1 con un puerto libre y empieza a enviar.
func NewServer() (*Server, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	s := &Server{ln: ln, clients: map[net.Conn]chan []byte{}, done: make(chan struct{}), now: time.Now}
	go s.accept()
	go s.pace()
	return s, nil
}

// Addr es la URL para ffmpeg: tcp://127.0.0.1:PUERTO.
func (s *Server) Addr() string { return "tcp://" + s.ln.Addr().String() }

// Write recibe audio capturado (s16le estéreo 48 kHz). Lo llama la captura.
func (s *Server) Write(p []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	s.buf = append(s.buf, p...)
	if limit := bytesFor(maxBuffer); len(s.buf) > limit {
		// La placa de sonido va un poco más rápido que el reloj de la PC: se tira lo más viejo.
		drop := (len(s.buf) - limit) / frameBytes * frameBytes
		s.buf = append(s.buf[:0], s.buf[drop:]...)
	}
}

// Close deja de aceptar y corta a los ffmpeg conectados (leen fin de archivo).
func (s *Server) Close() {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.closed = true
	for c, ch := range s.clients {
		close(ch)
		delete(s.clients, c)
	}
	s.mu.Unlock()
	close(s.done)
	_ = s.ln.Close()
}

// Clients dice cuántos ffmpeg están conectados.
func (s *Server) Clients() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.clients)
}

func bytesFor(d time.Duration) int {
	return int(d.Seconds()*SampleRate) * frameBytes
}

func (s *Server) accept() {
	for {
		c, err := s.ln.Accept()
		if err != nil {
			return
		}
		ch := make(chan []byte, 64) // ~1,3 s de margen; si el ffmpeg se atrasa más, se pierden trozos
		s.mu.Lock()
		if s.closed {
			s.mu.Unlock()
			c.Close()
			return
		}
		s.clients[c] = ch
		s.mu.Unlock()
		go func() {
			defer c.Close()
			for chunk := range ch {
				_ = c.SetWriteDeadline(time.Now().Add(5 * time.Second))
				if _, err := c.Write(chunk); err != nil {
					s.mu.Lock()
					if _, ok := s.clients[c]; ok {
						delete(s.clients, c)
						close(ch)
					}
					s.mu.Unlock()
					for range ch { // vaciar lo pendiente
					}
					return
				}
			}
		}()
	}
}

// pace envía a ritmo de reloj: en cada vuelta, exactamente el audio que
// corresponde al tiempo pasado; primero lo capturado y, si falta, silencio.
func (s *Server) pace() {
	start := s.now()
	var sentFrames int64
	t := time.NewTicker(tick)
	defer t.Stop()
	for {
		select {
		case <-s.done:
			return
		case <-t.C:
		}
		due := int64(s.now().Sub(start).Seconds()*SampleRate) - sentFrames
		if due <= 0 {
			continue
		}
		chunk := s.take(int(due))
		sentFrames += due
		s.broadcast(chunk)
	}
}

// take saca frames del búfer y completa con silencio.
func (s *Server) take(frames int) []byte {
	n := frames * frameBytes
	out := make([]byte, n) // los ceros son silencio
	s.mu.Lock()
	k := copy(out, s.buf)
	s.buf = append(s.buf[:0], s.buf[k:]...)
	s.mu.Unlock()
	return out
}

func (s *Server) broadcast(chunk []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, ch := range s.clients {
		select {
		case ch <- chunk:
		default: // ese ffmpeg va atrasado: se salta este trozo
		}
	}
}
