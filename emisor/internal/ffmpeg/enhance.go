package ffmpeg

import (
	"fmt"
	"strconv"
)

// MEJORA es el procesador de sonido del emisor (como el de las radios FM):
//
//	highpass  50 Hz        quita retumbe y golpes de micrófono
//	bass      +4 dB 100 Hz cuerpo
//	treble    +2,5 dB 8 kHz brillo y presencia de la voz
//	acompressor 3:1 desde -18 dBFS, +8 dB de ganancia: amplifica y empareja el volumen
//	alimiter  -0,5 dBFS    nunca satura, aunque se suba la ganancia
//
// La cadena está SIEMPRE en el filtro, con valores neutros cuando MEJORA está
// apagada. Así se puede encender y apagar al aire, sin reiniciar la conexión:
// el emisor le escribe a ffmpeg comandos por su entrada («c» + destino tiempo
// comando valor), que cambian los parámetros de cada filtro en caliente.
// Los medidores muestran el audio ya procesado.

// enhanceParam es un parámetro de un filtro de MEJORA con su valor apagado y encendido.
type enhanceParam struct {
	filter, param string
	off, on       string
}

// Orden de envío: ffmpeg aplica un comando cada 0,25 s (ver common), así que
// primero va lo que más se nota (ganancia, compresión y limitador).
var enhanceParams = []enhanceParam{
	{"acompressor@mejora", "makeup", "1", "2.5"}, // 2,5 veces ≈ +8 dB
	{"acompressor@mejora", "ratio", "1", "3"},
	{"alimiter@mejora", "enable", "0", "1"},
	{"bass@mejora", "g", "0", "4"},
	{"treble@mejora", "g", "0", "2.5"},
	{"highpass@mejora", "enable", "0", "1"},
}

func enhanceValue(p enhanceParam, on bool) string {
	if on {
		return p.on
	}
	return p.off
}

// enhanceChain es la parte del filtro con MEJORA en el estado inicial pedido.
func enhanceChain(on bool) string {
	v := map[string]string{}
	for _, p := range enhanceParams {
		v[p.filter+"."+p.param] = enhanceValue(p, on)
	}
	return fmt.Sprintf("highpass@mejora=f=50:enable=%s,"+
		"bass@mejora=g=%s:f=100,"+
		"treble@mejora=g=%s:f=8000,"+
		"acompressor@mejora=threshold=0.125:ratio=%s:attack=20:release=250:makeup=%s,"+
		"alimiter@mejora=limit=0.944:level=0:enable=%s",
		v["highpass@mejora.enable"], v["bass@mejora.g"], v["treble@mejora.g"],
		v["acompressor@mejora.ratio"], v["acompressor@mejora.makeup"], v["alimiter@mejora.enable"])
}

// EnhanceCommands son las líneas que se escriben en la entrada de ffmpeg para
// encender (on) o apagar MEJORA sin cortar. Cada una empieza con «c»: la tecla
// de ffmpeg para enviar un comando a un filtro; -1 = ahora mismo.
func EnhanceCommands(on bool) []string {
	out := make([]string, 0, len(enhanceParams))
	for _, p := range enhanceParams {
		out = append(out, "c"+p.filter+" -1 "+p.param+" "+enhanceValue(p, on)+"\n")
	}
	return out
}

// gainFilter es el ajuste de ganancia manual («Ganancia (dB)»). Está siempre
// (0 dB = sin efecto) para poder cambiarlo al aire con GainCommand.
func gainFilter(gainDB float64) string {
	return ",volume@ganancia=volume=" + gainValue(gainDB)
}

func gainValue(gainDB float64) string { return strconv.FormatFloat(gainDB, 'f', 1, 64) + "dB" }

// GainCommand cambia la ganancia al aire, sin cortar.
func GainCommand(gainDB float64) []string {
	return []string{"cvolume@ganancia -1 volume " + gainValue(gainDB) + "\n"}
}
