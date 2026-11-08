# Cuñas, tandas, hora/temperatura y eventos

## Cuñas (`service/spots`)

- Las cuñas vienen del panel web (`remote_id` > 0, fase 7) o se crean localmente (`remote_id` = 0).
- Datos: anunciante, nombre, archivo, duración, vigencia, **veces por día** y **franja horaria**
  `[hour_from, hour_to)` en hora de la radio.
- Las cuñas del panel web no se editan aquí: se editan en el panel y llegan con la sincronización.

## Reparto en tandas (`spots.Plan`, función pura)

Una tanda es un elemento de la playlist de tipo `spots` (slot "Tanda de cuñas" de un reloj).

1. Para cada cuña: emisiones pendientes = veces por día − las que ya sonaron hoy.
2. Tandas válidas = tandas pendientes futuras dentro de la franja y vigencia de la cuña.
3. Reparto equitativo: con n emisiones y m tandas válidas, posiciones `(k + ½) · m / n`.
4. Nunca dos cuñas del **mismo anunciante** en una tanda (comparado sin mayúsculas, tildes ni espacios),
   ni la misma cuña dos veces; si la tanda ideal choca, se usa la libre más cercana.
5. Primero se reparten las cuñas con menos tandas posibles.
6. Lo que no cabe se informa (`missing`) en la pantalla Cuñas.

`Replan(day)` guarda el contenido de cada tanda (tabla `tanda_spots`) y actualiza su duración en la
playlist: cuñas + 2 cortinas (promedio de la categoría **Cortinas**). Se ejecuta al cambiar la playlist
de un día, al guardar/borrar una cuña y cada 10 minutos para hoy.

Al aire (fase 6) una tanda suena así: cortina de entrada → cuñas en orden → cortina de salida. Cada cuña
emitida queda en el historial (`plays.spot_id`) y, si viene del panel, se envía como emisión (fase 7).

## Hora y temperatura locutadas (`service/timeannounce`)

Archivos en la carpeta de locuciones (Ajustes → Locuciones), con prefijos configurables:

| Archivo | Dice |
|---|---|
| `HRS00` … `HRS23` | "Son las doce", "Es la una", "Son las tres"… |
| `MIN00` … `MIN59` | "en punto", "con un minuto", "con quince minutos"… |
| `TMP-10` … `TMP45` | "La temperatura en Huacho es de 22 grados" |

Se aceptan `.mp3 .wav .ogg .flac .m4a`. "Verificar archivos" lista los que faltan. Provisionales:
`task locutions` (voz de Windows). Si falta un archivo, el aire omite la locución y lo registra.

La temperatura viene de **Open-Meteo** (`platform/weather`) con caché de 15 min; si la red falla usa el
último valor de menos de 1 h; si no hay valor, solo se locuta la hora.

## Eventos duros y blandos (`service/events.Next`, función pura)

En cada aviso del motor (punto de mezcla, fin) y en un temporizador, el aire pregunta qué hacer:

1. Si llegó la hora de un **evento duro** (faltan < 150 ms): fundir lo que suena y arrancarlo (`HardCut`);
   los blandos anteriores se saltan. Un duro que pasó hace más de 3 min se salta (no se locuta una hora vieja).
2. Si el aire está libre o lo que suena llegó a su punto de mezcla: el siguiente **blando** que no esté
   atrasado (debía terminar hace más de 2 min → se salta: recuperación tras un corte) ni sea un
   **opcional** tardío (más de 30 s tarde). Un evento duro futuro no se adelanta.

## Reportes (`service/reports`)

- **Emisiones** (as-run): todo lo que sonó, por rango de fechas.
- **Comprobante de cuñas**: emisiones de cuñas, opcionalmente de un anunciante.
- Exportación CSV con `;` y BOM (Excel), igual que el panel web.

## Pruebas

`plan_test.go`, `spots_test.go` (con SQLite y el programador), `timeannounce_test.go`, `weather_test.go`
(servidor simulado), `events_test.go` (duro, blando, no adelantar, recuperación, opcional) y `reports_test.go`.
