; Instalador de Club25 Emisor (NSIS). Lo arma scripts\instalador.ps1 (wails build -nsis).
;
; Qué hace:
;   1. Comprueba que la PC sea Windows 10/11 de 64 bits.
;   2. Instala WebView2 si falta (motor de la ventana; el instalador oficial de Microsoft).
;   3. Copia el programa, emisor-consola.exe y ffmpeg.exe (todo lo necesario para emitir:
;      no hace falta instalar nada más) y crea los accesos directos.
;   4. Instala POR USUARIO, sin administrador, en %LOCALAPPDATA%\Programs\Club25 Emisor:
;      así las actualizaciones automáticas pueden reemplazar el .exe (en Archivos de
;      programa necesitarían permisos de administrador y fallarían).
;   5. Si el emisor está abierto, pide cerrarlo antes de reemplazarlo.
;   6. Al desinstalar NO borra la configuración (%APPDATA%\Club25 Emisor): servidor,
;      claves cifradas, ecualizador, etc. se conservan para una reinstalación.

Unicode true
SetCompressor /SOLID lzma

!define REQUEST_EXECUTION_LEVEL "user"
; Sin esto, wails.writeUninstaller escribe en HKLM (necesita administrador) y el
; programa no aparece en Configuración → Aplicaciones.
!define WAILS_INSTALL_SCOPE "user"
; El mismo nombre que deja wails build (outputfilename de wails.json) y que usa el zip.
!define PRODUCT_EXECUTABLE "club25-emisor.exe"

!include "LogicLib.nsh"
!include "wails_tools.nsh"

VIProductVersion "${INFO_PRODUCTVERSION}.0"
VIFileVersion    "${INFO_PRODUCTVERSION}.0"

VIAddVersionKey "CompanyName"     "${INFO_COMPANYNAME}"
VIAddVersionKey "FileDescription" "${INFO_PRODUCTNAME} — instalador"
VIAddVersionKey "ProductVersion"  "${INFO_PRODUCTVERSION}"
VIAddVersionKey "FileVersion"     "${INFO_PRODUCTVERSION}"
VIAddVersionKey "LegalCopyright"  "${INFO_COPYRIGHT}"
VIAddVersionKey "ProductName"     "${INFO_PRODUCTNAME}"

ManifestDPIAware true

!include "MUI.nsh"

!define MUI_ICON "..\icon.ico"
!define MUI_UNICON "..\icon.ico"
!define MUI_FINISHPAGE_NOAUTOCLOSE
!define MUI_ABORTWARNING
!define MUI_FINISHPAGE_RUN "$INSTDIR\${PRODUCT_EXECUTABLE}"
!define MUI_FINISHPAGE_RUN_TEXT "Abrir Club25 Emisor"

!insertmacro MUI_PAGE_WELCOME
!insertmacro MUI_PAGE_DIRECTORY
!insertmacro MUI_PAGE_INSTFILES
!insertmacro MUI_PAGE_FINISH
!insertmacro MUI_UNPAGE_CONFIRM
!insertmacro MUI_UNPAGE_INSTFILES

!insertmacro MUI_LANGUAGE "Spanish"

; Archivos que no compila Wails: los deja scripts\instalador.ps1 en una carpeta corta
; (%TEMP%\club25-emisor-instalador) y la pasa en la variable CLUB25_INSTALADOR. NSIS no
; acepta rutas de más de 260 caracteres, y dentro de una carpeta de proyecto profunda las
; rutas relativas (..\..\build\…) se pasan de ese largo. El script copia luego el
; instalador a build\bin\.
!define STAGE "$%CLUB25_INSTALADOR%"
!define EXTRA "${STAGE}\archivos"

Name "${INFO_PRODUCTNAME} ${INFO_PRODUCTVERSION}"
OutFile "${STAGE}\Club25Emisor-${INFO_PRODUCTVERSION}-instalador.exe"
InstallDir "$LOCALAPPDATA\Programs\${INFO_PRODUCTNAME}"
InstallDirRegKey HKCU "Software\${INFO_COMPANYNAME}\${INFO_PRODUCTNAME}" "InstallDir"
ShowInstDetails show
ShowUninstDetails show

Function .onInit
    !insertmacro wails.checkArchitecture
FunctionEnd

; esperarCierre: si el .exe está en uso (el emisor abierto, quizá oculto junto al
; reloj), pide cerrarlo. Abrir el archivo para escribir falla mientras corre.
!macro esperarCierre EXE
    ${If} ${FileExists} "$INSTDIR\${EXE}"
        ${Do}
            ClearErrors
            FileOpen $0 "$INSTDIR\${EXE}" a
            ${IfNot} ${Errors}
                FileClose $0
                ${Break}
            ${EndIf}
            MessageBox MB_RETRYCANCEL|MB_ICONEXCLAMATION "Club25 Emisor está abierto.$\r$\n$\r$\nCiérrelo (clic derecho en el ícono junto al reloj → Salir) y pulse Reintentar.$\r$\nSi está emitiendo, la emisión se detendrá." IDRETRY +2
            Abort
        ${Loop}
    ${EndIf}
!macroend

Section
    !insertmacro wails.setShellContext

    !insertmacro esperarCierre "${PRODUCT_EXECUTABLE}"
    !insertmacro esperarCierre "emisor-consola.exe"

    DetailPrint "Revisando WebView2 (motor de la ventana)…"
    !insertmacro wails.webview2runtime

    SetOutPath $INSTDIR
    !insertmacro wails.files
    File "${EXTRA}\emisor-consola.exe"
    File "${EXTRA}\ffmpeg.exe"
    File "${EXTRA}\FFMPEG-LICENCIA.txt"
    File "${EXTRA}\emisor.defaults.ejemplo.json"
    File "${EXTRA}\LEEME.txt"
    ; Restos de una actualización automática anterior.
    Delete "$INSTDIR\${PRODUCT_EXECUTABLE}.old"

    CreateShortcut "$SMPROGRAMS\${INFO_PRODUCTNAME}.lnk" "$INSTDIR\${PRODUCT_EXECUTABLE}"
    CreateShortcut "$DESKTOP\${INFO_PRODUCTNAME}.lnk" "$INSTDIR\${PRODUCT_EXECUTABLE}"

    WriteRegStr HKCU "Software\${INFO_COMPANYNAME}\${INFO_PRODUCTNAME}" "InstallDir" "$INSTDIR"
    !insertmacro wails.writeUninstaller
SectionEnd

Section "uninstall"
    !insertmacro wails.setShellContext

    !insertmacro esperarCierre "${PRODUCT_EXECUTABLE}"

    RMDir /r "$AppData\${PRODUCT_EXECUTABLE}" ; datos de WebView2 (no la configuración)
    RMDir /r $INSTDIR
    Delete "$SMPROGRAMS\${INFO_PRODUCTNAME}.lnk"
    Delete "$DESKTOP\${INFO_PRODUCTNAME}.lnk"
    ; «Iniciar con Windows»: si quedara, intentaría abrir un programa que ya no está.
    DeleteRegValue HKCU "Software\Microsoft\Windows\CurrentVersion\Run" "Club25 Emisor"
    DeleteRegKey HKCU "Software\${INFO_COMPANYNAME}\${INFO_PRODUCTNAME}"
    !insertmacro wails.deleteUninstaller
SectionEnd
