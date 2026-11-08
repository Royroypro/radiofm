// Package ffmpeg envuelve ffmpeg.exe/ffprobe.exe como subprocesos: decodificar
// cualquier formato a PCM float32 (streaming por stdout), leer etiquetas y
// duración (ffprobe), analizar loudness y silencios (filtros ebur128 y
// silencedetect) y codificar MP3 hacia Icecast (stdin PCM → icecast://).
//
// Los ejecutables se buscan en third_party/ffmpeg (scripts/get-ffmpeg.ps1)
// y luego en el PATH. Fases 1–3.
package ffmpeg
