# Publicar «Radio Club 25» en Google Play — pasos para el dueño de la app

Esta guía es lo que **solo usted puede hacer** (cuenta de Google, pagos, firma, envíos a revisión).
Todo lo técnico ya está hecho: la app compila, la web ya tiene `/api/app/config` y
`/privacidad` en producción, y en [`play/`](play/) están el ícono, el gráfico y capturas.
Para compilar y firmar vea [README.md](README.md).

| Dato fijo | Valor |
|---|---|
| ID de la app (`applicationId`) | `pe.club25peru.radio` — **no se puede cambiar nunca** después de subirla |
| Servidor | `https://radioclub25peru.com` |
| Política de privacidad | `https://radioclub25peru.com/privacidad` |
| Enlace de la ficha (cuando exista) | `https://play.google.com/store/apps/details?id=pe.club25peru.radio` |

Orden recomendado: 1 → 2 → 3 → 4 → 5 → 6 → 7 → 8. El paso 6 (prueba cerrada) dura al menos
14 días si la cuenta es personal: empiece pronto.

---

## 1. Cuenta de desarrollador de Google Play

**Para qué:** sin cuenta no se puede publicar. Google verifica la identidad de quien publica.

1. Entre a <https://play.google.com/console/signup> con la cuenta de Google que será dueña de la
   app (mejor una cuenta de la radio, no una personal que se pueda perder).
2. Elija el tipo de cuenta:
   - **Organización** (recomendado si la radio es una empresa): pide el número **D-U-N-S** de la
     empresa (gratis en <https://www.dnb.com/duns-number/get-a-duns.html>; tarda días o semanas),
     RUC/razón social, web y teléfono. **No exige la prueba cerrada de 14 días.**
   - **Personal**: más rápida, pero exige la prueba cerrada del paso 6 antes de producción.
3. Pague la cuota única (US$ 25) con tarjeta.
4. Complete la **verificación de identidad** (DNI o documento de la empresa) y verifique el
   teléfono y el correo de contacto. Puede tardar unos días.

## 2. Clave de subida (firma de la app)

**Para qué:** Google solo acepta apps firmadas, y cada actualización futura debe firmarse con la
misma clave de subida. Si se pierde, hay que pedirle a Google que la cambie (tarda días).

1. En el PC donde compila, cree una carpeta **fuera del repositorio**, p. ej. `C:\claves`.
2. Ejecute (le pedirá una contraseña y sus datos; anote la contraseña en un gestor de claves):
   ```bash
   keytool -genkeypair -v -keystore C:/claves/club25-upload.jks -alias club25 -keyalg RSA -keysize 4096 -validity 10000
   ```
   (`keytool` viene con el JDK: `"C:\Program Files\Java\jdk-21\bin\keytool.exe"`).
3. Cree `android/keystore.properties` con:
   ```properties
   storeFile=C:/claves/club25-upload.jks
   storePassword=SU_CONTRASEÑA
   keyAlias=club25
   keyPassword=SU_CONTRASEÑA
   ```
   Este archivo y el `.jks` **nunca** se suben a GitHub (ya están en `.gitignore`).
4. Haga **copia de seguridad** del `.jks` y de la contraseña (USB + nube privada).
5. Compile el paquete firmado:
   ```bash
   cd android
   ./gradlew bundleRelease -Pclub25.versionCode=1 -Pclub25.versionName=1.0.0
   ```
   Resultado: `android/app/build/outputs/bundle/release/app-release.aab` (este es el que se sube).

## 3. Crear la app en Play Console

**Para qué:** reserva el nombre y abre el formulario de la ficha y de las políticas.

1. Play Console → **Crear app**.
2. Nombre: `Radio Club 25` · Idioma predeterminado: **Español (Latinoamérica) – es-419**
   · Tipo: **App** · **Gratis** (una app gratis no puede pasar a ser de pago).
3. Acepte las declaraciones (políticas del programa y leyes de exportación de EE. UU.).

## 4. Ficha de Play Store (Crecer → Presencia en Play Store → Ficha principal)

**Para qué:** es lo que ven las personas antes de instalar. Google la revisa.

- **Nombre** (máx. 30): `Radio Club 25`
- **Descripción breve** (máx. 80):
  `Radio Club 25 de Huacho en vivo: 93.3 FM, más frecuencias y TV en vivo.`
- **Descripción completa** (máx. 4000), texto sugerido:
  ```
  Escucha Radio Club 25 desde Huacho donde estés: la música que va contigo, noticias locales y entretenimiento las 24 horas.

  • Todas nuestras frecuencias en vivo (93.3 FM Huacho y más).
  • Sigue escuchando con la pantalla apagada o usando otras apps, con controles en la notificación y en la pantalla de bloqueo.
  • Mira nuestra TV en vivo a pantalla completa cuando estamos transmitiendo.
  • Mira qué programa está al aire y quién lo conduce.
  • Escríbenos por WhatsApp, llámanos o síguenos en redes con un toque.

  No necesitas crear una cuenta. Solo necesitas conexión a Internet.
  ```
- **Ícono de la app** (512×512 PNG): [`play/icono-512.png`](play/icono-512.png).
  Es el isotipo provisional; reemplácelo cuando tenga el definitivo.
- **Gráfico de funciones** (1024×500): [`play/grafico-funciones-1024x500.png`](play/grafico-funciones-1024x500.png).
- **Capturas de teléfono** (mín. 2): en [`play/`](play/) hay dos, pero se tomaron con la radio
  **fuera del aire**. Para que luzcan mejor, tómelas de nuevo **mientras se transmite** (con la
  app instalada en su teléfono: botón de encendido + bajar volumen) mostrando: la radio sonando,
  la TV en vivo y la notificación con los controles.
- **Categoría:** Música y audio · **Etiquetas:** Radio, Música.
- **Datos de contacto:** el correo de la radio (obligatorio y público), web
  `https://radioclub25peru.com`, teléfono opcional.

## 5. Contenido de la app (Política → Contenido de la app)

**Para qué:** Google no deja publicar hasta responder todos estos formularios. Respuestas
sugeridas según lo que hace la app hoy (si se agregan anuncios, cuentas o estadísticas, cambian):

| Formulario | Respuesta |
|---|---|
| **Política de privacidad** | `https://radioclub25peru.com/privacidad` |
| **Anuncios** | **No**, la app no contiene anuncios |
| **Acceso a la app** | Toda la funcionalidad está disponible sin restricciones (no hay inicio de sesión) |
| **Clasificación de contenido** | Cuestionario IARC: correo de la radio, categoría «Todos los demás tipos de apps». Violencia, sexo, lenguaje, drogas, apuestas: **No**. ¿Los usuarios interactúan o comparten contenido entre sí? **No**. ¿Comparte la ubicación? **No**. ¿Compras digitales? **No**. Si la radio emite contenido para adultos o publicidad de alcohol, respóndalo con honestidad |
| **Público objetivo** | **13 años o más** (13–15, 16–17, 18+). No marque menores de 13: activaría las reglas de «Familias» |
| **¿Puede atraer a niños sin querer?** | No |
| **App de noticias** | No (es una radio musical; si quiere declararla de noticias, Google pide más datos editoriales) |
| **Apps gubernamentales / financieras / de salud / VPN** | No |
| **Seguridad de los datos** | ¿Recopila o comparte datos del usuario? **No**. La app no tiene cuentas, ni estadísticas, ni anuncios, ni lee datos del teléfono; solo descarga la configuración y el audio. Cifrado en tránsito: **Sí** (HTTPS). Eliminación de datos: no aplica (no hay cuentas) |
| **Permisos de servicio en primer plano** | Ver abajo |

### Servicio en primer plano (FOREGROUND_SERVICE_MEDIA_PLAYBACK)

**Para qué:** la app sigue sonando con la pantalla apagada gracias a un «servicio en primer
plano». Google pide justificarlo y verlo en video.

1. Tipo: **Reproducción de contenido multimedia**.
2. Descripción sugerida:
   `Reproduce la radio en vivo cuando el usuario pulsa ▶, también con la pantalla apagada o desde otras apps. Se muestra una notificación con controles de pausa; al pausar o cerrar, el servicio se detiene.`
3. **Video** (30–60 s, puede ser un enlace de YouTube **no listado**): grábelo en su teléfono
   (grabador de pantalla del panel rápido) con la radio al aire: abrir la app → pulsar ▶ →
   salir a la pantalla de inicio (sigue sonando) → bajar la barra y mostrar la notificación con
   pausa → bloquear la pantalla y mostrar los controles → pausar.

## 6. Prueba cerrada (obligatoria con cuenta personal)

**Para qué:** con cuentas personales nuevas Google exige que **al menos 12 personas** tengan la
app instalada desde Play durante **14 días seguidos** antes de dejar pedir acceso a producción.

1. Probar y publicar → **Pruebas → Prueba cerrada** → Crear pista (p. ej. «Testers Club25»).
2. **Testers:** cree una lista de correo con los Gmail de 12 a 20 personas (equipo de la radio,
   familiares, oyentes de confianza). Mejor 15 o más por si alguno se sale.
3. **Países:** Perú.
4. **Crear versión** → suba `app-release.aab` (paso 2) → acepte **Play App Signing** (Google
   guarda la clave final; usted se queda con la de subida) → notas de la versión:
   `Primera versión: radio en vivo, TV en vivo y contacto.`
5. Envíe a revisión (la primera revisión puede tardar varios días).
6. Cuando esté aprobada, copie el **enlace para unirse a la prueba** y envíelo a los testers:
   cada uno debe abrirlo con su Gmail, aceptar y **instalar la app desde Play**. No deben
   desinstalarla durante los 14 días.
7. Pídales que la usen y que comenten en la ficha (privada para testers) si algo falla.
8. Pasados los 14 días: Panel → **Solicitar acceso a producción**. Google pregunta cómo se
   reclutaron los testers, qué opinaron y qué se mejoró: responda con detalle.

Con **cuenta de organización** puede saltarse este paso, pero conviene igual una **prueba
interna** (hasta 100 testers, sin revisión larga) para probar en teléfonos reales.

## 7. Publicar en producción

1. Probar y publicar → **Producción** → Países: Perú (o todos).
2. **Crear versión** → «Agregar desde biblioteca» el mismo AAB de la prueba → notas → Revisar →
   **Iniciar lanzamiento**. Puede elegir un lanzamiento gradual (p. ej. 20 % y luego 100 %).
3. La revisión suele tardar de horas a unos días. Google avisa por correo.

## 8. Después de publicar

1. En la web: **Admin → Marca y contacto → «App Android» → Enlace en Google Play**:
   `https://play.google.com/store/apps/details?id=pe.club25peru.radio`.
   **Para qué:** lo usan el botón «Compartir app», el aviso «Actualice la app» y el botón
   «Disponible en Google Play» del pie de la web. Si además deja subido el APK, la web muestra los
   dos botones; si prefiere solo Play, borre el enlace del APK.
2. **Cada actualización:** suba `versionCode` (2, 3, …) y `versionName` (1.0.1, …), compile con
   la misma clave, súbala en Producción. Si una versión vieja deja de funcionar con el servidor,
   ponga en el panel «Versión mínima» = el nuevo `versionCode` y las apps viejas pedirán
   actualizar.
3. **Cada año** (antes del 31 de agosto) Google exige subir el nivel de API objetivo; vea
   «Nivel de API objetivo» en el README. Play Console avisa con meses de anticipación.
4. Si cambia lo que la web o la app guardan de los oyentes (estadísticas, anuncios en la app,
   cuentas), actualice `/privacidad` y el formulario de **Seguridad de los datos**.
