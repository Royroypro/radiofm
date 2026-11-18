# Empaqueta Club25 Emisor para Windows en build\dist\Club25Emisor-<versión>.zip:
#   club25-emisor.exe   la ventana (Wails)
#   emisor-consola.exe  el mismo emisor sin ventana (diagnóstico, tareas programadas)
#   ffmpeg.exe          el que hace la codificación (se busca junto al programa)
#   emisor.defaults.ejemplo.json, LEEME.txt
#
# Uso (desde la carpeta emisor\):
#   powershell -ExecutionPolicy Bypass -File scripts\empaquetar.ps1 [-FFmpeg C:\ruta\ffmpeg.exe]
#
# Requisitos: Go, Wails CLI v2 (go install github.com/wailsapp/wails/v2/cmd/wails@v2.17.1), un
# compilador C para CGO (gcc de llvm-mingw o MSYS2: la captura del audio de la PC usa miniaudio) y un
# ffmpeg.exe estático para Windows (por ejemplo, el «essentials» de gyan.dev). No hace falta Node.
param(
    [string]$FFmpeg = ""
)
$ErrorActionPreference = "Stop"
$root = Split-Path -Parent $PSScriptRoot
Set-Location $root

$version = (Get-Content wails.json -Raw | ConvertFrom-Json).info.productVersion
Write-Host "== Club25 Emisor $version =="

# 1. Compilar la ventana y la consola, con CGO (captura del audio de los altavoces).
if (-not (Get-Command gcc -ErrorAction SilentlyContinue)) { throw "Falta gcc (CGO): instale llvm-mingw o MSYS2" }
$env:CGO_ENABLED = "1"
wails build -clean -trimpath -platform windows/amd64
if ($LASTEXITCODE -ne 0) { throw "wails build falló" }
go build -trimpath -ldflags "-s -w" -o build\bin\emisor-consola.exe .\cmd\emisor-consola
if ($LASTEXITCODE -ne 0) { throw "go build (consola) falló" }

# 2. Encontrar ffmpeg. El de Chocolatey (C:\ProgramData\chocolatey\bin) es un lanzador:
#    se copia el ffmpeg real que está en lib\ffmpeg\tools.
if (-not $FFmpeg) {
    $cmd = Get-Command ffmpeg -ErrorAction SilentlyContinue
    if (-not $cmd) { throw "No se encontró ffmpeg. Pase -FFmpeg C:\ruta\ffmpeg.exe" }
    $FFmpeg = $cmd.Source
    if ($FFmpeg -like "*\chocolatey\bin\*") {
        $real = Get-ChildItem "$env:ProgramData\chocolatey\lib\ffmpeg*\tools" -Recurse -Filter ffmpeg.exe -ErrorAction SilentlyContinue | Select-Object -First 1
        if ($real) { $FFmpeg = $real.FullName }
    }
}
Write-Host "ffmpeg: $FFmpeg"

# 3. Armar la carpeta y el zip.
$dist = Join-Path $root "build\dist"
# Carpeta con la versión en el nombre: no pisa un emisor que esté al aire desde un paquete anterior.
$dir = Join-Path $dist "Club25Emisor-$version"
if (Test-Path $dir) { Remove-Item $dir -Recurse -Force }
New-Item -ItemType Directory -Force $dir | Out-Null
Copy-Item build\bin\club25-emisor.exe, build\bin\emisor-consola.exe $dir
Copy-Item $FFmpeg (Join-Path $dir "ffmpeg.exe")
Copy-Item emisor.defaults.ejemplo.json $dir
@"
Club25 Emisor $version
======================

1. Abra club25-emisor.exe (deje ffmpeg.exe en la misma carpeta).
2. Al abrir, escriba el dominio de la web de su radio y el token de Admin → Estudio (PC): el programa
   solo funciona autenticado. Luego, en Ajustes → Datos del panel, pulse «Traer datos del panel».
3. Radio → Entrada / Stream: elija el tipo de entrada (micrófono/línea o audio de la PC = lo que suena en los
   altavoces), el dispositivo, el formato y escriba la contraseña de fuente.
4. TV → Entradas / Stream: elija el video (cámara, pantalla o una dirección http/https) y el audio de la TV.
5. Marque «Emitir» en lo que quiera enviar (radio, TV o ambas) y pulse ▶ Iniciar.
6. Radio → Mejoras (y TV → Mejoras del audio): ganancia, MEJORA y ecualizador: elija un perfil (Radio FM, Modo fiesta…) o mueva las bandas;
   guarde sus propios perfiles con nombre. Cambia al aire, sin cortar.
7. Radio → Canciones: elija «Jazler RadioStar 2 (automático)» para que salga solo lo que suena en
   Jazler, o cargue una lista de canciones (.m3u, .txt).
8. Ajustes → Arranque: «Iniciar con Windows» + «Empezar a emitir al abrir» = la PC transmite sola al
   encender e iniciar sesión. Con «Seguir en segundo plano», la X no corta: el programa queda junto al
   reloj (clic = abrir; clic derecho = Salir).

Para otra frecuencia u otro servidor: copie emisor.defaults.ejemplo.json como emisor.defaults.json
junto al programa y cambie los datos (sin claves).

ffmpeg se distribuye bajo GPL; su código fuente está en https://ffmpeg.org y en la página del build usado.
"@ | Set-Content -Encoding UTF8 (Join-Path $dir "LEEME.txt")

$zip = Join-Path $dist "Club25Emisor-$version.zip"
if (Test-Path $zip) { Remove-Item $zip -Force }
Compress-Archive -Path "$dir\*" -DestinationPath $zip
Write-Host "Listo: $zip"
