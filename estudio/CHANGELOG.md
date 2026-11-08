# Cambios

Formato: una sección por fase del plan ([../docs/estudio/plan.md](../docs/estudio/plan.md)).

## Fase 8 — robustez e instalador (versión 1.0.0)

- `platform/winsys`: instancia única (mutex global), evitar la suspensión, arranque con Windows (clave Run
  del usuario, sin administrador), aviso nativo si falla el arranque.
- `platform/tools`: revisión de la PC y descarga de herramientas (ffmpeg) desde el servidor de la radio
  según `/uploads/estudio/manifest.json`, con SHA-256; modo `club25-estudio.exe --setup` para el instalador
  y descarga automática al abrir si falta.
- Copia diaria de la base (`backups`, 14 días); aviso si el reloj de la PC difiere del servidor.
- Instalador NSIS en español: WebView2, accesos directos, `--setup` al final; no borra los datos al desinstalar.
- Ajustes → Sistema: revisión y descarga de herramientas. Documentación: `docs/user-guide.md`.

## Fase 7 — sincronización con el panel web

- `platform/apiclient`: cliente de `/api/studio/*` (config, cuñas con ETag, heartbeat, lo que suena,
  emisiones) y descarga verificada por SHA-256.
- `service/sync`: bucle de sincronización, cola local de emisiones (tabla `airplay_queue`) que se envía al
  volver la red, desactivación de cuñas retiradas, aplicación de la configuración del panel.
- Token del panel cifrado (DPAPI); Ajustes → Panel web; indicador en Al aire.
- Pendiente: prueba de punta a punta completa contra el servidor local (iniciada: token y cuña creados).

## Fase 6 — Al aire

- `service/onair`: orquestación del aire (un goroutine dueño del estado), expansión de elementos en
  segmentos (tanda con cortinas, hora, temperatura, silencio, modo), crossfade, eventos duros, modos
  Automático/Asistido/En vivo, registro de emisiones, emergencia por silencio o lista vacía, recuperación.
- Botonera (`domain.Cart`, tabla `carts`, audio en memoria, varios a la vez) y micrófono con ducking.
- `service/streaming`: Icecast con contraseña cifrada (`platform/secrets`, DPAPI), fuente bus/captura y metadata.
- Tarjeta de salida y micrófono según Ajustes (con respaldo al reloj de software si la tarjeta falla).
- Interfaz: **Al aire** (decks, cuenta atrás, intro, lo que sigue, vúmetros, modos, mic, botonera, estado de
  Icecast, avisos) como pantalla inicial, **Botonera** (configuración) y atajos (Espacio, F1–F12, numérico, Ctrl+M).
- `Scheduler.Upcoming` empieza cerca de la hora actual.
- Documentación: `docs/onair.md`.

## Fase 5 — tandas, hora, temperatura y eventos

- `service/spots`: cuñas locales y del panel, reparto equitativo en tandas por franja y vigencia sin
  repetir anunciante, replanificación automática (al cambiar la playlist, al cambiar cuñas y cada 10 min)
  y duración de la tanda con cortinas.
- `service/timeannounce` (HRS/MIN/TMP con verificación) y `platform/weather` (Open-Meteo con caché).
- `service/events`: decisión pura de eventos duros/blandos, saltos de atrasados y opcionales.
- `service/reports`: as-run y comprobante de cuñas, CSV.
- `Scheduler.Apply` para editar un día sin provocar bucles de aviso.
- Interfaz: **Cuñas** (lista, cuñas locales, tandas del día, replanificar) y **Reportes**; Ajustes con
  verificación de locuciones y prueba de temperatura; la Playlist se refresca sola.
- Documentación: `docs/spots-and-events.md`.

## Fase 4 — programación

- `service/rotation`: elección de la pista menos reciente con separación de título/artista, intérprete
  distinto del anterior, comparación normalizada (tildes, "feat.") y relajación en orden.
- `service/scheduler`: relojes (validación), parrilla 7×24 con excepciones, generador del día (eventos
  duros al minuto exacto, relleno, cierre de hora, opcionales), conservación de bloqueados y de lo emitido,
  edición (mover, quitar, bloquear, insertar) con horas recalculadas e IDs estables, generación automática
  de hoy y los días siguientes, reloj y parrilla iniciales.
- Interfaz: **Playlist** (día, horas, edición), **Relojes** (editor de slots) y **Parrilla** (pintar celdas,
  excepciones).
- Documentación: `docs/scheduling.md`.

## Fase 3 — biblioteca

- Base local SQLite (`platform/sqlite`, migraciones embebidas): categorías, pistas e historial.
- `service/library`: categorías iniciales del plan, importación de carpetas con ffprobe (4 en paralelo),
  título desde el nombre del archivo, re-importación sin perder cambios manuales, analizador en segundo
  plano (EBU R128 con ffmpeg → ganancia a −16 LUFS, cue-in/out, punto de mezcla) y verificación de archivos.
- `platform/config` (settings.json, carpeta de datos `%ProgramData%\Club25Estudio`), `platform/logger`
  (log rotativo) e `internal/app` (raíz de composición compartida por la ventana y la CLI).
- Interfaz: navegación lateral, pantalla **Biblioteca** (categorías, búsqueda, importación con progreso,
  editor de pista con cues y vigencia) y **Ajustes** (dispositivos, mezcla, Icecast, emisora, rotación,
  locuciones, seguridad).
- `with-toolchain.ps1` admite comandos con un solo argumento (`wails dev`).
- Documentación: `docs/library.md`.

## Fase 2 — emisión a Icecast

- `platform/ffmpeg.Encoder`: PCM del motor → ffmpeg → `icecast://` (MP3 128 kbps o AAC), contraseña
  escapada, descarte del audio viejo al reconectar, reconexión con espera exponencial (1 s → 60 s) y
  estado (conectado, desde, reintentos, último error, bytes).
- `platform/icecast.Client.UpdateSong`: metadata con `/admin/metadata` y usuario de fuente.
- `estudio-cli encode`: lista con crossfade hacia Icecast, con o sin tarjeta, y metadata por pista.
- Tests con un Icecast simulado (conexión cruda con `Expect: 100-continue`, como el real).
- Documentación: `docs/icecast.md`.

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
