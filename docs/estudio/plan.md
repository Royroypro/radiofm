# Plan: "Club25 Estudio" — automatización de radio para Windows

> Estado: Fase S (servidor) implementada en la rama `fase-s-api-estudio`. El contrato vigente de la
> API está en [docs/api.md](../api.md) → "API del estudio"; si difiere de este plan, manda `docs/api.md`.

## 0. Qué cambia respecto al plan original (resumen del afinado)
0. **Decisiones confirmadas:** licencia del estudio **GPL-3.0-or-later**; **monorepo** (el estudio vive en `radiofm/estudio/` con su propio `go.mod`); **una rama + PR por fase**; locuciones de hora/temperatura **provisionales generadas con la voz de Windows (SAPI)**.
1. **Todo se hace en esta PC** (`E:\RADIOFM`): se clona `https://github.com/Royroypro/radiofm.git` en `E:\RADIOFM\radiofm`; el servidor sigue en la raíz y el estudio en `radiofm/estudio/`. **No hace falta otro servidor**: el VPS ya tiene web + Icecast (8010 público) + `/uploads`; el clima es Open-Meteo (externo, sin clave).
2. **Cuñas FM en tablas propias (`fm_spots`, `fm_spot_airplays`), no como tipo nuevo en `ads`.** Motivo: `ads.type` tiene un `CHECK (type IN (...))` en `0001_init.sql`; SQLite no permite cambiarlo sin reconstruir la tabla, y `db.Migrate` corre cada migración en una transacción con `foreign_keys=ON` → hacer `DROP TABLE ads` borraría en cascada `ad_events`. Además la cuña FM necesita campos que `ads` no tiene (anunciante, sha256, duración, veces/día, franja).
3. **Fuente de Icecast configurable** (respuesta "ambas"): `bus` = mezcla interna de la PC (mic entra a la PC) o `captura` = entrada de línea que trae la salida de la consola (mic en consola). El ducking automático solo aplica en modo `bus`.
4. **Reloj maestro y deriva de audio** explicitados: el motor lo marca la tarjeta de salida; el codificador lee de un ring buffer que nunca bloquea al motor; la captura de mic con otro dispositivo pasa por un buffer con corrección de deriva.
5. **Decodificación en streaming** (no pre-decodificar la pista entera: 5 min en float32 estéreo ≈ 106 MB). Se arranca el decoder de la siguiente pista ~20 s antes.
6. **Limitador en el bus master** (techo −1 dBTP) antes de la tarjeta y el codificador, para no saturar el transmisor.
7. **Relleno y back-timing** en la parrilla: categoría de relleno y tolerancia para que la hora cuadre con los eventos duros.
8. **Seguridad local**: token de la web y clave de fuente guardados con DPAPI (no en texto plano); una sola instancia; evitar suspensión de Windows; reloj sincronizado por NTP.
9. **Metadata Icecast**: se verifica en la fase 2 que `/admin/metadata` con usuario `source` funciona con el `stream_auth` por URL de Icecast 2.4.4; si no, plan B: la web la actualiza con sus credenciales de admin al recibir `POST /api/studio/nowplaying` (el servidor ya tiene `platform/icecast.Client` con admin).
10. Se mantiene el **orden original de fases** (sin hito MVP temprano).

## 1. Contexto
La web (Go, repo `radiofm`, módulo `radioclub25`) recibe la radio por **Icecast 2.4.4** (`radioclub25.sehuacho.com:8010` → contenedor `:8000`, mount `/radio`, usuario `source`, clave `ICECAST_SOURCE_PASSWORD`). Icecast valida la fuente con `stream_auth` → `POST /internal/icecast/source` → `gate.RadioSourceAllowed` (exige radio habilitada y licencia no bloqueada). Hoy la PC usaría BUTT, que solo emite.

Se quiere un programa de escritorio que **programe la radio** (biblioteca, relojes, parrilla, tandas, jingles, hora locutada) y **saque el audio** a la tarjeta (consola/transmisor 93.3 FM) **y** a Icecast, sincronizado con el panel web (cuñas, programación, tema al aire, comprobante de emisión). Nombre propio; no se copia marca, interfaz ni recursos de Jazler.

**Entorno de esta PC (comprobado):** Go 1.27.3 ✔, git ✔, ffmpeg (choco) ✔, Node v23.3 (no LTS → instalar Node 24 LTS), **falta** Wails CLI, gcc (MSYS2 mingw-w64 o llvm-mingw) y Task. `make` no existe en Windows: en el servidor se usan los comandos Go directos (`go test ./...`, `gofmt -l .`, `go vet ./...`).

## 2. Arquitectura del estudio
```
┌──────────── PC Windows: Club25 Estudio (un .exe, Wails v2) ───────────────┐
│ UI (Svelte+TS, WebView2)  ⇄  bindings Go (internal/ui)                    │
│ Programador: parrilla → relojes → playlist del día + eventos/tandas       │
│ Motor de audio (Go, float32 44.1k estéreo, bloques de 10 ms)              │
│   decks A/B (decoder ffmpeg en streaming) + carts + mic ─► mixer          │
│   ─► ducking ─► limitador master ─► BUS                                   │
│ BUS ─► malgo/WASAPI ─► tarjeta principal ─► consola/FM   (reloj maestro)  │
│ BUS ─► (opcional) pre-escucha                                             │
│ Fuente Icecast = BUS  ó  CAPTURA (line-in = salida de consola)            │
│   └► ring buffer no bloqueante ─► ffmpeg.exe (stdin PCM → MP3 128k)       │
│        ─► icecast://source@host:8010/radio                                │
│ Sync (HTTPS + Bearer) ⇄ /api/studio/* ; SQLite en %ProgramData%\Club25Estudio │
└───────────────────────────────────────────────────────────────────────────┘
```
| Pieza | Proyecto (licencia) | Uso |
|---|---|---|
| Ventana/UI | Wails v2 (MIT) + Svelte/Vite | App nativa con WebView2 |
| Audio E/S | `github.com/gen2brain/malgo` (miniaudio) | Salida y captura WASAPI (CGO) |
| Decodificar / analizar / codificar | ffmpeg/ffprobe 7.x como subproceso | PCM de MP3/AAC/FLAC/WAV/OGG; `ebur128`, `silencedetect`, picos para forma de onda; MP3 a Icecast. Build GPL de BtbN (compatible con la licencia GPL del estudio), versión fijada y verificada por sha256 |
| BD | `modernc.org/sqlite` (como el servidor) | Biblioteca, relojes, historial, cola de sync |
| Clima | Open-Meteo | Temperatura de Huacho (caché 15 min) |
| Secretos | DPAPI (`golang.org/x/sys/windows` CryptProtectData) | Token web y clave de fuente |

**Reglas del motor de audio (críticas):**
- El callback de la tarjeta principal pide N frames → el mixer los produce. Si no hay tarjeta, un reloj de software (`time.Ticker`) hace de maestro.
- Nada en el callback bloquea ni asigna memoria: decks leen de ring buffers llenados por goroutines de decodificación.
- El tap hacia el codificador escribe en un ring buffer; si ffmpeg/Icecast se atasca, se **descartan** muestras y se cuenta (nunca se frena el FM).
- Mic en otro dispositivo: ring buffer con corrección de deriva (mantener nivel de llenado objetivo descartando/duplicando frames de a uno en zonas de bajo nivel). Recomendado usar la misma tarjeta (dúplex de miniaudio) para evitarlo.
- Modo `captura`: el codificador se alimenta directo del callback de captura (no se mezcla con nada → sin deriva).
- Tarjetas a 48 kHz: miniaudio convierte; el motor trabaja siempre a 44.1 kHz.
- Ganancia por pista = −16 LUFS − loudness medido (tope +6 dB); limitador look-ahead en el master.

## 3. Carpeta `radiofm/estudio/` (módulo Go `radioclub25/estudio`, mismas normas que `radiofm/AGENTS.md`)
Módulo aparte para que el servidor siga compilando con `CGO_ENABLED=0` en Linux sin arrastrar malgo/Wails. El `Dockerfile` y el `Makefile` del servidor no tocan `estudio/` (añadir `estudio/` a `.dockerignore`). Un `go.work` en la raíz permite abrir ambos módulos en el editor. El contrato de la API vive en un solo sitio: `docs/api.md` (sección Estudio), y el cliente del estudio tiene tests contra respuestas de ejemplo copiadas de ese documento.
```
estudio/
LICENSE                      # GPL-3.0-or-later (el servidor conserva su licencia actual)
cmd/estudio/main.go          # arranque Wails + inyección (único sitio que construye todo)
cmd/estudio-cli/             # sin UI: play, encode, analyze, generate-log, render (WAV offline)
internal/domain/             # Track, Category, Clock, ClockSlot, GridCell, Event, LogItem, Spot, Play, Cart; reglas puras
internal/service/
  library/ rotation/ scheduler/ events/ spots/ timeannounce/ sync/ onair/ reports/
internal/audio/              # engine, deck, cart, mixer, ducking, limiter, meter, crossfade, ringbuf, drift
internal/platform/
  ffmpeg/ (decoder, probe, analyzer, encoder)  icecast/ (metadata)  apiclient/  sqlite/ (+migrations)
  weather/  config/  logger/  secrets/ (DPAPI)  winsys/ (instancia única, no-sleep, autoarranque)
internal/ui/                 # structs para Wails, sin lógica de negocio
frontend/                    # Svelte: OnAir, Library, Clocks, Grid, Log, Events, Spots, Carts, Reports, Settings
build/windows/               # icono, manifest, NSIS
third_party/ffmpeg/          # ffmpeg.exe/ffprobe.exe/LICENSE (fuera de git; scripts/get-ffmpeg.ps1 con sha256)
scripts/                     # get-ffmpeg.ps1, make-locutions.ps1 (SAPI), dev-setup.ps1
testdata/                    # audios cortos generados (senos, silencios) — nunca música con copyright
docs/ (setup-windows, architecture, audio-engine, scheduling, glossary, third-party, user-guide, adr/)
AGENTS.md  CLAUDE.md  README.md  CHANGELOG.md  Taskfile.yml
```
Capas: `domain` no depende de nadie; `audio` no conoce la BD ni los servicios; `service` usa interfaces de `domain`; `ui` solo llama a servicios. Comentarios en español, identificadores en inglés. Tests de reglas puras sin audio.

### 3.1 Código abierto en que nos apoyamos
Regla: el estudio es GPL-3.0-or-later, así que se puede **reutilizar o portar** código de proyectos GPL-2.0-*or-later*, GPL-3.0, LGPL, MIT, BSD, Apache-2.0 o dominio público. **No** se copia código de proyectos solo GPL-2.0 (sin "or later") ni AGPL (LibreTime): de esos se toman ideas y diseño. Todo lo reutilizado o portado se anota en `docs/third-party.md` (proyecto, versión/commit, archivo de origen, licencia, qué se tomó) y con un comentario `// Basado en <proyecto>/<archivo> (<licencia>)` en el archivo Go. Antes de portar, comprobar la cabecera de licencia del archivo concreto.

| Proyecto | Licencia | Qué tomamos |
|---|---|---|
| **miniaudio** / **malgo** | MIT-0 / Unlicense | Dependencias directas: E/S WASAPI, dúplex, conversión de formato. Sus ejemplos (`simple_duplex`, ring buffer `ma_pcm_rb`) guían `audio/ringbuf` y `audio/drift` |
| **Wails v2** (plantilla `svelte-ts`) | MIT | Esqueleto de la app, bindings, eventos Go→UI, `wails build -nsis` |
| **FFmpeg** | LGPL/GPL | Subproceso: decodificar, `ebur128`, `silencedetect`, codificar y enviar a Icecast. Su `af_alimiter.c` es referencia para nuestro limitador |
| **Mixxx** | GPL-2.0-or-later | Crossfade y curvas del crossfader, *talkover ducking*, AutoDJ (transición por punto de mezcla), detección de intro/outro, render de forma de onda por picos, mapeo de teclado |
| **Liquidsoap** | GPL-2.0-or-later | Diseño de `smart_crossfade` (transición según niveles de salida/entrada), fallback por silencio (`fallback` + `blank.strip`), normalización |
| **Rivendell** | GPL-2.0 (verificar archivo por archivo) | Modelo de datos de radio: carts, relojes, parrilla, *log* del día, eventos duros/blandos, tipos de transición (PLAY/SEGUE/STOP), reglas de separación de artista |
| **LibreTime** | AGPL-3.0 | Solo ideas: *smart blocks*, criterios de selección, comprobantes |
| **BUTT** / **libshout** | GPL-2.0 / LGPL | Protocolo de fuente Icecast y actualización de metadata |
| **go-mp3** (hajimehoshi) | Apache-2.0 | Opcional: decodificar MP3 en Go puro para tests sin ffmpeg |

### 3.2 Documentación para personas y agentes de IA
- `estudio/AGENTS.md` (y `CLAUDE.md` que lo enlaza): mapa de carpetas, reglas de capas, comandos (`task dev|test|lint|build|ffmpeg`), cómo probar sin tarjeta de sonido, "recetas" (añadir un tipo de evento, una categoría, un endpoint de sync, una pantalla), qué **no** hacer (bloquear el callback de audio, SQL fuera de `platform/sqlite`, lógica en `ui`).
- Cada paquete con `doc.go` en español: responsabilidad, quién lo usa, invariantes.
- `docs/glossary.md`: cuña, tanda, cortina, ID, reloj, parrilla, log, segue, cue-in/out, intro, evento duro/blando, ducking, as-run, comprobante — con el identificador Go equivalente.
- `docs/architecture.md` (diagrama + flujo de un bloque de audio), `docs/audio-engine.md` (reloj maestro, buffers, deriva, latencias), `docs/scheduling.md` (generación del día paso a paso con un ejemplo).
- ADRs en `estudio/docs/adr/` para cada decisión: Wails, ffmpeg como subproceso, GPL, monorepo, fuente Icecast bus/captura, tablas `fm_spots`.
- `CHANGELOG.md` por fase; cada PR describe qué se hizo, cómo se verificó y qué queda.
- El `AGENTS.md` raíz del repo añade una sección que apunta a `estudio/AGENTS.md`.

### 3.3 Flujo de trabajo
- Una rama por fase (`fase-s-api-estudio`, `fase-0-esqueleto`, …), commits pequeños, PR a `main` con la checklist de verificación de la fase; no se fusiona sin tu revisión.
- Antes de programar cada fase se revisa si alguna duda nueva cambia el plan y se pregunta.

## 4. Funciones v1
**Biblioteca**
- Importar carpetas (ffprobe: etiquetas, duración). Ruta guardada relativa a una "raíz de música" configurable (para mover discos).
- Análisis en segundo plano (cola, 1–2 procesos ffmpeg): loudness R128, cue-in/cue-out por silencio (−45 dB), punto de mezcla (segue, por caída de nivel en los últimos 15 s), intro manual o detectada, picos para la forma de onda (archivo `.peaks` junto a la BD).
- Categorías/subcategorías (Éxitos, Retro, Salsa, Cumbia, Baladas, Jingles, Cortinas, IDs, Cuñas, Hora, Temperatura, Emergencia, Relleno).
- Activar/desactivar, vigencia, búsqueda; marca "archivo no encontrado" sin romper la rotación.

**Relojes y parrilla**
- Reloj = plantilla de una hora: slots (categoría, pista fija, evento, tanda).
- Parrilla 7×24 con excepciones por fecha. Zona horaria `America/Lima` fija (sin horario de verano).
- **Relleno y back-timing:** el generador calcula horas estimadas; si la hora queda corta, añade de la categoría Relleno; si queda larga, marca pistas "recortables" al final. En ejecución, el siguiente evento duro funde la pista que suene (fade 2 s).

**Rotación (`service/rotation`, pura y testeada)**
- Menos reciente de la categoría; separación de artista (min) y de título (h); no repetir intérprete en slots seguidos.
- Si nada cumple, relaja en orden (artista → título → cualquiera) y lo registra en el log de generación.

**Playlist del día**
- Se genera hoy + mañana a las 00:05 y bajo demanda; editable (arrastrar, insertar, borrar, bloquear). Regenerar respeta lo bloqueado.

**Eventos**: duros (hora exacta, funden lo actual) o blandos (al terminar la pista). Tipos: tanda, hora, temperatura, pista/carpeta, cambio de modo, silencio. Prioridad si coinciden: tanda > hora > resto.

**Tandas y cuñas** (`service/spots`)
- Cortina de entrada, cuñas, cortina de salida.
- Cuñas del panel web con vigencia, veces/día, franja horaria, reparto equitativo entre tandas de la franja, sin repetir anunciante en la misma tanda.
- **Replanificación:** cada sync que cambie las cuñas recalcula solo las tandas futuras del día (no regenera la música).
- "Emitida completa" = sonó ≥ duración − 0,5 s.

**Hora y temperatura**: locuciones `HRS00–23`, `MIN00–59`, `TMP-10…45` (nombres configurables). Provisionales: `scripts/make-locutions.ps1` las genera con SAPI (voz española de Windows) en WAV y ffmpeg las pasa a MP3; se reemplazan por las del locutor copiando archivos con el mismo nombre. Sin red → solo hora. Comprobar al arrancar que el reloj de Windows está sincronizado (`w32tm /query /status`) y avisar si no.

**Al aire**: decks A/B con forma de onda, cuenta atrás e intro, lo que sigue, VU (pico + RMS), reloj, estado Icecast (conectado, kbps, reintentos, muestras descartadas), estado de sync. Modos **Automático**, **Asistido** (Espacio), **En vivo** (música en pausa, mic + botonera).

**Botonera**: páginas 4×6, F1–F12 y teclado numérico, varios carts a la vez, "stop al pulsar de nuevo". Carts pre-decodificados en memoria (son cortos; límite 30 s por cart).

**Micrófono / entrada**: dispositivo de captura y ganancia; ducking (−12 dB, ataque 50 ms, relajación 500 ms configurables) por botón/tecla o por umbral de voz. En modo `captura` el botón de mic solo baja la música de la PC (opcional).

**Salidas**
- Principal (FM/consola), pre-escucha opcional.
- Icecast: fuente `bus` o `captura`; MP3 128 kbps 44.1 kHz (AAC opcional); `-content_type audio/mpeg`, `-ice_name "Radio Club 25"`; reconexión con espera exponencial (1 s → 60 s) relanzando ffmpeg.
- Metadata: `GET http://host:8010/admin/metadata?mount=/radio&mode=updinfo&charset=UTF-8&song=Artista%20-%20Título` (Basic `source`); excluir cuñas/jingles (configurable, muestra el nombre de la radio). Plan B en §0.9.

**Seguridad operativa**
- Detector de silencio en el BUS (−50 dB durante N s) → playlist de emergencia; si el motor se cuelga, un watchdog lo reinicia.
- Estado persistido cada 5 s → tras cierre inesperado retoma la playlist en el ítem que sonaba.
- Instancia única (mutex con nombre); `SetThreadExecutionState` para que Windows no suspenda.
- Arranque con Windows por **Programador de tareas al iniciar sesión** (no servicio: los servicios no tienen sesión de audio/WebView).
- Log rotativo; copia diaria de la BD (`VACUUM INTO`), 14 días.

**Reportes**: as-run; comprobante de cuñas por anunciante y fechas en CSV (fecha, hora, duración, completa). La web tiene el comprobante oficial a partir de los airplays sincronizados.

## 5. Cambios en el servidor (`E:\RADIOFM\radiofm`)
Seguir los pasos que documentan `internal/repository/sqlite/sqlite.go` (tabla nueva) y `router.go` (ruta nueva).

- **Migración `0002_studio.sql`** (no toca `ads`):
  - `fm_spots` (id, advertiser, name, file_url, sha256, duration_ms, starts_at, ends_at, plays_per_day, hour_from, hour_to, active, created_at, updated_at).
  - `fm_spot_airplays` (id, spot_id → fm_spots ON DELETE CASCADE, aired_at, duration_ms, complete, play_uuid UNIQUE); índices por (spot_id, aired_at) y aired_at.
  - `studio_state` (id=1): token_hash, token_created_at, last_seen, version, mode, encoder_json, now_playing_json, now_playing_at.
- **Dominio**: `domain/studio.go` (FMSpot con `IsLive(t)` y `InHours(h)`, Airplay, StudioState, interfaces de repositorio). Repos en `repository/sqlite/fm_spots.go` y `studio.go` con tests.
- **Servicio `service/studio`**: generar/revocar token (32 bytes aleatorios, guarda SHA-256, se muestra una vez), `Authenticate` en tiempo constante, heartbeat, nowplaying, `RecordAirplays` idempotente (`INSERT ... ON CONFLICT(play_uuid) DO NOTHING`), comprobante. Al subir una cuña, calcular sha256 y duración (duración la envía el formulario o se lee con un parser MP3 simple; si no, la PC la mide y la reporta).
- **Middleware `StudioToken`** en `middleware/` y grupo `/api/studio` con `PublicLicenseGuard` → con bloqueo total responde `503 {"error":"suspended"}` (comportamiento existente del guard para `/api/`). `/api/` ya está exento de CSRF.

  | Método y ruta | Uso |
  |---|---|
  | `GET /api/studio/config` | Nombre, frecuencia, host/puerto/mount de Icecast, zona horaria, coordenadas (no la clave de fuente: se configura en la PC) |
  | `GET /api/studio/spots` | Cuñas vigentes: id, anunciante, URL, sha256, duración, vigencia, veces/día, franja; con `ETag` |
  | `GET /api/studio/programs` | Programación semanal (de `service/programs`) |
  | `POST /api/studio/heartbeat` | Versión, modo, estado del codificador |
  | `POST /api/studio/nowplaying` | Artista, título, tipo, fin previsto |
  | `POST /api/studio/airplays` | Lote (≤500) con `play_uuid`; responde cuántos nuevos |

- **`/api/status`**: si `now_playing_at` tiene < 2 min y la fuente está online, usar ese título en lugar del de Icecast.
- **Panel admin**: página "Estudio (PC)" (rol admin): token, última conexión, versión, modo, lo que suena; sección "Cuñas FM" (editor) con formulario y subida vía `storage`; comprobante CSV en `/admin/fm-spots/report.csv`; acciones del token en `audit`. Plantillas en `web/templates/admin/`.
- **Límites del plan**: decidir si las cuñas FM cuentan en `max_active_ads` (propuesta: no, contador aparte).
- **Docs**: `docs/api.md`, `docs/streaming.md` (Club25 Estudio junto a BUTT), `docs/database.md`, `docs/adr/0005-estudio-windows.md`, `docs/estudio/plan.md` (sustituir por este plan), `AGENTS.md`.
- **Reutilizar**: `platform/storage`, patrón `writeJSON` de `public.go`, `service/audit`, `internal/testutil`, `platform/icecast.Client` (plan B de metadata).

## 6. Fases (cada una: código + tests + docs + verificación)
| # | Repo | Entrega |
|---|---|---|
| S | radiofm | Clonar en `E:\RADIOFM\radiofm`, migración 0002, dominio/repos/servicio studio, `/api/studio/*`, panel Estudio y Cuñas FM, comprobante, docs, tests `httptest` |
| 0 | estudio | Toolchain (Wails, gcc, Node LTS, Task), `radiofm/estudio/` + `go.work`, Wails "hola", `get-ffmpeg.ps1`, `make-locutions.ps1`, LICENSE GPL, AGENTS.md, glossary, third-party, setup-windows, architecture, ADRs iniciales |
| 1 | estudio | Motor: decoder streaming → deck → mixer → limitador → malgo; crossfade y cue; render offline. `estudio-cli play a.mp3 b.mp3` |
| 2 | estudio | Codificador Icecast (fuente bus/captura), ring buffer, metadata (verificar plan A/B), reconexión. `estudio-cli encode`; la web pasa a "En vivo" |
| 3 | estudio | Biblioteca: importar, análisis, categorías, UI |
| 4 | estudio | Rotación, relojes, parrilla, relleno, generador y editor de playlist |
| 5 | estudio | Eventos duros/blandos, tandas (con replanificación), hora y temperatura |
| 6 | estudio | Al aire, modos, botonera, mic + ducking + deriva, atajos, VU |
| 7 | estudio | Sync: config, cuñas (descarga + sha256), nowplaying, heartbeat, cola de airplays, DPAPI |
| 8 | estudio | Robustez: silencio, watchdog, recuperación, instancia única, no-sleep, autoarranque, backups, NSIS, docs/user-guide.md |

S y 0–2 en paralelo; 7 depende de S.

## 7. Verificación
- **Servidor** (en Windows, sin make): `gofmt -l .` vacío, `go vet ./...`, `go test ./...`. Arrancar en local con las variables de `DEV_ENV` del Makefile en PowerShell y probar con `curl`: `GET /api/studio/spots` con y sin token (401), `POST /api/studio/airplays` repetido no duplica, CSV del comprobante, licencia en `total` → 503.
- **Estudio, unitarios** (`go test ./...`): rotación (separaciones y relajación), parrilla + relleno, eventos duros/blandos, reparto y replanificación de cuñas, mezcla renderizada sin tarjeta (crossfade, ducking, limitador sobre senos), ring buffer con productor lento (descarta sin bloquear), corrección de deriva.
- **Punta a punta en la PC**:
  1. Importar ~50 pistas, 2 relojes, parrilla, generar el día.
  2. Automático suena por la tarjeta.
  3. https://radioclub25.sehuacho.com pasa a "En vivo" con el título correcto.
  4. Una tanda emite la cuña del panel y aparece en el comprobante web.
  5. Hora locutada en punto (con reloj NTP).
  6. Desconectar la red: el FM sigue sin cortes, el codificador reconecta solo y la cola de airplays se vacía al volver.
  7. Matar el proceso y reabrir: retoma la playlist.
  8. Modo `captura`: lo que entra por line-in llega a la web.
  9. 24 h seguidas sin descartes de muestras ni crecimiento de memoria.
- **Requisito externo:** ya documentado en `docs/deployment.md` (puerto 8010/tcp abierto hacia el servidor). No se necesita ningún servidor adicional.
