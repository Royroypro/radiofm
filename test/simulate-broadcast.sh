#!/usr/bin/env bash
# =====================================================================
# Simula la PC emisora con ffmpeg: envía un tono de audio a Icecast (radio)
# y una barra de colores con tono a MediaMTX (TV), por separado.
#
# Uso:
#   test/simulate-broadcast.sh [radio|tv|ambos] [segundos]
# Variables (se leen de .env si existe):
#   INGEST_HOST (localhost), ICECAST_PUBLIC_PORT (8010), ICECAST_SOURCE_PASSWORD,
#   RTMP_PUBLIC_PORT (1935), TV_USER (emisora), TV_PASS (clave de Admin → Streams)
# =====================================================================
set -euo pipefail

cd "$(dirname "$0")/.."
if [[ -f .env ]]; then
  set -a; source <(grep -E '^[A-Z_]+=' .env | sed 's/[[:space:]]*#.*$//'); set +a
fi

MODE="${1:-ambos}"
DURATION="${2:-60}"
HOST="${SIM_HOST:-localhost}"
ICE_PORT="${ICECAST_PUBLIC_PORT:-8010}"
ICE_PASS="${ICECAST_SOURCE_PASSWORD:?Defina ICECAST_SOURCE_PASSWORD}"
RTMP_PORT="${RTMP_PUBLIC_PORT:-1935}"
TV_USER="${TV_USER:-emisora}"
TV_PASS="${TV_PASS:?Defina TV_PASS (Admin → Streams y emisión → clave de retransmisión)}"

radio() {
  echo "→ Radio: tono 440 Hz a icecast://$HOST:$ICE_PORT/radio durante ${DURATION}s"
  ffmpeg -hide_banner -loglevel warning -re -f lavfi -i "sine=frequency=440:sample_rate=44100" -t "$DURATION" \
    -ac 2 -c:a libmp3lame -b:a 128k -content_type audio/mpeg \
    -ice_name "Prueba RadioClub25" -metadata title="Prueba - Tono 440 Hz" \
    -f mp3 "icecast://source:${ICE_PASS}@${HOST}:${ICE_PORT}/radio"
}

tv() {
  echo "→ TV: barras de color + tono a rtmp://$HOST:$RTMP_PORT/tv durante ${DURATION}s"
  ffmpeg -hide_banner -loglevel warning -re \
    -f lavfi -i "testsrc2=size=1280x720:rate=30" -f lavfi -i "sine=frequency=660:sample_rate=44100" -t "$DURATION" \
    -c:v libx264 -preset veryfast -tune zerolatency -b:v 2500k -g 60 -pix_fmt yuv420p \
    -c:a aac -b:a 128k -ar 44100 \
    -f flv "rtmp://${HOST}:${RTMP_PORT}/tv?user=${TV_USER}&pass=${TV_PASS}"
}

case "$MODE" in
  radio) radio ;;
  tv) tv ;;
  ambos) radio & tv & wait ;;
  *) echo "Modo desconocido: $MODE (use radio, tv o ambos)"; exit 1 ;;
esac
