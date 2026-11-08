; Instalador de Club25 Estudio (NSIS). Lo compila `task installer` (wails build -nsis).
;
; Qué hace:
;   1. Comprueba que la PC sea Windows de 64 bits.
;   2. Instala WebView2 si falta (motor de la ventana; lo baja de Microsoft).
;   3. Copia el programa a Archivos de programa y crea los accesos directos.
;   4. Ejecuta "club25-estudio.exe --setup": revisa la PC y descarga desde el
;      servidor de la radio (radioclub25.sehuacho.com) lo que falte, como ffmpeg,
;      verificando el SHA-256 (ver internal/platform/tools). Si no hay internet,
;      el programa lo reintenta al abrirse.
;   5. Al desinstalar NO borra la carpeta de datos (%ProgramData%\Club25Estudio):
;      biblioteca, playlist e historial se conservan para una reinstalación.

Unicode true

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
!define MUI_FINISHPAGE_RUN_TEXT "Abrir Club25 Estudio"

!insertmacro MUI_PAGE_WELCOME
!insertmacro MUI_PAGE_DIRECTORY
!insertmacro MUI_PAGE_INSTFILES
!insertmacro MUI_PAGE_FINISH
!insertmacro MUI_UNPAGE_INSTFILES

!insertmacro MUI_LANGUAGE "Spanish"

Name "${INFO_PRODUCTNAME}"
OutFile "..\..\bin\${INFO_PROJECTNAME}-${ARCH}-installer.exe"
InstallDir "$PROGRAMFILES64\${INFO_COMPANYNAME}\${INFO_PRODUCTNAME}"
ShowInstDetails show

Function .onInit
   !insertmacro wails.checkArchitecture
FunctionEnd

Section
    !insertmacro wails.setShellContext

    DetailPrint "Revisando WebView2 (motor de la ventana)…"
    !insertmacro wails.webview2runtime

    SetOutPath $INSTDIR
    !insertmacro wails.files

    CreateShortcut "$SMPROGRAMS\${INFO_PRODUCTNAME}.lnk" "$INSTDIR\${PRODUCT_EXECUTABLE}"
    CreateShortCut "$DESKTOP\${INFO_PRODUCTNAME}.lnk" "$INSTDIR\${PRODUCT_EXECUTABLE}"

    !insertmacro wails.writeUninstaller

    DetailPrint "Revisando la PC y descargando del servidor lo que falte (ffmpeg)…"
    DetailPrint "Puede tardar unos minutos según la conexión."
    ExecWait '"$INSTDIR\${PRODUCT_EXECUTABLE}" --setup' $0
    ${If} $0 == 0
        DetailPrint "Herramientas listas."
    ${Else}
        DetailPrint "No se pudieron descargar todas las herramientas (código $0). El programa lo reintentará al abrirse."
    ${EndIf}
SectionEnd

Section "uninstall"
    !insertmacro wails.setShellContext
    RMDir /r "$AppData\${PRODUCT_EXECUTABLE}" ; datos de WebView2
    RMDir /r $INSTDIR
    Delete "$SMPROGRAMS\${INFO_PRODUCTNAME}.lnk"
    Delete "$DESKTOP\${INFO_PRODUCTNAME}.lnk"
    DeleteRegValue HKCU "Software\Microsoft\Windows\CurrentVersion\Run" "Club25 Estudio"
    !insertmacro wails.deleteUninstaller
SectionEnd
