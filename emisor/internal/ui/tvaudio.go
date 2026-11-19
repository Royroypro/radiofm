package ui

import (
	"context"
	"fmt"
	"os/exec"
	"sync"
	"time"

	"radioclub25/emisor/internal/config"
	"radioclub25/emisor/internal/feed"
	"radioclub25/emisor/internal/ffmpeg"
	"radioclub25/emisor/internal/runner"
)

// El audio de la TV con video por dirección o RTSP (1.7.0):
//
//   - «Solo el audio del video», «solo el otro audio» o «los dos mezclados»
//     (config.TV.AudioMode). El otro audio puede ser, entre otros, el de la radio
//     (la misma entrada que la pestaña Radio) o la radio desde el servidor Icecast.
//   - Antes de cada intento se mira si el video trae audio. Si no trae (videos de
//     fondo), se usa solo el otro audio, para que la TV no salga muda ni falle.

// probeTTL es cuánto vale lo que se averiguó de una dirección: un archivo que
// termina vuelve a empezar como reconexión y no hace falta mirarlo cada vez.
const probeTTL = 10 * time.Minute

// probeTimeout es lo máximo que se espera a que la fuente responda al mirarla.
const probeTimeout = 20 * time.Second

// tvAudioState recuerda si la TV usa el bus de audio y lo que se averiguó del video.
type tvAudioState struct {
	mu       sync.Mutex
	direct   bool   // true: la TV usa solo el audio del video (sin bus)
	lastNote string // último aviso escrito en el registro (para no repetirlo)
	probeURL string
	probeHas bool
	probeAt  time.Time
}

// resolveTVAudio convierte la fuente elegida para la TV en la que se abre de
// verdad: «audio de la radio» es la entrada de la radio y «radio desde el
// servidor» es la dirección de oyente de Icecast.
func resolveTVAudio(c config.Config) config.Source {
	switch c.TV.Audio.Kind {
	case config.SourceRadio:
		return c.Radio.Input
	case config.SourceIcecast:
		return config.Source{Kind: config.SourceIcecast, Name: c.Radio.ListenURL()}
	}
	return c.TV.Audio
}

// liveSource es lo que debe alimentar el bus de which con la configuración c.
func liveSource(c config.Config, which string) config.Source {
	if which == "radio" {
		return c.Radio.Input
	}
	return resolveTVAudio(c)
}

// tvUsesBus dice si la TV, tal como está al aire, lee del bus de audio.
func (a *App) tvUsesBus() bool {
	a.tva.mu.Lock()
	defer a.tva.mu.Unlock()
	return !a.tva.direct
}

// tvAudioMode decide el modo de audio para este intento: si el modo pide el
// audio del video y el video no lo trae (o no se pudo mirar), usa el otro audio.
func (a *App) tvAudioMode(c config.Config) string {
	mode := c.TV.EffectiveAudioMode()
	if mode == config.AudioModeOther {
		a.noteTV("")
		return mode
	}
	has, err := a.videoHasAudio(c.TV.Video)
	other := feed.Describe(resolveTVAudio(c))
	if c.TV.Audio.Kind == config.SourceRadio {
		other = "el audio de la radio (" + other + ")"
	}
	switch {
	case err != nil:
		a.noteTV(fmt.Sprintf("[tv] no se pudo revisar si el video trae audio (%v): se usa %s", err, other))
		return config.AudioModeOther
	case !has:
		a.noteTV("[tv] el video no trae audio: se usa " + other)
		return config.AudioModeOther
	}
	if mode == config.AudioModeVideo {
		a.noteTV("[tv] audio: el que trae el video")
	} else {
		a.noteTV("[tv] audio: el del video mezclado con " + other)
	}
	return mode
}

// noteTV escribe un aviso del audio de la TV solo si cambió desde el anterior.
func (a *App) noteTV(msg string) {
	a.tva.mu.Lock()
	changed := msg != a.tva.lastNote
	a.tva.lastNote = msg
	a.tva.mu.Unlock()
	if changed && msg != "" {
		a.addLog(msg)
	}
}

// videoHasAudio abre la fuente de video (sin emitir) y mira si trae pista de audio.
func (a *App) videoHasAudio(src config.Source) (bool, error) {
	a.tva.mu.Lock()
	if a.tva.probeURL == src.Name && time.Since(a.tva.probeAt) < probeTTL {
		has := a.tva.probeHas
		a.tva.mu.Unlock()
		return has, nil
	}
	a.tva.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), probeTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, a.ffmpeg, ffmpeg.ProbeArgs(src)...)
	runner.HideWindow(cmd)
	cmd.Cancel = func() error { return runner.KillTree(cmd) }
	cmd.WaitDelay = 3 * time.Second
	out, _ := cmd.CombinedOutput() // sin salida, ffmpeg siempre termina con error: se mira lo que escribió
	text := string(out)
	if !ffmpeg.HasVideoStream(text) && !ffmpeg.HasAudioStream(text) {
		if ctx.Err() != nil {
			return false, fmt.Errorf("la fuente no respondió en %d s", int(probeTimeout.Seconds()))
		}
		if h := ffmpeg.Hint(text); h != "" {
			return false, fmt.Errorf("%s", h)
		}
		return false, fmt.Errorf("no se pudo abrir la fuente")
	}
	has := ffmpeg.HasAudioStream(text)
	a.tva.mu.Lock()
	a.tva.probeURL, a.tva.probeHas, a.tva.probeAt = src.Name, has, time.Now()
	a.tva.mu.Unlock()
	return has, nil
}

// forgetProbe olvida lo averiguado del video (al cambiar la dirección o al aplicar).
func (a *App) forgetProbe() {
	a.tva.mu.Lock()
	a.tva.probeURL = ""
	a.tva.mu.Unlock()
}

// followRadio: si la TV usa el audio de la radio (o la radio desde el servidor)
// y está al aire leyendo del bus, le pasa en el acto la entrada nueva de la
// radio, sin reconectar la TV.
func (a *App) followRadio(c config.Config) {
	if c.TV.Audio.Kind != config.SourceRadio && c.TV.Audio.Kind != config.SourceIcecast {
		return
	}
	if !a.tv.Running() || !a.tvUsesBus() {
		return
	}
	_ = a.switchLive("tv", resolveTVAudio(c))
}
