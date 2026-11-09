package rds

import (
	"bufio"
	"context"
	"io"
	"log/slog"
	"net"
	"strings"
	"testing"
	"time"
)

func TestTextAndClean(t *testing.T) {
	set := Settings{Station: "Radio Club 25", RTFormat: "{artist} - {title}", SkipSpots: true}
	if got := Text(Now{Artist: "Corazón Serrano", Title: "Mi último adiós", Music: true}, set); got != "Corazon Serrano - Mi ultimo adios" {
		t.Fatalf("texto = %q", got)
	}
	if got := Text(Now{Artist: "Ferretería", Title: "Verano"}, set); got != "Radio Club 25" {
		t.Fatalf("en cuñas debe ir el nombre de la radio: %q", got)
	}
	if got := Text(Now{Title: strings.Repeat("x", 100), Music: true}, set); len(got) != 64 {
		t.Fatalf("RadioText máximo 64: %d", len(got))
	}
	if got := PS("Club 25 Radio"); got != "Club 25 " || len(PS("X")) != 8 {
		t.Fatalf("PS = %q", got)
	}
}

// Envío real por TCP: al conectar manda el PS; luego el RadioText de cada
// cambio, sin repetir el mismo texto.
func TestSendTCP(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	lines := make(chan string, 10)
	go func() {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		sc := bufio.NewScanner(c)
		for sc.Scan() {
			lines <- strings.TrimRight(sc.Text(), "\r")
		}
	}()
	set := Settings{Enabled: true, Transport: "tcp", Address: ln.Addr().String(), PS: "CLUB 25", Station: "Radio Club 25",
		PSTemplate: `PS={ps}\r\n`, RTTemplate: `RT={text}\r\n`, RTFormat: "{artist} - {title}"}
	s := New(func() Settings { return set }, slog.New(slog.NewTextHandler(io.Discard, nil)))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go s.Run(ctx)
	s.Update(Now{Artist: "Grupo 5", Title: "Motor", Music: true})
	want := []string{"PS=CLUB 25 ", "RT=Grupo 5 - Motor"}
	for _, w := range want {
		select {
		case got := <-lines:
			if got != w {
				t.Fatalf("recibido %q, quería %q", got, w)
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("no llegó %q", w)
		}
	}
	s.Update(Now{Artist: "Grupo 5", Title: "Motor", Music: true}) // mismo texto: no se reenvía
	s.Update(Now{Artist: "Agua Marina", Title: "Paloma", Music: true})
	select {
	case got := <-lines:
		if got != "RT=Agua Marina - Paloma" {
			t.Fatalf("recibido %q", got)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no llegó el segundo texto")
	}
	if st := s.Status(); !st.Connected || st.LastText != "Agua Marina - Paloma" {
		t.Fatalf("estado = %+v", st)
	}
}
