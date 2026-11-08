# Guía de uso de Club25 Estudio

## Instalar

1. Descargue el instalador desde el servidor de la radio (Admin → Estudio (PC) indica el enlace) o use
   el programa portable `club25-estudio.exe`.
2. El instalador revisa la PC (Windows de 64 bits), instala WebView2 si falta y, al terminar, ejecuta
   `club25-estudio.exe --setup`, que **descarga del servidor** `radioclub25.sehuacho.com` lo que falte
   (ffmpeg y ffprobe), verificando su huella SHA-256.
3. Si no había internet, el programa lo vuelve a intentar al abrirse (Ajustes → Sistema →
   "Revisar y descargar herramientas del servidor").

Los datos (biblioteca, playlist, historial, ajustes, copias) quedan en `%ProgramData%\Club25Estudio`.
Desinstalar no los borra.

## Primera configuración (Ajustes)

1. **Audio**: salida principal (la tarjeta que va a la consola/transmisor), micrófono si lo usa en la PC.
   "Guardar y aplicar audio".
2. **Panel web**: URL `https://radioclub25.sehuacho.com` y el token de Admin → Estudio (PC) → Generar token.
   "Guardar y sincronizar": llegan las cuñas FM y la web empieza a mostrar lo que suena.
3. **Emisión a la web (Icecast)**: marque "Emitir a Icecast", escriba la contraseña de fuente (Admin →
   Streams y emisión) y "Guardar y reiniciar emisión". Si el micrófono está en una consola, elija la
   fuente "Entrada de línea" y la entrada donde llega la salida de la consola.
4. **Locuciones**: carpeta con `HRS00…23`, `MIN00…59`, `TMP-10…45` ("Verificar archivos").
5. **Seguridad**: "Arrancar con Windows" y "Empezar a sonar al abrir" para una PC dedicada.

## Uso diario

- **Biblioteca**: "Importar carpeta…" con la categoría correspondiente. Las pistas se analizan solas.
- **Relojes** y **Parrilla**: definen qué suena cada hora. La **Playlist** del día se genera sola (hoy y
  mañana) y se puede editar: mover, quitar, bloquear (🔒 sobrevive a la regeneración) e insertar.
- **Cuñas**: llegan del panel web; las tandas se reparten solas. "Replanificar" si cambió algo.
- **Al aire**:
  - Automático: avanza solo. Asistido: espera **Espacio** después de cada elemento. En vivo: la música
    se pausa y quedan micrófono y botonera.
  - **F1–F12** disparan la botonera; **Ctrl+M** abre/cierra el micrófono.
  - La barra de abajo muestra la tarjeta, Icecast y la sincronización con el panel.
- **Reportes**: lo emitido y el comprobante de cuñas en CSV.

## Si algo falla

| Síntoma | Qué hacer |
|---|---|
| "ffmpeg no encontrado" | Ajustes → Sistema → descargar herramientas y reiniciar el programa |
| Icecast "desconectado" | revisar contraseña y que no haya otro programa (BUTT) emitiendo en `/radio` |
| Panel "sin conexión" / "token inválido" | revisar internet; generar un token nuevo en el panel |
| "Servicio suspendido" | la licencia del panel está bloqueada: la radio sigue sonando por FM |
| Aviso de reloj adelantado/atrasado | sincronizar la hora de Windows (Configuración → Hora) |
| Silencio en el aire | el programa pone la categoría Emergencia; cargue pistas en ella |

Logs: `%ProgramData%\Club25Estudio\logs\estudio.log`. Copias diarias de la base: `...\backups`.
