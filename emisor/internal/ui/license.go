package ui

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"time"

	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"radioclub25/emisor/internal/config"
	"radioclub25/emisor/internal/license"
	"radioclub25/emisor/internal/runner"
	"radioclub25/emisor/internal/secret"
	"radioclub25/emisor/internal/updater"
)

// LicenseCheckEvery es cada cuánto se vuelve a autenticar con la web.
var LicenseCheckEvery = 30 * time.Minute

// HeartbeatEvery es cada cuánto el emisor avisa al panel que está funcionando.
// También es lo que tarda, como máximo, en notar un token revocado o reemplazado.
var HeartbeatEvery = time.Minute

// BackupDelay es la espera tras el último cambio antes de subir la copia de la configuración.
var BackupDelay = 8 * time.Second

// ErrLocked: el programa no está autenticado (no se puede emitir ni aplicar cambios).
var ErrLocked = errors.New("El programa no está autenticado. Autentíquelo con el dominio y el token de su radio, " +
	"o contáctese con el proveedor para adquirir una licencia.")

// licState es la licencia del programa (autenticación contra la web).
type licState struct {
	mu        sync.Mutex
	st        license.State
	cache     *license.Cache
	path      string        // archivo cifrado con la última autenticación correcta
	firstDone chan struct{} // se cierra tras la primera comprobación
	once      sync.Once

	backupMu    sync.Mutex
	backupTimer *time.Timer
	backupOK    bool // ya se avisó en el registro que la copia funciona

	updating sync.Mutex // una sola actualización a la vez
}

// UpdateInfo es la actualización disponible (si la hay).
type UpdateInfo struct {
	Available bool   `json:"available"`
	Current   string `json:"current"`
	Version   string `json:"version"`
	Notes     string `json:"notes"`
	Size      int64  `json:"size"`
}

// ---------- arranque ----------

// initLicense carga la última autenticación guardada y empieza a comprobarla.
func (a *App) initLicense(ctx context.Context, dataDir string) {
	a.lic.firstDone = make(chan struct{})
	a.lic.path = filepath.Join(dataDir, "licencia.dat")
	if b, err := os.ReadFile(a.lic.path); err == nil {
		if plain, err := secret.Open(string(b)); err == nil {
			var c license.Cache
			if json.Unmarshal([]byte(plain), &c) == nil {
				a.lic.cache = &c
			}
		}
	}
	// Mientras llega la primera respuesta, vale la última autenticación (si corresponde).
	a.mu.Lock()
	base, token := a.cfg.Panel.URL, a.cfg.Panel.Token
	a.mu.Unlock()
	st, _ := license.Decide(base, token, license.Info{}, &license.OfflineError{Err: errors.New("comprobando…")}, a.lic.cache, time.Now())
	if !st.Authenticated {
		st.Reason = ""
	}
	a.lic.st = st
	go a.licenseLoop(ctx)
}

func (a *App) licenseLoop(ctx context.Context) {
	a.CheckLicense()
	t := time.NewTicker(LicenseCheckEvery)
	defer t.Stop()
	// Se mira cada 5 s: el latido sale cada HeartbeatEvery o en cuanto la radio o
	// la TV cambian de estado (al aire, detenida…), para que el panel lo vea en seguida.
	poll := time.NewTicker(5 * time.Second)
	defer poll.Stop()
	var lastSent time.Time
	var lastStates string
	send := func() {
		lastSent, lastStates = time.Now(), a.outputStates()
		a.sendHeartbeat()
	}
	send()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			a.CheckLicense()
		case <-poll.C:
			if time.Since(lastSent) >= HeartbeatEvery || a.outputStates() != lastStates {
				send()
			}
		}
	}
}

// outputStates resume el estado de las dos salidas ("en_vivo/detenido").
func (a *App) outputStates() string {
	if a.radio == nil || a.tv == nil {
		return ""
	}
	return string(a.radio.Status().State) + "/" + string(a.tv.Status().State)
}

// sendHeartbeat avisa al panel que el emisor funciona y qué está emitiendo. Si
// el servidor rechaza el token (revocado o reemplazado), se vuelve a autenticar
// en seguida, lo que bloquea el programa y detiene la emisión.
func (a *App) sendHeartbeat() {
	st := a.LicenseState()
	if !st.Authenticated || st.Base == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	latest, err := license.Heartbeat(ctx, st.Base, a.token(), Version, a.heartbeatReport())
	switch {
	case errors.Is(err, license.ErrUnauthorized), errors.Is(err, license.ErrSuspended):
		a.CheckLicense()
	case err != nil:
		// Sin conexión: no se avisa cada minuto en el registro (ya lo indica la licencia).
	case latest != "" && updater.Newer(latest, Version) && (st.Latest == nil || st.Latest.Version != latest):
		// Se publicó una versión nueva: se trae su información (tamaño, SHA-256) para ofrecerla ya.
		a.CheckLicense()
	}
}

// heartbeatReport arma el estado de las salidas para el panel (sin claves).
func (a *App) heartbeatReport() license.Report {
	a.mu.Lock()
	c := a.cfg
	a.mu.Unlock()
	out := func(s runnerStatus, target string) license.Output {
		return license.Output{State: string(s.State), BitrateKbps: s.BitrateKbps, Message: s.Message, Target: target}
	}
	tvPort := c.TV.RTMPPort
	if c.TV.Protocol == config.ProtoSRT {
		tvPort = c.TV.SRTPort
	}
	return license.Report{
		Radio:      out(a.radio.Status(), fmt.Sprintf("%s:%d%s", c.Radio.Host, c.Radio.Port, c.Radio.Mount)),
		TV:         out(a.tv.Status(), fmt.Sprintf("%s://%s:%d/%s", c.TV.Protocol, c.TV.Host, tvPort, c.TV.Path)),
		NowPlaying: a.NowPlaying().Title,
	}
}

// waitFirstCheck espera la primera comprobación (para emitir solo al abrir).
func (a *App) waitFirstCheck(ctx context.Context) {
	select {
	case <-ctx.Done():
	case <-a.lic.firstDone:
	case <-time.After(30 * time.Second):
	}
}

// ---------- lo que llama la ventana ----------

// CheckLicense autentica con el dominio y el token guardados y avisa a la ventana.
func (a *App) CheckLicense() license.State {
	a.mu.Lock()
	base, token := a.cfg.Panel.URL, a.cfg.Panel.Token
	a.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	info, err := license.Check(ctx, base, token)
	a.lic.mu.Lock()
	prev := a.lic.st
	st, cache := license.Decide(base, token, info, err, a.lic.cache, time.Now())
	if !st.Authenticated && st.Provider == license.DefaultProvider && base != "" {
		// Sin autenticar: el contacto del proveedor se pide a la web (endpoint público).
		if p, perr := license.FetchProvider(ctx, base); perr == nil {
			st.Provider = p
		}
	}
	a.lic.st, a.lic.cache = st, cache
	a.lic.mu.Unlock()
	a.saveLicenseCache(cache)
	a.lic.once.Do(func() { close(a.lic.firstDone) })
	if prev.Authenticated && !st.Authenticated && (errors.Is(err, license.ErrUnauthorized) || errors.Is(err, license.ErrSuspended)) {
		// Token revocado o reemplazado en el panel (o servicio suspendido): se bloquea
		// el programa y se detiene lo que esté al aire. Un corte de internet no hace esto.
		if a.radio != nil && (a.radio.Running() || a.tv.Running()) {
			a.log("Licencia: %s Se detiene la emisión.", st.Reason)
			go a.Stop("todo")
		}
	}

	switch {
	case st.Authenticated && !prev.Authenticated:
		if st.Offline {
			a.log("Licencia: %s", st.Reason)
		} else {
			a.log("Licencia: autenticado con %s (%s %s).", st.Base, st.Station.Name, st.Station.Frequency)
		}
		a.scheduleBackup()
	case !st.Authenticated && st.Reason != "" && st.Reason != prev.Reason:
		a.log("Licencia: %s", st.Reason)
	}
	if u := a.updateInfo(); u.Available && (prev.Latest == nil || prev.Latest.Version != u.Version) {
		a.log("Actualización disponible: versión %s (actual %s).", u.Version, Version)
	}
	a.emitLicense()
	return st
}

// Authenticate guarda el dominio y el token y autentica. Devuelve error si no se pudo.
func (a *App) Authenticate(base, token string) (license.State, error) {
	nb, err := license.NormalizeBase(base)
	if err != nil {
		return a.LicenseState(), err
	}
	a.mu.Lock()
	c := a.cfg
	a.mu.Unlock()
	c.Panel.URL, c.Panel.Token = nb, token
	if err := a.saveConfigQuiet(c); err != nil {
		return a.LicenseState(), err
	}
	st := a.CheckLicense()
	if !st.Authenticated {
		return st, errors.New(st.Reason)
	}
	return st, nil
}

// LicenseState devuelve el estado de la licencia.
func (a *App) LicenseState() license.State {
	a.lic.mu.Lock()
	defer a.lic.mu.Unlock()
	return a.lic.st
}

func (a *App) authenticated() bool {
	a.lic.mu.Lock()
	defer a.lic.mu.Unlock()
	return a.lic.st.Authenticated
}

// requireAuth bloquea emitir y aplicar cambios sin autenticación.
func (a *App) requireAuth() error {
	if a.authenticated() {
		return nil
	}
	return ErrLocked
}

// OpenURL abre una dirección http(s) en el navegador (contacto del proveedor).
func (a *App) OpenURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
		return errors.New("Dirección no válida.")
	}
	if a.ctx != nil {
		wruntime.BrowserOpenURL(a.ctx, u.String())
	}
	return nil
}

func (a *App) emitLicense() {
	if a.ctx != nil {
		a.Emit(a.ctx, "licencia", map[string]any{"license": a.LicenseState(), "update": a.updateInfo()})
	}
}

func (a *App) saveLicenseCache(c *license.Cache) {
	if a.lic.path == "" {
		return
	}
	if c == nil {
		_ = os.Remove(a.lic.path)
		return
	}
	b, err := json.Marshal(c)
	if err != nil {
		return
	}
	if enc, err := secret.Seal(string(b)); err == nil {
		_ = os.WriteFile(a.lic.path, []byte(enc), 0o600)
	}
}

// ---------- copia de la configuración en el servidor ----------

// sanitized es la configuración sin claves (lo que se sube al servidor).
func sanitized(c config.Config) config.Config {
	c.Radio.Password, c.TV.Password, c.Panel.Token = "", "", ""
	return c
}

// scheduleBackup sube la configuración BackupDelay después del último cambio.
func (a *App) scheduleBackup() {
	a.lic.backupMu.Lock()
	defer a.lic.backupMu.Unlock()
	if a.lic.backupTimer != nil {
		a.lic.backupTimer.Stop()
	}
	a.lic.backupTimer = time.AfterFunc(BackupDelay, a.uploadBackup)
}

func (a *App) uploadBackup() {
	st := a.LicenseState()
	if !st.Authenticated || st.Offline {
		return
	}
	a.mu.Lock()
	c := sanitized(a.cfg)
	a.mu.Unlock()
	b, err := json.Marshal(c)
	if err != nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err := license.UploadConfig(ctx, c.Panel.URL, a.token(), b); err != nil {
		a.log("Copia de la configuración en el servidor: %v", err)
		return
	}
	a.lic.backupMu.Lock()
	first := !a.lic.backupOK
	a.lic.backupOK = true
	a.lic.backupMu.Unlock()
	if first {
		a.log("Copia de la configuración guardada en el servidor (se actualiza sola con cada cambio).")
	}
}

func (a *App) token() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.cfg.Panel.Token
}

// RestoreConfig trae la última copia del servidor. Conserva las claves, el
// dominio, el token y el arranque de esta PC. Devuelve la configuración y la fecha.
func (a *App) RestoreConfig() (map[string]any, error) {
	if err := a.requireAuth(); err != nil {
		return nil, err
	}
	a.mu.Lock()
	cur := a.cfg
	a.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	raw, ts, err := license.DownloadConfig(ctx, cur.Panel.URL, cur.Panel.Token)
	if err != nil {
		return nil, err
	}
	c := cur
	if err := json.Unmarshal(raw, &c); err != nil {
		return nil, fmt.Errorf("La copia del servidor no se pudo leer: %w", err)
	}
	c.Radio.Password, c.TV.Password = cur.Radio.Password, cur.TV.Password
	c.Panel, c.Startup, c.FFmpegPath = cur.Panel, cur.Startup, cur.FFmpegPath
	if err := a.SaveConfig(c); err != nil {
		return nil, err
	}
	a.log("Configuración restaurada desde el servidor (copia del %s).", ts.Local().Format("02/01/2006 15:04"))
	return map[string]any{"config": c, "saved_at": ts}, nil
}

// ---------- actualización ----------

func (a *App) updateInfo() UpdateInfo {
	st := a.LicenseState()
	u := UpdateInfo{Current: Version}
	if st.Authenticated && st.Latest != nil && updater.Newer(st.Latest.Version, Version) {
		u.Available, u.Version, u.Notes, u.Size = true, st.Latest.Version, st.Latest.Notes, st.Latest.Size
	}
	return u
}

// UpdateState devuelve la actualización disponible.
func (a *App) UpdateState() UpdateInfo { return a.updateInfo() }

// ApplyUpdate descarga la versión nueva, la verifica, la instala y reinicia el
// programa. Lo que estaba al aire se reanuda solo en la versión nueva (corte de
// unos segundos). Si algo falla, el programa actual sigue igual y al aire.
func (a *App) ApplyUpdate() error {
	if !a.lic.updating.TryLock() {
		return errors.New("Ya se está actualizando.")
	}
	defer a.lic.updating.Unlock()
	if err := a.requireAuth(); err != nil {
		return err
	}
	st := a.LicenseState()
	u := a.updateInfo()
	if !u.Available || st.Latest == nil {
		return errors.New("No hay una actualización disponible.")
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	if exe, err = filepath.EvalSymlinks(exe); err != nil {
		return err
	}
	progress := func(stage string, pct int) {
		if a.ctx != nil {
			a.Emit(a.ctx, "actualizacion", map[string]any{"stage": stage, "pct": pct})
		}
	}
	a.log("Actualizando a la versión %s: descargando…", u.Version)
	progress("descargando", 0)
	newExe := exe + ".new"
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()
	err = license.Download(ctx, st.Base, a.token(), *st.Latest, newExe, func(done, total int64) {
		if total > 0 {
			progress("descargando", int(done*100/total))
		}
	})
	if err != nil {
		a.log("Actualización: %v", err)
		return err
	}
	progress("instalando", 100)
	var resume []string
	if a.radio.Running() {
		resume = append(resume, "radio")
	}
	if a.tv.Running() {
		resume = append(resume, "tv")
	}
	if err := updater.Swap(exe, newExe); err != nil {
		_ = os.Remove(newExe)
		a.log("Actualización: %v", err)
		return err
	}
	if err := updater.Relaunch(exe, updater.RelaunchArgs(os.Getpid(), resume, a.DataDir)); err != nil {
		// El programa nuevo no abrió: se vuelve al anterior y se sigue al aire.
		_ = os.Rename(exe, newExe)
		_ = os.Rename(exe+".old", exe)
		a.log("Actualización: %v", err)
		return err
	}
	a.log("Versión %s instalada: se reinicia el programa%s.", u.Version,
		map[bool]string{true: " y vuelve a emitir solo", false: ""}[len(resume) > 0])
	progress("reiniciando", 100)
	go func() {
		time.Sleep(300 * time.Millisecond) // que la ventana alcance a mostrar «reiniciando»
		a.quitting.Store(true)
		a.Stop("todo")
		if a.ctx != nil {
			wruntime.Quit(a.ctx)
		}
	}()
	return nil
}

// resumeOutputs vuelve a emitir lo que estaba al aire antes de actualizar.
func (a *App) resumeOutputs(ctx context.Context, which []string) {
	a.waitFirstCheck(ctx)
	if !a.authenticated() {
		a.log("Tras actualizar: el programa no está autenticado; no se reanuda la emisión.")
		return
	}
	a.mu.Lock()
	c := a.cfg
	a.mu.Unlock()
	for _, w := range which {
		if err := a.Start(w, c); err != nil {
			a.log("Tras actualizar: %v", err)
		}
	}
	a.log("Tras actualizar a la versión %s: emisión reanudada (%v).", Version, which)
}

type runnerStatus = runner.Status
