# Emisión a Icecast

## Cómo funciona

```
Engine (bus) ─► EncoderOutput() ring 2 s ─► Encoder (platform/ffmpeg) ─► ffmpeg.exe ─► icecast://source@host:8010/radio
                                                   │
                                                   └─ si ffmpeg termina: espera 1 s, 2 s, 4 s… hasta 60 s y reintenta
```

- ffmpeg recibe PCM float32 por stdin y codifica **MP3 128 kbps** (o AAC 96 kbps con `Format: "aac"`).
- La contraseña va escapada en la URL (`url.UserPassword`): admite `@`, `:`, `/`, espacios.
- Al (re)conectar se descarta el audio acumulado: los oyentes reciben el directo, no audio viejo.
- "Conectado" = ffmpeg lleva 3 s enviando sin terminar (si Icecast rechaza la fuente, ffmpeg sale enseguida).
- Si la conexión dura más de 30 s, el contador de reintentos y la espera vuelven a cero.
- Si la red está caída, el motor descarta lo que no cabe en el ring (`Status.EncDropped`): la salida FM no se detiene.

## Metadata ("lo que suena")

`platform/icecast.Client.UpdateSong` llama a
`GET /admin/metadata?mount=/radio&mode=updinfo&song=Artista%20-%20Título&charset=UTF-8` con
Basic `source:<clave>`, como BUTT. Icecast solo la acepta con la fuente conectada.

Plan B si el servidor no aceptara la metadata con el usuario `source`: la web la actualiza con sus
credenciales de administrador al recibir `POST /api/studio/nowplaying` (fase 7 / servidor).

## Fuente: bus o captura

Ver [adr/0005-fuente-icecast.md](adr/0005-fuente-icecast.md). En modo captura el codificador lee de un
ring que llena `audio.OpenCapture` (entrada de línea con la salida de la consola) y el motor deja de
copiar el bus (`Engine.SetEncoderTap(false)`).

## Probar

Tests (sin servidor real): `go test ./internal/platform/ffmpeg/ ./internal/platform/icecast/`. Usan un
Icecast simulado que acepta la conexión cruda de ffmpeg y comprueban: tipo `audio/mpeg`, usuario y
clave, mount, bytes recibidos y reconexión tras un rechazo.

Contra el servidor real (la web debe pasar a "En vivo" con el título en ~15 s):

```powershell
$env:CLUB25_ICECAST_PASSWORD = '<clave de Admin → Streams y emisión>'
task cli -- encode -host radioclub25peru.com -port 8010 -mount /radio C:\Musica\a.mp3 C:\Musica\b.mp3
```

Sin `-device` no usa tarjeta (reloj de software); con `-device "Altavoces"` también suena en la PC.
No use BUTT a la vez: Icecast acepta una sola fuente en `/radio`.
