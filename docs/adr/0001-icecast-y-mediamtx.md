# ADR 0001 — Icecast para la radio y MediaMTX para la TV

- Estado: aceptada
- Fecha: 2026-11-07

## Contexto
La PC emisora envía dos señales separadas: audio de radio y video con su propio audio. Se pidió
no construir desde cero los servidores ni codificadores, y usar Go.

## Decisión
- **Radio → Icecast 2.4** (imagen `libretime/icecast:2.4.4`). Es el estándar de las radios por
  Internet: BUTT, Mixxx, RadioDJ, SAM, Zara… emiten a Icecast sin configuración extra.
  Sirve MP3/AAC directamente a `<audio>`.
- **TV → MediaMTX 1.21** (`bluenviron/mediamtx`, escrito en Go). Recibe RTMP (OBS) y SRT, y
  publica HLS/LL-HLS sin transcodificar. Tiene API HTTP para estado y para expulsar
  conexiones, y autenticación delegada por HTTP.
- La app Go no toca los medios: consulta estado y decide permisos (licencia, claves).

## Consecuencias
- + Compatibilidad inmediata con el software habitual de radio y con OBS.
- + Sin carga de transcodificación (OBS ya entrega H.264/AAC).
- + Bloqueo real: MediaMTX (`authMethod: http`) e Icecast (`stream_auth` por URL) preguntan a la app.
- − Dos servicios más que operar (contenedores).
- Alternativas descartadas: un solo MediaMTX para audio y video (BUTT/RadioDJ no emiten RTMP);
  Owncast (orientado a un solo canal de video, sin radio); servidor propio en Go (reinventar).
