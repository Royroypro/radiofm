package config

import (
	"fmt"
	"strings"
)

// EQBands son las frecuencias centrales (Hz) del ecualizador gráfico de 10 bandas (una octava cada una).
var EQBands = [10]int{32, 64, 125, 250, 500, 1000, 2000, 4000, 8000, 16000}

// EQMaxDB es el máximo que sube o baja cada banda.
const EQMaxDB = 12.0

// EQ es el ecualizador de una salida.
type EQ struct {
	Enabled bool        `json:"enabled"`
	Preset  string      `json:"preset"` // nombre del perfil elegido ("" = ajuste manual)
	Gains   [10]float64 `json:"gains"`  // dB por banda, en el orden de EQBands
}

// Effective devuelve las ganancias que se aplican de verdad (ceros si está apagado).
func (e EQ) Effective() [10]float64 {
	if !e.Enabled {
		return [10]float64{}
	}
	return e.Gains
}

// EQProfile es un perfil de ecualización con nombre (de fábrica o guardado por la persona).
type EQProfile struct {
	Name    string      `json:"name"`
	Gains   [10]float64 `json:"gains"`
	BuiltIn bool        `json:"built_in"`
}

// EQPresets son los perfiles de fábrica. Pensados para escucharse en radios,
// celulares y parlantes; se pueden copiar y ajustar como perfil propio.
var EQPresets = []EQProfile{
	{Name: "Plano", Gains: [10]float64{}},
	{Name: "Radio FM", Gains: [10]float64{3, 2.5, 1.5, 0, -0.5, 0, 1, 2, 3, 3.5}},
	{Name: "Modo fiesta", Gains: [10]float64{6, 5, 3, 0, -1.5, -1, 0, 2, 4, 5}},
	{Name: "Voz / programa hablado", Gains: [10]float64{-6, -4, -1, 0, 1, 2, 3, 3, 1, 0}},
	{Name: "Reggaetón / urbano", Gains: [10]float64{6, 5.5, 3, 0.5, -1, -0.5, 0.5, 1.5, 2.5, 3}},
	{Name: "Cumbia / salsa", Gains: [10]float64{3, 2.5, 1, 0, 0, 1, 2, 2.5, 2.5, 2}},
	{Name: "Rock", Gains: [10]float64{4, 3, 1.5, -0.5, -1, 0.5, 2, 3, 3.5, 3.5}},
	{Name: "Baladas", Gains: [10]float64{2, 1.5, 1, 0.5, 0, 0, 0.5, 1, 1.5, 2}},
	{Name: "Música suave", Gains: [10]float64{1, 1, 0.5, 0, 0, 0, -0.5, 0, 1, 1.5}},
	{Name: "Clásica / instrumental", Gains: [10]float64{0, 0, 0, 0, 0, 0, -1, -1, 0, 1}},
}

func init() {
	for i := range EQPresets {
		EQPresets[i].BuiltIn = true
	}
}

// ValidateEQ revisa los límites de un ecualizador.
func ValidateEQ(field string, e EQ) error {
	for i, g := range e.Gains {
		if g < -EQMaxDB || g > EQMaxDB {
			return invalid(field, "La banda de %s debe estar entre -%g y +%g dB.", BandLabel(i), EQMaxDB, EQMaxDB)
		}
	}
	return nil
}

// BandLabel es el nombre corto de una banda: "32", "1k", "16k".
func BandLabel(i int) string {
	f := EQBands[i]
	if f >= 1000 {
		return fmt.Sprintf("%dk", f/1000)
	}
	return fmt.Sprint(f)
}

// CleanProfileName valida el nombre de un perfil propio.
func CleanProfileName(name string) (string, error) {
	name = strings.TrimSpace(name)
	switch {
	case name == "":
		return "", invalid("eq.profile", "Escriba un nombre para el perfil.")
	case len([]rune(name)) > 40:
		return "", invalid("eq.profile", "El nombre del perfil es muy largo (máximo 40 letras).")
	}
	for _, p := range EQPresets {
		if strings.EqualFold(p.Name, name) {
			return "", invalid("eq.profile", "«%s» es un perfil de fábrica: use otro nombre.", p.Name)
		}
	}
	return name, nil
}

// UpsertProfile agrega o reemplaza (mismo nombre, sin distinguir mayúsculas) un perfil propio.
func UpsertProfile(list []EQProfile, p EQProfile) []EQProfile {
	p.BuiltIn = false
	for i := range list {
		if strings.EqualFold(list[i].Name, p.Name) {
			list[i] = p
			return list
		}
	}
	return append(list, p)
}

// RemoveProfile quita un perfil propio por nombre.
func RemoveProfile(list []EQProfile, name string) []EQProfile {
	out := list[:0]
	for _, p := range list {
		if !strings.EqualFold(p.Name, name) {
			out = append(out, p)
		}
	}
	return out
}

// Titles es de dónde sale el nombre de la canción que se envía a Icecast.
type Titles struct {
	Source   string `json:"source"`    // TitlesManual, TitlesJazler o TitlesFile
	File     string `json:"file"`      // archivo vigilado (Jazler: NowOnAir.xml; otro programa: un .txt)
	IdleText string `json:"idle_text"` // texto cuando no suena una canción (cuñas, jingles); "" = no cambiar
	Playlist string `json:"playlist"`  // última lista de canciones cargada
}

// Fuentes del título.
const (
	TitlesManual = "manual"
	TitlesJazler = "jazler"
	TitlesFile   = "file"
)

// JazlerNowOnAir es donde Jazler RadioStar 2 publica lo que suena (instalación por defecto).
const JazlerNowOnAir = `C:\Jazler RadioStar 2\Exports\NowOnAir.xml`
