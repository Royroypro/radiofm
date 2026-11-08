# Biblioteca (`service/library`)

## Datos

- **Categorías** (`domain.Category`): nombre, tipo (`music`, `jingle`, `spot`, `voice`, `emergency`,
  `filler`), color y categoría padre (subcategorías). Se crean las del plan al primer arranque.
- **Pistas** (`domain.Track`): ruta absoluta (única, sin distinguir mayúsculas), etiquetas, categoría,
  duración, activa, vigencia, análisis (loudness, ganancia, cue-in, cue-out, punto de mezcla, intro),
  falta el archivo, última emisión y contador.
- **Historial** (`domain.Play`): cada emisión con inicio, duración real y si fue completa. Lo usa la
  rotación (separaciones) y los reportes.

Tablas en `platform/sqlite/migrations/0001_library.sql`. La base vive en la carpeta de datos
(`%ProgramData%\Club25Estudio\estudio.db`, o `CLUB25_DATA_DIR`).

## Importar

`ImportFolder(dir, categoría, recursivo)`:
1. Busca archivos `.mp3 .wav .flac .ogg .m4a .aac .wma .opus`.
2. Lee etiquetas y duración con **ffprobe** (4 en paralelo).
3. Sin etiqueta de título, usa el nombre del archivo `Artista - Título.mp3`.
4. Si la ruta ya existía, actualiza etiquetas y duración (si la duración cambió, se reanaliza) sin tocar
   categoría, cues manuales ni estado.

## Analizar (segundo plano)

`RunAnalyzer` procesa las pistas con `analyzed = 0` de una en una:

1. `ffmpeg -af ebur128=framelog=info` (una pasada, ~100× tiempo real) devuelve el loudness integrado y la
   sonoridad momentánea cada 100 ms.
2. `Derive` (función pura, `analysis.go`) calcula:
   - **Ganancia** = −16 LUFS − integrado, como mucho +6 dB.
   - **Cue-in**: inicio de la primera ventana por encima de −50 LUFS.
   - **Cue-out**: fin de la última ventana por encima de −50 LUFS (0 si suena hasta el final).
   - **Punto de mezcla**: en los últimos 15 s, el último instante a menos de 10 LU del integrado
     (donde empieza el fundido final). Final seco: 1 s antes del cue-out.
3. Los errores se guardan (`analyze_err`) para no reintentar sin fin; "Reanalizar" lo vuelve a intentar.

Los valores se pueden corregir a mano en el editor de la pista.

## Verificar archivos

`CheckMissing` marca las pistas cuyo archivo ya no existe (no se programan) y desmarca las que volvieron.

## Pruebas

- `analysis_test.go`: `Derive` con series sintéticas (fundido, final seco, silencio).
- `library_test.go`: importación (etiquetas, nombre de archivo, errores, subcarpetas, re-importación),
  analizador y faltantes, con un ffprobe/ffmpeg falso y SQLite real.
- `platform/ffmpeg`: `Analyze` y `Probe` con ffmpeg real (se saltan si no está).
