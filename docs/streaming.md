# Cómo emitir desde la PC (radio y TV por separado)

La PC emisora envía **dos señales independientes**:

1. **Radio (solo audio)** → Icecast, con BUTT, Mixxx, RadioDJ, SAM Broadcaster, Zara, etc.
2. **TV (video con su propio audio)** → MediaMTX, con OBS Studio, vMix o ffmpeg.

Los datos exactos (servidor, claves) están en el panel: **Admin → Streams y emisión**.

## 1. Radio con BUTT (gratuito: https://danielnoethen.de/butt/)

Settings → Main → Server → **ADD**:

| Campo | Valor |
|---|---|
| Name | Radio Club 25 |
| Type | **Icecast** |
| Address | `radioclub25.sehuacho.com` |
| Port | `8010` |
| Password | la de `ICECAST_SOURCE_PASSWORD` (se ve en el panel) |
| Icecast mountpoint | `radio` (o `/radio`) |
| Icecast user | `source` |

Settings → Audio: códec **MP3**, 128 kbps, 44100 Hz, estéreo (o AAC 96 kbps).
Settings → Stream → "Update song name" para enviar el título de la canción (aparece en la web).

Pulse ▶ (Play). En el panel la radio pasa a **En vivo** en menos de 15 segundos.

### Alternativa: Club25 Estudio (automatización)

En lugar de BUTT, la PC puede usar **Club25 Estudio** (carpeta `estudio/` de este repositorio):
programa la radio (biblioteca, relojes, tandas de cuñas, hora locutada), saca el audio a la consola
y a Icecast con los mismos datos de arriba, y se sincroniza con el panel:

1. Admin → **Estudio (PC)** → *Generar token* y copiarlo (se muestra una sola vez).
2. En Club25 Estudio → Ajustes → Sincronización: URL del sitio y token; en Salidas → Icecast:
   servidor, puerto `8010`, punto de montaje `/radio`, usuario `source` y la contraseña.
3. Las cuñas se suben en Admin → **Cuñas FM**; la PC las descarga sola. El comprobante de emisión
   está en Cuñas FM → *Comprobante de emisión* (CSV por anunciante y fechas).

No use BUTT y Club25 Estudio a la vez: Icecast acepta una sola fuente en `/radio`.

## 2. TV con OBS Studio

Ajustes → **Emisión**:

| Campo | Valor |
|---|---|
| Servicio | **Personalizado…** |
| Servidor | `rtmp://radioclub25.sehuacho.com:1935` |
| Clave de retransmisión | `tv?user=emisora&pass=CLAVE` (cópiela del panel) |

Ajustes → **Salida** (modo avanzado):

| Opción | Valor recomendado |
|---|---|
| Codificador de video | x264 o NVENC **H.264** |
| Control de tasa | CBR, **2500 kbps** |
| Intervalo de fotogramas clave | **2 s** |
| Perfil | main o high |
| Audio | AAC 128 kbps, 44.1 kHz |

Ajustes → **Video**: resolución de salida **1280×720**, 30 fps.

> El audio de la TV va dentro del video (pista de OBS). Es independiente del audio de la radio.

### Alternativa SRT (conexiones inestables)
OBS 30+: servidor `srt://radioclub25.sehuacho.com:8890?streamid=publish:tv:emisora:CLAVE&pkt_size=1316`
y clave vacía.

## Cambiar la clave de la TV
Admin → Streams y emisión → "Generar una clave nueva". La anterior deja de funcionar al instante
(la próxima vez que OBS conecte). Actualice OBS.

## Probar sin la PC emisora
En el servidor:
```bash
make simulate TV_PASS=CLAVE_DE_TV            # radio y TV durante 60 s
make simulate TV_PASS=CLAVE MODE=radio SECS=300
```
Envía un tono a la radio y barras de color con tono a la TV usando ffmpeg.

## Problemas frecuentes

| Síntoma | Causa probable |
|---|---|
| BUTT: "connect: server refused" / 401 | contraseña de fuente incorrecta, radio desactivada en el panel o servicio bloqueado |
| OBS: "No se pudo conectar" | clave mal copiada, TV desactivada, plan sin TV o servicio bloqueado |
| La web dice "Fuera del aire" | la PC no está enviando; revise Admin → Streams y emisión |
| La TV no se ve en algunos navegadores | el códec no es H.264/AAC; ver `deploy/mediamtx.yml` (transcodificación) |
| Mucho retraso en la TV | baje el intervalo de keyframes a 1–2 s en OBS |
