package domain

import (
	"context"
	"time"
)

// Program es un espacio de la programación semanal de la radio.
// StartTime y EndTime usan el formato "HH:MM" en la zona horaria de la radio.
// Si EndTime es menor que StartTime el programa cruza la medianoche.
type Program struct {
	ID          int64
	Title       string
	Host        string
	Description string
	ImageURL    string
	Weekday     time.Weekday // 0 = domingo ... 6 = sábado
	StartTime   string
	EndTime     string
	Active      bool
}

// WeekdayNames son los nombres de los días en español, indexados por time.Weekday.
var WeekdayNames = [7]string{"Domingo", "Lunes", "Martes", "Miércoles", "Jueves", "Viernes", "Sábado"}

// WeekdayName devuelve el nombre del día en español.
func (p Program) WeekdayName() string { return WeekdayNames[p.Weekday] }

// minutes convierte "HH:MM" a minutos desde la medianoche. Devuelve -1 si es inválido.
func minutes(hhmm string) int {
	t, err := time.Parse("15:04", hhmm)
	if err != nil {
		return -1
	}
	return t.Hour()*60 + t.Minute()
}

// ValidTime indica si s tiene formato "HH:MM" válido.
func ValidTime(s string) bool { return minutes(s) >= 0 }

// IsOnAir indica si el programa está al aire en el instante t (ya convertido a la zona de la radio).
func (p Program) IsOnAir(t time.Time) bool {
	if !p.Active {
		return false
	}
	start, end := minutes(p.StartTime), minutes(p.EndTime)
	if start < 0 || end < 0 {
		return false
	}
	now := t.Hour()*60 + t.Minute()
	today := t.Weekday()
	if start < end { // mismo día
		return p.Weekday == today && now >= start && now < end
	}
	// cruza la medianoche: parte del día del programa y parte del día siguiente
	if p.Weekday == today && now >= start {
		return true
	}
	return (p.Weekday+1)%7 == today && now < end
}

// ProgramRepository persiste la programación.
type ProgramRepository interface {
	Create(ctx context.Context, p *Program) error
	Get(ctx context.Context, id int64) (*Program, error)
	List(ctx context.Context) ([]Program, error) // ordenados por día y hora
	Update(ctx context.Context, p *Program) error
	Delete(ctx context.Context, id int64) error
}
