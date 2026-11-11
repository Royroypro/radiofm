// Package updater busca versiones nuevas de Club25 Estudio en el servidor de la
// radio (el mismo manifiesto que usan las herramientas), descarga el instalador
// en segundo plano, lo verifica (SHA-256) y, cuando el operador lo pide, lo
// instala en silencio y vuelve a abrir el programa.
//
// No se instala solo: actualizar corta el aire unos segundos y Windows pide
// confirmar (el programa está en Archivos de programa). El operador elige el momento.
package updater

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"radioclub25/estudio/internal/platform/tools"
)

// Status es el estado para la interfaz.
type Status struct {
	Current   string    `json:"current"`
	Latest    string    `json:"latest"`    // versión publicada en el servidor
	Available bool      `json:"available"` // hay una más nueva
	Ready     bool      `json:"ready"`     // descargada y verificada: se puede instalar
	Progress  int       `json:"progress"`  // 0–100 mientras se descarga
	Checking  bool      `json:"checking"`
	Error     string    `json:"error"`
	CheckedAt time.Time `json:"checked_at"`
}

// Service busca e instala actualizaciones.
type Service struct {
	server  func() string
	current string
	dir     string // <datos>\updates
	log     *slog.Logger
	launch  func(script string, args ...string) error // arranca el ayudante (separado del programa)

	mu        sync.Mutex
	st        Status
	installer string // ruta del instalador listo
	wake      chan struct{}
}

// New crea el servicio. dataDir es la carpeta de datos del programa.
func New(server func() string, current, dataDir string, log *slog.Logger) *Service {
	return &Service{server: server, current: current, dir: filepath.Join(dataDir, "updates"), log: log,
		launch: launchDetached, st: Status{Current: current}, wake: make(chan struct{}, 1)}
}

// Run busca al minuto de abrir y después cada 30 minutos (o al pedirlo con Check).
func (s *Service) Run(ctx context.Context) {
	if s.current == "dev" {
		return // compilación de desarrollo: no se actualiza
	}
	t := time.NewTimer(time.Minute)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		case <-s.wake:
		}
		s.check(ctx)
		t.Reset(30 * time.Minute)
	}
}

// Check pide buscar ahora (no espera).
func (s *Service) Check() {
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

// Status devuelve el estado.
func (s *Service) Status() Status {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.st
}

func (s *Service) set(f func(*Status)) {
	s.mu.Lock()
	f(&s.st)
	s.mu.Unlock()
}

func (s *Service) check(ctx context.Context) {
	s.set(func(st *Status) { st.Checking, st.Error = true, "" })
	defer s.set(func(st *Status) { st.Checking, st.CheckedAt = false, time.Now() })
	m, err := tools.Fetch(ctx, s.server())
	if err != nil {
		s.set(func(st *Status) { st.Error = err.Error() })
		return
	}
	s.set(func(st *Status) { st.Latest = m.AppVersion })
	if !Newer(m.AppVersion, s.current) || m.Installer.URL == "" || m.Installer.SHA256 == "" {
		s.set(func(st *Status) { st.Available, st.Ready = false, false })
		return
	}
	s.set(func(st *Status) { st.Available = true })
	dst := filepath.Join(s.dir, m.AppVersion, "club25-estudio-installer.exe")
	if ok, _ := sameFile(dst, m.Installer.Size); ok && s.Status().Ready {
		return // ya está
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		s.set(func(st *Status) { st.Error = err.Error() })
		return
	}
	url := m.Installer.URL
	if strings.HasPrefix(url, "/") {
		url = strings.TrimRight(s.server(), "/") + url
	}
	total := max(m.Installer.Size, 1)
	_, err = tools.Download(ctx, url, dst, m.Installer.SHA256, func(n int64) {
		s.set(func(st *Status) { st.Progress = int(n * 100 / total) })
	})
	if err != nil {
		s.set(func(st *Status) { st.Error = "descargar la actualización: " + err.Error(); st.Progress = 0 })
		return
	}
	s.mu.Lock()
	s.installer = dst
	s.st.Ready, s.st.Progress = true, 100
	s.mu.Unlock()
	s.log.Info("actualización lista", "versión", m.AppVersion)
	s.cleanOld(m.AppVersion)
}

// cleanOld borra instaladores de versiones anteriores.
func (s *Service) cleanOld(keep string) {
	entries, _ := os.ReadDir(s.dir)
	for _, e := range entries {
		if e.IsDir() && e.Name() != keep {
			_ = os.RemoveAll(filepath.Join(s.dir, e.Name()))
		}
	}
}

func sameFile(p string, size int64) (bool, error) {
	st, err := os.Stat(p)
	if err != nil {
		return false, err
	}
	return st.Size() == size, nil
}

// Install arranca el ayudante que espera a que el programa se cierre, instala
// en silencio (Windows pide confirmar) y vuelve a abrir el programa. Después de
// llamarlo, el programa debe cerrarse.
func (s *Service) Install() error {
	s.mu.Lock()
	inst, ready := s.installer, s.st.Ready
	s.mu.Unlock()
	if !ready || inst == "" {
		return errors.New("la actualización todavía no se descargó")
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	script := filepath.Join(s.dir, "actualizar.ps1")
	if err := os.WriteFile(script, []byte(helperScript), 0o644); err != nil {
		return err
	}
	return s.launch(script, "-ProcId", strconv.Itoa(os.Getpid()), "-Installer", inst, "-Exe", exe)
}

// helperScript espera a que el programa termine, corre el instalador en modo
// silencioso (/S, con la confirmación de Windows) y vuelve a abrir el programa
// (también si se cancela la confirmación).
const helperScript = `param([int]$ProcId, [string]$Installer, [string]$Exe)
try { Wait-Process -Id $ProcId -Timeout 60 -ErrorAction SilentlyContinue } catch {}
Start-Sleep -Milliseconds 500
try { Start-Process -FilePath $Installer -ArgumentList '/S' -Verb RunAs -Wait } catch {}
Start-Sleep -Seconds 1
Start-Process -FilePath $Exe
`

// Newer dice si a es una versión más nueva que b ("1.10.0" > "1.9.2").
func Newer(a, b string) bool {
	pa, pb := parts(a), parts(b)
	if pa == nil || pb == nil {
		return false
	}
	for i := 0; i < 3; i++ {
		if pa[i] != pb[i] {
			return pa[i] > pb[i]
		}
	}
	return false
}

func parts(v string) []int {
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	if i := strings.IndexAny(v, "-+"); i >= 0 {
		v = v[:i]
	}
	f := strings.Split(v, ".")
	if len(f) != 3 {
		return nil
	}
	out := make([]int, 3)
	for i, x := range f {
		n, err := strconv.Atoi(x)
		if err != nil {
			return nil
		}
		out[i] = n
	}
	return out
}

// String para los registros.
func (st Status) String() string {
	return fmt.Sprintf("actual %s, servidor %s, lista %v", st.Current, st.Latest, st.Ready)
}
