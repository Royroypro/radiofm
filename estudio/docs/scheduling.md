# Programación: rotación, relojes, parrilla y playlist

## Conceptos

| Pieza | Qué es | Dónde |
|---|---|---|
| Reloj | plantilla de una hora: lista ordenada de slots | `domain.Clock`, tabla `clocks` |
| Slot | categoría (rotación), pista fija, tanda, hora, temperatura, cambio de modo o silencio; **duro** = minuto exacto | `domain.ClockSlot` |
| Parrilla | qué reloj suena en cada hora de la semana (7 × 24) | `domain.GridCell`, tabla `grid` |
| Excepción | otro reloj para una hora de una fecha | `domain.GridException` |
| Playlist (log) | lista concreta del día, generada con antelación y editable | `domain.LogItem`, tabla `log_items` |

## Rotación (`service/rotation.PickFlow`, función pura)

Entre las pistas disponibles de la categoría (activas, con archivo y vigentes) elige la **menos reciente**
(historial + lo ya planificado; las nunca usadas primero; empate → menor ID, así es determinista).

Reglas, de la más a la menos importante (las de flujo usan los atributos de la Biblioteca y solo actúan
si ambas pistas tienen el dato; 0 = desactivada, Ajustes → Rotación):

| # | Regla | Por defecto |
|---|---|---|
| 1 | intérprete distinto del elemento anterior | siempre |
| 2 | separación de título | 3 h |
| 3 | separación de artista | 45 min |
| 4 | voz: no dos femeninas (o masculinas) seguidas | no |
| 5 | salto máximo de energía (1–5) | 2 |
| 6 | salto máximo de tempo (%) | no |
| 7 | máximo seguidas del mismo género | 3 |
| 8 | máximo seguidas de la misma década | no |

Si ninguna pista las cumple todas, se relajan **de a una desde la menos importante** (8 → 1); lo que se
relajó queda anotado en la playlist («rotación: se relajó …»). Si no queda ninguna, «todas las reglas».

**Atributos**: el BPM se estima en el análisis (autocorrelación de los ataques en 60 s del medio de la
canción, `library/tempo.go`; puede salir el doble o la mitad: se corrige a mano); la energía se estima del
BPM y la sonoridad si la pista no tiene una puesta a mano. Voz, idioma, ánimo, género y año se cargan en
la Biblioteca, uno por uno o **en lote** (todas las pistas del filtro).

Los intérpretes se comparan normalizados (minúsculas, sin tildes) y separando "feat.", "ft.", "&", "y",
"con", "x", ",": `Grupo 5 feat. Daniela Darcourt` comparte artista con `Daniela Darcourt`.

## Generación de un día (`service/scheduler.Generate`, función pura)

Ejemplo con el reloj "Musical básico" a las 10:00:

```
10:00:00  Hora locutada        (duro, minuto 0)
10:00:05  ID                   (categoría IDs)
10:00:15  Éxitos · Cumbia · Salsa · Baladas · Retro   (en orden, cada una hasta su punto de mezcla)
10:17:40  … relleno hasta 10:29:30 (30 min − tolerancia de 30 s) con la categoría Relleno
10:30:00  Tanda de cuñas       (duro, minuto 30)
10:32:00  Jingle · Éxitos · Cumbia · Salsa · Retro
10:46:..  … relleno hasta 10:59:30
```

Reglas:
- Cada hora se planifica desde su inicio. Los slots blandos van en orden.
- Antes de un slot duro se rellena el hueco (si falta más que la tolerancia).
- Al final de la hora se rellena hasta (fin − tolerancia). Si la hora se pasa de largo, las últimas
  canciones se marcan **opcionales** (el aire las salta si va tarde).
- La duración de una pista en la playlist es hasta su **punto de mezcla** (se solapa con la siguiente).
- Sin reloj en la parrilla: suena la categoría Relleno.
- El historial de rotación incluye las emisiones reales (tabla `plays`) y la playlist del día anterior.
- El historial se consulta por ventana de tiempo (búsqueda binaria) y las claves normalizadas se
  precalculan: un día de 5000 elementos se genera en menos de 2 s (`TestGenerateManyShortTracksIsFast`).

## Voice tracking (`service/voicetrack`)

El locutor graba su intervención entre dos canciones (pantalla **Voice tracking**). La toma se graba del
micrófono de Ajustes → Audio **sin salir al aire** (`Engine.SetRecording`), se escucha la transición por
los audífonos y al guardarla: WAV a `<datos>/voicetracks`, pista nueva en la categoría **Voice tracking**
(tipo locución) con punto de mezcla = duración − solape, e inserción **bloqueada** después de la canción
elegida. Al aire entra en el punto de mezcla de la canción que sale (que se funde debajo) y la siguiente
canción entra «solape» segundos antes de que termine (reglas de `onair/transition.go`).

## Regenerar y editar

- `Generate(day)` conserva lo **bloqueado** (en su hora y posición) y lo que **ya sonó o está sonando**.
- Mover, quitar, bloquear e insertar recalculan las horas previstas (`Retime`). Los IDs de los elementos
  no cambian al editar (el aire y la interfaz los siguen).
- Lo que ya sonó no se puede mover ni quitar.
- Al arrancar y cada 10 minutos se generan los días que falten (hoy + `schedule.days_ahead`).
- Primer arranque: se crea el reloj "Musical básico" y se asigna a toda la semana.

## Pruebas

- `rotation_test.go`: menos reciente, separación con "feat.", tildes, orden de relajación, vencidas.
- `generate_test.go`: eventos duros y relleno, sin repetir intérprete seguido, bloqueados, sin reloj,
  horas recalculadas y rendimiento.
- `scheduler_test.go`: con SQLite real — reloj y parrilla iniciales, generar, bloquear, insertar,
  regenerar conservando lo bloqueado, no mover lo emitido y validación de relojes.
