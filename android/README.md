# Club25 — app Android

App de la radio para Android: reproductor de cada frecuencia activa (audio en segundo plano con
controles en la notificación y en la pantalla de bloqueo), TV en vivo (HLS) a pantalla completa,
estado «En vivo / Fuera del aire» y la marca de la radio (logo, colores, aviso).

**Espectro de audio** como el de la web (`site.js` §8): detrás del logo y en la tarjeta de la
frecuencia que suena. En reposo se simula; con la radio sonando usa el audio real, tomado del
propio reproductor (`TeeAudioProcessor` + FFT), así que funciona también con frecuencias externas
y **no pide permiso de micrófono**. Si el teléfono tiene las animaciones desactivadas, se queda quieto.

**Se configura desde el panel admin de la web, sin publicar versiones nuevas.** Cada vez que se
abre (o vuelve a primer plano) la app pide `GET /api/app/config` al servidor radiofm y, mientras
está visible, `GET /api/status` cada 15 s. Contrato de ambos en [`docs/api.md`](../docs/api.md).

| Qué se cambia | Dónde, en el panel |
|---|---|
| Nombre, eslogan, logo, isotipo, WhatsApp, teléfono, email, redes | Admin → Marca y contacto |
| Colores de la app (fondo con degradado, tarjetas, avisos, play, textos), aparte de los de la web | Admin → Marca y contacto → «App Android» → Colores de la app |
| Aviso en la app (y su enlace), versión mínima, enlace de Google Play, política de privacidad | Admin → Marca y contacto → «App Android» |
| Botones «Descarga la app» en la web (Google Play y/o APK subido) | Admin → Marca y contacto → «App Android» → Descarga en la web |
| Frecuencias (etiqueta, ciudad, montaje o URL externa, orden, activa) | Admin → Frecuencias |
| Radio y TV activadas | Admin → Streams |

Lo único fijo al compilar es la **URL del servidor** (`club25.serverUrl` en `gradle.properties`).
La última configuración recibida se guarda en el teléfono, así la app abre aunque no haya conexión.

## Por qué este stack

**Kotlin nativo + Jetpack Compose + AndroidX Media3 (ExoPlayer)**, sin frameworks multiplataforma:

- **Media3** es la librería oficial de Google para audio y video: con `MediaSessionService` el
  servicio en primer plano, la notificación, los controles en la pantalla de bloqueo, Bluetooth y
  el foco de audio vienen hechos y siguen las reglas de cada versión de Android. Reproduce el MP3/AAC
  de Icecast y el **HLS** de MediaMTX con el mismo motor.
- **Compose** con Material 3: una sola pantalla declarativa, colores de la marca aplicados en tiempo
  de ejecución.
- Pocas dependencias (Media3, Compose, Coil para el logo SVG/PNG, corrutinas); el HTTP y el JSON
  usan lo que trae Android. Menos librerías = menos actualizaciones obligadas para cumplir con Play.

## Estructura

```
android/
  gradle.properties             URL del servidor (release y depuración)
  gradle/libs.versions.toml     versiones de plugins y librerías
  app/build.gradle.kts          applicationId, versionCode, firma de release
  app/src/main/java/pe/club25/radio/
    Club25App.kt                Coil con soporte SVG
    data/Models.kt              lectura de /api/app/config y /api/status
    data/ServerApi.kt           HTTP + caché de la configuración
    playback/PlaybackService.kt reproductor de radio en segundo plano (MediaSessionService)
    playback/Spectrum.kt        análisis del audio que suena (FFT) para el espectro
    ui/MainActivity.kt          pantalla principal; controla el servicio con MediaController
    ui/HomeScreen.kt            interfaz (Compose)
    ui/HomeViewModel.kt         pide configuración y estado
    ui/TvActivity.kt            TV en vivo a pantalla completa
    ui/SpectrumView.kt          espectro de audio animado, como el de la web
  app/src/test/                 tests de JVM (lectura del JSON)
  app/src/debug/                permite http (solo depuración) y otro nombre: «Club25 (dev)»
```

- `applicationId`: `pe.club25peru.radio` (la de depuración es `pe.club25peru.radio.debug`, se instala
  al lado). **No lo cambie: en Play es la identidad de la app para siempre.** El paquete del código
  Kotlin (`namespace`) sigue siendo `pe.club25.radio`; es interno y no hace falta que coincida.
- `minSdk 26` (Android 8.0), `targetSdk 36` / `compileSdk 36` (Android 16).

## Requisitos para compilar

- **JDK 17 o 21** (AGP 8.13). En este equipo: `C:\Program Files\Java\jdk-21`.
- **Android SDK** con `platforms;android-36` y `build-tools;36.0.0`
  (en este equipo: `%LOCALAPPDATA%\Android\Sdk`; `local.properties` apunta allí y no se versiona).
  Desde la línea de comandos:
  ```bash
  sdkmanager "platforms;android-36" "build-tools;36.0.0" "platform-tools"
  ```
- Gradle no hace falta instalarlo: `gradlew` descarga la versión fijada (8.14.3).

## Compilar y probar (depuración)

```bash
cd android
export JAVA_HOME="/c/Program Files/Java/jdk-21"   # PowerShell: $env:JAVA_HOME="C:\Program Files\Java\jdk-21"
./gradlew testDebugUnitTest assembleDebug
```

APK: `app/build/outputs/apk/debug/app-debug.apk`. La versión de depuración usa
`club25.serverUrl.debug` (por defecto `http://10.0.2.2:8120`, que en el emulador es el
`127.0.0.1:8120` del PC):

1. En `radiofm`: `make dev`.
2. Emulador: `emulator -avd Medium_Phone` (o Android Studio → Device Manager).
3. `adb install -r app/build/outputs/apk/debug/app-debug.apk` y abrir «Club25 (dev)».

En un teléfono real por Wi-Fi use la IP del PC y que el servidor escuche en ella:
`./gradlew assembleDebug -Pclub25.serverUrl.debug=http://192.168.1.50:8120` y
`HTTP_ADDR=0.0.0.0:8120` al levantar el servidor.

En local no hay Icecast ni MediaMTX: las frecuencias con montaje propio (`/radio`) y la TV salen
«Fuera del aire» y al pulsar ▶ se muestra «No se pudo reproducir». Para oír algo sin Docker,
agregue en Admin → Frecuencias una frecuencia **externa** que apunte a un stream de prueba, por
ejemplo un tono con ffmpeg en el PC:
```bash
ffmpeg -re -f lavfi -i "sine=frequency=440" -c:a libmp3lame -b:a 128k -f mp3 -listen 1 http://0.0.0.0:8000/radio
```
y URL `http://10.0.2.2:8000/radio`. Con `make up` (Docker) se prueba la cadena completa.

## APK para probar en un teléfono real (variante «preview»)

**El APK de depuración (`app-debug.apk`) apunta al PC** (`http://10.0.2.2:8120`): en un teléfono
real no carga ni suena. Para instalar la app en teléfonos sin pasar por Google Play use:

```bash
./gradlew assemblePreview
```

APK: `app/build/outputs/apk/preview/app-preview.apk` (~3 MB). Es igual que la de release (servidor
`https://radioclub25peru.com`, solo HTTPS, R8) pero firmada con la clave de depuración, así que
no necesita la clave de subida. Se llama «radioclub25peru (prueba)» (`pe.club25peru.radio.preview`) y se
instala al lado de la de Play. No la suba a Play ni a la web para el público: para eso, el APK de
release firmado.

## Release: AAB firmado para Google Play

### 1. Clave de subida (una sola vez, la genera el dueño de la app)

Play usa **Play App Signing**: Google guarda la clave con la que se firma la app final y usted solo
necesita una **clave de subida**. Genérela y guárdela fuera del repositorio, con copia de seguridad
(si se pierde, se puede pedir a Google que la reemplace, pero tarda):

```bash
keytool -genkeypair -v -keystore C:/claves/club25-upload.jks -alias club25 -keyalg RSA -keysize 4096 -validity 10000
```

Cree `android/keystore.properties` (está en `.gitignore`; **nunca** lo suba al repositorio):

```properties
storeFile=C:/claves/club25-upload.jks
storePassword=...
keyAlias=club25
keyPassword=...
```

### 2. Compilar

```bash
./gradlew bundleRelease -Pclub25.versionCode=1 -Pclub25.versionName=1.0.0
```

AAB: `app/build/outputs/bundle/release/app-release.aab`. Sin `keystore.properties` el AAB sale sin
firmar (sirve para comprobar que compila, no para subirlo).

- `club25.serverUrl` debe ser `https://` (la compilación falla si no). Por defecto
  `https://radioclub25peru.com`; para otro dominio: `-Pclub25.serverUrl=https://...`.
- La versión de release **solo permite HTTPS**, también para frecuencias externas: un stream
  `http://` no sonará (la web con HTTPS tampoco puede reproducirlo).
- **`versionCode` debe subir en cada envío a Play.** Si sube la «versión mínima» en el panel, las
  apps con `versionCode` menor muestran «Actualice la app».
- R8 (minificación) está activado; `./gradlew lintRelease` debe terminar sin errores.

## Publicar en Google Play

Guía detallada para el dueño de la app: **[GOOGLE-PLAY.md](GOOGLE-PLAY.md)**. Resumen:


1. **Cuenta de desarrollador** en <https://play.google.com/console> (pago único). Las cuentas
   personales nuevas deben hacer una **prueba cerrada con al menos 12 testers durante 14 días**
   antes de poder pasar a producción; las cuentas de organización no (requieren número D-U-N-S).
2. **Crear la app**: nombre «radioclub25peru», idioma español (Perú), App, Gratis.
3. **Ficha de Play Store**:
   - Descripción breve (80 caracteres) y completa: radio de Huacho, las frecuencias, TV en vivo.
   - Ícono 512×512 PNG, gráfico de funciones 1024×500, al menos 2 capturas de teléfono
     (se sacan del emulador: `adb exec-out screencap -p > captura.png`).
   - Categoría: **Música y audio**. Email de contacto.
4. **Contenido de la app** (Política → Contenido de la app):
   - **Política de privacidad**: URL pública obligatoria: `https://radioclub25peru.com/privacidad`
     (plantilla `web/templates/public/privacy.html`; la app la enlaza sola). Si cambia lo que se
     guarda de los oyentes, actualice ese texto y su fecha.
   - **Seguridad de los datos**: la app no recopila datos personales en el teléfono ni los comparte;
     declare lo que guarde el servidor (IP en registros técnicos). Tráfico cifrado (HTTPS): sí.
   - **Anuncios**: «No» (la app no muestra los anuncios de la web). Si en el futuro los muestra, cambie la respuesta.
   - **Clasificación de contenido** (cuestionario IARC), **público objetivo** (mayores de 13, no
     dirigida a niños), **app de noticias**: no, salvo que la radio lo quiera declarar.
   - **Servicios en primer plano**: la app declara `FOREGROUND_SERVICE_MEDIA_PLAYBACK`. Play pide
     explicar el uso: «Reproducción de la radio en vivo en segundo plano iniciada por el usuario,
     con controles en la notificación», y un **video corto** (puede ser un enlace de YouTube no
     listado) que muestre pulsar ▶, salir de la app y la notificación con los controles.
5. **Versión**: Prueba interna → Prueba cerrada → Producción. Suba `app-release.aab`, acepte Play App
   Signing, escriba las notas de la versión en español.
6. Tras la aprobación, copie el enlace de la ficha
   (`https://play.google.com/store/apps/details?id=pe.club25peru.radio`) en Admin → Marca y contacto →
   «Enlace en Google Play»: lo usan «Compartir app» y el aviso de actualización.

### Nivel de API objetivo

Google Play exige que las apps nuevas y sus actualizaciones apunten a una versión reciente de
Android (en general, la del año anterior, con fecha límite cada 31 de agosto). Esta app compila con
`targetSdk 36` (Android 16). Revise cada año los
[requisitos de nivel de API objetivo](https://developer.android.com/google/play/requirements/target-sdk)
y, cuando cambien, suba `compileSdk`/`targetSdk` en `app/build.gradle.kts`, actualice las versiones
en `gradle/libs.versions.toml`, compile, pruebe en el emulador y publique.

## Pendiente / ideas

Antes de la primera publicación (lo decide o hace el dueño de la app):

- [x] `applicationId` confirmado: `pe.club25peru.radio`.
- [x] Dominio de release confirmado: `https://radioclub25peru.com`.
- [ ] **Generar la clave de subida** y `keystore.properties` (sección «Release», paso 1). No se generó.
- [x] Servidor desplegado con `GET /api/app/config` y `/privacidad` (2026-11-17).
- [ ] Cuenta de Google Play, ficha, formularios de contenido, video del servicio en primer plano,
  prueba cerrada y producción: paso a paso en **[GOOGLE-PLAY.md](GOOGLE-PLAY.md)**. Ícono, gráfico
  y capturas provisionales en [`play/`](play/).

Ideas y faltas conocidas:

- La TV y la radio solo se probaron «Fuera del aire» contra producción (2026-11-17). Pruébelas
  mientras se transmite: `./gradlew assembleDebug -Pclub25.serverUrl.debug=https://radioclub25peru.com`
  e instale el APK en un teléfono.
- Ícono: el de la app es una versión vectorial del isotipo provisional
  (`app/src/main/res/drawable/ic_launcher_foreground.xml`); reemplácelo por el definitivo.
- Android Auto, Chromecast y programación semanal en la app no están incluidos.
