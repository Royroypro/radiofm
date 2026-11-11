# Cambios

Formato: una sección por fase del plan ([../docs/estudio/plan.md](../docs/estudio/plan.md)).

## 1.9.2 — actualizaciones automáticas

- El programa **busca versiones nuevas solo** (al minuto de abrir y cada 30 min) en el manifiesto del
  servidor, **descarga el instalador en segundo plano** y lo verifica (SHA-256).
- Cuando está listo aparece un aviso abajo a la derecha: **Actualizar ahora** cierra el programa, instala en
  silencio (Windows pide confirmar con «Sí») y lo **vuelve a abrir solo**. «Después» lo deja para la próxima vez.
- No se instala sin permiso: reiniciar corta el aire unos segundos; el aviso lo recuerda si está al aire.
- Clic en la versión (barra lateral, «v1.9.2 ⟳») busca actualizaciones en el momento.

## 1.9.1 — ordenar la playlist en Al aire y silenciar videos

- **Al aire → Playlist**: cambiar el orden de lo que va a sonar **arrastrando** un elemento (una línea naranja
  marca dónde queda) o con los botones **↑ ↓** del elemento elegido. Lo que suena y lo que ya sonó no se mueven.
- **TV → Fuentes**: botón **🔇 Video silenciado / 🔊 Audio del video** en cada fuente (silenciado = se ve el
  video y se oye la radio, sin mezclarse). Cambia al instante si la fuente está al aire.
- La vista previa de video de la Biblioteca empieza **sin sonido**, para no mezclarse con lo que está al aire.

## 1.9.0 — fuentes de video (como OBS)

- Pantalla TV → **Fuentes**: **video local** (con bucle opcional) o **URL en línea** (http/https, HLS `.m3u8`,
  rtmp, rtsp, srt), como el «Origen multimedia» de OBS.
- **▶ Al aire** pone la fuente en la TV con fundido; **Automático** vuelve a seguir a la radio. Las URL que se
  cortan se reconectan solas; un archivo sin bucle vuelve solo a automático al terminar.
- Audio por fuente: el de la **radio** o el **de la fuente** (la TV lleva su sonido; la radio no cambia).
  Sin audio en la fuente, el espectro de la radio va abajo.

## 1.8.1 — espectro sobre los videos

- Los **videos sin audio** (por ejemplo un video de fondo puesto a una canción) muestran el **espectro del audio
  de la radio** en la franja de abajo, encima del video, con una sombra suave para que se lea. Opción en la
  pantalla TV: solo en videos sin audio (por defecto), en todos o nunca.
- El **fondo del visualizador** puede ser un **video en bucle** (además de una imagen): el espectro va en la
  franja de abajo.

## 1.8.0 — videoclips en la TV (fase TV-B)

- **Videoclips**: si la canción que suena tiene video, la TV lo muestra **sincronizado con el audio** (con el
  logo, el reloj y el cintillo encima); sin video, vuelve el visualizador.
- Al importar, el video con el mismo nombre que el audio se asigna solo; un video sin audio gemelo se
  importa como pista propia (**promos y videos** en la misma playlist: su audio en la radio, su imagen en la TV).
- Biblioteca: marca 🎬, **Video para la TV** con vista previa para asignar o quitar a mano.
- Pantalla TV: muestra el videoclip que suena y permite ajustar la sincronía (ms).

## 1.7.1 — credenciales de la TV desde el panel

- El modo TV toma solo del panel web la dirección RTMP y el usuario y la clave de publicación
  (Ajustes → Streams); si cambian allá, el Estudio los renueva y se reconecta. Ya no hay que copiar la
  clave a mano (evita el error «Operation not permitted» por una clave distinta).

## 1.7.0 — modo TV (fase TV-A)

- **Modo TV**: nueva pantalla **TV**. El Estudio arma el canal de TV de la web y lo emite por RTMP al
  servidor (mediamtx) con el mismo audio de la radio:
  - **visualizador** de espectro animado con el audio que suena (sobre un fondo o el degradado de la marca);
  - capas de **logo**, **reloj**, cintillo **«Ahora suena»**, **ticker** y placa **«Publicidad»** en las tandas;
  - escena **Placa** (imagen a pantalla completa) con fundido;
  - monitor del programa en la pantalla TV.
- Audio y video en paso fijo: nunca se desincronizan. H.264 por x264 o por hardware (NVENC, Quick Sync,
  AMF); 480p, 720p o 1080p a 25 o 30 fps.
- Ver `docs/tv.md`. Próximas fases: videoclips, cámaras y fuentes en línea, escenas y parrilla de TV.

## 1.6.0 — ecualizador y ampliación estéreo

- **Ecualizador gráfico de 10 bandas** (31 Hz a 16 kHz, ±12 dB) y **ampliación estéreo** (0 % mono,
  100 % normal, hasta 200 %) en el procesador del bus: después del AGC y antes de los compresores.
  Ajustes → Ecualizador y ampliación estéreo, con presets (Plano, Más graves, Voz clara, Brillo,
  FM con cuerpo). Se aplica al instante mientras se mueven los controles.
- Funciona también con el procesador en «Sin procesar» (solo ecualizador y ampliación).
- Guardar los Ajustes aplica el procesador al momento (antes había que reiniciar el motor).

## 1.5.2 — zona horaria

- Corrige «zona horaria inválida: unknown time zone America/Lima» al cambiar ajustes (p. ej. apagar la
  hora locutada) en PCs con Windows: la base de zonas horarias va ahora dentro del programa.

## 1.5.1 — relojes

- En Relojes, el slot Satélite y el Enlace a cadena muestran el nombre y la URL uno debajo del otro
  (antes la URL quedaba escondida). Botón «+ Satélite».

## 1.5.0 — satélite

- **Satélite**: nuevo tipo de slot en los relojes. Se pone la URL de una señal (red nacional, otra
  emisora) y esa señal cubre la hora entera. Las tandas, la hora locutada y los IDs marcados como
  **duros** la cortan a su minuto; al terminar la tanda **vuelve la señal en vivo** (entra con un
  fundido de 1 s).
- Si la señal se cae o no responde, suena música de respaldo (categoría de emergencia o cualquier
  música) y luego se reintenta, hasta la próxima tanda.
- Un elemento cortado por un evento duro queda cerrado en la playlist (antes podía quedar «sonando»).

## 1.4.1 — biblioteca de Al aire

- La biblioteca de Al aire muestra primero el **título** (ordenada por título) y luego el artista.
- **Buscador** arriba de la biblioteca (título, artista o álbum), con botón para limpiar; Esc limpia y
  Enter inserta si queda una sola canción.

## 1.4.0 — operación y cumplimiento

- **Grabador de la emisión**: graba lo que sale al aire en MP3 por hora; borra lo más viejo según el
  límite de tamaño (1 GB por defecto) y de días. Reportes → Grabación de la emisión: escuchar y exportar
  tramos. **Grabaciones programadas** de programas (recortes automáticos al terminar la franja).
- **Enlace a cadena**: nuevo tipo de slot en los relojes para hacer sonar un stream externo N minutos;
  si se cae, la playlist sigue.
- **RDS**: nombre de la radio y lo que suena al codificador del transmisor por TCP, UDP o puerto serie,
  con comandos configurables.
- **Informe musical (APDAYC)** por canción con compositor, en Reportes y en CSV.
- Ver [docs/operacion.md](docs/operacion.md).

## 1.3.0 — programación musical y voice tracking

- **Atributos por canción**: BPM (estimado solo al analizar), energía 1–5 (estimada; corregible), voz,
  idioma, ánimo, género y año. Se editan en la Biblioteca o **en lote** para todo el filtro.
- **Reglas de flujo** en la rotación (Ajustes → Rotación): no dos voces femeninas/masculinas seguidas,
  salto máximo de energía y de tempo, máximo seguidas del mismo género o década. Si ninguna canción cumple,
  se relajan de la menos a la más importante y queda anotado en la playlist.
- **Voice tracking**: nueva pantalla para grabar la intervención del locutor entre dos canciones (sin salir
  al aire), escuchar la transición y dejarla bloqueada en la playlist.

## 1.2.0 — publicidad profesional (tráfico)

Requiere el panel web con campañas (migración 0005 del servidor).

- Las cuñas llegan como **versiones de campañas**: las veces por día son de la campaña y las versiones
  **rotan según su peso**.
- Reglas al armar las tandas: **días de la semana**, **prioridad** (si no hay espacio cede la baja),
  **competidores** (mismo rubro) nunca en la misma tanda, **posición** primera/última, **topes** de cuñas
  y de segundos por tanda (Ajustes → Tandas).
- **Reposición automática**: cada 10 min se replanifica el día contando lo que ya sonó; lo que no salió
  se repone en las siguientes tandas de su franja.
- Cuñas → **No entran**: qué campañas no caben hoy y por qué.

## 1.1.0 — sonido y transiciones profesionales

- **Análisis de finales** (idea de autocue, MIT): cada pista sabe si termina con **fundido** o **seco** y
  cuánto dura su fundido; el umbral de silencio es relativo a la sonoridad de la pista. Toda la biblioteca
  se vuelve a analizar en segundo plano al actualizar.
- **Transiciones según el contenido** (solo en los puntos de mezcla automáticos):

  | De → a | Qué pasa |
  |---|---|
  | canción con fundido → canción | se cruza a lo largo de su propio fundido |
  | canción con final seco → canción | no se corta el golpe final |
  | canción → jingle | el jingle entra sobre el final |
  | jingle → canción | la cola del jingle termina sola |
  | canción → tanda | la música se funde (1 s, ajustable) y la tanda entra al terminar, sin pisarse |

- **Fundidos equal-power** en todo el motor (sin bache de volumen a mitad del crossfade).
- **Editor de marcas** en la Biblioteca: forma de onda con cue-in, intro, mezcla y cue-out arrastrables; zoom
  al inicio o al final; escuchar desde cualquier punto por los audífonos de la PC (no sale al aire); botones
  «… aquí» para marcar mientras se escucha; **probar transición** con otra pista usando las reglas del aire.
  Las marcas ajustadas a mano quedan «a mano» y el análisis no las pisa («Marcas automáticas» las descarta).
- **Procesador de audio**: AGC + compresor de 3 bandas antes del limitador. Presets Natural (por defecto),
  FM comercial, Hablado o sin procesar; se aplica a FM y stream o solo al stream. Medidor «GR» en Al aire.
- La compilación ya no se cuelga si Club25 Estudio está abierto en la misma PC.

> Nota: las marcas que se hubieran ajustado a mano **antes** de esta versión se recalculan una vez con el
> nuevo análisis (no había forma de distinguirlas). Desde la 1.1.0 los ajustes manuales se conservan.
## 1.0.8 — el audio también por radioclub25peru.com (se publica junto con 1.2.0)

- El audio (Icecast 8010) va a `radioclub25peru.com`, igual que la web: el subdominio
  `radioclub25.sehuacho.com` se desactiva. Las instalaciones que lo tenían guardado se actualizan
  solas al abrir (además, el panel web ya envía el host nuevo). Un host puesto a mano se respeta.
- Publicar solo con la ingesta abierta en el VPS (`scripts/ha/vps-radioclub25-ingesta.sh`).

## 1.0.7 — dominio radioclub25peru.com

- La web y la API del estudio pasan a `https://radioclub25peru.com` (servidor con nodos de reserva).
  Las instalaciones que tenían `https://radioclub25.sehuacho.com` se actualizan solas al abrir; el
  token sigue valiendo. Si la URL se cambió a mano por otra, se respeta.
- El audio sigue yendo directo a `radioclub25.sehuacho.com:8010` (Icecast del nodo principal).

## 1.0.6 — lo que no tiene audio ya no traba el aire

- Interruptor **Hora: sí / no** en Al aire. Apagado, la hora locutada de la playlist se salta sin
  sonar. Encendido y sin locución para esa hora, también se salta (con un aviso en «Problemas»).
- Las cuñas sin audio (no descargadas del panel o borradas del disco) se sacan de la tanda antes de
  empezarla, con un aviso por cada una. Si la tanda se queda sin cuñas, se salta entera (sin
  cortinas sueltas).
- Un elemento cuyo audio no se puede abrir (archivo dañado, locución que no abre) queda «saltado» y
  la playlist sigue. Antes quedaba como «sonando» para siempre y la pantalla parecía trabada.

## 1.0.5 — el aire no espera en silencio

- Si lo siguiente es un evento duro que todavía no llega (p. ej. la tanda de las 19:30) y no queda
  nada antes (lo anterior se saltó por atrasado tras reiniciar), el aire ya no espera en silencio:
  adelanta lo que viene después y el evento duro corta a su hora. «Siguiente» también adelanta.
- La emergencia, si su categoría (o la de relleno) no tiene pistas, toca una canción de cualquier
  categoría de música. El aviso de «Problemas» dice lo que realmente pasó.
- El candado de la playlist explica qué es al pasar el ratón (pista insertada a mano: se conserva
  al regenerar; no impide que suene).

## 1.0.4 — corrige el cierre al abrir en algunas PC

- El analizador de la biblioteca se caía con archivos en los que ffmpeg informa la sonoridad como
  "nan" en algunos tramos (el índice de la forma de onda salía negativo). Como el análisis se retoma
  al arrancar, el programa se cerraba a los pocos segundos cada vez. Ahora esos tramos cuentan como
  silencio.
- Si una pista provoca cualquier otro error inesperado al analizarse, queda marcada con el error y
  el analizador sigue con la siguiente: un archivo raro ya no puede cerrar el programa.

## 1.0.3 — registro de caídas

- Si el programa se cae (error de Go o de código nativo: audio, WebView2), el detalle queda en
  `%ProgramData%\Club25Estudio\logs\crash.log`. Al abrirlo de nuevo avisa y muestra esa ruta.
  Un corte de luz no genera aviso (no escribe `crash.log`).
- Si la ventana no puede abrirse (normalmente porque falta WebView2), muestra un aviso con el
  enlace de descarga en vez de cerrarse sin decir nada.

## 1.0.2 — temas, ventana y pantallas pequeñas

- **Temas**: Oscuro (grafito, por defecto, poco azul), Claro, Azul Club25 y Personalizado (color principal,
  fondo, paneles, menú y texto sobre un tema base). Vista previa al instante en Ajustes → Apariencia;
  se guarda en `settings.json` (`ui.theme`, `ui.custom_*`). Todos los colores salen de variables CSS
  (`frontend/src/lib/theme.ts`): ningún componente fija colores a mano.
- Botones neutros por defecto; el color principal solo en lo activo o importante.
- **Ventana**: arranca maximizada (la barra de título con minimizar/maximizar/cerrar siempre visible),
  tamaño mínimo 1024×600, botones minimizar/maximizar/pantalla completa en Al aire y Ctrl+F11.
- **Al aire adaptable**: medidas con `clamp()`; en anchos ≤ 1280 px la botonera usa 3 columnas; en
  ≤ 1150 px historial/tanda/estado pasan a una franja inferior; en alturas ≤ 700 px la cabecera se compacta.

## 1.0.1 — pantalla Al aire todo en uno

- Nueva pantalla **Al aire** en una sola vista, con la distribución habitual de un automatizador (y la
  identidad de Club25, sin marcas ni gráficos de terceros): AHORA/SIGUE con cuentas atrás grandes
  (rojo en los últimos 10 s) e intro, forma de onda con lo sonado, intro y punto de mezcla, transporte
  (Iniciar/Detener, **Parar tras actual**, Micrófono), vúmetro, reloj con anillo de segundos, modos;
  debajo playlist (duración restante del día, quitar), biblioteca con pestañas por tipo y búsqueda
  (doble clic = insertar después de lo que suena o del elemento elegido), botonera 4×6 con páginas,
  historial, **cuenta atrás a la próxima tanda**, estado y problemas.
- En Al aire la barra lateral se oculta; se abre con "Menú".
- Envolvente de sonoridad por pista (migración `0006_envelope`: las pistas se reanalizan solas).
- Publicado en el servidor el instalador 1.0.1.

## Fase 8 — robustez e instalador (versión 1.0.0)

- `platform/winsys`: instancia única (mutex global), evitar la suspensión, arranque con Windows (clave Run
  del usuario, sin administrador), aviso nativo si falla el arranque.
- `platform/tools`: revisión de la PC y descarga de herramientas (ffmpeg) desde el servidor de la radio
  según `/uploads/estudio/manifest.json`, con SHA-256; modo `club25-estudio.exe --setup` para el instalador
  y descarga automática al abrir si falta.
- Copia diaria de la base (`backups`, 14 días); aviso si el reloj de la PC difiere del servidor.
- Instalador NSIS en español: WebView2, accesos directos, `--setup` al final; no borra los datos al desinstalar.
- Ajustes → Sistema: revisión y descarga de herramientas. Documentación: `docs/user-guide.md`.
- Publicado en el servidor (`scripts/publish-server.ps1`): instalador, programa, ffmpeg/ffprobe y `manifest.json`.

## Fase 7 — sincronización con el panel web

- `platform/apiclient`: cliente de `/api/studio/*` (config, cuñas con ETag, heartbeat, lo que suena,
  emisiones) y descarga verificada por SHA-256.
- `service/sync`: bucle de sincronización, cola local de emisiones (tabla `airplay_queue`) que se envía al
  volver la red, desactivación de cuñas retiradas, aplicación de la configuración del panel.
- Token del panel cifrado (DPAPI); Ajustes → Panel web; indicador en Al aire.
- Probado de punta a punta contra el servidor (misma versión que producción): cuña descargada y medida,
  heartbeat (versión, modo, codificador), "lo que suena" en Admin → Estudio (PC), reloj a 0,6 s.

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
