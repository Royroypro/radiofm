// Package audio es el motor de audio de Club25 Estudio: decks A/B con
// crossfade, carts de la botonera, entrada de micrófono con ducking,
// limitador del bus master, medidores y la E/S con la tarjeta de sonido
// (miniaudio vía github.com/gen2brain/malgo, WASAPI en Windows).
//
// Formato interno: float32 entrelazado, 44 100 Hz, estéreo, bloques de 10 ms.
//
// Reglas (ver docs/audio-engine.md):
//   - El reloj maestro es el callback de la tarjeta de salida. Dentro del
//     callback no se bloquea ni se reserva memoria: los decks leen de ring
//     buffers que llenan goroutines de decodificación (ffmpeg).
//   - La salida hacia el codificador de Icecast es un ring buffer que descarta
//     muestras si se atasca: nunca frena la salida FM.
//   - Este paquete no conoce la base de datos ni los servicios: recibe pistas
//     ya resueltas (ruta, cue-in, cue-out, ganancia) y emite eventos.
//
// Estado actual (fase 0): solo ListDevices. El motor llega en la fase 1.
package audio
