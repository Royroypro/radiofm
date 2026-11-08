<#
.SYNOPSIS
  Ejecuta un comando con el toolchain de Club25 Estudio en el PATH (gcc de llvm-mingw, Wails, Task).

.DESCRIPTION
  malgo (miniaudio) necesita CGO, y Wails compila con CGO al generar los bindings. Este envoltorio
  añade llvm-mingw y %GOPATH%\bin al PATH solo para ese comando y activa CGO, así funciona aunque
  la terminal no tenga el PATH actualizado. Lo usa Taskfile.yml.

.EXAMPLE
  pwsh -File scripts/with-toolchain.ps1 go test ./...
  pwsh -File scripts/with-toolchain.ps1 wails build -clean
#>
param([Parameter(ValueFromRemainingArguments = $true)][string[]]$Command)

$ErrorActionPreference = 'Stop'
if (-not $Command) { throw 'Uso: with-toolchain.ps1 <comando> [argumentos...]' }

# Debe coincidir con $LlvmTag de scripts/dev-setup.ps1.
$LlvmTag = '20261104'
$llvm = Join-Path $env:LOCALAPPDATA "Programs\llvm-mingw-$LlvmTag-ucrt-x86_64\bin"
if (-not (Test-Path (Join-Path $llvm 'gcc.exe'))) {
    throw "No se encontró gcc en $llvm. Ejecute scripts/dev-setup.ps1 (o task setup)."
}
$gobin = Join-Path (go env GOPATH) 'bin'
$env:Path = "$llvm;$gobin;$env:Path"
# NSIS (makensis) para el instalador, si está instalado
foreach ($n in @("$env:LOCALAPPDATA\Programs\nsis-3.11", "${env:ProgramFiles(x86)}\NSIS", "$env:ProgramFiles\NSIS")) {
    if (Test-Path (Join-Path $n 'makensis.exe')) { $env:Path = "$n;$env:Path"; break }
}
$env:CGO_ENABLED = '1'

$exe = $Command[0]
$rest = @($Command | Select-Object -Skip 1)   # siempre un array (con un solo argumento también)
& $exe @rest
exit $LASTEXITCODE
