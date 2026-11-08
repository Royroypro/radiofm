# Motor de audio (`internal/audio`)

## Formato y bloques

- float32 entrelazado (L, R, L, R…), estéreo, **44 100 Hz**. Tarjetas a 48 kHz: miniaudio convierte.
- `Render(out)` procesa como máximo 4096 frames de una vez; los buffers internos se reservan al crear el motor.
- El reloj de software (`RunClock`) usa bloques de 10 ms y calcula los frames por tiempo real (sin deriva).

## Piezas

```
                 ┌────────── hilo de audio (callback de la tarjeta o RunClock) ─────────┐
 ffmpeg ─► ring ─┤ deck A ┐                                                            │
 ffmpeg ─► ring ─┤ deck B ┴─► música ─► × ducking ┐                                    │
 memoria ► ring ─┤ carts 0–23 ──────────────────► + ─► × master ─► limitador ─► out ───┼─► tarjeta
 captura ► ring ─┤ mic × ganancia ─(pico)─► Ducker┘                     │              │
                 │                                       medidores, silencio           │
                 │                                       ring del codificador ◄────────┼─► ffmpeg ─► Icecast
                 └─────────────────────────────────────────────────────────────────────┘
```

| Archivo | Qué hace |
|---|---|
| `ringbuf.go` | cola circular SPSC sin bloqueos (un productor, un consumidor) |
| `source.go` | `Source` (fuente de muestras), `PCMSource` (bytes f32le de ffmpeg), `MemorySource`, `Sine` |
| `player.go` | `playItem`: un ítem cargado en un slot; goroutine que llena su ring; volumen con rampas; eventos |
| `dsp.go` | `Ducker` (ataque/relajación/retención), `Limiter` (look-ahead 1,5 ms, techo −1 dBFS), medidores |
| `engine.go` | `Engine`: slots, comandos, `Render`, eventos, estado publicado |
| `clock.go` | `RunClock` (sin tarjeta), `RenderOffline` (tests), `RenderOfflineWaiting` (render a archivo) |
| `device.go` | `ListDevices`, `OpenOutput`, `OpenCapture` con malgo (solo con CGO) |

## Slots

- `DeckA` (0) y `DeckB` (1): música; reciben ducking.
- `CartSlot(0…23)`: botonera; no reciben ducking; pueden sonar varios a la vez.

## Reglas del hilo de audio

1. `Render` no bloquea, no reserva memoria, no escribe logs ni hace E/S.
2. Los demás métodos (`Load`, `Play`, `Fade`, `Stop`…) envían un **comando** por un canal con buffer;
   `Render` los aplica al principio de cada bloque. Si la cola está llena devuelven `ErrBusy`.
3. Los ítems que `Render` deja de usar se mandan a otra goroutine para cancelarlos (`collectTrash`).
4. El estado para la interfaz se publica con atómicos (`Status`, `Slot`): leerlo nunca frena el audio.
5. Los eventos (`Started`, `Segue`, `Ended`, `Silence`) se envían sin esperar a un canal de 256;
   si nadie lo lee, se pierden.

## Ciclo de una pista

1. `Load(slot, src, LoadOptions{Tag, GainDB, Length, SegueAt})` — empieza a decodificar y a llenar el
   ring (6 s por defecto). Conviene cargar ~20 s antes.
2. `Play(slot, fadeIn)` → `EventStarted`.
3. Al llegar a `SegueAt` → `EventSegue`: el orquestador (service/onair) lanza lo siguiente y funde esta
   pista (`Fade(slot, 0, d, true)`).
4. Al llegar a `Length` (cue-out − cue-in), al fin del archivo, al terminar un fundido con `stop`, o con
   `Stop` → `EventEnded`.

Si el decodificador no llega a tiempo, se emite silencio sin avanzar la posición y se cuenta en
`Status.Dropouts`.

## Micrófono y deriva

El callback de captura escribe en `MicInput()`. Como la tarjeta de captura puede tener otro reloj, si se
acumulan más de 200 ms el motor descarta el exceso hasta dejar 100 ms (latencia acotada). Si falta, el mic
aporta silencio. Usar la misma tarjeta para salida y entrada evita la deriva.

## Codificador

Cada bloque del bus se copia a `EncoderOutput()` (2 s). Si el codificador se atasca (red caída), lo que
no cabe se descarta y se cuenta en `Status.EncDropped`: la salida FM nunca se detiene.
Con la fuente de Icecast en modo "captura", `SetEncoderTap(false)` y el codificador lee de la entrada.

## Pruebas

`go test ./internal/audio/` renderiza sin tarjeta: crossfade, ducking (−12 dB con voz y vuelta a 0 dB),
limitador (pico ≤ techo), eventos, dropouts, descarte hacia el codificador, silencio y el ring buffer con
productor/consumidor concurrentes. Con `-race` (requiere CGO) no debe haber carreras.

Desde la línea de comandos:

```powershell
task cli -- render -xfade 2s mezcla.wav a.mp3 b.mp3   # sin tarjeta
task cli -- play -device "Altavoces" a.mp3 b.mp3        # por la tarjeta
```
