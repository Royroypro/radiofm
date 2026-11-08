# Glosario

Términos de radio usados en el código y la documentación. La columna "En el código" es el
identificador que se usa (o se usará) en Go.

| Término | Qué es | En el código |
|---|---|---|
| Pista | Archivo de audio de la biblioteca (canción, jingle, cuña…) | `domain.Track` |
| Categoría | Grupo de pistas para la rotación (Éxitos, Salsa, Jingles, Cuñas…) | `domain.Category` |
| Rotación | Elegir la siguiente pista de una categoría respetando separaciones | `service/rotation` |
| Separación de artista / título | Tiempo mínimo antes de repetir un intérprete (min) o un tema (h) | `rotation.Rules` |
| Reloj | Plantilla de una hora: lista ordenada de slots | `domain.Clock` |
| Slot | Posición de un reloj: categoría, pista fija, evento o tanda | `domain.ClockSlot` |
| Parrilla | Qué reloj suena en cada hora de la semana (7×24), con excepciones por fecha | `domain.GridCell` |
| Log / playlist del día | Lista concreta de lo que sonará, generada con antelación | `domain.LogItem`, `service/scheduler` |
| Relleno | Pistas que se añaden para completar una hora corta | categoría Relleno |
| Back-timing | Ajustar la playlist para llegar a tiempo al siguiente evento duro | `scheduler` |
| Evento duro | Ocurre a la hora exacta: funde lo que suena | `domain.Event{Hard: true}` |
| Evento blando | Ocurre al terminar la pista en curso | `domain.Event{Hard: false}` |
| Tanda | Bloque de publicidad: cortina de entrada + cuñas + cortina de salida | `service/spots` |
| Cuña (FM) | Anuncio de audio que suena en la emisión. Viene del panel web | `domain.Spot` (servidor: `FMSpot`) |
| Cortina | Pieza musical corta que abre o cierra una tanda o sección | categoría Cortinas |
| ID / identificación | Pieza que dice el nombre de la radio | categoría IDs |
| Jingle | Pieza corta de imagen de la radio | categoría Jingles |
| Cart / botonera | Botón que dispara un audio corto al instante | `domain.Cart` |
| Deck | Reproductor del motor (A y B se alternan para mezclar) | `audio.Deck` |
| Crossfade | Mezcla: una pista baja mientras la otra sube | `audio/crossfade.go` |
| Cue-in / cue-out | Inicio y fin útil de la pista (sin silencios) | `Track.CueIn`, `Track.CueOut` |
| Segue / punto de mezcla | Momento en que empieza la siguiente pista | `Track.SegueAt` |
| Intro | Duración de la parte instrumental inicial (para que el locutor hable encima) | `Track.Intro` |
| Ducking | Bajar la música automáticamente cuando habla el micrófono | `audio/ducking.go` |
| Loudness (LUFS) | Volumen percibido (EBU R128); se normaliza a −16 LUFS | `Track.Loudness` |
| Limitador | Impide que la señal pase de un techo (−1 dBTP) | `audio/limiter.go` |
| Bus | Mezcla final que va a la tarjeta y al codificador | `audio.Engine` |
| Pre-escucha | Salida aparte para escuchar sin salir al aire | salida secundaria |
| Hora locutada | Locución "Son las tres con quince minutos" armada con archivos HRS/MIN | `service/timeannounce` |
| Temperatura locutada | "La temperatura en Huacho es de 22 grados" (archivos TMP) | `service/timeannounce` |
| Modo automático / asistido / en vivo | Avanza solo / avanza el operador / mic + botonera | `domain.StudioMode` (servidor) |
| As-run | Registro de lo que realmente sonó | `domain.Play` |
| Comprobante de emisión | Reporte de cuñas emitidas para el anunciante | panel web y `service/reports` |
| Fuente (Icecast) | El programa que envía el audio al servidor de streaming | `platform/icecast` |
| Mount | Punto de montaje de Icecast (`/radio`) | configuración |
