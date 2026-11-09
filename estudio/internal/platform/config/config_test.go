package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStoreRoundTripAndDefaults(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(dir)
	cfg, err := s.Load()
	if err != nil || cfg.Audio.CrossfadeMs != 3000 || cfg.Icecast.Port != 8010 {
		t.Fatalf("sin archivo debe dar los valores por defecto: %+v, %v", cfg, err)
	}
	cfg.Audio.OutputDevice = "Altavoces (M-Audio)"
	cfg.Icecast.Source = "capture"
	if err := s.Save(cfg); err != nil {
		t.Fatal(err)
	}
	got, err := s.Load()
	if err != nil || got.Audio.OutputDevice != cfg.Audio.OutputDevice || got.Icecast.Source != "capture" {
		t.Fatalf("Load = %+v, %v", got, err)
	}
	// un archivo viejo sin campos nuevos conserva los valores por defecto
	if err := os.WriteFile(filepath.Join(dir, "settings.json"), []byte(`{"audio":{"output_device":"X"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	got, _ = s.Load()
	if got.Audio.OutputDevice != "X" || got.Audio.DuckDepthDB != -12 || got.Station.Timezone != "America/Lima" {
		t.Fatalf("campos faltantes deben tomar el valor por defecto: %+v", got)
	}
	bad := Defaults()
	bad.Icecast.Source = "otra"
	if err := s.Save(bad); err == nil {
		t.Fatal("fuente de Icecast inválida debe rechazarse")
	}
}

// 1.0.7: quien tenía guardado el dominio anterior pasa a radioclub25peru.com;
// una URL puesta a mano se respeta.
func TestLoadMigratesOldServerURL(t *testing.T) {
	for old, want := range map[string]string{
		"https://radioclub25.sehuacho.com":  ServerURL,
		"https://radioclub25.sehuacho.com/": ServerURL,
		"https://otra-radio.pe":             "https://otra-radio.pe",
	} {
		dir := t.TempDir()
		body := `{"server":{"url":"` + old + `","token_set":true},"icecast":{"host":"radioclub25.sehuacho.com","port":8010,"mount":"/radio"}}`
		if err := os.WriteFile(filepath.Join(dir, "settings.json"), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		got, err := NewStore(dir).Load()
		if err != nil || got.Server.URL != want {
			t.Errorf("%s → %q, %v (quería %q)", old, got.Server.URL, err, want)
		}
		if got.Icecast.Host != IngestHost {
			t.Errorf("el audio sigue yendo a %s, dio %q", IngestHost, got.Icecast.Host)
		}
	}
	if d := Defaults(); d.Server.URL != ServerURL || d.Icecast.Host != IngestHost {
		t.Fatalf("valores por defecto = %+v / %+v", d.Server, d.Icecast)
	}
}
