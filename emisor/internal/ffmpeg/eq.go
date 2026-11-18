package ffmpeg

import (
	"fmt"
	"strconv"
	"strings"

	"radioclub25/emisor/internal/config"
)

// Ecualizador gráfico de 10 bandas: un filtro «equalizer» (pico, una octava de
// ancho) por banda, con nombre equalizer@eqN. Como MEJORA, está siempre en el
// filtro (0 dB = sin efecto) y se cambia al aire con comandos «g» por la
// entrada de ffmpeg, sin reiniciar la conexión.

func eqFilterName(i int) string { return fmt.Sprintf("equalizer@eq%d", i) }

func dB(g float64) string { return strconv.FormatFloat(g, 'f', -1, 64) }

// eqChain arma las 10 bandas con las ganancias iniciales.
func eqChain(gains [10]float64) string {
	parts := make([]string, len(config.EQBands))
	for i, f := range config.EQBands {
		parts[i] = fmt.Sprintf("%s=f=%d:t=o:w=1:g=%s", eqFilterName(i), f, dB(gains[i]))
	}
	return strings.Join(parts, ",")
}

// EQCommands son los comandos para pasar del ecualizador from al to: solo las
// bandas que cambian (ffmpeg aplica un comando cada 0,25 s, ver common).
func EQCommands(from, to [10]float64) []string {
	var out []string
	for i := range to {
		if from[i] != to[i] {
			out = append(out, "c"+eqFilterName(i)+" -1 g "+dB(to[i])+"\n")
		}
	}
	return out
}

// safetyLimiter va al final de todo: aunque se suban bandas o ganancia, la
// salida nunca pasa de -0,3 dBFS (sin distorsión por recorte).
const safetyLimiter = "alimiter@seguro=limit=0.966:level=0:attack=5:release=50"
