<#
.SYNOPSIS
  Arma el instalador de Club25 Emisor: build\bin\Club25Emisor-<versión>-instalador.exe

.DESCRIPTION
  Incluye todo lo necesario para emitir: club25-emisor.exe (la ventana), emisor-consola.exe,
  ffmpeg.exe (con su licencia), LEEME.txt y emisor.defaults.ejemplo.json. Si la PC no tiene
  WebView2, el instalador lo instala. Se instala por usuario, sin administrador (ver
  build\windows\installer\project.nsi).

  Requisitos para compilar: Go 1.27+, Wails CLI v2.17.1, gcc para CGO (llvm-mingw) y NSIS 3
  (makensis en el PATH o en su carpeta de instalación). ffmpeg se descarga solo con
  scripts\get-ffmpeg.ps1 (versión fijada y verificada con SHA-256).

.EXAMPLE
  powershell -ExecutionPolicy Bypass -File scripts\instalador.ps1
#>
$ErrorActionPreference = 'Stop'
$root = Split-Path -Parent $PSScriptRoot
Set-Location $root

$version = (Get-Content wails.json -Raw | ConvertFrom-Json).info.productVersion
Write-Host "== Instalador de Club25 Emisor $version =="

# makensis: Wails lo busca en el PATH.
if (-not (Get-Command makensis -ErrorAction SilentlyContinue)) {
    foreach ($d in @("${env:ProgramFiles(x86)}\NSIS", "$env:ProgramFiles\NSIS")) {
        if (Test-Path (Join-Path $d 'makensis.exe')) { $env:Path = "$d;$env:Path"; break }
    }
}
if (-not (Get-Command makensis -ErrorAction SilentlyContinue)) { throw 'Falta NSIS (makensis). Instálelo: winget install NSIS.NSIS' }
if (-not (Get-Command gcc -ErrorAction SilentlyContinue)) { throw 'Falta gcc (CGO): winget install MartinStorsjo.LLVM-MinGW.UCRT' }
if (-not (Get-Command wails -ErrorAction SilentlyContinue)) { throw 'Falta Wails: go install github.com/wailsapp/wails/v2/cmd/wails@v2.17.1' }

# 1. ffmpeg (versión fijada, verificada).
& (Join-Path $PSScriptRoot 'get-ffmpeg.ps1')
if ($LASTEXITCODE) { throw 'No se pudo conseguir ffmpeg' }

# 2. Lo que el instalador copia además del programa, en una carpeta de ruta corta: NSIS no acepta
#    rutas de más de 260 caracteres (ver project.nsi). La lee de la variable CLUB25_INSTALADOR.
$stage = Join-Path ([IO.Path]::GetTempPath()) 'club25-emisor-instalador'
if (Test-Path $stage) { Remove-Item $stage -Recurse -Force }
$extra = Join-Path $stage 'archivos'
New-Item -ItemType Directory -Force $extra | Out-Null
$env:CLUB25_INSTALADOR = $stage
$env:CGO_ENABLED = '1'
go build -trimpath -ldflags "-s -w" -o (Join-Path $extra 'emisor-consola.exe') .\cmd\emisor-consola
if ($LASTEXITCODE) { throw 'go build (consola) falló' }
Copy-Item third_party\ffmpeg\ffmpeg.exe $extra
Copy-Item third_party\ffmpeg\LICENSE.txt (Join-Path $extra 'FFMPEG-LICENCIA.txt')
Copy-Item emisor.defaults.ejemplo.json $extra
$ffv = (Get-Content third_party\ffmpeg\VERSION.txt -Raw).Trim()
@"
Club25 Emisor $version
======================

Todo lo necesario ya está instalado (ffmpeg incluido). Para empezar:

1. Abra Club25 Emisor (escritorio o menú Inicio).
2. Escriba el dominio de la web de su radio y el token de Admin → Estudio (PC): el programa solo
   funciona autenticado. Luego, en Ajustes → Datos del panel, pulse «Traer datos del panel».
3. Radio → Entrada / Stream: tipo de entrada (micrófono/línea o audio de la PC), dispositivo,
   formato y contraseña de fuente.
4. TV → Entradas: video (cámara, pantalla, dirección http/https o cámara IP RTSP) y su audio. Con
   video por dirección o RTSP elija qué audio sale: el del video, el de la radio o los dos mezclados.
   Si el video no trae audio, sale el de la radio.
5. Marque «Emitir» en lo que quiera enviar y pulse ▶ Iniciar.
6. Ajustes → Arranque: «Iniciar con Windows» + «Empezar a emitir al abrir» = la PC transmite sola.

Para otra frecuencia u otro servidor: copie emisor.defaults.ejemplo.json como emisor.defaults.json
junto al programa y cambie los datos (sin claves).

Desinstalar: Configuración de Windows → Aplicaciones → Club25 Emisor. La configuración
(%APPDATA%\Club25 Emisor) se conserva.

ffmpeg ($ffv, build GPL de BtbN) se distribuye bajo GPL (ver FFMPEG-LICENCIA.txt); su código
fuente está en https://ffmpeg.org y en https://github.com/BtbN/FFmpeg-Builds.
"@ | Set-Content -Encoding UTF8 (Join-Path $extra 'LEEME.txt')

# 3. Programa + instalador NSIS (project.nsi).
wails build -clean -trimpath -platform windows/amd64 -nsis
if ($LASTEXITCODE) { throw 'wails build -nsis falló' }

$name = "Club25Emisor-$version-instalador.exe"
$built = Join-Path $stage $name
if (-not (Test-Path $built)) { throw "No apareció $built" }
$out = Join-Path $root "build\bin\$name"
Copy-Item $built $out -Force
Remove-Item $stage -Recurse -Force -ErrorAction SilentlyContinue
$mb = [math]::Round((Get-Item $out).Length / 1MB, 1)
Write-Host "Listo: $out ($mb MB)"
