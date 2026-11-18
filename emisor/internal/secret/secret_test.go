package secret

import (
	"strings"
	"testing"
)

func TestSealOpenRoundTrip(t *testing.T) {
	for _, plain := range []string{"", "hackme", "clave con espacios y ñ:@/?&"} {
		enc, err := Seal(plain)
		if err != nil {
			t.Fatal(err)
		}
		if plain != "" && (!strings.HasPrefix(enc, prefix) || strings.Contains(enc, plain)) {
			t.Fatalf("Seal(%q) = %q: debe ir con prefijo y sin el texto plano", plain, enc)
		}
		got, err := Open(enc)
		if err != nil || got != plain {
			t.Fatalf("Open(Seal(%q)) = %q, %v", plain, got, err)
		}
	}
}

func TestOpenPlainValue(t *testing.T) {
	if got, _ := Open("escrita-a-mano"); got != "escrita-a-mano" {
		t.Fatalf("un valor sin prefijo debe devolverse tal cual, obtuve %q", got)
	}
	if _, err := Open("enc:%%%"); err == nil {
		t.Fatal("base64 inválido debe dar error")
	}
}
