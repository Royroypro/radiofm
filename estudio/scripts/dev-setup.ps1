<#
.SYNOPSIS
  Prepara una PC Windows para desarrollar Club25 Estudio (sin permisos de administrador).

.DESCRIPTION
  Comprueba e instala lo que falte (ver docs/setup-windows.md para el detalle):
    1. Go 1.27+ y Node 22.12+   → solo se comprueban (instálelos usted si faltan).
    2. Wails CLI v2 y Task      → go install.
    3. gcc (llvm-mingw)         → zip oficial de GitHub, verificado por SHA-256,
                                  en %LOCALAPPDATA%\Programs. malgo (miniaudio) necesita CGO.
    4. PATH del usuario         → añade llvm-mingw\bin y %GOPATH%\bin.
    5. ffmpeg                   → scripts/get-ffmpeg.ps1.
    6. Dependencias del frontend → npm install.
  Al final ejecuta `wails doctor`.

.EXAMPLE
  pwsh -File scripts/dev-setup.ps1
#>
[CmdletBinding()]
param()

$ErrorActionPreference = 'Stop'
$ProgressPreference = 'SilentlyContinue'
$root = Split-Path -Parent $PSScriptRoot

# --- Versiones fijadas ------------------------------------------------------
$WailsVersion = 'v2.17.1'
$TaskVersion  = 'v3.54.0'
$LlvmTag      = '20261104'
$LlvmZip      = "llvm-mingw-$LlvmTag-ucrt-x86_64.zip"
$LlvmSha256   = '552191b5754c49a5a20723276a6fc56541ea1922c4c50b7b669022333fc1b40f'
# ----------------------------------------------------------------------------

function Need([string]$cmd, [string]$hint) {
    if (-not (Get-Command $cmd -ErrorAction SilentlyContinue)) { throw "Falta $cmd. $hint" }
}

Need go   'Instale Go 1.27+ desde https://go.dev/dl/'
Need node 'Instale Node.js LTS (22.12 o superior) desde https://nodejs.org/'
Write-Host (go version)
Write-Host ("node " + (node --version))

$gobin = Join-Path (go env GOPATH) 'bin'
$llvmDir = Join-Path $env:LOCALAPPDATA "Programs\llvm-mingw-$LlvmTag-ucrt-x86_64"
$llvmBin = Join-Path $llvmDir 'bin'

# Wails y Task
if (-not (Test-Path (Join-Path $gobin 'wails.exe'))) {
    Write-Host "Instalando Wails $WailsVersion ..."
    go install "github.com/wailsapp/wails/v2/cmd/wails@$WailsVersion"
}
if (-not (Test-Path (Join-Path $gobin 'task.exe'))) {
    Write-Host "Instalando Task $TaskVersion ..."
    go install "github.com/go-task/task/v3/cmd/task@$TaskVersion"
}

# gcc (llvm-mingw)
if (-not (Test-Path (Join-Path $llvmBin 'gcc.exe'))) {
    $tmp = Join-Path ([IO.Path]::GetTempPath()) ("club25-llvm-" + [guid]::NewGuid())
    New-Item -ItemType Directory -Path $tmp | Out-Null
    try {
        $zip = Join-Path $tmp $LlvmZip
        Write-Host "Descargando $LlvmZip (~190 MB) ..."
        Invoke-WebRequest -UseBasicParsing -OutFile $zip `
            -Uri "https://github.com/mstorsjo/llvm-mingw/releases/download/$LlvmTag/$LlvmZip"
        $hash = (Get-FileHash -Algorithm SHA256 $zip).Hash.ToLower()
        if ($hash -ne $LlvmSha256) { throw "SHA-256 de llvm-mingw no coincide ($hash)." }
        Expand-Archive -Path $zip -DestinationPath (Join-Path $env:LOCALAPPDATA 'Programs') -Force
    }
    finally { Remove-Item -Recurse -Force $tmp -ErrorAction SilentlyContinue }
}

# PATH del usuario (persistente) y de esta sesión
$userPath = [Environment]::GetEnvironmentVariable('Path', 'User')
foreach ($p in @($llvmBin, $gobin)) {
    if ($userPath -notlike "*$p*") { $userPath = "$userPath;$p"; Write-Host "PATH += $p" }
    if ($env:Path -notlike "*$p*") { $env:Path = "$env:Path;$p" }
}
[Environment]::SetEnvironmentVariable('Path', $userPath, 'User')

# ffmpeg y frontend
& (Join-Path $PSScriptRoot 'get-ffmpeg.ps1')
Push-Location (Join-Path $root 'frontend')
try { npm install } finally { Pop-Location }

wails doctor
Write-Host "`nListo. Abra una terminal nueva (para el PATH) y ejecute: task dev"
