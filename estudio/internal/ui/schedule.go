package ui

import (
	"context"
	"time"

	"radioclub25/estudio/internal/domain"
)

// SlotKinds devuelve los tipos de slot de reloj.
func (a *App) SlotKinds() []domain.SlotKind { return domain.SlotKinds }

// Clocks devuelve los relojes.
func (a *App) Clocks() ([]domain.Clock, error) {
	list, err := a.core.Scheduler.Clocks(context.Background())
	if list == nil {
		list = []domain.Clock{}
	}
	return list, err
}

// SaveClock crea o modifica un reloj.
func (a *App) SaveClock(c domain.Clock) (domain.Clock, error) {
	err := a.core.Scheduler.SaveClock(context.Background(), &c)
	return c, err
}

// DeleteClock borra un reloj.
func (a *App) DeleteClock(id int64) error {
	return a.core.Scheduler.DeleteClock(context.Background(), id)
}

// Grid devuelve la parrilla semanal.
func (a *App) Grid() ([]domain.GridCell, error) {
	cells, err := a.core.Scheduler.Grid(context.Background())
	if cells == nil {
		cells = []domain.GridCell{}
	}
	return cells, err
}

// SetGrid guarda celdas de la parrilla.
func (a *App) SetGrid(cells []domain.GridCell) error {
	return a.core.Scheduler.SetGrid(context.Background(), cells)
}

// Exceptions devuelve las excepciones de parrilla desde hoy.
func (a *App) Exceptions() ([]domain.GridException, error) {
	list, err := a.core.Scheduler.Exceptions(context.Background())
	if list == nil {
		list = []domain.GridException{}
	}
	return list, err
}

// SetException crea o borra (clock_id 0) una excepción.
func (a *App) SetException(e domain.GridException) error {
	return a.core.Scheduler.SetException(context.Background(), e)
}

// Today devuelve el día de hoy (YYYY-MM-DD) en la zona de la radio.
func (a *App) Today() string { return a.core.Scheduler.DayOf(time.Now()) }

// LogDay devuelve la playlist de un día.
func (a *App) LogDay(day string) ([]domain.LogItem, error) {
	items, err := a.core.Scheduler.Day(context.Background(), day)
	if items == nil {
		items = []domain.LogItem{}
	}
	return items, err
}

// GenerateDay (re)genera la playlist de un día conservando lo bloqueado y lo ya emitido.
func (a *App) GenerateDay(day string) error {
	_, err := a.core.Scheduler.Generate(context.Background(), day)
	return err
}

// MoveLogItem mueve un elemento (−1 arriba, +1 abajo).
func (a *App) MoveLogItem(id int64, delta int) error {
	return a.core.Scheduler.Move(context.Background(), id, delta)
}

// RemoveLogItem quita un elemento pendiente.
func (a *App) RemoveLogItem(id int64) error { return a.core.Scheduler.Remove(context.Background(), id) }

// ToggleLogLock bloquea o desbloquea un elemento.
func (a *App) ToggleLogLock(id int64) error {
	return a.core.Scheduler.ToggleLock(context.Background(), id)
}

// InsertLogTrack inserta una pista después de un elemento.
func (a *App) InsertLogTrack(afterID, trackID int64) error {
	return a.core.Scheduler.InsertTrack(context.Background(), afterID, trackID)
}

// nextDay devuelve el día siguiente a day (YYYY-MM-DD).
func nextDay(day string) string {
	t, err := time.Parse(domain.DayLayout, day)
	if err != nil {
		return day
	}
	return t.AddDate(0, 0, 1).Format(domain.DayLayout)
}
