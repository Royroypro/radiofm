<#
.SYNOPSIS
  Genera locuciones PROVISIONALES de hora y temperatura con la voz de Windows (SAPI).

.DESCRIPTION
  La hora y la temperatura locutadas se arman con archivos sueltos (ver
  internal/service/timeannounce):

    HRS00 … HRS23   "Son las tres" / "Es la una"           (hora, formato de 12 h)
    MIN00 … MIN59   "en punto" / "con quince minutos"
    TMP-10 … TMP45  "La temperatura en Huacho es de 22 grados"

  Sirven para probar mientras el locutor graba las definitivas. Para reemplazarlas
  basta copiar archivos con el mismo nombre en la carpeta de locuciones.

  Requiere una voz SAPI en español (Windows trae "Microsoft Helena" o "Sabina"; se
  instalan más en Configuración → Hora e idioma → Voz) y ffmpeg (scripts/get-ffmpeg.ps1
  o en el PATH) para convertir a MP3.

.EXAMPLE
  pwsh -File scripts/make-locutions.ps1
  pwsh -File scripts/make-locutions.ps1 -OutDir D:\Radio\Locuciones -City Huacho
#>
[CmdletBinding()]
param(
    [string]$OutDir,
    [string]$City = 'Huacho',
    [int]$Rate = 0   # velocidad de la voz: -10 (lenta) a 10 (rápida)
)

$ErrorActionPreference = 'Stop'
$root = Split-Path -Parent $PSScriptRoot
if (-not $OutDir) { $OutDir = Join-Path $root 'testdata\locutions' }
New-Item -ItemType Directory -Force -Path $OutDir | Out-Null

# ffmpeg: el de third_party o el del PATH.
$ffmpeg = Join-Path $root 'third_party\ffmpeg\ffmpeg.exe'
if (-not (Test-Path $ffmpeg)) {
    $cmd = Get-Command ffmpeg -ErrorAction SilentlyContinue
    if (-not $cmd) { throw 'No se encontró ffmpeg. Ejecute scripts/get-ffmpeg.ps1 primero.' }
    $ffmpeg = $cmd.Source
}

# Voz en español (idiomas SAPI: C0A = es-ES, 80A = es-MX, 2C0A = es-AR, 280A = es-PE...).
$voice = New-Object -ComObject SAPI.SpVoice
$spanish = $null
foreach ($v in $voice.GetVoices()) {
    $langs = $v.GetAttribute('Language') -split ';'
    if ($langs | Where-Object { $_ -match '^[0-9A-Fa-f]*0[Aa]$' }) { $spanish = $v; break }
}
if (-not $spanish) { throw 'No hay ninguna voz SAPI en español instalada.' }
$voice.Voice = $spanish
$voice.Rate = $Rate
Write-Host ("Voz: " + $spanish.GetDescription())

# --- Números en palabras (0–59) ---------------------------------------------
$units = @('cero','uno','dos','tres','cuatro','cinco','seis','siete','ocho','nueve','diez',
    'once','doce','trece','catorce','quince','dieciséis','diecisiete','dieciocho','diecinueve','veinte',
    'veintiuno','veintidós','veintitrés','veinticuatro','veinticinco','veintiséis','veintisiete','veintiocho','veintinueve')
$tens = @{ 3 = 'treinta'; 4 = 'cuarenta'; 5 = 'cincuenta' }
function Words([int]$n) {
    if ($n -lt 0) { return 'menos ' + (Words (-$n)) }
    if ($n -lt 30) { return $units[$n] }
    $t = [math]::Floor($n / 10); $u = $n % 10
    if ($u -eq 0) { return $tens[$t] }
    return $tens[$t] + ' y ' + $units[$u]
}

$tmp = Join-Path ([IO.Path]::GetTempPath()) ("club25-loc-" + [guid]::NewGuid())
New-Item -ItemType Directory -Path $tmp | Out-Null

function Say([string]$name, [string]$text) {
    $wav = Join-Path $tmp "$name.wav"
    $stream = New-Object -ComObject SAPI.SpFileStream
    $stream.Open($wav, 3)          # 3 = SSFMCreateForWrite
    $voice.AudioOutputStream = $stream
    [void]$voice.Speak($text)
    $stream.Close()
    $mp3 = Join-Path $OutDir "$name.mp3"
    & $ffmpeg -hide_banner -loglevel error -y -i $wav -ar 44100 -ac 2 -b:a 128k $mp3
    if ($LASTEXITCODE -ne 0) { throw "ffmpeg falló con $name" }
}

try {
    for ($h = 0; $h -le 23; $h++) {
        $h12 = $h % 12; if ($h12 -eq 0) { $h12 = 12 }
        # "una" (no "uno") para la hora: "Es la una".
        $text = if ($h12 -eq 1) { 'Es la una' } else { 'Son las ' + (Words $h12) }
        Say ('HRS{0:D2}' -f $h) $text
    }
    for ($m = 0; $m -le 59; $m++) {
        $w = (Words $m) -replace 'uno$', 'un'      # "veintiún minutos", "un minuto"
        $w = $w -replace 'veintiun$', 'veintiún'
        $text = switch ($m) { 0 { 'en punto' } 1 { 'con un minuto' } default { "con $w minutos" } }
        Say ('MIN{0:D2}' -f $m) $text
    }
    for ($t = -10; $t -le 45; $t++) {
        $unit = if ([math]::Abs($t) -eq 1) { 'grado' } else { 'grados' }
        $w = (Words $t) -replace 'uno$', 'un'
        Say ("TMP$t") "La temperatura en $City es de $w $unit"
    }
    Write-Host ("Listo: {0} archivos en {1}" -f (Get-ChildItem $OutDir -Filter *.mp3).Count, $OutDir)
}
finally {
    Remove-Item -Recurse -Force $tmp -ErrorAction SilentlyContinue
}
