# Cuñas, tandas, hora/temperatura y eventos

## Cuñas (`service/spots`)

- Las cuñas vienen del panel web (`remote_id` > 0, fase 7) o se crean localmente (`remote_id` = 0).
- Datos: anunciante, nombre, archivo, duración, vigencia, **veces por día** y **franja horaria**
  `[hour_from, hour_to)` en hora de la radio.
- Las cuñas del panel web no se editan aquí: se editan en el panel y llegan con la sincronización.

## Reparto en tandas (`spots.Plan`, función pura) — tráfico (1.2)

Las cuñas llegan del panel como **versiones de campañas** (`campaign_id`, `weight`, `days_mask`,
`position`, `priority`, `sector`). Las veces por día son de la **campaña** (`spots.needs` agrupa por
`Spot.CampaignKey`; una cuña local es su propia campaña).

1. Cada campaña va solo a tandas dentro de su **franja**, **días** y **vigencia** (`Spot.LiveAt`).
2. Orden de reparto: **prioridad** (alta primero); a igual prioridad, la que tiene **menos tandas
   posibles**. Si no hay espacio, la de prioridad baja es la que queda fuera.
3. **Reparto parejo**: n emisiones en m tandas válidas van a (k+½)·m/n; si esa tanda no sirve, la válida
   libre más cercana.
4. En una tanda: nunca el **mismo anunciante** dos veces ni dos del **mismo rubro** (competidores);
   respeta **Máx. cuñas** y **Máx. segundos** por tanda (Ajustes → Rotación/Tandas; 0 = sin tope).
5. **Posición**: «primera» y «última» se reservan una por tanda (si no hay otra opción se comparten). Al
   final cada tanda se ordena: primeras, el resto por prioridad, últimas.
6. **Versiones**: rotan con round-robin ponderado suave (pesos 2:1 → A, B, A), continuando desde lo
   emitido hoy.
7. Lo que no entra queda en `Missing` con el motivo («no hay tandas suficientes…» o «tandas llenas o
   con un competidor…»); se ve en **Cuñas → No entran** y en el log.

**Reposición**: `Replan` cuenta lo ya emitido hoy y solo toca tandas futuras pendientes. Se ejecuta al
cambiar las cuñas, al generar el día y **cada 10 minutos**: lo que no salió en una tanda saltada (evento
duro, PC apagada, archivo que faltaba) se repone en las siguientes tandas de su franja.

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
