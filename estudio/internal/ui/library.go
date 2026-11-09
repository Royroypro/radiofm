package ui

import (
	"context"

	"radioclub25/estudio/internal/domain"
	"radioclub25/estudio/internal/service/library"
)

// CategoryView es una categoría con su número de pistas.
type CategoryView struct {
	domain.Category
	Tracks int `json:"tracks"`
}

// Categories devuelve las categorías con su conteo de pistas.
func (a *App) Categories() ([]CategoryView, error) {
	ctx := context.Background()
	cats, err := a.core.Library.Categories(ctx)
	if err != nil {
		return nil, err
	}
	counts, err := a.core.Library.CountByCategory(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]CategoryView, 0, len(cats))
	for _, c := range cats {
		out = append(out, CategoryView{Category: c, Tracks: counts[c.ID]})
	}
	return out, nil
}

// CategoryKinds devuelve los tipos de categoría válidos.
func (a *App) CategoryKinds() []domain.CategoryKind { return domain.CategoryKinds }

// SaveCategory crea o modifica una categoría.
func (a *App) SaveCategory(c domain.Category) (domain.Category, error) {
	err := a.core.Library.SaveCategory(context.Background(), &c)
	return c, err
}

// DeleteCategory borra una categoría vacía.
func (a *App) DeleteCategory(id int64) error {
	return a.core.Library.DeleteCategory(context.Background(), id)
}

// TrackPage es una página de pistas.
type TrackPage struct {
	Items []domain.Track `json:"items"`
	Total int            `json:"total"`
}

// Tracks busca pistas.
func (a *App) Tracks(f domain.TrackFilter) (TrackPage, error) {
	items, total, err := a.core.Library.Tracks(context.Background(), f)
	if items == nil {
		items = []domain.Track{}
	}
	return TrackPage{Items: items, Total: total}, err
}

// UpdateTrack guarda la edición de una pista.
func (a *App) UpdateTrack(t domain.Track) error {
	return a.core.Library.UpdateTrack(context.Background(), &t)
}

// DeleteTrack quita una pista de la biblioteca.
func (a *App) DeleteTrack(id int64) error {
	return a.core.Library.DeleteTrack(context.Background(), id)
}

// ReanalyzeTrack vuelve a medir una pista ahora.
func (a *App) ReanalyzeTrack(id int64) (domain.Track, error) {
	ctx := context.Background()
	t, err := a.core.Library.Track(ctx, id)
	if err != nil {
		return domain.Track{}, err
	}
	a.core.Library.AnalyzeTrack(ctx, *t)
	t, err = a.core.Library.Track(ctx, id)
	if err != nil {
		return domain.Track{}, err
	}
	return *t, nil
}

// ImportFolder importa una carpeta. Emite "library:import" con el progreso.
func (a *App) ImportFolder(dir string, categoryID int64, recursive bool) (library.ImportResult, error) {
	return a.core.Library.ImportFolder(context.Background(), dir, categoryID, recursive, func(p library.ImportProgress) {
		a.emit("library:import", p)
	})
}

// CheckMissing marca las pistas cuyo archivo ya no existe.
func (a *App) CheckMissing() (int, error) { return a.core.Library.CheckMissing(context.Background()) }

// AnalyzerStatus devuelve el estado de la cola de análisis.
func (a *App) AnalyzerStatus() library.AnalyzerStatus { return a.core.Library.AnalyzerState() }

// SetTrackAttributes aplica atributos (los no vacíos) a todas las pistas del
// filtro actual de la Biblioteca. Devuelve cuántas cambió.
func (a *App) SetTrackAttributes(f domain.TrackFilter, attrs domain.TrackAttributes) (int, error) {
	return a.core.Library.SetAttributes(context.Background(), f, attrs)
}

// TrackByID devuelve una pista (para el voice tracking).
func (a *App) TrackByID(id int64) (domain.Track, error) {
	t, err := a.core.Library.Track(context.Background(), id)
	if err != nil {
		return domain.Track{}, err
	}
	return *t, nil
}
