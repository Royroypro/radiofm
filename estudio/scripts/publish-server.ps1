<#
.SYNOPSIS
  Publica en el servidor de la radio las herramientas y el programa que descarga el instalador.

.DESCRIPTION
  Sube por SSH/SCP a <proyecto en el servidor>/data/uploads/estudio/ (la app los sirve en
  https://radioclub25peru.com/uploads/estudio/...):
    tools/ffmpeg-<versión>/ffmpeg.exe, ffprobe.exe, LICENSE.txt   (de third_party/ffmpeg)
    app/<versión>/club25-estudio.exe                               (de build/bin)
    app/<versión>/club25-estudio-amd64-installer.exe               (si se compiló con task installer)
    manifest.json                                                  (URLs + SHA-256 + tamaños)
  El programa (platform/tools) lee manifest.json y verifica cada archivo antes de usarlo.
  No necesita sudo: la carpeta data/ es del usuario que corre los contenedores.

.EXAMPLE
  pwsh -File scripts/publish-server.ps1 -Version 1.0.0
#>
[CmdletBinding()]
param(
    [string]$Version = '1.0.0',
    [string]$SshHost = '20.20.40.1',
    [int]$Port = 1522,
    [string]$User = 'lima',
    [string]$RemoteProject = 'sehuacho/radioclub25'   # relativo a la carpeta del usuario (scp no expande ~)
)

$ErrorActionPreference = 'Stop'
$root = Split-Path -Parent $PSScriptRoot
$ff = Join-Path $root 'third_party\ffmpeg'
$ffVersion = (Get-Content (Join-Path $ff 'VERSION.txt') -Raw).Trim()
$bin = Join-Path $root 'build\bin'
$remote = "$RemoteProject/data/uploads/estudio"

function Entry($localPath, $name, $url) {
    $item = Get-Item $localPath
    [ordered]@{ name = $name; url = $url; sha256 = (Get-FileHash $localPath -Algorithm SHA256).Hash.ToLower(); size = $item.Length }
}

$toolDir = "tools/ffmpeg-$ffVersion"
$appDir = "app/$Version"
$files = @(
    @{ local = (Join-Path $ff 'ffmpeg.exe');  remote = "$toolDir/ffmpeg.exe" },
    @{ local = (Join-Path $ff 'ffprobe.exe'); remote = "$toolDir/ffprobe.exe" },
    @{ local = (Join-Path $ff 'LICENSE.txt'); remote = "$toolDir/LICENSE.txt" },
    @{ local = (Join-Path $bin 'club25-estudio.exe'); remote = "$appDir/club25-estudio.exe" }
)
# wails build -nsis lo nombra con el nombre del proyecto ("Club25 Estudio-amd64-installer.exe")
$installer = (Get-ChildItem $bin -Filter '*-amd64-installer.exe' -ErrorAction SilentlyContinue | Select-Object -First 1).FullName
if (-not $installer) { $installer = Join-Path $bin 'club25-estudio-amd64-installer.exe' }
if (Test-Path $installer) { $files += @{ local = $installer; remote = "$appDir/club25-estudio-amd64-installer.exe" } }

$manifest = [ordered]@{
    app_version = $Version
    installer   = if (Test-Path $installer) { Entry $installer 'club25-estudio-amd64-installer.exe' "/uploads/estudio/$appDir/club25-estudio-amd64-installer.exe" }
                  else { Entry (Join-Path $bin 'club25-estudio.exe') 'club25-estudio.exe' "/uploads/estudio/$appDir/club25-estudio.exe" }
    tools       = @([ordered]@{
        name = 'ffmpeg'; version = $ffVersion
        files = @(
            (Entry (Join-Path $ff 'ffmpeg.exe')  'ffmpeg.exe'  "/uploads/estudio/$toolDir/ffmpeg.exe"),
            (Entry (Join-Path $ff 'ffprobe.exe') 'ffprobe.exe' "/uploads/estudio/$toolDir/ffprobe.exe"),
            (Entry (Join-Path $ff 'LICENSE.txt') 'LICENSE.txt' "/uploads/estudio/$toolDir/LICENSE.txt")
        )
    })
    published_at = (Get-Date).ToUniversalTime().ToString('o')
}
$tmp = Join-Path ([IO.Path]::GetTempPath()) 'club25-manifest.json'
$manifest | ConvertTo-Json -Depth 6 | Set-Content -Path $tmp -Encoding utf8NoBOM

$target = "$User@$SshHost"
# ruta explícita: en algunas PC "ssh" resuelve a un archivo sin extensión que no hace nada
$ssh = Join-Path $env:windir 'System32\OpenSSH\ssh.exe'
$scp = Join-Path $env:windir 'System32\OpenSSH\scp.exe'
& $ssh -o BatchMode=yes -p $Port $target "mkdir -p '$remote/$toolDir' '$remote/$appDir'"
if ($LASTEXITCODE -ne 0) { throw "no se pudieron crear las carpetas en el servidor" }
foreach ($f in $files) {
    Write-Host "Subiendo $($f.remote) ..."
    & $scp -q -o BatchMode=yes -P $Port $f.local "${target}:$remote/$($f.remote)"
    if ($LASTEXITCODE -ne 0) { throw "scp falló: $($f.remote)" }
}
& $scp -q -o BatchMode=yes -P $Port $tmp "${target}:$remote/manifest.json"
Write-Host "Publicado: https://radioclub25peru.com/uploads/estudio/manifest.json"
