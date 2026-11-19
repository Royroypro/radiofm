package ui

import (
	"testing"

	"radioclub25/emisor/internal/config"
)

func TestResolveTVAudio(t *testing.T) {
	c := config.Defaults()
	c.Radio.Input = config.Source{Kind: config.SourceLoopback, Name: "CABLE Input (VB-Audio Virtual Cable)"}

	// «Audio de la radio» abre la misma entrada que la radio.
	c.TV.Audio = config.Source{Kind: config.SourceRadio}
	if got := resolveTVAudio(c); got != c.Radio.Input {
		t.Errorf("audio de la radio: %+v", got)
	}
	// «La radio desde el servidor» escucha el montaje en Icecast como un oyente.
	c.TV.Audio = config.Source{Kind: config.SourceIcecast}
	if got := resolveTVAudio(c); got.Kind != config.SourceIcecast || got.Name != "http://radioclub25peru.com:8010/radio" {
		t.Errorf("radio desde el servidor: %+v", got)
	}
	// Las demás fuentes no cambian.
	c.TV.Audio = config.Source{Kind: config.SourceDShow, Name: "Micrófono (USB)"}
	if got := liveSource(c, "tv"); got != c.TV.Audio {
		t.Errorf("micrófono: %+v", got)
	}
	if got := liveSource(c, "radio"); got != c.Radio.Input {
		t.Errorf("radio: %+v", got)
	}
}

// La TV con el modo «solo el audio del video» no usa el bus: cambiar el otro
// audio al aire no debe abrir nada.
func TestTVAudioModeOther(t *testing.T) {
	a := NewApp()
	c := config.Defaults()
	c.TV.Video = config.Source{Kind: config.SourceScreen}
	c.TV.AudioMode = config.AudioModeVideo // sin video de red el modo se ignora
	if got := a.tvAudioMode(c); got != config.AudioModeOther {
		t.Errorf("con pantalla el modo efectivo debe ser other, es %q", got)
	}
}
