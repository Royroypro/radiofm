// Package emisor atiende al programa Club25 Emisor (carpeta emisor/ del repositorio):
//
//   - contacto del proveedor del servicio (teléfono y web) que el programa
//     muestra cuando no está autenticado; lo edita el superadmin;
//   - versiones publicadas del programa (.exe) para la actualización automática:
//     el superadmin las sube, las PC autenticadas las descargan;
//   - copia de la configuración de cada PC (sin claves), por frecuencia.
//
// No sabe de HTTP: los handlers están en superadmin y studioapi. Los archivos
// van a disco (DATA_DIR/emisor y la carpeta de copias del estudio), no a /uploads:
// no son públicos.
package emisor

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"radioclub25/internal/domain"
	"radioclub25/internal/service/audit"
)

// Límites.
const (
	MaxReleaseBytes = 100 << 20 // un .exe del emisor pesa ~13 MB
	MaxConfigBytes  = 256 << 10 // configuración JSON del emisor
	KeepReleases    = 5         // archivos .exe que se conservan en disco
	KeepConfigs     = 30        // copias de configuración por frecuencia
	ReleaseFileName = "club25-emisor.exe"
)

var (
	versionRe = regexp.MustCompile(`^\d{1,3}\.\d{1,3}\.\d{1,4}$`)
	phoneRe   = regexp.MustCompile(`^\+?[0-9 ]{6,20}$`)
)

// Service gestiona proveedor, versiones y copias de configuración.
type Service struct {
	settings   domain.SettingsRepository
	releases   domain.EmisorReleaseRepository
	audit      *audit.Service
	releaseDir string // DATA_DIR/emisor
	backupDir  string // carpeta de copias de las PC del estudio (por frecuencia)
	now        func() time.Time
}

// New crea el servicio.
func New(settings domain.SettingsRepository, releases domain.EmisorReleaseRepository, auditSvc *audit.Service,
	releaseDir, backupDir string) *Service {
	return &Service{settings: settings, releases: releases, audit: auditSvc, releaseDir: releaseDir,
		backupDir: backupDir, now: time.Now}
}

// ---------- proveedor ----------

// Provider devuelve el contacto del proveedor (o el de fábrica si no se cambió).
func (s *Service) Provider(ctx context.Context) (domain.ProviderInfo, error) {
	p := domain.DefaultProviderInfo()
	m, err := s.settings.GetAll(ctx)
	if err != nil {
		return p, err
	}
	if v := m[domain.ProviderPhoneKey]; v != "" {
		p.Phone = v
	}
	if v := m[domain.ProviderURLKey]; v != "" {
		p.URL = v
	}
	return p, nil
}

// SaveProvider valida y guarda el contacto del proveedor.
func (s *Service) SaveProvider(ctx context.Context, actor domain.Actor, p domain.ProviderInfo) error {
	p.Phone = strings.TrimSpace(p.Phone)
	p.URL = strings.TrimSpace(p.URL)
	if !phoneRe.MatchString(p.Phone) {
		return domain.Invalid("phone", "Escriba el número del proveedor (solo dígitos, por ejemplo 9488933154).")
	}
	if u, err := url.Parse(p.URL); err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
		return domain.Invalid("url", "La dirección debe empezar con https:// (por ejemplo https://sehuacho.com/radio).")
	}
	if err := s.settings.SetMany(ctx, map[string]string{domain.ProviderPhoneKey: p.Phone, domain.ProviderURLKey: p.URL}); err != nil {
		return err
	}
	s.audit.Log(ctx, actor, "update", "provider", 0, "%s %s", p.Phone, p.URL)
	return nil
}

// ---------- versiones del programa ----------

// CompareVersions compara "1.10.0" con "1.9.3": -1, 0 o 1.
func CompareVersions(a, b string) int {
	pa, pb := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < 3; i++ {
		var x, y int
		if i < len(pa) {
			x, _ = strconv.Atoi(pa[i])
		}
		if i < len(pb) {
			y, _ = strconv.Atoi(pb[i])
		}
		switch {
		case x < y:
			return -1
		case x > y:
			return 1
		}
	}
	return 0
}

// Releases lista las versiones publicadas (más nuevas primero).
func (s *Service) Releases(ctx context.Context) ([]domain.EmisorRelease, error) {
	list, err := s.releases.List(ctx)
	sort.SliceStable(list, func(i, j int) bool { return CompareVersions(list[i].Version, list[j].Version) > 0 })
	return list, err
}

// Latest devuelve la versión más alta publicada (nil si no hay).
func (s *Service) Latest(ctx context.Context) (*domain.EmisorRelease, error) {
	list, err := s.Releases(ctx)
	if err != nil || len(list) == 0 {
		return nil, err
	}
	return &list[0], nil
}

// ReleasePath es dónde está el .exe de una versión.
func (s *Service) ReleasePath(r *domain.EmisorRelease) string {
	return filepath.Join(s.releaseDir, strconv.FormatInt(r.ID, 10), r.FileName)
}

// Publish guarda una versión nueva del programa. body es el .exe (se comprueba
// que sea un ejecutable de Windows) y la versión debe ser mayor que todas las publicadas.
func (s *Service) Publish(ctx context.Context, actor domain.Actor, version, notes string, body io.Reader) (*domain.EmisorRelease, error) {
	version = strings.TrimPrefix(strings.TrimSpace(version), "v")
	if !versionRe.MatchString(version) {
		return nil, domain.Invalid("version", "La versión debe tener la forma 1.5.0.")
	}
	if latest, err := s.Latest(ctx); err != nil {
		return nil, err
	} else if latest != nil && CompareVersions(version, latest.Version) <= 0 {
		return nil, domain.Invalid("version", fmt.Sprintf("La versión debe ser mayor que la publicada (%s).", latest.Version))
	}
	if body == nil {
		return nil, domain.Invalid("file", "Elija el archivo club25-emisor.exe.")
	}
	if err := os.MkdirAll(s.releaseDir, 0o750); err != nil {
		return nil, err
	}
	tmp, err := os.CreateTemp(s.releaseDir, "subida-*.part")
	if err != nil {
		return nil, err
	}
	defer os.Remove(tmp.Name())
	h := sha256.New()
	var head bytes.Buffer
	n, err := io.Copy(io.MultiWriter(tmp, h, &limitedBuf{b: &head, max: 2}), io.LimitReader(body, MaxReleaseBytes+1))
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return nil, err
	}
	switch {
	case n > MaxReleaseBytes:
		return nil, domain.Invalid("file", fmt.Sprintf("El archivo pasa de %d MB.", MaxReleaseBytes>>20))
	case n < 1024 || head.String() != "MZ":
		return nil, domain.Invalid("file", "El archivo no es un programa de Windows (.exe).")
	}
	rel := &domain.EmisorRelease{Version: version, FileName: ReleaseFileName, SHA256: hex.EncodeToString(h.Sum(nil)),
		Size: n, Notes: strings.TrimSpace(notes), CreatedAt: s.now(), CreatedBy: actor.Email}
	if err := s.releases.Create(ctx, rel); err != nil {
		return nil, err
	}
	dst := s.ReleasePath(rel)
	if err := os.MkdirAll(filepath.Dir(dst), 0o750); err == nil {
		err = os.Rename(tmp.Name(), dst)
		if err != nil {
			_ = s.releases.Delete(ctx, rel.ID)
			return nil, err
		}
	} else {
		_ = s.releases.Delete(ctx, rel.ID)
		return nil, err
	}
	s.audit.Log(ctx, actor, "create", "emisor_release", rel.ID, "%s (%d bytes, sha256 %s)", rel.Version, rel.Size, rel.SHA256[:12])
	s.prune(ctx)
	return rel, nil
}

// DeleteRelease borra una versión (registro y archivo).
func (s *Service) DeleteRelease(ctx context.Context, actor domain.Actor, id int64) error {
	rel, err := s.releases.Get(ctx, id)
	if err != nil {
		return err
	}
	if err := s.releases.Delete(ctx, id); err != nil {
		return err
	}
	_ = os.RemoveAll(filepath.Dir(s.ReleasePath(rel)))
	s.audit.Log(ctx, actor, "delete", "emisor_release", id, "%s", rel.Version)
	return nil
}

// prune deja en disco solo los archivos de las KeepReleases versiones más nuevas
// (los registros viejos quedan como historial, sin archivo).
func (s *Service) prune(ctx context.Context) {
	list, err := s.Releases(ctx)
	if err != nil {
		return
	}
	for i := KeepReleases; i < len(list); i++ {
		_ = os.RemoveAll(filepath.Dir(s.ReleasePath(&list[i])))
	}
}

// limitedBuf guarda los primeros max bytes que pasan por él.
type limitedBuf struct {
	b   *bytes.Buffer
	max int
}

func (l *limitedBuf) Write(p []byte) (int, error) {
	if rest := l.max - l.b.Len(); rest > 0 {
		if len(p) < rest {
			rest = len(p)
		}
		l.b.Write(p[:rest])
	}
	return len(p), nil
}

// ---------- copia de la configuración de la PC ----------

// secretKeys son campos que nunca se guardan en el servidor, aunque lleguen.
var secretKeys = map[string]bool{"password": true, "token": true, "pass": true}

// SaveConfig guarda la configuración del emisor de una frecuencia (JSON). Las
// claves que vengan se borran antes de guardar. Quedan las KeepConfigs últimas.
func (s *Service) SaveConfig(fid int64, raw []byte) error {
	if len(raw) > MaxConfigBytes {
		return domain.Invalid("config", "La configuración es demasiado grande.")
	}
	var v map[string]any
	if err := json.Unmarshal(raw, &v); err != nil || v == nil {
		return domain.Invalid("config", "La configuración no es un JSON válido.")
	}
	stripSecrets(v)
	out, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	dir := filepath.Join(s.backupDir, strconv.FormatInt(fid, 10))
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return err
	}
	name := "emisor-config-" + s.now().UTC().Format("20060102-150405") + ".json"
	tmp := filepath.Join(dir, name+".part")
	if err := os.WriteFile(tmp, out, 0o640); err != nil {
		return err
	}
	if err := os.Rename(tmp, filepath.Join(dir, name)); err != nil {
		return err
	}
	files, _ := filepath.Glob(filepath.Join(dir, "emisor-config-*.json"))
	sort.Strings(files)
	for len(files) > KeepConfigs {
		_ = os.Remove(files[0])
		files = files[1:]
	}
	return nil
}

// LatestConfig devuelve la última copia de la configuración de una frecuencia.
func (s *Service) LatestConfig(fid int64) ([]byte, time.Time, error) {
	files, _ := filepath.Glob(filepath.Join(s.backupDir, strconv.FormatInt(fid, 10), "emisor-config-*.json"))
	if len(files) == 0 {
		return nil, time.Time{}, domain.ErrNotFound
	}
	sort.Strings(files)
	last := files[len(files)-1]
	b, err := os.ReadFile(last)
	if err != nil {
		return nil, time.Time{}, err
	}
	ts, _ := time.Parse("20060102-150405", strings.TrimSuffix(strings.TrimPrefix(filepath.Base(last), "emisor-config-"), ".json"))
	return b, ts, nil
}

// ConfigCount cuenta las copias guardadas de una frecuencia (para el panel).
func (s *Service) ConfigCount(fid int64) int {
	files, _ := filepath.Glob(filepath.Join(s.backupDir, strconv.FormatInt(fid, 10), "emisor-config-*.json"))
	return len(files)
}

func stripSecrets(v any) {
	switch t := v.(type) {
	case map[string]any:
		for k, val := range t {
			if secretKeys[strings.ToLower(k)] {
				t[k] = ""
				continue
			}
			stripSecrets(val)
		}
	case []any:
		for _, e := range t {
			stripSecrets(e)
		}
	}
}
