# ADR 0002 — ffmpeg como subproceso para decodificar, analizar y codificar

- Estado: aceptada
- Fecha: 2026-11-08

## Contexto
La biblioteca tiene MP3, AAC/M4A, FLAC, WAV y OGG. Hay que decodificarlos a PCM, medir loudness y
silencios, y codificar MP3 hacia Icecast. Enlazar códecs en C con CGO complica la compilación y
el mantenimiento.

## Decisión
- Usar **ffmpeg.exe / ffprobe.exe como subprocesos**:
  - decodificar: `ffmpeg -i pista -f f32le -ar 44100 -ac 2 -` (PCM por stdout, en streaming);
  - analizar: filtros `ebur128` y `silencedetect`; `ffprobe` para etiquetas y duración;
  - codificar: PCM por stdin → `-c:a libmp3lame -b:a 128k -f mp3 icecast://…`.
- Build **GPL de BtbN**, versión fijada y verificada por SHA-256 en `scripts/get-ffmpeg.ps1`.
  Se distribuye como ejecutable aparte, con su licencia.
- La única dependencia CGO es miniaudio (malgo) para la E/S con la tarjeta.

## Consecuencias
- + Lee cualquier formato; análisis de loudness probado; sin códecs en CGO.
- + Si ffmpeg falla con una pista, solo cae ese subproceso, no la aplicación.
- − Arranque de un proceso por pista (~decenas de ms): se compensa iniciándolo ~20 s antes.
- − Hay que distribuir y actualizar los .exe (script con versión fijada).
