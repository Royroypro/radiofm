# Plan: "Club25 Estudio" — automatización de radio para Windows (tipo Jazler SOHO)

## 1. Contexto
La web (Go, este repo) ya recibe la radio por **Icecast** (`radioclub25.sehuacho.com:8010`, mount `/radio`, usuario `source`, clave de `ICECAST_SOURCE_PASSWORD`; Icecast valida la fuente con `POST /internal/icecast/source` → `gate.RadioSourceAllowed`). Hoy la PC usaría BUTT, que solo emite: no programa.

Se quiere un programa de escritorio para la PC de la radio que **programe la radio** (biblioteca, relojes, parrilla, tandas de cuñas, jingles, hora locutada) y **saque el audio** a la tarjeta de sonido (consola/transmisor 93.3 FM) **y** a Icecast. Se desarrollará en **otro entorno Windows**, así que el plan debe bastar para que otro programador o agente lo construya.

Decisiones tomadas:
- Tecnología: **Go + Wails**, con miniaudio (malgo) y ffmpeg.
- Salidas: **tarjeta de sonido + Icecast**.
- La v1 incluye:
  - Funciones básicas tipo Jazler.
  - Botonera de jingles.
  - Micrófono o entrada en vivo con ducking.
  - Hora y temperatura locutadas.
- Integración **sincronizada con el panel web**: cuñas, programación, tema al aire y comprobante de emisión.

El nombre es propio ("Club25 Estudio"). No se copia la marca, la interfaz ni los recursos de Jazler; solo se toma la idea funcional.

## 2. Arquitectura
```
┌──────────── PC Windows: Club25 Estudio (un .exe, Wails) ─────────────┐
│ UI (Svelte+TS, WebView2)  ⇄  bindings Go                             │
│ Programador: parrilla → relojes → playlist del día + eventos/tandas │
│ Motor de audio (Go): decks A/B + crossfade, carts, mic (ducking)    │
│      │ PCM float32 44.1k estéreo                                     │
│      ├──► malgo/miniaudio ──► tarjeta de sonido ──► consola/FM 93.3  │
│      ├──► (opcional) salida de pre-escucha                           │
│      └──► ffmpeg.exe (stdin PCM → MP3 128k) ──► icecast://…:8010/radio│
│ Sync (HTTPS + token) ⇄ /api/studio/* de la web                       │
│ SQLite local (%ProgramData%\Club25Estudio)                           │
└──────────────────────────────────────────────────────────────────────┘
```
| Pieza | Proyecto (licencia) | Uso |
|---|---|---|
| Ventana/UI | Wails v2 (MIT) + Svelte/Vite | App nativa con WebView2 |
| Audio E/S | miniaudio vía `github.com/gen2brain/malgo` (MIT/Unlicense) | Salida a tarjeta y captura de mic/línea (WASAPI) |
| Decodificación, análisis y codificación | ffmpeg/ffprobe (LGPL/GPL, se distribuyen los .exe junto a la app) | Decodifica a PCM cualquier formato. `ebur128` y `silencedetect` sacan ganancia y puntos de cue. Codifica MP3 hacia Icecast |
| BD | `modernc.org/sqlite` (igual que el servidor) | Biblioteca, relojes, historial |
| Clima | Open-Meteo (gratis, sin clave) | Temperatura de Huacho |

Decodificar con ffmpeg como subproceso evita códecs en CGO y lee MP3, AAC, FLAC, WAV y OGG. El motor pre-decodifica la siguiente pista antes de que termine la actual.

## 3. Repositorio nuevo `radioclub25-estudio` (mismas normas que este)
```
cmd/estudio/main.go          # arranque Wails + inyección
cmd/estudio-cli/             # pruebas sin UI: play, encode, generate-log
internal/domain/             # Track, Category, Clock, ClockSlot, GridCell, Event, LogItem, Spot, Play; reglas puras
internal/service/
  library/   rotation/   scheduler/   events/   timeannounce/   sync/   onair/ (orquesta modos auto/asistido/vivo)
internal/audio/              # engine.go, deck.go, cart.go, mixer.go, ducking.go, meter.go, crossfade.go
internal/platform/
  ffmpeg/ (decoder, probe, analyzer, encoder)  icecast/ (metadata)  apiclient/ (web)  sqlite/  weather/  config/  logger/
internal/ui/                 # structs expuestos a Wails (sin lógica de negocio)
frontend/                    # Svelte: OnAir, Library, Clocks, Grid, Events, Spots, Reports, Settings
build/windows/               # icono, manifest, instalador NSIS
third_party/ffmpeg/          # ffmpeg.exe + ffprobe.exe + LICENSE (no en git; script de descarga)
docs/  AGENTS.md  CLAUDE.md  README.md  Taskfile.yml (o scripts PowerShell)
```
Las reglas de capas son las de `AGENTS.md` de este repo:
- `domain` no depende de nadie.
- `audio` no conoce la base de datos.
- `ui` solo llama a servicios.

Comentarios en español e identificadores en inglés. Tests de las reglas puras (rotación, parrilla, eventos) sin audio.

**Entorno Windows:**
- Go 1.27.
- Node LTS.
- Wails CLI v2.
- MSYS2 mingw-w64 gcc, porque malgo necesita CGO.
- WebView2 Runtime.
- ffmpeg 7.x.

`docs/setup-windows.md` lo explica paso a paso.

## 4. Funciones v1
**Biblioteca**
- Importar carpetas: ffprobe lee las etiquetas y la duración.
- Análisis en segundo plano:
  - Loudness EBU R128, para normalizar a −16 LUFS.
  - Cue-in y cue-out por silencio.
  - Punto de mezcla (segue) y duración de la intro.
- Categorías y subcategorías: Éxitos, Retro, Salsa, Cumbia, Baladas, Jingles, Cortinas, IDs, Cuñas, Hora, Temperatura, Emergencia.
- Cada pista se puede activar o desactivar y tiene fechas de vigencia.
- Búsqueda en la biblioteca.

**Relojes y parrilla**
- Un reloj es una plantilla de una hora: una lista de slots. Cada slot es una categoría, una pista fija, un evento o una tanda.
- La parrilla asigna un reloj a cada hora de la semana (7×24), con excepciones por fecha.

**Rotación (`service/rotation`, pura y testeada)**
- Elige la pista menos reciente de la categoría.
- Separación mínima de artista (min) y de título (h).
- Evita repetir al mismo intérprete en slots seguidos.
- Si ninguna pista cumple, relaja las reglas en orden y lo registra.

**Playlist del día**
- Se genera con antelación (hoy y mañana) y se puede editar: arrastrar, insertar, borrar y bloquear.

**Eventos**
- Pueden ser **duros** (a la hora exacta: funden la pista actual) o **blandos** (al terminar la pista).
- Tipos de evento:
  - Tanda de cuñas.
  - Hora locutada.
  - Temperatura.
  - Pista o carpeta.
  - Cambio de modo.
  - Silencio.

**Tandas**
- Cortina de entrada, cuñas y cortina de salida.
- Las cuñas vienen del panel web, con:
  - vigencia;
  - veces por día;
  - franja horaria;
  - reparto equitativo entre tandas;
  - sin repetir anunciante en la misma tanda.

**Hora y temperatura**
- Usa locuciones grabadas: `HRS00–23`, `MIN00–59`, `TMP-10…45`, con nombres configurables.
- La temperatura viene de Open-Meteo con caché de 15 minutos. Si no hay red, solo se locuta la hora.

**Al aire**
- Pantalla principal:
  - Decks A y B con forma de onda simple.
  - Cuenta atrás e intro.
  - Lo que sigue.
  - Medidores VU.
  - Reloj.
  - Estado de Icecast (conectado, kbps y reintentos).
  - Estado de la sincronización.
- Modos:
  - **Automático**.
  - **Asistido**: avanza con el botón o la tecla Espacio.
  - **En vivo**: la música se pausa y queda el mic más la botonera.

**Botonera**
- Páginas de 4×6 botones de color.
- Atajos F1–F12 y teclado numérico.
- Varios carts pueden sonar a la vez.
- Modo "stop al pulsar de nuevo".

**Micrófono o entrada de línea**
- Se elige el dispositivo de captura, con ganancia.
- Ducking automático de la música (por ejemplo, −12 dB, con ataque y relajación configurables).
- Se activa con un botón o una tecla.

**Salidas**
- Dispositivo principal (FM/consola).
- Pre-escucha opcional.
- Codificador Icecast:
  - MP3 128 kbps a 44,1 kHz; AAC opcional.
  - Reconexión con espera exponencial.
  - La metadata se actualiza con `GET /admin/metadata?mount=/radio&mode=updinfo&song=Artista - Título`, usando el usuario `source` en el puerto 8010.
  - Cuñas y jingles se pueden excluir de la metadata.

**Seguridad operativa**
- Detector de silencio: si el bus queda en silencio N segundos, suena la playlist de emergencia.
- Tras un cierre inesperado retoma la posición de la playlist.
- Puede arrancar con Windows.
- Log en archivo rotativo.
- Copia diaria de la base de datos.

**Reportes**
- Historial de lo emitido (as-run).
- Comprobante de cuñas por anunciante y rango de fechas, en CSV, con fecha, hora, duración y si se emitió completa.

## 5. Cambios en el servidor (este repo, se pueden hacer aquí en Linux)
- **Migración `0002_studio.sql`:**
  - Nuevo tipo de anuncio `fm_spot` (cuña para la emisión) en `ads`, con columnas `plays_per_day`, `hour_from` y `hour_to`.
  - Tabla `ad_airplays` con `id`, `ad_id`, `aired_at`, `duration_ms`, `complete` y `play_uuid` (único, para no duplicar reenvíos).
  - Tabla `studio_state` (una fila) con `token_hash`, `last_seen`, `version` y `now_playing_json`.
- **Dominio y servicio:** `domain/studio.go` y `service/studio`, que se encarga de:
  - autenticar el token;
  - guardar heartbeats y lo que suena;
  - registrar las emisiones de forma idempotente.

  `service/ads` admite el tipo `fm_spot` y suma las emisiones en las estadísticas.
- **API** en `/api/studio/*`, con el middleware `StudioToken` (Bearer contra el SHA-256 guardado, comparación en tiempo constante). No usa CSRF, igual que el resto de `/api/`.

  | Método y ruta | Uso |
  |---|---|
  | `GET config` | Nombre, frecuencia, mount, host/puerto de Icecast, zona horaria, coordenadas para el clima |
  | `GET spots` | Cuñas `fm_spot` vigentes: id, anunciante, URL del archivo, sha256, duración, vigencia, veces/día, franja |
  | `GET programs` | Programación semanal (la que ya existe), para nombrar los bloques |
  | `POST heartbeat` | Versión, modo, estado del codificador |
  | `POST nowplaying` | Artista, título, tipo y fin previsto. `/api/status` lo usa si es más reciente que la metadata de Icecast |
  | `POST airplays` | Lote de emisiones con `play_uuid`. La PC guarda una cola local si no hay red |

  Con la licencia en bloqueo total, la API responde 403 y Icecast ya rechaza la fuente; la PC sigue sonando por FM y muestra "servicio web suspendido".
- **Panel admin → "Estudio (PC)":**
  - Generar o revocar el token (se muestra una sola vez).
  - Última conexión, versión y lo que suena.
  - Formulario de cuñas FM dentro de Publicidad.
  - Comprobante de emisión en CSV.
- **Documentación:** `docs/api.md`, `docs/streaming.md` (la opción "Club25 Estudio" junto a BUTT), `docs/adr/0005-estudio-windows.md` y `AGENTS.md`.
- **Reutilizar:**
  - `storage` (archivos de cuñas).
  - El patrón de handlers de `public.APIStatus` / `APIEvent`.
  - `audit` para las acciones sobre el token.
  - `testutil` para los tests.

## 6. Fases (cada una con código, tests, documentación y verificación)
| # | Dónde | Entrega |
|---|---|---|
| S | Servidor (aquí) | Migración, `service/studio`, `/api/studio/*`, página admin, cuñas FM, comprobante, docs y tests con `httptest` |
| 0 | Windows | Repo, toolchain, Wails "hola", script que baja ffmpeg, AGENTS.md, docs/setup-windows.md, docs/architecture.md |
| 1 | Windows | Motor de audio: decoder ffmpeg → deck → mixer → malgo; crossfade y cue. Probado con `estudio-cli play a.mp3 b.mp3` |
| 2 | Windows | Codificador a Icecast, metadata, reconexión. `estudio-cli encode` y la web muestra "En vivo" con el título |
| 3 | Windows | Biblioteca: importar, analizar loudness/cue, categorías, UI de la biblioteca |
| 4 | Windows | Rotación, relojes, parrilla, generador y editor de playlist (con tests de reglas puras) |
| 5 | Windows | Eventos duros y blandos, tandas, hora y temperatura |
| 6 | Windows | Pantalla Al aire, modos, botonera, mic con ducking, atajos, VU |
| 7 | Windows | Sync con la web: config, cuñas (descarga y verificación sha256), nowplaying, heartbeat, cola de emisiones |
| 8 | Windows | Robustez: silencio, recuperación, arranque con Windows, backups, instalador NSIS (`wails build -nsis`), docs/user-guide.md |

S y 0–2 pueden avanzar en paralelo. 7 depende de S.

## 7. Verificación
- **Servidor:** `make lint && make test`. Con `curl` y un token: `GET /api/studio/spots`, un `POST /api/studio/airplays` repetido que no duplica, y el CSV del comprobante en el panel. Con la licencia en `total`, la API responde 403.
- **Escritorio, tests unitarios** (`go test ./...` en Windows):
  - rotación (separaciones y relajación);
  - generación de playlist por parrilla;
  - colocación de eventos duros y blandos;
  - reparto de cuñas por tandas;
  - mezcla del motor renderizada sin tarjeta, comparando niveles de crossfade y ducking sobre senos generados.
- **De punta a punta en la PC:**
  1. Importar unas 50 pistas, crear 2 relojes y la parrilla, y generar el día.
  2. En modo automático suena por la tarjeta.
  3. https://radioclub25.sehuacho.com pasa a "En vivo" con el título correcto.
  4. Una tanda emite la cuña creada en el panel y aparece en Estadísticas y en el comprobante.
  5. La hora locutada sale a la hora en punta.
  6. Al desconectar la red, el FM sigue y el codificador reconecta solo.
  7. Al matar el proceso y reabrirlo, retoma la playlist.
- **Requisito externo:** el router debe reenviar 8010/tcp a 15.14.12.2 para que la PC emita desde fuera (ya documentado en `docs/deployment.md`).
