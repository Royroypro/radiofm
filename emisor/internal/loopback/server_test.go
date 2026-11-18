package loopback

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"io"
	"net"
	"strings"
	"testing"
	"time"
)

func dial(t *testing.T, s *Server) net.Conn {
	t.Helper()
	c, err := net.Dial("tcp", strings.TrimPrefix(s.Addr(), "tcp://"))
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for s.Clients() == 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	return c
}

// Sin audio capturado debe llegar silencio continuo, a ritmo de tiempo real.
func TestSilenceAtRealTimeRate(t *testing.T) {
	s, err := NewServer()
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	c := dial(t, s)
	defer c.Close()
	start := time.Now()
	buf := make([]byte, bytesFor(500*time.Millisecond))
	if _, err := io.ReadFull(c, buf); err != nil {
		t.Fatal(err)
	}
	el := time.Since(start)
	if el < 400*time.Millisecond || el > 900*time.Millisecond {
		t.Fatalf("500 ms de audio llegaron en %s: el ritmo no es de tiempo real", el)
	}
	if !bytes.Equal(buf, make([]byte, len(buf))) {
		t.Fatal("sin captura debe enviarse silencio (ceros)")
	}
}

// Lo capturado sale en orden, y lo que falta se completa con silencio.
func TestCapturedAudioPassesThrough(t *testing.T) {
	s, err := NewServer()
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	c := dial(t, s)
	defer c.Close()
	// 100 ms de una rampa reconocible.
	frames := SampleRate / 10
	var in bytes.Buffer
	for i := 0; i < frames; i++ {
		v := int16(i%30000 + 1)
		binary.Write(&in, binary.LittleEndian, v)
		binary.Write(&in, binary.LittleEndian, -v)
	}
	s.Write(in.Bytes())
	r := bufio.NewReader(c)
	got := make([]byte, bytesFor(400*time.Millisecond))
	if _, err := io.ReadFull(r, got); err != nil {
		t.Fatal(err)
	}
	i := bytes.Index(got, in.Bytes()[:64])
	if i < 0 || i%frameBytes != 0 {
		t.Fatalf("no apareció el audio capturado alineado (índice %d)", i)
	}
	if !bytes.Equal(got[i:i+in.Len()], in.Bytes()) {
		t.Fatal("el audio capturado llegó alterado")
	}
}

// Un búfer más grande que maxBuffer descarta lo viejo (no acumula retraso).
func TestBufferDropsOldest(t *testing.T) {
	s := &Server{clients: map[net.Conn]chan []byte{}}
	big := make([]byte, bytesFor(time.Second))
	for i := range big {
		big[i] = byte(i / frameBytes)
	}
	s.Write(big)
	if len(s.buf) != bytesFor(maxBuffer) || len(s.buf)%frameBytes != 0 {
		t.Fatalf("búfer = %d bytes, esperaba %d", len(s.buf), bytesFor(maxBuffer))
	}
	if !bytes.Equal(s.buf, big[len(big)-len(s.buf):]) {
		t.Fatal("debía quedar lo más reciente")
	}
}

// Close corta a los ffmpeg conectados (fin de archivo) y se puede llamar dos veces.
func TestCloseEndsClients(t *testing.T) {
	s, err := NewServer()
	if err != nil {
		t.Fatal(err)
	}
	c := dial(t, s)
	defer c.Close()
	s.Close()
	s.Close()
	_ = c.SetReadDeadline(time.Now().Add(3 * time.Second))
	if _, err := io.Copy(io.Discard, c); err != nil {
		t.Fatalf("tras Close debía llegar fin de archivo: %v", err)
	}
}
