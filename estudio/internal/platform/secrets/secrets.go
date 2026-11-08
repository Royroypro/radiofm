package secrets

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
)

// Nombres de los secretos que guarda el estudio.
const (
	ServerToken     = "server_token"     // token Bearer del panel web (/api/studio/*)
	IcecastPassword = "icecast_password" // contraseña de fuente de Icecast
)

// Store guarda secretos cifrados en secrets.json (carpeta de datos). En Windows
// el cifrado es DPAPI (CryptProtectData) ligado a la máquina: solo esta PC puede
// descifrarlos; copiar el archivo a otra PC no sirve. Ver protect_windows.go.
type Store struct {
	mu   sync.Mutex
	path string
}

// NewStore crea el almacén en dir.
func NewStore(dir string) *Store { return &Store{path: filepath.Join(dir, "secrets.json")} }

func (s *Store) load() (map[string]string, error) {
	m := map[string]string{}
	b, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return m, nil
	}
	if err != nil {
		return nil, err
	}
	return m, json.Unmarshal(b, &m)
}

// Get devuelve un secreto ("" si no existe).
func (s *Store) Get(name string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	m, err := s.load()
	if err != nil {
		return "", err
	}
	enc, ok := m[name]
	if !ok || enc == "" {
		return "", nil
	}
	raw, err := base64.StdEncoding.DecodeString(enc)
	if err != nil {
		return "", err
	}
	plain, err := unprotect(raw)
	if err != nil {
		return "", err
	}
	return string(plain), nil
}

// Has indica si hay un secreto guardado (sin descifrarlo).
func (s *Store) Has(name string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	m, err := s.load()
	return err == nil && m[name] != ""
}

// Set guarda (o con valor "" borra) un secreto.
func (s *Store) Set(name, value string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	m, err := s.load()
	if err != nil {
		m = map[string]string{}
	}
	if value == "" {
		delete(m, name)
	} else {
		enc, err := protect([]byte(value))
		if err != nil {
			return err
		}
		m[name] = base64.StdEncoding.EncodeToString(enc)
	}
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}
