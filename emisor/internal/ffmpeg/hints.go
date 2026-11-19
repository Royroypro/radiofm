package ffmpeg

import "strings"

// hints traduce los errores más comunes de ffmpeg a una explicación para el operador.
// El orden importa: gana la primera coincidencia.
var hints = []struct{ match, hint string }{
	// RTSP (cámara IP) antes que los genéricos: un 401 de la cámara no es la clave del servidor.
	{"describe failed: 401", hintRTSPAuth},
	{"describe failed: 404", "La cámara RTSP no tiene esa ruta (404). Revise la dirección rtsp:// (cada marca usa la suya, por ejemplo /stream1 o /Streaming/Channels/101)."},
	{"describe failed", "La cámara RTSP no aceptó la conexión. Revise la dirección rtsp://, el usuario y la clave."},
	{"setup failed: 461", "La cámara RTSP no acepta video por TCP. Actualice la cámara o use otra ruta de la cámara."},
	{"401 unauthorized", hintAuth},
	{"authentication required", hintAuth},
	{"authentication failed", "El servidor rechazó el usuario o la clave. Revise los datos en Admin → Streams y emisión."},
	{"403 forbidden", "El servidor no aceptó la emisión (403): montaje desactivado, servicio bloqueado o ya hay otra fuente conectada."},
	{"already publishing", "Ya hay otra fuente emitiendo en ese montaje o ruta."},
	{"connection refused", "No hay servidor escuchando en ese puerto (conexión rechazada). Revise servidor y puerto."},
	{"connection timed out", "El servidor no responde (tiempo agotado). Revise internet, el servidor y el puerto."},
	{"connection setup failure", "No se pudo conectar por SRT (servidor, puerto o firewall)."},
	{"failed to resolve", "No se encontró el servidor (DNS). Revise el nombre del servidor y la conexión a internet."},
	{"no such host", "No se encontró el servidor (DNS). Revise el nombre del servidor y la conexión a internet."},
	{"could not find audio only device", "No se encontró el micrófono o la entrada elegida. ¿Se desconectó?"},
	{"could not find video device", "No se encontró la cámara elegida. ¿Se desconectó o la usa otro programa?"},
	{"could not run graph", "Windows no pudo abrir el dispositivo (¿lo está usando otro programa?)."},
	{"i/o error", "Se cortó la conexión con el servidor."},
	{"input/output error", "Se cortó la conexión con el servidor."},
	{"broken pipe", "Se cortó la conexión con el servidor."},
	{"error writing trailer", "Se cortó la conexión con el servidor."},
	{"404 not found", "No se encontró nada en esa dirección (404). Revise la dirección del video o el montaje."},
	{"stream specifier ':a' in filtergraph", "El video de esa dirección no trae audio: elija otro audio para la TV."},
	{"matches no streams", "La fuente elegida no trae la pista pedida (por ejemplo, un video sin audio): elija otro audio para la TV."},
	{"tcp://127.0.0.1", "Se cortó la captura del audio de la PC (altavoces)."},
	{"unknown encoder 'libfdk_aac'", "Este ffmpeg no trae AAC+ (libfdk_aac). Use AAC, MP3 u Opus, o un ffmpeg con libfdk_aac (ver README)."},
	{"unknown encoder", "Este ffmpeg no trae el codificador elegido."},
	{"cannot load nvcuda", "No hay tarjeta NVIDIA disponible: elija el codificador x264."},
	{"no nvenc capable devices", "No hay tarjeta NVIDIA disponible: elija el codificador x264."},
}

const hintAuth = "El servidor rechazó la clave (401). Revise la contraseña en Admin → Streams y emisión."

const hintRTSPAuth = "La cámara RTSP rechazó el usuario o la clave (401). Revise la dirección: rtsp://usuario:clave@cámara/ruta."

// IsAuthHint dice si la explicación es un rechazo de usuario o clave: reintentar
// no lo arregla (hay que corregir la clave), así que el emisor deja de insistir.
func IsAuthHint(h string) bool {
	return h == hintAuth || h == hintRTSPAuth || strings.HasPrefix(h, "El servidor rechazó el usuario o la clave")
}

// Hint devuelve una explicación en español para un mensaje de ffmpeg, o "".
func Hint(msg string) string {
	m := strings.ToLower(msg)
	for _, h := range hints {
		if strings.Contains(m, h.match) {
			return h.hint
		}
	}
	return ""
}
