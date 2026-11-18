package ui

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"radioclub25/emisor/internal/config"
	"radioclub25/emisor/internal/nowplaying"
	"radioclub25/emisor/internal/panel"
	runnerpkg "radioclub25/emisor/internal/runner"
)

// TitlePoll es cada cuánto se mira el archivo de Jazler (o de otro programa).
var TitlePoll = 1500 * time.Millisecond

// NowPlaying es lo que muestra la ventana en «Sonando ahora».
type NowPlaying struct {
	Title  string `json:"title"`  // último título enviado (o por enviar)
	Source string `json:"source"` // manual, jazler, file
	Kind   string `json:"kind"`   // song, spot, idle, texto…
	Status string `json:"status"` // explicación: archivo no encontrado, Opus no admite títulos…
	Sent   bool   `json:"sent"`   // ya llegó a Icecast
}

// titles guarda el estado del envío de títulos.
type titles struct {
	mu        sync.Mutex
	np        NowPlaying
	want      string    // título que debería estar en Icecast
	sentTitle string    // último título que aceptó Icecast
	sentLive  time.Time // conexión de la radio en la que se envió (si cambia, se reenvía)
	fileMod   time.Time
	fileSize  int64
	lastErr   string
}

// titleLoop vigila el archivo de lo que suena y mantiene el título en Icecast.
func (a *App) titleLoop(ctx context.Context) {
	t := time.NewTicker(TitlePoll)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			a.pollTitleFile()
			a.syncTitle()
		}
	}
}

// pollTitleFile lee el archivo de Jazler (u otro programa) si cambió.
func (a *App) pollTitleFile() {
	a.mu.Lock()
	tc := a.cfg.Radio.Titles
	a.mu.Unlock()
	if tc.Source != config.TitlesJazler && tc.Source != config.TitlesFile {
		return
	}
	path := tc.File
	if path == "" && tc.Source == config.TitlesJazler {
		path = config.JazlerNowOnAir
	}
	st, err := os.Stat(path)
	if err != nil {
		a.setTitleStatus(tc.Source, "No se encuentra "+path+(map[bool]string{true: " (¿Jazler está instalado en otra carpeta?)"}[tc.Source == config.TitlesJazler]))
		return
	}
	a.tt.mu.Lock()
	same := st.ModTime().Equal(a.tt.fileMod) && st.Size() == a.tt.fileSize
	a.tt.mu.Unlock()
	if same {
		return
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return // Jazler lo está escribiendo: próxima vuelta
	}
	tr, ok := nowplaying.Parse(data)
	if !ok {
		return // a medio escribir
	}
	a.tt.mu.Lock()
	a.tt.fileMod, a.tt.fileSize = st.ModTime(), st.Size()
	want := tr.Title
	if !tr.IsSong {
		want = strings.TrimSpace(tc.IdleText) // cuñas/jingles/pausa: texto de la radio ("" = dejar el anterior)
		if want == "" {
			want = a.tt.want
		}
	}
	changed := want != a.tt.want
	a.tt.want = want
	a.tt.np = NowPlaying{Title: want, Source: tc.Source, Kind: tr.Kind, Sent: want == a.tt.sentTitle}
	a.tt.mu.Unlock()
	if changed {
		a.log("Sonando (%s): %s", map[bool]string{true: "Jazler", false: "archivo"}[tc.Source == config.TitlesJazler], orDash(want))
	}
	a.emitNowPlaying()
}

func orDash(s string) string {
	if s == "" {
		return "—"
	}
	return s
}

// syncTitle manda a Icecast el título pendiente cuando la radio está al aire,
// y lo reenvía si la radio se reconectó (Icecast lo olvida con cada conexión).
func (a *App) syncTitle() {
	st := a.radio.Status()
	if st.State != runnerpkg.Live {
		return
	}
	a.tt.mu.Lock()
	want, sent, sentLive := a.tt.want, a.tt.sentTitle, a.tt.sentLive
	a.tt.mu.Unlock()
	if want == "" || (want == sent && sentLive.Equal(st.LiveSince)) {
		return
	}
	// Esperar 2 s tras conectar: Icecast tarda un instante en tener el montaje listo.
	if time.Since(st.LiveSince) < 2*time.Second {
		return
	}
	if err := a.pushTitle(want); err != nil {
		a.tt.mu.Lock()
		repeat := a.tt.lastErr == err.Error()
		a.tt.lastErr = err.Error()
		a.tt.np.Status = err.Error()
		a.tt.np.Sent = false
		a.tt.mu.Unlock()
		if !repeat {
			a.log("Título: %v", err)
		}
		a.emitNowPlaying()
		return
	}
	a.tt.mu.Lock()
	a.tt.sentTitle, a.tt.sentLive, a.tt.lastErr = want, st.LiveSince, ""
	a.tt.np.Title, a.tt.np.Sent, a.tt.np.Status = want, true, ""
	a.tt.mu.Unlock()
	a.log("Título enviado a Icecast: %s", want)
	a.emitNowPlaying()
}

func (a *App) pushTitle(title string) error {
	a.mu.Lock()
	r := a.cfg.Radio
	a.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	return panel.UpdateSong(ctx, r, title)
}

func (a *App) setTitleStatus(source, status string) {
	a.tt.mu.Lock()
	changed := a.tt.np.Status != status
	a.tt.np.Source, a.tt.np.Status = source, status
	a.tt.mu.Unlock()
	if changed {
		a.log("Título: %s", status)
		a.emitNowPlaying()
	}
}

func (a *App) emitNowPlaying() {
	if a.ctx != nil {
		a.Emit(a.ctx, "sonando", a.NowPlaying())
	}
}

// NowPlaying devuelve lo que suena según el emisor.
func (a *App) NowPlaying() NowPlaying {
	a.tt.mu.Lock()
	defer a.tt.mu.Unlock()
	return a.tt.np
}

// SendSong pone un título a mano (o desde la lista de canciones). Si la radio
// no está al aire, queda pendiente y se envía al conectar.
func (a *App) SendSong(title string) error {
	title = strings.TrimSpace(title)
	if title == "" {
		return errors.New("Escriba el título.")
	}
	a.tt.mu.Lock()
	a.tt.want = title
	a.tt.np = NowPlaying{Title: title, Source: config.TitlesManual, Kind: "song"}
	a.tt.mu.Unlock()
	if a.radio.Status().State != runnerpkg.Live {
		a.log("Título pendiente (se enviará al estar la radio al aire): %s", title)
		a.emitNowPlaying()
		return nil
	}
	if err := a.pushTitle(title); err != nil {
		return err
	}
	a.tt.mu.Lock()
	a.tt.sentTitle, a.tt.sentLive = title, a.radio.Status().LiveSince
	a.tt.np.Sent = true
	a.tt.mu.Unlock()
	a.log("Título enviado a Icecast: %s", title)
	a.emitNowPlaying()
	return nil
}

// Playlist es una lista de canciones cargada.
type Playlist struct {
	Path   string   `json:"path"`
	Titles []string `json:"titles"`
}

// LoadPlaylist abre el diálogo de Windows para elegir una lista de canciones.
func (a *App) LoadPlaylist() (Playlist, error) {
	if a.ctx == nil {
		return Playlist{}, errors.New("sin ventana")
	}
	a.mu.Lock()
	dir := filepath.Dir(a.cfg.Radio.Titles.Playlist)
	a.mu.Unlock()
	path, err := wruntime.OpenFileDialog(a.ctx, wruntime.OpenDialogOptions{
		Title: "Lista de canciones", DefaultDirectory: dir,
		Filters: []wruntime.FileFilter{
			{DisplayName: "Listas (*.m3u, *.m3u8, *.pls, *.txt, *.csv)", Pattern: "*.m3u;*.m3u8;*.pls;*.txt;*.csv"},
			{DisplayName: "Todos los archivos", Pattern: "*.*"},
		},
	})
	if err != nil || path == "" {
		return Playlist{}, err
	}
	return a.ReadPlaylist(path)
}

// ReadPlaylist lee una lista de canciones (y la recuerda para la próxima vez).
func (a *App) ReadPlaylist(path string) (Playlist, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Playlist{}, errors.New("No se pudo abrir la lista: " + err.Error())
	}
	titles := nowplaying.ParsePlaylist(path, data)
	if len(titles) == 0 {
		return Playlist{}, errors.New("La lista no tiene canciones reconocibles.")
	}
	a.mu.Lock()
	c := a.cfg
	a.mu.Unlock()
	if c.Radio.Titles.Playlist != path {
		c.Radio.Titles.Playlist = path
		if err := a.SaveConfig(c); err != nil {
			return Playlist{}, err
		}
	}
	a.log("Lista de canciones: %s (%d)", filepath.Base(path), len(titles))
	return Playlist{Path: path, Titles: titles}, nil
}

// LastPlaylist vuelve a leer la última lista usada (al abrir el programa).
func (a *App) LastPlaylist() Playlist {
	a.mu.Lock()
	p := a.cfg.Radio.Titles.Playlist
	a.mu.Unlock()
	if p == "" {
		return Playlist{}
	}
	pl, err := a.ReadPlaylist(p)
	if err != nil {
		return Playlist{}
	}
	return pl
}

// DetectJazler busca dónde publica Jazler lo que suena.
func (a *App) DetectJazler() (string, error) {
	cands := []string{config.JazlerNowOnAir,
		`C:\Program Files (x86)\Jazler RadioStar 2\Exports\NowOnAir.xml`,
		`C:\Program Files\Jazler RadioStar 2\Exports\NowOnAir.xml`,
		`D:\Jazler RadioStar 2\Exports\NowOnAir.xml`}
	for _, p := range cands {
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}
	return "", errors.New("No se encontró Jazler RadioStar 2. Escriba la ruta de NowOnAir.xml (carpeta Exports de Jazler).")
}

// resetTitleFile hace que el vigilante vuelva a leer el archivo (al cambiar de fuente).
func (a *App) resetTitleFile() {
	a.tt.mu.Lock()
	a.tt.fileMod, a.tt.fileSize = time.Time{}, 0
	a.tt.np.Status = ""
	a.tt.mu.Unlock()
}
