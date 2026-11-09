#!/usr/bin/env bash
#
# promover.sh — convierte este nodo (B o C) en la principal de RadioClub25.
#
# Se ejecuta EN EL NODO (o desde el panel HA, botón "Promover"). Arranca el stack con
# la última copia de A; en ~10 s HAProxy marca el edge del nodo UP y la web vuelve.
#
# ANTES: que A no esté sirviendo (caído, o `docker compose stop` en A). Si A sigue
# viva, HAProxy le seguiría mandando el tráfico a ella (es la principal) y habría dos
# bases separadas. Con A viva el script se niega salvo FORZAR=si.
#
# La PC del estudio emite a INGEST_HOST (A). Tras promover, para que la radio y la TV
# vuelvan en vivo hay que cambiar en el estudio el host a la IP pública del nodo y
# sus puertos (ver scripts/ha/README.md). La web, la programación y el archivo
# funcionan sin eso.
#
# Uso: CONFIRMAR=si scripts/ha/promover.sh
set -euo pipefail
cd "$(dirname "$0")/../.."

[ "${CONFIRMAR:-}" = "si" ] || { echo "Uso: CONFIRMAR=si $0"; exit 1; }
[ -f .env ] && [ -f data/radioclub25.db ] || { echo "falta .env o data/radioclub25.db (¿se ejecutó desplegar-nodos.sh?)"; exit 1; }

if curl -s -o /dev/null -m 4 -w '%{http_code}' http://10.10.0.2:8125/healthz | grep -q 200 && [ "${FORZAR:-}" != "si" ]; then
  echo "A (10.10.0.2) sigue sirviendo RadioClub25: deténgala primero o use FORZAR=si."; exit 1
fi

echo "== última copia recibida de A: $(cat data/.ha-sync 2>/dev/null || echo desconocida)"
date -Is > data/.ha-principal
docker compose up -d
sleep 5
curl -s -o /dev/null -w "== edge local /healthz → %{http_code}\n" "http://$(grep '^EDGE_BIND=' .env | cut -d= -f2):8125/healthz"
echo "== promovido. Volver a A: ver 'Failback' en scripts/ha/README.md"
