package ui

import (
	"context"
	"os"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"radioclub25/estudio/internal/domain"
	"radioclub25/estudio/internal/service/reports"
	"radioclub25/estudio/internal/service/spots"
)

// SpotView es una cuña con cuántas veces sonó hoy.
type SpotView struct {
	domain.Spot
	AiredToday int  `json:"aired_today"`
	FromWeb    bool `json:"from_web"`
}

// Spots devuelve las cuñas.
func (a *App) Spots() ([]SpotView, error) {
	ctx := context.Background()
	list, err := a.core.Spots.List(ctx)
	if err != nil {
		return nil, err
	}
	aired, err := a.core.Spots.AiredToday(ctx)
	if err != nil {
		return nil, err
	}
	out := []SpotView{}
	for _, s := range list {
		out = append(out, SpotView{Spot: s, AiredToday: aired[s.ID], FromWeb: s.RemoteID != 0})
	}
	return out, nil
}

// SaveSpot crea o modifica una cuña local y replanifica hoy y mañana.
func (a *App) SaveSpot(s domain.Spot) (domain.Spot, error) {
	if err := a.core.Spots.SaveLocal(context.Background(), &s); err != nil {
		return s, err
	}
	a.replanSoon()
	return s, nil
}

// DeleteSpot borra una cuña local.
func (a *App) DeleteSpot(id int64) error {
	if err := a.core.Spots.Delete(context.Background(), id); err != nil {
		return err
	}
	a.replanSoon()
	return nil
}

// replanSoon replanifica las tandas de hoy y mañana en segundo plano.
func (a *App) replanSoon() { a.core.ReplanSoon() }

// ReplanResult resume una replanificación.
type ReplanResult struct {
	Missing []spots.Missing `json:"missing"` // campañas con emisiones que no entraron (y por qué)
}

// ReplanDay vuelve a repartir las cuñas en las tandas pendientes de un día.
func (a *App) ReplanDay(day string) (ReplanResult, error) {
	_, err := a.core.Spots.Replan(context.Background(), day)
	return ReplanResult{Missing: a.SpotsMissing(day)}, err
}

// SpotsMissing devuelve lo que no entró en las tandas de un día (última replanificación).
func (a *App) SpotsMissing(day string) []spots.Missing {
	m := a.core.Spots.Missing(day)
	if m == nil {
		m = []spots.Missing{}
	}
	return m
}

// DayTandas devuelve las tandas de un día con sus cuñas.
func (a *App) DayTandas(day string) ([]spots.TandaView, error) {
	ctx := context.Background()
	items, err := a.core.Scheduler.Day(ctx, day)
	if err != nil {
		return nil, err
	}
	v, err := a.core.Spots.DayTandas(ctx, items)
	if v == nil {
		v = []spots.TandaView{}
	}
	return v, err
}

// ReportAsRun devuelve lo emitido entre dos fechas (YYYY-MM-DD, inclusive).
func (a *App) ReportAsRun(from, to string) ([]domain.Play, error) {
	list, err := a.core.Reports.AsRun(context.Background(), from, to)
	if list == nil {
		list = []domain.Play{}
	}
	return list, err
}

// ReportSpots devuelve el comprobante de cuñas.
func (a *App) ReportSpots(from, to, advertiser string) ([]reports.ProofRow, error) {
	return a.core.Reports.SpotProof(context.Background(), from, to, advertiser)
}

// ExportReport pide dónde guardar y escribe el CSV ("asrun" o "spots"). Devuelve la ruta ("" si se canceló).
func (a *App) ExportReport(kind, from, to, advertiser string) (string, error) {
	ctx := context.Background()
	name := "emisiones_" + from + "_" + to + ".csv"
	if kind == "spots" {
		name = "comprobante_" + from + "_" + to + ".csv"
	}
	path, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{Title: "Guardar CSV", DefaultFilename: name,
		Filters: []runtime.FileFilter{{DisplayName: "CSV", Pattern: "*.csv"}}})
	if err != nil || path == "" {
		return "", err
	}
	f, err := os.Create(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	if kind == "spots" {
		rows, err := a.core.Reports.SpotProof(ctx, from, to, advertiser)
		if err != nil {
			return "", err
		}
		return path, a.core.Reports.WriteProofCSV(f, rows)
	}
	plays, err := a.core.Reports.AsRun(ctx, from, to)
	if err != nil {
		return "", err
	}
	return path, a.core.Reports.WriteAsRunCSV(f, plays)
}

// CheckLocutions devuelve las locuciones que faltan en la carpeta configurada.
func (a *App) CheckLocutions() []string {
	m := a.core.Locutions().Check()
	if m == nil {
		m = []string{}
	}
	return m
}

// CurrentTemperature consulta la temperatura actual (para probar la configuración).
func (a *App) CurrentTemperature() (float64, error) {
	st := a.core.Settings().Station
	return a.core.Weather.Current(context.Background(), st.Latitude, st.Longitude)
}
