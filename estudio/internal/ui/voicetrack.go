package ui

import (
	"context"
	"time"

	"radioclub25/estudio/internal/domain"
	"radioclub25/estudio/internal/service/voicetrack"
)

// VoiceTrackStatus es el estado de la grabación (se consulta varias veces por segundo).
type VoiceTrackStatus struct {
	Recording bool          `json:"recording"`
	Elapsed   time.Duration `json:"elapsed"`
	Level     float32       `json:"level"` // pico del micrófono (0–1)
	MicReady  bool          `json:"mic_ready"`
}

// VTStatus devuelve si se está grabando, cuánto va y el nivel del micrófono.
func (a *App) VTStatus() VoiceTrackStatus {
	rec, el := a.core.Voice.Recording()
	st := VoiceTrackStatus{Recording: rec, Elapsed: el, MicReady: a.core.Audio().Mic != ""}
	if rec {
		st.Level = a.core.Engine.Status().RecPeak
	}
	return st
}

// VTStart empieza a grabar el micrófono (no sale al aire).
func (a *App) VTStart() error { return a.core.Voice.Start() }

// VTStop termina la toma y la devuelve para escucharla (/preview/take/{file}).
func (a *App) VTStop() (voicetrack.Take, error) { return a.core.Voice.Stop() }

// VTDiscard borra una toma.
func (a *App) VTDiscard(file string) error { return a.core.Voice.Discard(file) }

// VTSave guarda la toma en la biblioteca y la inserta después del elemento afterID.
// overlapMs: cuánto antes del final de la locución entra la canción siguiente.
func (a *App) VTSave(file string, afterID int64, overlapMs int, title string) (domain.Track, error) {
	t, err := a.core.Voice.Save(context.Background(), file, afterID, time.Duration(overlapMs)*time.Millisecond, title)
	if err != nil {
		return domain.Track{}, err
	}
	a.core.OnAir.Refresh()
	return *t, nil
}
