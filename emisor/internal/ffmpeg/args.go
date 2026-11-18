// Package ffmpeg arma los argumentos de ffmpeg para cada salida (radio y TV) y
// lee lo que ffmpeg escribe: lista de dispositivos, códecs disponibles,
// progreso y niveles de audio.
//
// Es lógica pura (texto entra, texto sale): no ejecuta procesos ni abre
// dispositivos, por eso se prueba sin hardware. Quien ejecuta es internal/runner.
//
// Por cada salida corre un ffmpeg aparte: si se cae la conexión de la TV, la
// radio sigue al aire (y al revés). Windows permite abrir el mismo micrófono
// desde los dos procesos.
//
// Salidas estándar de cada proceso:
//   - stdout: niveles de audio (filtro astats + ametadata, 10 por segundo).
//   - stderr: progreso (-progress pipe:2) y mensajes de error de ffmpeg.
package ffmpeg

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"radioclub25/emisor/internal/config"
	"radioclub25/emisor/internal/loopback"
)

// MeterRate son las mediciones de nivel por segundo.
const MeterRate = 10

// common son las opciones que van al principio de todo comando.
func common() []string {
	// Sin -nostdin: el emisor detiene ffmpeg escribiendo «q» en su entrada (cierre limpio).
	return []string{"-hide_banner", "-loglevel", "warning", "-nostats",
		// stats_period también marca cada cuánto ffmpeg lee su entrada: un comando de
		// MEJORA (o la «q» de detener) por vuelta. Con 0,25 s se aplican en seguida.
		"-progress", "pipe:2", "-stats_period", "0.25"}
}

// audioInput devuelve los argumentos de entrada de una fuente de audio.
func audioInput(s config.Source, sampleRate, channels int) []string {
	switch s.Kind {
	case config.SourceDShow:
		// audio_buffer_size en ms: baja la latencia de DirectShow (por defecto ~500 ms).
		return []string{"-thread_queue_size", "1024", "-f", "dshow", "-audio_buffer_size", "50",
			"-i", "audio=" + s.Name}
	case config.SourceLoopback:
		// Lo que suena en los altavoces: PCM que sirve el propio emisor (internal/loopback).
		// PCM crudo: no hay nada que analizar. Sin esto, ffmpeg lee 5 s antes de empezar
		// (demora al conectar y retrasa los medidores).
		return []string{"-thread_queue_size", "1024", "-probesize", "32", "-analyzeduration", "0", "-f", "s16le", "-ar", strconv.Itoa(loopback.SampleRate),
			"-ac", strconv.Itoa(loopback.Channels), "-i", s.Addr}
	case config.SourceTest:
		// Tono de 1 kHz (sale a -18 dBFS; en estéreo, unos -21 dBFS por canal).
		return []string{"-re", "-f", "lavfi", "-i", fmt.Sprintf("sine=frequency=1000:sample_rate=%d", sampleRate)}
	default: // SourceNone: silencio, para que la TV siempre lleve pista de audio.
		layout := "stereo"
		if channels == 1 {
			layout = "mono"
		}
		return []string{"-re", "-f", "lavfi", "-i", fmt.Sprintf("anullsrc=r=%d:cl=%s", sampleRate, layout)}
	}
}

// videoInput devuelve los argumentos de entrada de una fuente de video.
func videoInput(s config.Source, w, h, fps int) []string {
	switch s.Kind {
	case config.SourceDShow:
		return []string{"-thread_queue_size", "1024", "-f", "dshow", "-rtbufsize", "256M", "-i", "video=" + s.Name}
	case config.SourceURL:
		return URLInput(s.Name)
	case config.SourceScreen:
		// Todo el escritorio (todas las pantallas); el filtro escala al tamaño de salida.
		return []string{"-thread_queue_size", "1024", "-f", "gdigrab", "-framerate", strconv.Itoa(fps),
			"-draw_mouse", "1", "-i", "desktop"}
	default: // SourceTest: barras de color con reloj.
		return []string{"-re", "-f", "lavfi", "-i", fmt.Sprintf("testsrc2=size=%dx%d:rate=%d", w, h, fps)}
	}
}

// URLInput lee video desde http/https: un archivo (MP4…) o un directo (HLS
// .m3u8, MPEG-TS, FLV…). -re lo lee a velocidad real (si no, un archivo se
// enviaría de golpe) y las opciones reconnect reintentan si el servidor de
// origen corta; si igual se cae, el supervisor relanza todo.
func URLInput(raw string) []string {
	return []string{"-thread_queue_size", "1024", "-re",
		"-reconnect", "1", "-reconnect_streamed", "1", "-reconnect_on_network_error", "1", "-reconnect_delay_max", "5",
		"-rw_timeout", "15000000", // 15 s sin datos del origen = error (en microsegundos)
		"-user_agent", "Club25Emisor",
		"-i", strings.TrimSpace(raw)}
}

// meterChain es la rama del filtro que mide el nivel y lo escribe en stdout.
// asetnsamples agrupa 1/MeterRate s de audio por medición.
func meterChain(in string, sampleRate int) string {
	return fmt.Sprintf("[%s]asetnsamples=n=%d:p=0,astats=metadata=1:reset=1:measure_overall=none:"+
		"measure_perchannel=Peak_level+RMS_level,ametadata=mode=print:file='pipe\\:1':direct=1,anullsink",
		in, sampleRate/MeterRate)
}

// audioChain prepara el audio (formato, ganancia) y lo divide en la salida y el medidor.
func audioChain(in string, sampleRate, channels int, gainDB float64, enhance bool, eq [10]float64) string {
	layout := "stereo"
	if channels == 1 {
		layout = "mono"
	}
	f := fmt.Sprintf("[%s]aresample=%d:async=1,aformat=sample_rates=%d:channel_layouts=%s", in, sampleRate, sampleRate, layout)
	// Orden: ganancia → ecualizador → MEJORA → limitador de seguridad → (salida y medidor).
	f += gainFilter(gainDB) + "," + eqChain(eq) + "," + enhanceChain(enhance) + "," + safetyLimiter
	return f + ",asplit=2[aout][ameter];" + meterChain("ameter", sampleRate)
}

// codecArgs devuelve codificador, formato del contenedor y content-type para Icecast.
func codecArgs(r config.Radio) (args []string, format, contentType string) {
	b := strconv.Itoa(r.BitrateKbps) + "k"
	switch r.Codec {
	case config.CodecAAC:
		return []string{"-c:a", "aac", "-b:a", b}, "adts", "audio/aac"
	case config.CodecHEAAC:
		return []string{"-c:a", "libfdk_aac", "-profile:a", "aac_he", "-b:a", b}, "adts", "audio/aac"
	case config.CodecHEAACv2:
		return []string{"-c:a", "libfdk_aac", "-profile:a", "aac_he_v2", "-b:a", b}, "adts", "audio/aac"
	case config.CodecOpus:
		// Opus trabaja siempre a 48 kHz (RadioSampleRate lo fuerza).
		return []string{"-c:a", "libopus", "-b:a", b, "-vbr", "on", "-application", "audio"}, "ogg", "audio/ogg"
	case config.CodecVorbis:
		return []string{"-c:a", "libvorbis", "-b:a", b}, "ogg", "audio/ogg"
	default: // MP3, CBR como BUTT
		return []string{"-c:a", "libmp3lame", "-b:a", b, "-abr", "0"}, "mp3", "audio/mpeg"
	}
}

// RadioSampleRate es la frecuencia que se usará de verdad (Opus solo admite 48 kHz).
func RadioSampleRate(r config.Radio) int {
	if r.Codec == config.CodecOpus {
		return 48000
	}
	return r.SampleRate
}

// IcecastURL arma icecast://usuario:clave@host:puerto/montaje (clave escapada).
func IcecastURL(r config.Radio) string {
	u := url.URL{Scheme: "icecast", User: url.UserPassword(r.User, r.Password),
		Host: fmt.Sprintf("%s:%d", r.Host, r.Port), Path: r.Mount}
	return u.String()
}

// RadioArgs arma el comando para la radio (solo audio → Icecast).
func RadioArgs(r config.Radio) []string {
	sr := RadioSampleRate(r)
	args := common()
	args = append(args, audioInput(r.Input, sr, r.Channels)...)
	args = append(args, "-filter_complex", audioChain("0:a", sr, r.Channels, r.GainDB, r.Enhance, r.EQ.Effective()), "-map", "[aout]")
	codec, format, ctype := codecArgs(r)
	args = append(args, codec...)
	args = append(args, "-ar", strconv.Itoa(sr), "-ac", strconv.Itoa(r.Channels))
	args = append(args, "-f", format, "-content_type", ctype, "-ice_public", "0")
	for _, kv := range [][2]string{{"-ice_name", r.StreamName}, {"-ice_genre", r.Genre},
		{"-ice_description", r.Description}, {"-ice_url", r.Website}} {
		if kv[1] != "" {
			args = append(args, kv[0], kv[1])
		}
	}
	return append(args, IcecastURL(r))
}

// TVURL arma la URL de publicación en MediaMTX con usuario y clave, igual que el panel.
func TVURL(t config.TV) (target, format string) {
	if t.Protocol == config.ProtoSRT {
		q := "streamid=publish:" + t.Path + ":" + t.User + ":" + t.Password + "&pkt_size=1316"
		return fmt.Sprintf("srt://%s:%d?%s", t.Host, t.SRTPort, q), "mpegts"
	}
	q := url.Values{}
	q.Set("user", t.User)
	q.Set("pass", t.Password)
	// url.Values ordena las claves: "pass" va antes que "user"; MediaMTX acepta cualquier orden.
	return fmt.Sprintf("rtmp://%s:%d/%s?%s", t.Host, t.RTMPPort, t.Path, q.Encode()), "flv"
}

// videoEncoderArgs devuelve el codificador H.264 con bitrate constante y
// fotogramas clave fijos (MediaMTX y HLS cortan los segmentos en ellos).
func videoEncoderArgs(t config.TV) []string {
	b := strconv.Itoa(t.VideoKbps) + "k"
	buf := strconv.Itoa(t.VideoKbps*2) + "k"
	gop := strconv.Itoa(t.FPS * t.KeyframeSec)
	var enc []string
	switch t.Encoder {
	case "h264_nvenc":
		enc = []string{"-c:v", "h264_nvenc", "-preset", "p4", "-tune", "ll", "-rc", "cbr", "-profile:v", "high"}
	case "h264_qsv":
		enc = []string{"-c:v", "h264_qsv", "-preset", "veryfast", "-profile:v", "high"}
	case "h264_amf":
		enc = []string{"-c:v", "h264_amf", "-usage", "lowlatency", "-rc", "cbr", "-profile:v", "high"}
	default:
		enc = []string{"-c:v", "libx264", "-preset", "veryfast", "-tune", "zerolatency", "-profile:v", "high",
			"-x264-params", "nal-hrd=cbr"}
	}
	return append(enc, "-b:v", b, "-minrate", b, "-maxrate", b, "-bufsize", buf,
		"-g", gop, "-keyint_min", gop, "-sc_threshold", "0", "-bf", "0")
}

// TVArgs arma el comando para la TV (video H.264 + audio AAC → MediaMTX).
func TVArgs(t config.TV) []string {
	const sr, ch = 44100, 2
	args := common()
	args = append(args, videoInput(t.Video, t.Width, t.Height, t.FPS)...)
	audioIn := "1:a"
	if t.Audio.Kind == config.SourceFromVideo {
		audioIn = "0:a" // el audio que trae el propio video (URL); no hay segunda entrada
	} else {
		args = append(args, audioInput(t.Audio, sr, ch)...)
	}
	// Escala manteniendo la proporción y rellena con negro hasta el tamaño de salida.
	v := fmt.Sprintf("[0:v]scale=%d:%d:force_original_aspect_ratio=decrease,pad=%d:%d:(ow-iw)/2:(oh-ih)/2,"+
		"setsar=1,fps=%d,format=yuv420p[vout]", t.Width, t.Height, t.Width, t.Height, t.FPS)
	gain := t.GainDB
	if t.Audio.Kind == config.SourceNone {
		gain = 0
	}
	args = append(args, "-filter_complex", v+";"+audioChain(audioIn, sr, ch, gain, t.Enhance, t.EQ.Effective()), "-map", "[vout]", "-map", "[aout]")
	args = append(args, videoEncoderArgs(t)...)
	args = append(args, "-c:a", "aac", "-b:a", strconv.Itoa(t.AudioKbps)+"k", "-ar", strconv.Itoa(sr), "-ac", strconv.Itoa(ch))
	target, format := TVURL(t)
	if format == "flv" {
		args = append(args, "-flvflags", "no_duration_filesize")
	}
	return append(args, "-f", format, target)
}

// Redact oculta claves en un comando o URL para mostrarlo en el registro.
func Redact(s string, secrets ...string) string {
	for _, sec := range secrets {
		if sec == "" {
			continue
		}
		s = strings.ReplaceAll(s, sec, "****")
		if esc := url.QueryEscape(sec); esc != sec {
			s = strings.ReplaceAll(s, esc, "****")
		}
		if esc := url.PathEscape(sec); esc != sec {
			s = strings.ReplaceAll(s, esc, "****")
		}
	}
	return s
}
