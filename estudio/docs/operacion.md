# Operación y cumplimiento (Estudio 1.4)

## Grabador de la emisión (`service/airlog`)

Graba sin parar **lo que sale por la tarjeta** (después del procesador y del limitador: lo que oye el
oyente de FM) en MP3 mono (64 kbps por defecto), **un archivo por hora** (`2026-11-09_14-00-00.mp3`,
hora local de la PC; ffmpeg corta en cada cambio de hora con `segment_atclocktime`).

- El motor copia la salida a un ring buffer propio (`Engine.LoggerOutput`); si el grabador se atrasa,
  se descarta (nunca frena el aire). Un ffmpeg recibe PCM por stdin y escribe los MP3.
- **Retención** (cada 10 min): borra primero lo más viejo que **Máx. días** y luego, del más viejo al más
  nuevo, hasta quedar bajo **Máx. MB**. Nunca borra el archivo en curso. Por defecto 1024 MB y 30 días
  (el reglamento de la Ley de Radio y TV pide conservar lo emitido al menos 30 días: con 1 GB alcanza
  para ~1,5 días; súbalo en producción, ≈ 0,7 GB por día a 64 kbps).
- Al cerrar el programa se cierra la entrada de ffmpeg para que termine de escribir la hora en curso.
- **Reportes → Grabación de la emisión**: escuchar cada hora (por los audífonos) y **exportar un tramo**
  (de fecha/hora a fecha/hora; se une sin recodificar con el concatenador de ffmpeg).

### Grabaciones programadas
Ajustes → Grabador → «+ Grabación programada» (nombre, hora, minutos, días). Cada minuto se revisa si
terminó alguna franja de las últimas 6 h y, si su archivo no existe, se **recorta de la grabación** a
`<carpeta>/programas/<Nombre> AAAA-MM-DD HHMM.mp3`. Las grabaciones programadas no se borran con la
retención.

## Enlace a cadena (slot «relay»)

En un reloj, el tipo **Enlace a cadena** hace sonar un stream externo (URL `http(s)://`) durante N
segundos (p. ej. un noticiero de una cadena nacional). Con «Duro» entra al minuto exacto.

- ffmpeg reconecta ante cortes breves (`-reconnect`) y se rinde a los 10 s sin datos (`-rw_timeout`).
- Si el stream no responde o se corta, el elemento termina y **la playlist sigue** (idea de `fallback`
  de Liquidsoap). Queda en el historial como «Enlace a cadena».

## Satélite (slot «satellite»)

Para horas que se retransmiten de otra señal (una red, una emisora matriz) **sin perder la publicidad
local**. En el reloj se pone un slot **Satélite** con la URL y, como **duros**, las tandas (y la hora
locutada o IDs) en sus minutos. Ejemplo: Satélite + Tanda duro :20 + Tanda duro :50.

- Al generar la playlist, cada hueco de la hora se cubre con un elemento de satélite que dura
  **hasta el minuto exacto** del evento siguiente (sin tolerancia): 0–20, 22–50, 52–60.
- La tanda corta la señal a su minuto (fundido duro). Al terminar, entra el siguiente elemento de
  satélite: ffmpeg se vuelve a conectar y la señal **entra en vivo** (lo que se emitió durante la tanda
  se pierde, como en una afiliada de red), con fundido de entrada de 1 s.
- La duración de cada tramo se calcula al arrancar (hasta su hora prevista), así que si una tanda dura
  más o menos de lo estimado, la señal igual termina cuando entra el evento siguiente.
- **Si la señal se cae** (o no abre) con más de 20 s por delante: una pista de respaldo (categoría de
  emergencia o cualquier música) y luego reintento. Sin pistas: 5 s de espera y reintento.
- Un slot Satélite no puede ser duro: cubre la hora entera.

## RDS (`service/rds`)

Envía al codificador RDS del transmisor el **nombre de la radio** (PS, 8 caracteres) al conectar y el
**RadioText** (hasta 64) cada vez que cambia lo que suena (con un mínimo de segundos entre envíos; si
hay varios cambios seguidos gana el último).

- Conexión: TCP, UDP o **puerto serie** (COM3, 8N1, baudios configurables).
- Comandos con **plantilla** (Ajustes → RDS): `PS={ps}\r\n` y `RT={text}\r\n` por defecto; `\r`, `\n`,
  `\t` escritos así se convierten. Texto con `{artist}`, `{title}`, `{station}`.
- Sin tildes ni caracteres fuera de ASCII (`rds.Clean`), porque no todos los equipos los muestran igual.
- En cuñas, jingles y locuciones puede mostrar solo el nombre de la radio.
- Reconecta solo; el estado (conectado, último texto, error) se ve en Ajustes.

## Informe musical (APDAYC)

Reportes → **Informe musical**: por canción, entre dos fechas: título, intérprete, **compositor** (etiqueta
`composer` del archivo, se lee al importar), álbum, año, veces emitida, duración y tiempo total al aire.
Exportar CSV para la sociedad de autores. Solo música (no jingles, cuñas, locuciones ni enlaces).
