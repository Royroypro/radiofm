# Cambios

Formato: una sección por fase del plan ([../docs/estudio/plan.md](../docs/estudio/plan.md)).

## Fase 1 — motor de audio

- `internal/audio`: motor con 2 decks y 24 carts, ring buffers SPSC sin bloqueos, comandos por canal
  no bloqueante, rampas de volumen (crossfade), ducking con ataque/relajación/retención, limitador
  look-ahead (−1 dBFS), medidores pico/RMS, detector de silencio y salida al codificador que descarta
  si se atasca. Reloj de software (`RunClock`) y render offline.
- E/S real con malgo: `OpenOutput` y `OpenCapture` (por ID o nombre de dispositivo).
- `platform/ffmpeg`: `Locate`, `Decode` (PCM en streaming, sin ventana de consola) y `Probe` (ffprobe).
- `estudio-cli play` (tarjeta, crossfade) y `estudio-cli render` (WAV sin tarjeta).
- Tests renderizados sin tarjeta; `-race` limpio. Documentación: `docs/audio-engine.md`.

## Fase 0 — esqueleto (2026-11-08)

- Módulo Go `radioclub25/estudio` dentro del monorepo, con la estructura de carpetas del plan y un
  `doc.go` en cada paquete que describe lo que hará.
- Ventana Wails v2 + Svelte 5/TypeScript con la marca: versión y lista de tarjetas de sonido.
- `estudio-cli version` y `estudio-cli devices` (miniaudio vía malgo, WASAPI).
- Sin CGO, `audio.ListDevices` devuelve `audio.ErrNoCGO`: `go vet`/`go test` funcionan sin gcc.
- Scripts: `dev-setup.ps1` (Wails, Task, llvm-mingw verificado por SHA-256, PATH, ffmpeg, npm),
  `get-ffmpeg.ps1` (ffmpeg n8.1.3 GPL de BtbN, versión fijada y SHA-256) y `make-locutions.ps1`
  (140 locuciones provisionales HRS/MIN/TMP con SAPI).
- `Taskfile.yml` (setup, dev, build, installer, test, lint, cli, ffmpeg, locutions); los comandos con CGO
  pasan por `scripts/with-toolchain.ps1`, así funcionan aunque la terminal no tenga gcc en el PATH.
- Documentación: AGENTS.md, README, setup-windows, architecture, glossary, third-party y ADRs 0001–0005.
- Licencia GPL-3.0-or-later.

Cambios respecto al plan:
- `main.go` está en la raíz de `estudio/` (no en `cmd/estudio/`): Wails v2 compila el paquete main de
  la raíz del proyecto.
- ffmpeg 8.1 en lugar de 7.x: BtbN ya no publica builds 7.x.
- `go.work` no se versiona (se crea en local con `go work init . ./estudio`) para no cambiar la
  compilación del servidor en Docker/Linux.
