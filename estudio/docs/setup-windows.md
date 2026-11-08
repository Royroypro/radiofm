# Preparar una PC Windows para desarrollar Club25 Estudio

Todo se instala **sin permisos de administrador** (en la carpeta del usuario), salvo Go y Node
si no están.

## Resumen

| Herramienta | Versión | Para qué | Cómo |
|---|---|---|---|
| Go | 1.27+ | compilar | https://go.dev/dl/ (instalador) |
| Node.js | 22.12+ (LTS) | compilar el frontend (Vite 8) | https://nodejs.org/ |
| PowerShell | 7+ | scripts y Task | https://aka.ms/powershell |
| WebView2 Runtime | cualquiera | la ventana de la app | viene con Windows 11 / Edge |
| Wails CLI | v2.17.1 | dev/build de la app | `go install github.com/wailsapp/wails/v2/cmd/wails@v2.17.1` |
| Task | v3.54 | atajos (`task dev`, `task test`) | `go install github.com/go-task/task/v3/cmd/task@v3.54.0` |
| gcc (llvm-mingw) | 20261104 ucrt x86_64 | CGO para miniaudio (malgo) | zip de https://github.com/mstorsjo/llvm-mingw/releases |
| ffmpeg/ffprobe | n8.1.3 (GPL, BtbN) | decodificar, analizar, codificar | `scripts/get-ffmpeg.ps1` |
| NSIS | 3.x (opcional) | instalador (fase 8) | https://nsis.sourceforge.io/ |

## Paso a paso

```powershell
git clone https://github.com/Royroypro/radiofm.git
cd radiofm\estudio
pwsh -File scripts/dev-setup.ps1
```

`dev-setup.ps1` hace, en orden:
1. Comprueba Go y Node.
2. `go install` de Wails y Task (quedan en `%USERPROFILE%\go\bin`).
3. Descarga llvm-mingw, **verifica su SHA-256** y lo descomprime en `%LOCALAPPDATA%\Programs`.
4. Añade `llvm-mingw\bin` y `%USERPROFILE%\go\bin` al PATH **del usuario** (abra una terminal nueva).
5. Descarga ffmpeg (`get-ffmpeg.ps1`, también verificado por SHA-256) en `third_party/ffmpeg/`.
6. `npm install` del frontend y `wails doctor`.

Después:

```powershell
task dev          # ventana de desarrollo
task test         # tests
task cli -- devices
```

Locuciones provisionales para probar la hora y la temperatura: `task locutions` (necesita una voz
SAPI en español; Windows trae "Microsoft Helena" o "Sabina").

## Editor

Para que el editor (VS Code/GoLand) vea a la vez el servidor y el estudio, cree un workspace de Go
**local** (no se sube al repositorio):

```powershell
cd radiofm
go work init . ./estudio
```

## Problemas frecuentes

| Síntoma | Causa / solución |
|---|---|
| `cgo: C compiler "gcc" not found` | Falta llvm-mingw en el PATH. Abra una terminal nueva, use `task …` o anteponga `pwsh -File scripts/with-toolchain.ps1` al comando. |
| `audio: compilado sin CGO` | Se compiló con `CGO_ENABLED=0`. Use `task build` / `task cli`. |
| `npm error ERESOLVE` | Versiones del frontend incompatibles; use las de `frontend/package.json` (`npm install`). |
| La ventana sale en blanco | Falta WebView2 Runtime: https://developer.microsoft.com/microsoft-edge/webview2/ |
| `No hay ninguna voz SAPI en español` | Configuración → Hora e idioma → Voz → Agregar voces (español). |
| Antivirus bloquea el .exe | Ejecutables nuevos sin firmar; añada una exclusión para `build\bin`. |
