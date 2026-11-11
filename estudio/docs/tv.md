# Modo TV (Estudio 1.7 y 1.8)

El Estudio arma el canal de TV de la web y lo emite al servidor por RTMP (mediamtx, ruta `tv`), con el
**mismo audio** que va a Icecast. La web ya lo reproduce en la tarjeta TV (HLS con hls.js).

## Cómo se arma la imagen (`internal/video`)

- **Formato interno**: YUV 4:2:0 de 8 bits, BT.709 de rango limitado, 1280×720 a 30 fps por defecto
  (480p y 1080p, 25 o 30 fps en la pantalla TV).
- **Escenas** (fondo):
  - **Visualizador**: un fondo (imagen o degradado con el color de la marca) con el espectro del audio que
    suena, 56 barras de 40 Hz a 16 kHz en escala logarítmica, con picos y reflejo. Usa una FFT de 2048
    puntos con ventana de Hann sobre el mismo audio de la TV, así que se mueve exactamente con lo que se oye.
  - **Placa**: una imagen a pantalla completa.
  - Entre escenas, fundido de medio segundo.
- **Capas de grafismo** (siempre encima, se encienden y apagan al instante):
  - logo (PNG con transparencia, o el nombre de la radio en una placa del color principal);
  - reloj;
  - cintillo **«Ahora suena»** (título y artista, entra con fundido al cambiar de canción);
  - **ticker** (texto que corre abajo);
  - placa **«Publicidad»** durante las tandas.
- Las capas se dibujan en RGBA (`golang.org/x/image`, fuentes Go) **solo cuando cambian**, se convierten
  una vez a YUV con alfa y en cada cuadro solo se mezclan. El texto del cintillo se recorta con «…» si no
  entra en el 62 % del ancho.
- El cintillo sigue a las canciones (y a los enlaces o el satélite). Los jingles y locuciones dejan la
  última canción; las cuñas muestran «Publicidad».

## Codificación (`platform/ffmpeg/video.go`)

- Un ffmpeg recibe por **TCP local** dos entradas: el video crudo (yuv420p) y el audio (f32le, 44,1 kHz).
  En Windows ffmpeg no puede leer pipes extra además de stdin.
- **Paso fijo A/V**: el Estudio entrega cada cuadro junto con su audio exacto (1470 muestras a 30 fps,
  1764 a 25). Si ffmpeg se atrasa, se descartan **los dos juntos**, así que audio y video no se
  desincronizan nunca. Prueba: `TestEncodeToFile` codifica 6 s y compara con ffprobe los cuadros de video
  y de audio; la diferencia es menor a un cuadro AAC.
- El reloj de cuadros es el de la PC: si la PC se atrasa recupera los cuadros (hasta 1 s; más que eso se
  salta y lo cuenta como «atrasados»). El audio viene de la tarjeta (`Engine.TVOutput`); si por la deriva
  de relojes sobra más de 8 cuadros de audio, se descarta lo viejo, y si falta, se rellena con silencio.
- **Salida**: H.264 (x264 `veryfast` perfil main, o NVENC, Quick Sync o AMF) a tasa constante, con un
  keyframe cada 2 s (los segmentos HLS de mediamtx), y AAC a 128 kbps. Va a `-f flv` hacia
  `rtmp://radioclub25peru.com:1935/tv?user=…&pass=…`. El usuario y la clave son los de publicación del
  panel (Ajustes → Streams); la clave se guarda cifrada (DPAPI).
- Se reconecta solo con espera exponencial (2 s → 60 s). Al apagar, cierra las entradas para que ffmpeg
  termine ordenado y lo mata si no sale en 5 s.

## Pantalla TV

- **Monitor** del programa: un JPEG cada 200 ms, solo mientras alguien lo mira (`/preview/tv.jpg`).
- Botones **TV al aire / Apagar**, escena (Visualizador o Placa) y capas.
- Estado: tamaño, fps medidos, kbps, cuadros enviados, descartados y atrasados, y el error de ffmpeg.
- Configuración:
  - servidor RTMP, usuario y clave;
  - resolución, cuadros por segundo, tasa y codificador;
  - logo y su posición, fondo, placa, colores y texto del ticker.

## Consumo

720p30 con x264 `veryfast` usa del orden de un núcleo de CPU. Si la PC de cabina no alcanza (la pantalla
muestra «atrasados» o «descartados»), baje a 480p o use el codificador por hardware de la tarjeta de video.

## Videoclips, videos y promos (Estudio 1.8, fase TV-B)

- Cada pista puede tener un **video** (`tracks.video_path`). Al importar una carpeta:
  - un video (`.mp4`, `.mkv`, `.webm`, `.mov`, `.avi`, `.mpg`, `.m4v`, `.ts`) con **el mismo nombre** que un
    audio de la misma carpeta queda como su videoclip;
  - un video **sin audio gemelo** se importa como pista propia: su audio suena en la radio (ffmpeg lo
    decodifica igual) y su imagen sale en la TV. Así se programan **promos y videos** en la misma playlist.
- También se asigna a mano: Biblioteca → pista → **Video para la TV** (con vista previa). La lista marca 🎬.
- **Director**: cuando empieza a sonar una pista con video, la TV pasa al videoclip (con fundido); si la
  siguiente no tiene video, vuelve el visualizador. Si el operador eligió la Placa, la placa manda.
- **Sincronía**: el cuadro que se muestra sale de la hora en que empezó el audio (desde su cue-in), no del
  ritmo de ffmpeg. ffmpeg decodifica por delante (hasta 8 cuadros, escalado sin deformar con bandas negras y
  a los fps del canal). Si arrancó tarde o se atrasa, se saltan cuadros para alcanzar al audio; si va
  adelantado, se espera. **Sincronía de videoclips (ms)** en la pantalla TV compensa la latencia que quede.
- El audio de la TV queda como mucho ~4 cuadros detrás de lo que suena (se descarta lo que sobre), para que
  la referencia de hora valga también para lo que oye el televidente.
- Si el video termina antes que la canción, o no se puede abrir, vuelve el visualizador.

## Próximas fases

- Cuñas en video (versión en video de la cuña desde el panel).
- **TV-C**: cámara del estudio (dshow), fuentes en línea (http, HLS, rtsp, srt), escenas tipo vMix con
  previo y programa, y la regla «micrófono abierto → cámara».
- **TV-D**: parrilla de TV, placas programadas, ticker desde el panel y publicidad sobre el video.
