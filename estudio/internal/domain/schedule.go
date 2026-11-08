package domain

import (
	"context"
	"time"
)

// SlotKind es el tipo de un slot de reloj (y de un elemento de la playlist).
type SlotKind string

const (
	SlotCategory SlotKind = "category" // una pista de la categoría (rotación)
	SlotTrack    SlotKind = "track"    // una pista fija
	SlotSpots    SlotKind = "spots"    // tanda de cuñas
	SlotTime     SlotKind = "time"     // hora locutada
	SlotTemp     SlotKind = "temp"     // temperatura locutada
	SlotMode     SlotKind = "mode"     // cambio de modo (auto/asistido/en vivo)
	SlotSilence  SlotKind = "silence"  // silencio de N segundos
)

// SlotKinds en el orden de la interfaz.
var SlotKinds = []SlotKind{SlotCategory, SlotTrack, SlotSpots, SlotTime, SlotTemp, SlotMode, SlotSilence}

// Valid indica si el tipo es conocido.
func (k SlotKind) Valid() bool {
	for _, x := range SlotKinds {
		if x == k {
			return true
		}
	}
	return false
}

// ClockSlot es una posición de un reloj.
//   - Hard = evento duro: suena a la hora exacta (minuto Minute de la hora) y funde lo que suene.
//   - Si no es duro, va en orden después del slot anterior (evento blando).
type ClockSlot struct {
	Kind       SlotKind `json:"kind"`
	CategoryID int64    `json:"category_id"` // SlotCategory
	TrackID    int64    `json:"track_id"`    // SlotTrack
	Hard       bool     `json:"hard"`
	Minute     int      `json:"minute"`  // 0–59, solo si Hard
	Mode       string   `json:"mode"`    // SlotMode: auto | assisted | live
	Seconds    int      `json:"seconds"` // SlotSilence
}

// Clock es una plantilla de una hora: lista ordenada de slots.
type Clock struct {
	ID    int64       `json:"id"`
	Name  string      `json:"name"`
	Color string      `json:"color"`
	Slots []ClockSlot `json:"slots"`
}

// GridCell asigna un reloj a una hora de la semana. Weekday 0 = domingo.
type GridCell struct {
	Weekday int   `json:"weekday"`
	Hour    int   `json:"hour"`
	ClockID int64 `json:"clock_id"` // 0 = sin reloj
}

// GridException asigna otro reloj a una hora de una fecha concreta (feriados, transmisiones especiales).
type GridException struct {
	Date    string `json:"date"` // YYYY-MM-DD
	Hour    int    `json:"hour"`
	ClockID int64  `json:"clock_id"`
}

// DayLayout es el formato de fecha de un día de programación.
const DayLayout = "2006-01-02"

// LogStatus es el estado de un elemento de la playlist.
type LogStatus string

const (
	LogPending LogStatus = "pending"
	LogPlaying LogStatus = "playing"
	LogPlayed  LogStatus = "played"
	LogSkipped LogStatus = "skipped"
)

// LogItem es un elemento de la playlist de un día.
type LogItem struct {
	ID         int64         `json:"id"`
	Day        string        `json:"day"`  // YYYY-MM-DD (hora de la radio)
	Hour       int           `json:"hour"` // hora del reloj que lo generó
	Seq        int           `json:"seq"`  // orden dentro del día
	Kind       SlotKind      `json:"kind"`
	TrackID    int64         `json:"track_id"`
	CategoryID int64         `json:"category_id"`
	Hard       bool          `json:"hard"`
	At         time.Time     `json:"at"`     // hora prevista (exacta si Hard)
	Length     time.Duration `json:"length"` // duración prevista (hasta el punto de mezcla)
	Optional   bool          `json:"optional"`
	Locked     bool          `json:"locked"`
	Status     LogStatus     `json:"status"`
	Title      string        `json:"title"` // para mostrar sin consultar la pista
	Artist     string        `json:"artist"`
	Mode       string        `json:"mode"`
	Seconds    int           `json:"seconds"`
	Note       string        `json:"note"` // p. ej. reglas de rotación relajadas
}

// Display devuelve el texto del elemento para la interfaz.
func (l LogItem) Display() string {
	switch {
	case l.Artist != "" && l.Title != "":
		return l.Artist + " - " + l.Title
	case l.Title != "":
		return l.Title
	}
	return string(l.Kind)
}

// ClockRepository persiste relojes.
type ClockRepository interface {
	List(ctx context.Context) ([]Clock, error)
	Get(ctx context.Context, id int64) (*Clock, error)
	Save(ctx context.Context, c *Clock) error // crea si ID = 0
	Delete(ctx context.Context, id int64) error
}

// GridRepository persiste la parrilla semanal y sus excepciones.
type GridRepository interface {
	Cells(ctx context.Context) ([]GridCell, error)
	SetCells(ctx context.Context, cells []GridCell) error
	Exceptions(ctx context.Context, fromDate string) ([]GridException, error)
	SetException(ctx context.Context, e GridException) error // ClockID 0 = borrar
}

// LogRepository persiste la playlist por día.
type LogRepository interface {
	Day(ctx context.Context, day string) ([]LogItem, error)
	ReplaceDay(ctx context.Context, day string, items []LogItem) error
	Get(ctx context.Context, id int64) (*LogItem, error)
	Update(ctx context.Context, it *LogItem) error
	Delete(ctx context.Context, id int64) error
	SetStatus(ctx context.Context, id int64, s LogStatus) error
	Days(ctx context.Context, from string) ([]string, error) // días con playlist desde from
}
