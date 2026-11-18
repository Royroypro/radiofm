# Club25 Emisor

Emisor en vivo sencillo, parecido a BUTT, para enviar al servidor de Radio Club 25:

- **Radio (solo audio)** → Icecast (puerto 8010), en **MP3, AAC, AAC+ (HE-AAC v1/v2), Opus u Ogg Vorbis**, hasta **320 kbps**.
- **TV (video + audio)** → MediaMTX por **RTMP (1935)** o **SRT (8890)**, en **H.264 + AAC**, con video de hasta **6000 kbps**.

Ninguna salida es obligatoria: se puede emitir solo la radio, solo la TV o las dos a la vez. Cada una
corre en su propio proceso, así que si se cae una, la otra sigue al aire.

Es independiente de **Club25 Estudio** (`../estudio/`, la automatización de la PC de la radio): no
comparte código ni configuración con él. No los use a la vez sobre el mismo montaje, porque Icecast
acepta una sola fuente por montaje.

## Menú

- **Radio (audio):** Resumen (estado y configuración de un vistazo, con «Cambiar →» a cada apartado) ·
  Entrada · Stream (formato, servidor Icecast, datos del stream) · Mejoras (ganancia, MEJORA,
  ecualizador) · Canciones (nombre de la canción, Jazler, lista).
- **TV (video):** Resumen · Entradas · Stream (calidad y servidor MediaMTX) · Mejoras del audio.
- **Ajustes:** Datos del panel · Arranque · Programa.
- **Registro.**

Si al iniciar falta un dato, el aviso lleva directo al apartado donde está.

## Qué hace

| | |
|---|---|
| Entradas de audio | **micrófono o entrada de línea** (DirectShow), **audio de la PC**: lo que suena en los altavoces o en cualquier salida de reproducción (HDMI, auriculares, cable virtual…), o tono de prueba. Se elige primero el tipo y después el dispositivo, así los micrófonos y las salidas de reproducción van en listas separadas. |
| Entradas de video | cámara o capturadora (DirectShow), captura de pantalla (todo el escritorio), **video desde una dirección http:// o https://** (archivo MP4 o directo HLS `.m3u8`, MPEG-TS, FLV; se reconecta si el origen se corta), barras de prueba |
| Audio de la TV | micrófono o línea, audio de la PC, **el audio que trae el video** (si viene de una dirección), tono de prueba o «sin audio» (silencio, para que la pista AAC exista igual) |
| Medidores | pico y RMS por canal, 10 por segundo, de -60 a 0 dB |
| Estado | Detenido / Conectando / En vivo (con reloj, kbps y fps) / Reconectando en N s (intento N) |
| Reconexión | automática, con espera creciente (1, 2, 4… hasta 30 s). Si una conexión se queda sin avanzar 20 s, se reinicia. Si el servidor rechaza la clave 3 veces seguidas, se detiene y lo explica (reintentar no lo arregla). |
| Errores | los mensajes de ffmpeg se traducen a una explicación (clave rechazada, puerto cerrado, DNS, micrófono desconectado…) |
| **MEJORA** | botón en cada tarjeta (radio y TV): procesador de sonido como el de una emisora FM. Amplifica y empareja el volumen (compresor 3:1 desde -18 dBFS con +8 dB de ganancia), da cuerpo y brillo (+4 dB de graves y +2,5 dB de agudos), quita retumbe (pasa-altos de 50 Hz) y nunca satura (limitador a -0,5 dBFS). **Se enciende y apaga al aire, sin cortar** (en alrededor de 1,5 s). Los medidores muestran el audio ya procesado. |
| **Cambio de entrada al aire** | (1.6.0) la **entrada de audio** de la radio y la del audio de la TV (micrófono, línea, audio de la PC, tono, silencio) se cambian **al aire, sin reconectar**: los oyentes no se cortan. El codificador lee siempre de un bus de audio interno (`internal/feed`) y lo que lo alimenta se intercambia. Si el micrófono elegido se desconecta, la emisión sigue con silencio y la captura se reabre sola cada 3 s. Solo pasar a o desde «el audio del video» (TV desde una dirección) reconecta, en seguida y sin pulsar nada. |
| **Aplicar** | un botón en cada tarjeta y una barra arriba de las pestañas Radio y TV. Si cambia algo que **exige** reconectar con la salida al aire (formato, bitrate, servidor, clave, video…), avisa «Hay cambios sin aplicar» y, al pulsar Aplicar, esa conexión se reinicia sola en menos de un segundo con los cambios. La otra salida sigue al aire. Si la configuración nueva está incompleta, no corta y explica qué falta. |
| **Arranque y segundo plano** | pestaña «Arranque»: **Iniciar con Windows** (se abre solo al iniciar sesión, oculto junto al reloj), **Empezar a emitir al abrir** (inicia solo lo marcado con «Emitir») y **Seguir en segundo plano** (la X oculta la ventana y la emisión sigue). Ícono junto al reloj: muestra el estado al pasar el mouse; clic = abrir la ventana; clic derecho = Mostrar / Iniciar / Detener todo / Salir. Abrir el programa otra vez trae la ventana que ya está corriendo (una sola copia). |
| **Bloque «Mejoras»** | en las pestañas Radio y TV reúne la **ganancia** (deslizador de -30 a +30 dB), el botón **MEJORA** y el **ecualizador**. Todo se aplica al aire, sin cortar (la ganancia con el filtro `volume@ganancia`). |
| **Ecualizador** | 10 bandas (32 Hz a 16 kHz, ±12 dB) para la radio y para el audio de la TV, con deslizadores. Se cambia **al aire, sin cortar**. Perfiles de fábrica: Plano, Radio FM, Modo fiesta, Voz / programa hablado, Reggaetón / urbano, Cumbia / salsa, Rock, Baladas, Música suave y Clásica / instrumental. Se pueden **guardar perfiles propios** con nombre (y borrarlos). Doble clic en una banda la deja en 0. Un limitador final (-0,3 dBFS) evita que se sature al subir bandas. |
| **Nombre de la canción** | se ve en la web y en los reproductores (metadatos de Icecast, `/admin/metadata`; MP3 y AAC). Tres fuentes: **Jazler RadioStar 2** (automático), **archivo de texto de otro programa** (automático) o **a mano / lista de canciones** (`.m3u`, `.m3u8`, `.pls`, `.txt`; clic en ▶ o «Enviar la siguiente»). En cuñas, jingles y pausas envía un texto configurable (por ejemplo «Radio Club 25»). Se reenvía solo cada vez que la radio se reconecta. |
| Registro | pestaña Registro y `%APPDATA%\Club25 Emisor\emisor.log`, sin claves |

## Por qué este stack: Go + Wails v2 + ffmpeg

- **Go + Wails v2**, como `estudio/`: el equipo ya lo conoce, el mismo Go compila el servidor, el estudio y
  este emisor, y Wails da una ventana nativa (WebView2, que ya viene en Windows 10/11) con un `.exe` de unos
  12 MB. El frontend es HTML + CSS + JS simple (`frontend/dist/`), **sin npm ni Node**: no hay paso de
  compilación del frontend.
- **ffmpeg** hace todo lo pesado (captura DirectShow y gdigrab, MP3/AAC/Opus/Vorbis/H.264, Icecast, RTMP y
  SRT). Es lo más probado que existe para esto y evita escribir códecs y protocolos. El emisor solo arma los
  argumentos (`internal/ffmpeg`), lanza y vigila el proceso (`internal/runner`) y lee su progreso y sus
  niveles. Por eso casi toda la lógica se prueba sin dispositivos.
- Se descartó una app solo de navegador: un navegador no puede emitir a Icecast ni por RTMP/SRT sin un
  servidor intermedio. También se descartó OBS como base: es demasiado para un operador de radio, y BUTT no hace video.

### Audio de la PC (altavoces y salidas de reproducción)

ffmpeg no puede capturar en Windows lo que suena en una salida: DirectShow solo ve entradas. Por eso el
emisor lo captura por su cuenta con **WASAPI en modo loopback** (miniaudio, a través de
`github.com/gen2brain/malgo`, la misma biblioteca de `estudio/`) y se lo pasa a ffmpeg como PCM por una
conexión TCP local (`internal/loopback`). Detalles:

- «Salida predeterminada de Windows» sigue a la salida que esté elegida en Windows. También se puede
  elegir una salida concreta (altavoces, HDMI, auriculares, VB-Cable…).
- Cuando no suena nada, WASAPI no entrega audio. El emisor rellena con silencio a ritmo de reloj para
  que la emisión no se corte.
- Si en esa misma salida se escucha la radio en vivo, se produce un eco: escuche el retorno en otra
  salida o con auriculares en otra tarjeta.
- Si la radio y la TV usan la misma salida, comparten una sola captura.
- Requiere compilar con **CGO** (ver «Compilar»). Una compilación sin CGO funciona igual, pero sin esta
  opción.

### Video desde una dirección http/https

En la pestaña TV, tipo «Video desde una dirección http/https»: se escribe la dirección de un archivo
(`https://…/video.mp4`) o de un directo (`https://…/index.m3u8`). El video se lee a velocidad real, se
reescala a la resolución elegida y se recodifica a H.264 (y el audio a AAC) para MediaMTX. El audio de
la TV puede ser «el audio que trae el video» o cualquier otro (micrófono, audio de la PC…). Cuando un
archivo termina, el supervisor lo vuelve a empezar como una reconexión. Una dirección que no existe
muestra «No se encontró nada en esa dirección (404)».

## Los valores por defecto y cómo llegan los datos

El emisor arranca con los datos públicos de ingesta que muestra el panel (**Admin → Streams y emisión**),
tomados de `docs/streaming.md`: servidor `radioclub25peru.com`, Icecast `8010` montaje `/radio` usuario
`source`, RTMP `1935` y SRT `8890` con ruta `tv` y usuario `emisora`. Están en
`internal/config/defaults.json` (embebido) y **no traen ninguna clave**.

Las claves se cargan de una de tres maneras:

1. **Traer datos del panel** (recomendado): en «Datos del panel» se escribe la dirección de la web y el
   token de **Admin → Estudio (PC)**, el mismo que usa Club25 Estudio. El emisor llama a
   `GET /api/studio/config` (endpoint autenticado que ya existe; no hizo falta cambiar el servidor) y
   carga servidor, puertos, montaje de la frecuencia, ruta, usuario y **clave de la TV**. La contraseña de
   fuente de Icecast **no** viaja por esa API, por diseño (`docs/api.md`): se escribe una vez en la pestaña Radio.
2. **Pegar datos**: se pega lo que muestra el panel (la URL RTMP completa, la URL SRT, `icecast://…`, o
   servidor y clave por separado, como en OBS) y el emisor lo reparte en los campos.
3. **A mano**, en las pestañas Radio y TV.

Orden de carga de la configuración (cada capa pisa a la anterior):

1. `defaults.json` embebido.
2. `emisor.defaults.json` junto al `.exe`, opcional. Sirve para entregar el emisor ya apuntando a otra
   frecuencia u otro servidor sin recompilar; ver `emisor.defaults.ejemplo.json`. No ponga claves ahí.
3. `%APPDATA%\Club25 Emisor\emisor.json`: lo que guardó la persona. Las claves (fuente de Icecast,
   clave de la TV y token) se guardan **cifradas con DPAPI** del usuario de Windows, así que el archivo
   copiado a otra PC no sirve.

Los cambios se guardan solos. Con la salida al aire, la entrada de audio, MEJORA, el ecualizador, la
ganancia y la casilla «Emitir» se aplican en el acto, **sin reconectar**. Lo que de verdad exige una
conexión nueva (formato, bitrate, servidor, montaje, clave, fuente o tamaño del video) espera a
**Aplicar**, que reconecta en menos de un segundo.

### Arranque con Windows y segundo plano

- «Iniciar con Windows» escribe en `HKCU\Software\Microsoft\Windows\CurrentVersion\Run` la entrada
  `Club25 Emisor` = `"…\club25-emisor.exe" --autoarranque`. Es por usuario, no necesita administrador y
  se quita al desmarcar la opción. Si el programa se mueve de carpeta, la línea se corrige sola al abrirlo.
- Con `--autoarranque` la ventana no se muestra: solo aparece el ícono junto al reloj. En Windows 11 el
  ícono nuevo puede quedar en el desplegable «^»; arrástrelo a la barra para tenerlo siempre a la vista.
- «Empezar a emitir al abrir» espera 5 s, para que Windows cargue la red y el audio, y luego inicia lo
  marcado. Si la red todavía no está, la reconexión automática sigue intentando.
- Para que transmita sola tras un corte de luz, la PC debe iniciar sesión en Windows: configure el inicio
  de sesión automático de Windows (`netplwiz`) y, en la BIOS, el encendido al volver la energía.
- El modo consola no usa nada de esto: no pone ícono ni empieza a emitir solo.

### Trabajar junto con Jazler RadioStar 2

Jazler emite la programación (música, cuñas, jingles) por la tarjeta de sonido y el emisor la envía a
internet. Para unirlos:

1. **Audio:** en la pestaña Radio, elija como entrada «🔊 Audio de la PC» y la salida de reproducción
   que usa Jazler (o la entrada de línea, si Jazler sale por una consola). Con una salida dedicada,
   por ejemplo un cable virtual VB-Cable, lo que suene en otros programas no sale al aire.
2. **Nombre de la canción:** en «Nombre de la canción», elija «Jazler RadioStar 2 (automático)». El
   emisor lee `C:\Jazler RadioStar 2\Exports\NowOnAir.xml`, que Jazler reescribe con cada tema, y
   manda «Artista - Título». Los eventos que no son canciones (cuñas, jingles, sweepers, pausa) usan el
   «Texto en cuñas, jingles y pausas». El botón «🔍 Buscar Jazler» encuentra el archivo si Jazler está
   en otra carpeta. No hay que configurar nada en Jazler ni se escribe nada en sus archivos.
3. Archivos de Windows-1252 (acentos y ñ de programas viejos) se leen bien. Un archivo a medio escribir
   se ignora hasta la vuelta siguiente (se mira cada 1,5 s).

Otros programas (RadioDJ, ZaraRadio, mAirList…): configúrelos para escribir «lo que suena» en un archivo
de texto y elija «Archivo de texto de otro programa». Se usa la primera línea.

## Licencia, copia en el servidor y actualizaciones (1.5.0)

El programa **solo funciona autenticado** con el **dominio** de la web de la radio y el **token** de
Admin → Estudio (PC). Es el mismo token de Club25 Estudio: uno por frecuencia, sirve para los dos
programas. Ver `docs/api.md` → «API de Club25 Emisor».

- **Sin autenticar**, se ven todas las opciones atenuadas. Cualquier intento abre el aviso «Licencia
  requerida», con el contacto del proveedor (de fábrica 9488933154 · https://sehuacho.com/radio; se cambia
  en Superadmin → Club25 Emisor), WhatsApp, copiar el número y abrir la web. Al abrir sin autenticar
  se piden el dominio y el token. Emitir y aplicar cambios están bloqueados también por dentro, no
  solo en la ventana, igual que en el modo consola.
- **Sin internet**, una autenticación correcta vale 72 h (`licencia.dat`, cifrado con DPAPI). Un token
  rechazado o un servicio suspendido la anulan al instante.
- **Token revocado o regenerado en el panel:** el emisor lo nota en su siguiente latido (≤ 1 min), se
  bloquea y **detiene la emisión**. Un corte de internet no la detiene.
- **Latido:** cada minuto, y al cambiar el estado de la radio o la TV, el emisor avisa al servidor. Admin →
  Monitoreo del estudio muestra si la señal viene de Club25 Estudio, de Club25 Emisor o de ambos, y qué
  emite cada uno.
- **Copia de la configuración:** cada cambio se sube solo al servidor (8 s después del último), sin
  contraseñas ni token. Ajustes → Programa → «Restaurar la configuración desde el servidor».
- **Actualizaciones:** el superadmin sube el `.exe` en Superadmin → Club25 Emisor. Los emisores se enteran
  en menos de un minuto (el latido trae la versión vigente). Desde ese momento, cualquier opción abre el aviso
  «Actualización obligatoria»; la emisión sigue y «Detener» funciona. «Actualizar ahora» hace esto:
  1. descarga el programa y verifica su tamaño y su SHA-256;
  2. renombra el `.exe` en uso a `.old` y pone el nuevo en su lugar;
  3. abre el nuevo con `--esperar-pid` y `--reanudar`, y cierra el viejo;
  4. el nuevo vuelve a emitir lo que estaba al aire.
  En la prueba, el corte fue de 1 a 2 segundos. Si algo falla, queda la versión anterior y sigue al aire.
- **Instancia única por carpeta de datos:** la configuración normal es una sola copia; con `-datos` se
  puede abrir otro perfil a la vez (por ejemplo, otra frecuencia).

### Cómo funciona MEJORA al aire

La cadena de filtros de MEJORA está siempre en el audio, con valores neutros cuando está apagada. Al
pulsar el botón, el emisor le escribe a ffmpeg comandos por su entrada (`c<filtro> -1 <parámetro>
<valor>`), que cambian los filtros en caliente (`internal/ffmpeg/enhance.go`). ffmpeg lee un comando por
cada ciclo de estadísticas, por eso el emisor usa `-stats_period 0.25`.

Lo mismo vale para el ecualizador (`internal/ffmpeg/eq.go`): diez filtros `equalizer@eqN` siempre
presentes, en 0 dB si está apagado; al aire se mandan solo las bandas que cambian.

## Uso

1. Deje `ffmpeg.exe` en la misma carpeta que `club25-emisor.exe` (el paquete ya lo trae).
2. **Datos del panel** → token → *Traer datos del panel*.
3. **Radio**: entrada (micrófono o línea), ganancia, formato, bitrate y contraseña de fuente.
4. **TV**: video (cámara, capturadora o pantalla), audio de la TV, resolución, fps, bitrate,
   codificador (x264, o NVENC/Quick Sync/AMF si la PC los tiene) y RTMP o SRT (SRT aguanta mejor las
   conexiones inestables).
5. Marque **Emitir** en la radio, en la TV o en ambas, y pulse **▶ Iniciar** (o el botón de cada tarjeta).

Referencia de formatos para la radio:

| Formato | Bitrate permitido | Para qué |
|---|---|---|
| MP3 | 32–320 kbps | el más compatible; 128 kbps es lo habitual |
| AAC (AAC-LC) | 32–320 kbps | mejor que MP3 al mismo bitrate |
| AAC+ (HE-AAC v1) | 24–64 kbps (mono 16–48) | bitrates bajos, datos móviles |
| AAC+ v2 (HE-AAC v2) | 16–48 kbps, solo estéreo | el de menor consumo |
| Opus (Ogg) | 16–320 kbps, siempre 48 kHz | el más eficiente |
| Ogg Vorbis | 48–320 kbps | alternativa libre |

**AAC+ necesita un ffmpeg con `libfdk_aac`.** Esa biblioteca no es libre para redistribuir, así que los
ffmpeg comunes (gyan.dev, BtbN, Chocolatey) no la traen. El emisor detecta si está y, si no está,
muestra AAC+ desactivado con la explicación. Para tenerla hay que compilar ffmpeg con
`--enable-libfdk-aac --enable-nonfree` (por ejemplo, con *media-autobuild_suite*), ponerlo junto al
programa y **no redistribuirlo**. Con AAC-LC a 64–96 kbps se consigue casi lo mismo.

### Modo consola (`emisor-consola.exe`)

Es el mismo emisor, sin ventana y con la misma configuración. Sirve para diagnosticar y para tareas programadas:

```powershell
emisor-consola.exe -dispositivos
```
```powershell
emisor-consola.exe -panel https://radioclub25peru.com -token TOKEN -importar -clave-radio CLAVE
```
```powershell
emisor-consola.exe -radio -tv -segundos 60
```
```powershell
emisor-consola.exe -tv -prueba -protocolo srt
```

`-datos CARPETA` usa otra carpeta de configuración (también funciona con `club25-emisor.exe`).

## Compilar

Necesita Go 1.27+, Wails CLI v2 (`go install github.com/wailsapp/wails/v2/cmd/wails@v2.17.1`) y un
compilador C para **CGO**: la captura del audio de la PC usa miniaudio. Sirve el gcc de llvm-mingw o el de
MSYS2. No necesita Node. El `.exe` resultante solo depende del CRT universal de Windows, que ya viene
en Windows 10 y 11.

```powershell
cd emisor
```
```powershell
$env:CGO_ENABLED = "1"; wails build -clean -trimpath -platform windows/amd64
```

Con `CGO_ENABLED=0` también compila, pero sin la opción «Audio de la PC».

El resultado queda en `build\bin\club25-emisor.exe`. Para desarrollar la interfaz sin compilar, abra
`frontend/dist/index.html` en un navegador: sin Wails usa un backend de demostración.

## Empaquetar para Windows

```powershell
powershell -ExecutionPolicy Bypass -File scripts\empaquetar.ps1
```

El script compila la ventana y la consola, copia un `ffmpeg.exe` estático (si el del PATH es el lanzador
de Chocolatey, copia el ffmpeg real) y arma `build\dist\Club25Emisor-<versión>.zip` con LEEME. Se instala
descomprimiendo, sin instalador. Requiere WebView2, que ya viene en Windows 10 y 11. ffmpeg es GPL: al
entregarlo, indique de dónde sale su código fuente (el LEEME lo menciona).

## Pruebas

```powershell
go test -short ./...
```
```powershell
go test ./...
```

- `-short` corre solo la lógica sin ffmpeg: configuración y validación (límites de 320 y 6000 kbps),
  cifrado de claves, armado de argumentos para cada formato y protocolo, lectura de dispositivos,
  códecs, progreso y niveles, explicaciones de errores, búsqueda de ffmpeg, importación desde el panel
  (con un servidor HTTP falso), datos pegados, envío del título y el supervisor (reconexión, espera
  creciente, cuelgues, rechazo de clave y cierre con «q») con un ffmpeg falso.
- `internal/loopback`: el reparto de la captura de altavoces (ritmo de tiempo real, relleno con
  silencio, descarte de lo viejo, cierre) con datos falsos.
- Sin `-short` también corre `internal/integration`, que usa el **ffmpeg real** y señales de prueba
  (no necesita micrófono ni cámara):
  - radio en cada formato contra un Icecast mínimo (`internal/icefake`), comprobando con ffprobe lo recibido;
  - clave incorrecta;
  - TV por RTMP y por SRT a 6000 kbps contra ffmpeg en modo receptor;
  - radio y TV a la vez, comprobando que detener la TV no corta la radio;
  - la entrada de arranque en el registro, probada contra una clave de prueba que se borra al
    terminar (no la de arranque real);
  - ecualizador al aire (+6 dB en 1 kHz con un tono de 1 kHz sube el nivel 6 dB, sin reconectar);
  - autenticación, latido, copia de la configuración y descarga verificada, contra un servidor falso
    (`internal/license`), y el cambio del `.exe` con relanzamiento (`internal/updater`);
  - cambio de entrada al aire (tono → silencio → audio de la PC → tono) con Icecast recibiendo sin
    pausa y la misma conexión (`TestInputSwitchLive`); el bus con capturas falsas, incluida una que se
    corta y se reabre (`internal/feed`);
  - MEJORA al aire (el tono sube de -21 a -13 dB y vuelve, sin reconectar) y Aplicar (de MP3 a AAC en
    menos de un segundo);
  - TV desde una dirección: un MP4 por http y un HLS por https servidos en local, con el audio del video,
    y el aviso de 404;
  - radio con el audio de la PC: reproduce un tono en una salida **virtual** (VB-Cable) y lo captura por
    loopback. Si la PC no tiene una salida virtual, se salta, para no hacer sonar los altavoces.
  - Si no hay ffmpeg en el PATH, se salta.

`cmd/icecast-prueba` es ese Icecast mínimo como programa, para probar a mano. También recibe los títulos
(`/admin/metadata`) y los muestra:

```powershell
go run ./cmd/icecast-prueba -puerto 8010 -clave CLAVE
```

Para probar contra el servidor completo (`make up` en la raíz, o Icecast y MediaMTX instalados), use
los datos de Admin → Streams y emisión de esa instalación, o «Traer datos del panel» con su token.

## Mapa del código

```
main.go                  ventana Wails (embebe frontend/dist); -datos CARPETA
frontend/dist/           interfaz: index.html, style.css, app.js (sin npm)
internal/config/         ajustes, defaults.json embebido, validación, carga y guardado
internal/secret/         cifrado DPAPI de las claves
internal/autostart/      entrada «Iniciar con Windows» en el registro (HKCU\…\Run)
internal/license/        autenticación con la web, latido, copia de la configuración y descarga de versiones
internal/updater/        instalar una versión nueva: cambiar el .exe, relanzar y reanudar la emisión
internal/loopback/       captura de lo que suena en una salida (WASAPI loopback) servida a ffmpeg por TCP local
internal/ffmpeg/         argumentos de ffmpeg; lectura de dispositivos, códecs, progreso y niveles; explicaciones de errores
internal/runner/         supervisor de un proceso ffmpeg: estados, reconexión, cuelgues, cierre
internal/nowplaying/     lo que suena: NowOnAir.xml de Jazler, archivo de texto y listas de canciones
internal/panel/          GET /api/studio/config, datos pegados, título a Icecast
internal/ui/             puente con la ventana (métodos para JS y eventos "estado", "registro", "dispositivos")
internal/icefake/        Icecast mínimo para pruebas
internal/integration/    pruebas de punta a punta con ffmpeg real
cmd/emisor-consola/      el emisor sin ventana
cmd/icecast-prueba/      Icecast mínimo como programa
scripts/empaquetar.ps1   zip para Windows
scripts/logo.py          dibuja el logo (build/appicon.png y build/windows/icon.ico; el mismo de frontend/dist/logo.svg)
```

## Problemas conocidos

- Con ffmpeg 7.1 y una entrada DirectShow, al detener puede aparecer en el registro
  `Assertion best_input >= 0 failed`. Es un fallo de ffmpeg al cerrar; la emisión ya terminó y no
  afecta en nada.
- La captura de pantalla toma todo el escritorio (todas las pantallas) y lo escala a la resolución de salida.
- El logo (punto rojo «al aire» con ondas, sobre azul) es propio del emisor y distinto del de Club25
  Estudio. Para cambiarlo, edite `frontend/dist/logo.svg` y `scripts/logo.py`, y ejecute `python scripts/logo.py`.
