<#
.SYNOPSIS
  Descarga ffmpeg.exe y ffprobe.exe (build GPL de BtbN) en third_party/ffmpeg/.

.DESCRIPTION
  Club25 Estudio usa ffmpeg como subproceso para decodificar, analizar y codificar
  audio (ver docs/adr/0002-ffmpeg-subproceso.md). La versión está FIJADA aquí y el
  archivo se verifica con su SHA-256 antes de extraerlo: si no coincide, se aborta.

  Para actualizar ffmpeg: elegir otra build en
  https://github.com/BtbN/FFmpeg-Builds/releases (asset *-win64-gpl-X.Y.zip), copiar su
  URL y su "digest" (sha256) en las variables de abajo, ejecutar el script, probar
  (task test, estudio-cli) y anotar el cambio en docs/third-party.md y CHANGELOG.md.

.EXAMPLE
  pwsh -File scripts/get-ffmpeg.ps1
  pwsh -File scripts/get-ffmpeg.ps1 -Force   # vuelve a descargar aunque ya exista
#>
[CmdletBinding()]
param([switch]$Force)

$ErrorActionPreference = 'Stop'
$ProgressPreference = 'SilentlyContinue'

# --- Versión fijada ---------------------------------------------------------
$Version = 'n8.1.3-34-ga35157abb9'
$Url     = 'https://github.com/BtbN/FFmpeg-Builds/releases/download/autobuild-2026-11-07-12-47/ffmpeg-n8.1.3-34-ga35157abb9-win64-gpl-8.1.zip'
$Sha256  = '06bfc0069beb764b75f9eed956acc90c4d651bf06aed340fbf0708d10a6f76dc'
# ----------------------------------------------------------------------------

$root = Split-Path -Parent $PSScriptRoot           # carpeta estudio/
$dest = Join-Path $root 'third_party\ffmpeg'
$stamp = Join-Path $dest 'VERSION.txt'

if (-not $Force -and (Test-Path $stamp) -and ((Get-Content $stamp -Raw).Trim() -eq $Version) -and
    (Test-Path (Join-Path $dest 'ffmpeg.exe')) -and (Test-Path (Join-Path $dest 'ffprobe.exe'))) {
    Write-Host "ffmpeg $Version ya está en $dest (use -Force para descargar de nuevo)."
    exit 0
}

# Descarga en una carpeta temporal propia y vacía.
$tmp = Join-Path ([IO.Path]::GetTempPath()) ("club25-ffmpeg-" + [guid]::NewGuid())
New-Item -ItemType Directory -Path $tmp | Out-Null
try {
    $zip = Join-Path $tmp 'ffmpeg.zip'
    Write-Host "Descargando ffmpeg $Version ..."
    Invoke-WebRequest -UseBasicParsing -Uri $Url -OutFile $zip

    $hash = (Get-FileHash -Algorithm SHA256 $zip).Hash.ToLower()
    if ($hash -ne $Sha256) {
        throw "SHA-256 no coincide. Esperado $Sha256, obtenido $hash. No se instala nada."
    }
    Write-Host 'SHA-256 verificado.'

    $x = Join-Path $tmp 'x'
    Expand-Archive -Path $zip -DestinationPath $x
    $bin = Get-ChildItem -Path $x -Recurse -Filter 'ffmpeg.exe' | Select-Object -First 1
    if (-not $bin) { throw 'El zip no contiene ffmpeg.exe' }
    $top = Split-Path -Parent (Split-Path -Parent $bin.FullName)

    New-Item -ItemType Directory -Force -Path $dest | Out-Null
    Copy-Item (Join-Path $bin.DirectoryName 'ffmpeg.exe')  $dest -Force
    Copy-Item (Join-Path $bin.DirectoryName 'ffprobe.exe') $dest -Force
    $lic = Get-ChildItem -Path $top -Filter 'LICENSE*' | Select-Object -First 1
    if ($lic) { Copy-Item $lic.FullName (Join-Path $dest 'LICENSE.txt') -Force }
    Set-Content -Path $stamp -Value $Version -Encoding utf8

    & (Join-Path $dest 'ffmpeg.exe') -hide_banner -version | Select-Object -First 1
    Write-Host "Listo: $dest"
}
finally {
    Remove-Item -Recurse -Force $tmp -ErrorAction SilentlyContinue
}
