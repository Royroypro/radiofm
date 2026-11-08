# Guía del panel de la radio

Entrar en **https://radioclub25.sehuacho.com/login** con su email y contraseña.

## Roles
- **Administrador**: todo el panel de la radio.
- **Editor**: publicidad, estadísticas y programación.

## Resumen
Estado de la radio y la TV (en vivo / fuera del aire, oyentes, espectadores), anuncios vigentes,
programa al aire y anuncios más vistos de la semana.

## Publicidad
Tres tipos:

| Tipo | Dónde aparece | Archivo |
|---|---|---|
| **Banner** | en la web: cabecera, lateral, entre secciones o pie | JPG, PNG, WEBP o GIF. 970×250 (horizontal) o 300×250 / 300×600 (lateral) |
| **Pre-roll de video** | antes de la TV en vivo, al pulsar reproducir | MP4 (H.264) o WEBM, 10–30 s |
| **Cuña de audio** | antes de la radio, al pulsar reproducir | MP3 o AAC |

Campos:
- **Enlace**: a dónde va el oyente al hacer clic (opcional).
- **Desde / Hasta**: el anuncio solo sale en esas fechas. Vacío = siempre.
- **Peso**: si hay varios anuncios en el mismo lugar, uno con peso 3 sale 3 veces más que uno
  con peso 1.
- **Saltar a los N segundos**: para pre-roll y cuñas. 0 = no se puede saltar.
- **Activo**: desmarcar para pausarlo sin borrarlo.

Estados en la lista: **Al aire**, **Fuera de fecha** (activo pero fuera de su rango), **Pausado**.

En **Marca y contacto → Publicidad** se fija cada cuántos minutos, como mínimo, un mismo oyente
vuelve a ver/oír publicidad previa.

## Cuñas FM
Cuñas que suenan **en la emisión de radio (93.3 FM)**, no en la web. Las programa la PC del
estudio (Club25 Estudio), que las descarga sola. Por cada cuña: **anunciante**, nombre, audio,
**veces por día**, **franja horaria** (p. ej. 07:00–13:00) y vigencia opcional. La PC reparte las
veces por día entre las tandas de la franja y no repite anunciante en la misma tanda.
La columna **Hoy** muestra cuántas veces sonó hoy.

**Comprobante de emisión**: elija fechas y anunciante; lista cada emisión con fecha, hora,
duración y si sonó completa. **Descargar CSV** lo abre en Excel para enviarlo al anunciante.
Borrar una cuña no borra su historial.

## Estadísticas
Elija el rango de fechas. Por anuncio: impresiones, clics, CTR (clics ÷ impresiones),
completados y saltados. **Descargar CSV** abre en Excel.

## Programación
Programas por día y hora. El que esté en horario aparece como **Al aire** en el reproductor.
Si un programa termina después de medianoche (22:00 a 02:00), póngalo en el día en que empieza.

## Marca y contacto (administrador)
Nombre, frecuencia, eslogan, categorías, texto "quiénes somos", **logo**, **isotipo** (favicon),
imagen de fondo, **colores** (con enlace para volver al original), teléfono, WhatsApp, email,
dirección, mapa de Google, horario y redes sociales. Los cambios se ven al instante en la web.

## Streams y emisión (administrador)
Datos para configurar la PC emisora (ver `docs/streaming.md`), estado de las señales,
activar/desactivar radio o TV en la web y generar una clave nueva para OBS.

## Estudio (PC) (administrador)
Estado de la PC de la radio (conectada, versión, modo, lo que suena) y su **token de acceso**:
*Generar token* lo muestra **una sola vez** para pegarlo en Club25 Estudio. Generar uno nuevo o
**Revocar** deja sin acceso al anterior (por ejemplo, si cambia de PC).

## Usuarios (administrador)
Crear administradores y editores, desactivarlos o cambiar su contraseña.

## Avisos del servicio
- **Pago pendiente** (amarillo): hay un pago vencido; regularice antes de que termine el periodo
  de gracia.
- **Servicio restringido** (rojo): el panel queda en solo lectura (y, según el caso, se desactiva
  la publicidad). Contacte al proveedor.
