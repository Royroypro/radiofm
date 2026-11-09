# Código abierto usado

Club25 Estudio es **GPL-3.0-or-later**. Se puede reutilizar o portar código de proyectos
GPL-2.0-*or-later*, GPL-3.0, LGPL, MIT, BSD, Apache-2.0 o dominio público. **No** se copia código de
proyectos solo GPL-2.0 (sin "or later") ni AGPL: de esos se toman ideas y diseño.

Al reutilizar o portar código: añadir una fila en "Código portado" (proyecto, versión/commit, archivo
de origen, licencia, qué se tomó) y un comentario `// Basado en <proyecto>/<archivo> (<licencia>)` en
el archivo Go. Antes de portar, comprobar la cabecera de licencia del archivo concreto.

## Dependencias directas

| Proyecto | Versión | Licencia | Uso |
|---|---|---|---|
| [Wails](https://wails.io) | v2.17.1 | MIT | ventana nativa con WebView2, bindings Go↔TS, build e instalador |
| [malgo](https://github.com/gen2brain/malgo) / [miniaudio](https://miniaud.io) | ver `go.mod` | Unlicense / MIT-0 | E/S de audio WASAPI (salida y captura) |
| [Svelte](https://svelte.dev) | 5.x | MIT | interfaz |
| [Vite](https://vite.dev) | 8.x | MIT | compilación del frontend |
| [FFmpeg](https://ffmpeg.org) (build de [BtbN](https://github.com/BtbN/FFmpeg-Builds)) | n8.1.3 GPL | GPL-3.0 (build) | subproceso: decodificar, analizar, codificar. Se distribuye como ejecutable aparte con su `LICENSE.txt` |
| [llvm-mingw](https://github.com/mstorsjo/llvm-mingw) | 20261104 | Apache-2.0 con excepciones LLVM | solo para compilar (CGO), no se distribuye |
| [NSIS](https://nsis.sourceforge.io) | 3.11 | zlib/libpng | solo para compilar el instalador, no se distribuye por separado |
| Nunito (fuente) | v16 | OFL-1.1 | tipografía de la interfaz (`frontend/src/assets/fonts/OFL.txt`) |

## Proyectos de referencia

| Proyecto | Licencia | Qué tomamos |
|---|---|---|
| miniaudio (ejemplos `simple_duplex`, `ma_pcm_rb`) | MIT-0 | diseño de ring buffers y dúplex para `audio/ringbuf` y `audio/drift` |
| FFmpeg `libavfilter/af_alimiter.c` | LGPL-2.1+ | referencia para el limitador del bus |
| FFmpeg `af_acompressor.c`, `af_mcompand.c`; [LSP plugins](https://lsp-plug.in) | LGPL | referencia de algoritmos para el procesador (AGC, compresor por bandas, cruces Linkwitz-Riley); reescrito en `audio/processor.go` |
| "Audio EQ Cookbook" (R. Bristow-Johnson) | dominio público de facto | fórmulas de los filtros Butterworth de 2.º orden |
| [OpenBroadcaster](https://openbroadcaster.com) | AGPL-3.0 | solo ideas: campañas publicitarias con vigencia (fase B) |
| [Rotter](https://github.com/nick-prater/rotter) / Chronicle | GPL | solo ideas: grabador de emisión por horas con retención (fase D) |
| [Mixxx](https://mixxx.org) | GPL-2.0-or-later | curvas de crossfade, *talkover ducking*, AutoDJ, intro/outro, forma de onda por picos |
| [Liquidsoap](https://www.liquidsoap.info) | GPL-2.0-or-later | diseño de `smart_crossfade`, fallback por silencio, normalización |
| [Rivendell](https://github.com/ElvishArtisan/rivendell) | GPL-2.0 (verificar archivo por archivo) | modelo de datos de radio (carts, relojes, parrilla, log, transiciones) — solo ideas salvo archivos "or later" |
| [LibreTime](https://libretime.org) | AGPL-3.0 | solo ideas: *smart blocks*, comprobantes |
| [BUTT](https://danielnoethen.de/butt/) / libshout | GPL-2.0 / LGPL | protocolo de fuente Icecast y metadata |
| [go-mp3](https://github.com/hajimehoshi/go-mp3) | Apache-2.0 | opcional: decodificar MP3 en Go puro para tests |

## Código portado

| Archivo Go | Origen (proyecto, commit, archivo) | Licencia | Qué se tomó |
|---|---|---|---|
| `internal/service/library/analysis.go` (`Derive`, `EndingOf`) | [autocue](https://github.com/Moonbase59/autocue) de Moonbase59 (Python) | MIT | idea y umbrales: silencio relativo a la sonoridad de la pista (−42 LU), punto de entrada de la siguiente buscado hacia atrás desde el final (−8 LU), duración del fundido; reescrito en Go sobre la serie `ebur128` que ya medíamos |
