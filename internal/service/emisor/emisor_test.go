package emisor_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"log/slog"
	"os"
	"strings"
	"testing"

	"radioclub25/internal/domain"
	"radioclub25/internal/repository/sqlite"
	"radioclub25/internal/service/audit"
	"radioclub25/internal/service/emisor"
	"radioclub25/internal/testutil"
)

func newService(t *testing.T) *emisor.Service {
	d := testutil.DB(t)
	return emisor.New(sqlite.NewSettingsRepo(d), sqlite.NewEmisorReleaseRepo(d),
		audit.New(sqlite.NewAuditRepo(d), slog.New(slog.DiscardHandler)), t.TempDir(), t.TempDir())
}

var actor = domain.Actor{Email: "super@local.test", Role: domain.RoleSuperAdmin}

func fakeExe(size int) []byte {
	b := bytes.Repeat([]byte{0x90}, size)
	copy(b, "MZ")
	return b
}

func field(err error) string {
	var ve *domain.ValidationError
	if errors.As(err, &ve) {
		return ve.Field
	}
	return ""
}

func TestProvider(t *testing.T) {
	ctx := context.Background()
	s := newService(t)
	p, err := s.Provider(ctx)
	if err != nil || p.Phone != "9488933154" || p.URL != "https://sehuacho.com/radio" {
		t.Fatalf("de fábrica: %+v %v", p, err)
	}
	if f := field(s.SaveProvider(ctx, actor, domain.ProviderInfo{Phone: "abc", URL: "https://x.pe"})); f != "phone" {
		t.Fatalf("teléfono inválido: %q", f)
	}
	if f := field(s.SaveProvider(ctx, actor, domain.ProviderInfo{Phone: "999888777", URL: "x.pe"})); f != "url" {
		t.Fatalf("url inválida: %q", f)
	}
	if err := s.SaveProvider(ctx, actor, domain.ProviderInfo{Phone: " +51 999 888 777 ", URL: "https://radios.pe/contacto"}); err != nil {
		t.Fatal(err)
	}
	if p, _ := s.Provider(ctx); p.Phone != "+51 999 888 777" || p.URL != "https://radios.pe/contacto" {
		t.Fatalf("guardado: %+v", p)
	}
}

func TestCompareVersions(t *testing.T) {
	for _, c := range []struct {
		a, b string
		want int
	}{{"1.5.0", "1.4.9", 1}, {"1.10.0", "1.9.3", 1}, {"1.4.0", "1.4.0", 0}, {"0.9", "1.0.0", -1}, {"2.0.0", "10.0.0", -1}} {
		if got := emisor.CompareVersions(c.a, c.b); got != c.want {
			t.Errorf("CompareVersions(%s, %s) = %d", c.a, c.b, got)
		}
	}
}

func TestPublishReleases(t *testing.T) {
	ctx := context.Background()
	s := newService(t)
	if l, _ := s.Latest(ctx); l != nil {
		t.Fatal("sin versiones al principio")
	}
	exe := fakeExe(4096)
	rel, err := s.Publish(ctx, actor, "v1.4.0", "primera", bytes.NewReader(exe))
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(exe)
	if rel.Version != "1.4.0" || rel.Size != 4096 || rel.SHA256 != hex.EncodeToString(sum[:]) || rel.CreatedBy != actor.Email {
		t.Fatalf("versión: %+v", rel)
	}
	if b, err := os.ReadFile(s.ReleasePath(rel)); err != nil || !bytes.Equal(b, exe) {
		t.Fatalf("archivo guardado: %v", err)
	}
	// Rechazos: misma versión o menor, formato, no es .exe, vacío.
	cases := map[string]func() error{
		"misma": func() error { _, e := s.Publish(ctx, actor, "1.4.0", "", bytes.NewReader(fakeExe(2048))); return e },
		"menor": func() error { _, e := s.Publish(ctx, actor, "1.3.9", "", bytes.NewReader(fakeExe(2048))); return e },
		"forma": func() error { _, e := s.Publish(ctx, actor, "1.5", "", bytes.NewReader(fakeExe(2048))); return e },
		"no exe": func() error {
			_, e := s.Publish(ctx, actor, "1.5.0", "", strings.NewReader(strings.Repeat("x", 4096)))
			return e
		},
		"chico": func() error { _, e := s.Publish(ctx, actor, "1.5.0", "", bytes.NewReader([]byte("MZ"))); return e },
	}
	for name, f := range cases {
		if field(f()) == "" {
			t.Errorf("%s: debía rechazarse con error de validación", name)
		}
	}
	// Se conservan los archivos de las KeepReleases versiones más nuevas.
	var last *domain.EmisorRelease
	for i := 0; i < emisor.KeepReleases+1; i++ {
		last, err = s.Publish(ctx, actor, "1.5."+string(rune('0'+i)), "", bytes.NewReader(fakeExe(2048)))
		if err != nil {
			t.Fatal(err)
		}
	}
	if l, _ := s.Latest(ctx); l.ID != last.ID || l.Version != "1.5.5" {
		t.Fatalf("Latest = %+v", l)
	}
	if _, err := os.Stat(s.ReleasePath(rel)); !os.IsNotExist(err) {
		t.Fatal("el .exe de la versión más vieja debía borrarse del disco")
	}
	if err := s.DeleteRelease(ctx, actor, last.ID); err != nil {
		t.Fatal(err)
	}
	if l, _ := s.Latest(ctx); l.Version != "1.5.4" {
		t.Fatalf("tras borrar la última: %+v", l)
	}
}

func TestConfigBackup(t *testing.T) {
	s := newService(t)
	if _, _, err := s.LatestConfig(1); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("sin copias: %v", err)
	}
	raw := `{"radio":{"host":"radio.pe","password":"SECRETA","eq":{"gains":[1,2]}},"tv":{"password":"OTRA"},
		"panel":{"url":"https://radio.pe","token":"TOK"}}`
	if err := s.SaveConfig(1, []byte(raw)); err != nil {
		t.Fatal(err)
	}
	got, ts, err := s.LatestConfig(1)
	if err != nil || ts.IsZero() {
		t.Fatal(err)
	}
	for _, sec := range []string{"SECRETA", "OTRA", "TOK"} {
		if strings.Contains(string(got), sec) {
			t.Fatalf("la clave %q quedó en el servidor: %s", sec, got)
		}
	}
	if !strings.Contains(string(got), "radio.pe") || s.ConfigCount(1) != 1 || s.ConfigCount(2) != 0 {
		t.Fatalf("copia: %s", got)
	}
	if field(s.SaveConfig(1, []byte("no es json"))) != "config" {
		t.Fatal("JSON inválido debe rechazarse")
	}
	if field(s.SaveConfig(1, bytes.Repeat([]byte(" "), emisor.MaxConfigBytes+1))) != "config" {
		t.Fatal("demasiado grande debe rechazarse")
	}
}
