package ui

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	"radioclub25/estudio/internal/domain"
	"radioclub25/estudio/internal/service/onair"
)

// PreviewHandler sirve el audio de una pista de la biblioteca a la ventana:
// GET /preview/track/{id}. La ventana lo reproduce con un <audio> por la salida
// predeterminada de Windows (los audífonos del operador), aparte del aire.
// Solo sirve pistas de la biblioteca (por ID), nunca rutas arbitrarias.
// http.ServeFile atiende rangos: el editor puede saltar a cualquier punto.
func (a *App) PreviewHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.NotFound(w, r)
			return
		}
		// /preview/airlog/{archivo}: una hora de la grabación de la emisión
		if file, ok := strings.CutPrefix(r.URL.Path, "/preview/airlog/"); ok && a.core != nil {
			p, err := a.core.AirLog.FilePath(file)
			if err != nil {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Cache-Control", "no-store")
			http.ServeFile(w, r, p)
			return
		}
		// /preview/take/{archivo}: una toma de voice tracking (solo de su carpeta)
		if file, ok := strings.CutPrefix(r.URL.Path, "/preview/take/"); ok && a.core != nil {
			p, err := a.core.Voice.TakePath(file)
			if err != nil {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Cache-Control", "no-store")
			http.ServeFile(w, r, p)
			return
		}
		idStr, ok := strings.CutPrefix(r.URL.Path, "/preview/track/")
		if !ok {
			http.NotFound(w, r)
			return
		}
		id, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil || id <= 0 || a.core == nil {
			http.NotFound(w, r)
			return
		}
		t, err := a.core.Library.Track(r.Context(), id)
		if err != nil || t.Missing {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		http.ServeFile(w, r, t.Path)
	})
}

// ResetTrackMarks descarta las marcas puestas a mano y vuelve a analizar la pista.
func (a *App) ResetTrackMarks(id int64) (domain.Track, error) {
	ctx := context.Background()
	if err := a.core.Library.ResetMarks(ctx, id); err != nil {
		return domain.Track{}, err
	}
	t, err := a.core.Library.Track(ctx, id)
	if err != nil {
		return domain.Track{}, err
	}
	return *t, nil
}

// TransitionPreview es cómo el aire pasaría de la pista A a la B (para «probar transición»).
type TransitionPreview struct {
	StartAt time.Duration `json:"start_at"` // momento de A (desde el inicio del archivo) en que entra B
	OutFade time.Duration `json:"out_fade"` // fundido de A (negativo = A termina sola)
	InFade  time.Duration `json:"in_fade"`
	Delay   time.Duration `json:"delay"`  // B espera esto tras StartAt
	BFrom   time.Duration `json:"b_from"` // B empieza en su cue-in
	AEnd    time.Duration `json:"a_end"`  // fin útil de A
}

// PreviewTransition calcula la transición automática de A a B con las reglas
// actuales (las mismas que usa el aire).
func (a *App) PreviewTransition(aID, bID int64) (TransitionPreview, error) {
	ctx := context.Background()
	ta, err := a.core.Library.Track(ctx, aID)
	if err != nil {
		return TransitionPreview{}, err
	}
	tb, err := a.core.Library.Track(ctx, bID)
	if err != nil {
		return TransitionPreview{}, err
	}
	kinds := map[int64]domain.CategoryKind{}
	if cats, err := a.core.Library.Categories(ctx); err == nil {
		for _, c := range cats {
			kinds[c.ID] = c.Kind
		}
	}
	s := a.core.Settings().Audio
	tr, segA := onair.PreviewTransition(*ta, *tb, kinds, onair.TransitionRules{
		Crossfade: s.Crossfade(), BreakFade: time.Duration(s.BreakFadeMs) * time.Millisecond})
	return TransitionPreview{StartAt: ta.CueIn + segA, OutFade: tr.OutFade, InFade: tr.InFade, Delay: tr.Delay,
		BFrom: tb.CueIn, AEnd: ta.End()}, nil
}
