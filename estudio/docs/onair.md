# Al aire (`service/onair`) y emisión (`service/streaming`)

## Cómo decide qué suena

```
Run (un solo goroutine, dueño del estado)
 ├─ eventos del motor: Segue (punto de mezcla) · Ended (terminó) · Silence (silencio en el bus)
 ├─ órdenes de la interfaz: iniciar, detener, siguiente, modo, botonera
 └─ tick cada 100 ms: eventos duros en hora, fin de silencios programados, aire libre
        │
        ▼
 events.Next(ahora, playlist) ──► Start (blando) · HardCut (duro) · saltar atrasados/opcionales
        │
        ▼
 expand(elemento) ──► segmentos ──► Engine.Load/Play en el deck libre; Fade del anterior
```

| Elemento | Segmentos |
|---|---|
| Canción / pista fija | 1 (crossfade en su punto de mezcla) |
| Tanda | cortina + cuñas + cortina (categoría "Cortinas"; sin solape) |
| Hora | `HRShh` + `MINmm` |
| Temperatura | `TMPxx` (si no hay dato del clima, se omite) |
| Silencio | espera de N s sin audio |
| Cambio de modo | aplica el modo |

Estados en la playlist: `playing` al empezar el primer segmento; `played` al **terminar** el último;
`skipped` si se saltó. Cada segmento queda en el historial (`plays`) con su duración real y si fue
completo (sonó hasta su punto de mezcla o hasta 0,5 s del final).

## Modos

| Modo | Comportamiento |
|---|---|
| **Automático** | avanza solo; crossfade en el punto de mezcla; eventos duros a la hora exacta |
| **Asistido** | cada elemento suena hasta el final y espera; **Espacio** o "Siguiente" arranca el próximo (con crossfade si se pulsa antes) |
| **En vivo** | la música se funde y se pausa; quedan micrófono y botonera; al volver a Automático sigue la playlist por hora |

## Seguridad

- **Silencio**: si el bus queda por debajo de −50 dBFS durante `safety.silence_seconds`, suena una pista de la
  categoría Emergencia (o Relleno).
- **Lista vacía**: si no hay nada programado y no suena nada, también suena Emergencia.
- **Recuperación**: al reabrir el programa, lo que quedó "sonando" se marca emitido y `events.Next` salta los
  elementos atrasados: el aire retoma en el elemento que corresponde a la hora.
- Un segmento que no se puede abrir (archivo borrado) se avisa y se pasa al siguiente.

## Botonera

24 botones por página (10 páginas). Cada audio (máx. 30 s) se decodifica una vez en memoria y se dispara en los
slots de carts del motor (sin ducking). F1–F12 = botones 1–12; teclado numérico 1–9, 0 = botones 13–22.
"Pulsar de nuevo lo detiene" o "vuelve a empezar", por botón.

## Micrófono

Ajustes → Audio → Micrófono abre la entrada (WASAPI). El botón **Mic** (o Ctrl+M) lo abre/cierra en la mezcla;
con voz por encima del umbral la música baja (ducking, −12 dB por defecto). "Bajar música" fuerza el ducking.

## Emisión a Icecast (`service/streaming`)

- Se activa en Ajustes → Emisión; la contraseña de fuente se guarda **cifrada con DPAPI** (`platform/secrets`).
- Fuente **bus** (mezcla de la PC) o **captura** (entrada de línea con la salida de la consola).
- Metadata: al empezar cada segmento con audio; cuñas/jingles/locuciones envían el nombre de la radio si
  "solo la música" está marcado.
- Si la tarjeta de salida no abre, el motor sigue con el reloj de software: la web sigue sonando.

## Pruebas

`onair_test.go` usa el motor real con reloj de software y tonos generados: lista con tanda (orden, cuñas
registradas, completas), evento duro que corta a su hora (la pista cortada queda incompleta), modo en vivo
y asistido. Con `-race` no hay carreras.
