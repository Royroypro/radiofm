package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestRun(t *testing.T) {
	var out, errb bytes.Buffer
	if code := run([]string{"version"}, &out, &errb); code != 0 || !strings.Contains(out.String(), "Club25 Estudio") {
		t.Fatalf("version: código %d, salida %q", code, out.String())
	}
	if code := run(nil, &out, &errb); code != 2 {
		t.Fatalf("sin argumentos debe devolver 2, dio %d", code)
	}
	if code := run([]string{"otro"}, &out, &errb); code != 2 || !strings.Contains(errb.String(), "desconocido") {
		t.Fatalf("comando desconocido: código %d", code)
	}
}
