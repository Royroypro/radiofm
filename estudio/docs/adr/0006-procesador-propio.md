# ADR 0006 — Procesador de audio propio en Go

**Estado:** aceptada (Estudio 1.1.0)

## Contexto
Una radio comercial suena pareja: canciones grabadas a distinto nivel quedan iguales y la voz no se pierde.
Eso lo hace un procesador (AGC + compresión multibanda + limitador). Opciones: un procesador externo de
hardware (Optimod), un VST (Stereo Tool, comercial), los filtros de ffmpeg en un subproceso, o uno propio.

## Decisión
Procesador propio en Go dentro del motor (`audio/processor.go`): AGC lento con puerta, compresor de 3 bandas
con cruces Linkwitz-Riley y el limitador look-ahead existente. Presets fijos y opción de aplicarlo solo al stream.

## Motivos
- El motor ya trabaja en bloques de 10 ms sin asignaciones; meter ffmpeg en el camino del audio en vivo
  añadiría latencia y un proceso que puede caerse (y el FM nunca debe cortarse).
- Sin dependencias ni licencias nuevas (los algoritmos son conocidos; referencia FFmpeg/LSP, LGPL).
- Se prueba sin tarjeta de sonido (tests de transparencia, AGC, techo y ámbito).

## Consecuencias
- No compite con un Optimod: por eso existe «solo stream» para cuando el transmisor procesa la FM.
- Más presets o un editor de parámetros se pueden añadir sobre `ProcSettings` sin cambiar el motor.
