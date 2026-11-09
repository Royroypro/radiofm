#!/usr/bin/env bash
#
# sincronizar-nodos.sh — cron en el Nodo A, cada 5 minutos.
#
# RadioClub25 guarda todo en data/: la base SQLite y los archivos subidos. Esto deja
# en B y C una copia con un RPO de ~5 min:
#   - base: copia consistente con `radioweb backup` (VACUUM INTO), sin parar la app,
#     y se reemplaza entera en el nodo (se borran sus -wal/-shm viejos);
#   - uploads/: rsync que solo AÑADE y actualiza (no borra), como el mirror del legado.
#
# Nunca escribe en un nodo cuyo stack corre: ese nodo fue promovido y su base es la
# buena. Al terminar deja data/.ha-sync con la fecha (el panel HA la muestra).
#
#   */5 * * * * /home/lima/sehuacho/radioclub25/scripts/ha/sincronizar-nodos.sh >> /home/lima/logs/radioclub25-sync.log 2>&1
set -euo pipefail
source "$(dirname "$0")/comun.sh"

NODOS=("$@"); [ ${#NODOS[@]} -gt 0 ] || NODOS=(b c)
cd "$REPO_A"

exec 9>/tmp/radioclub25-ha-sync.lock
flock -n 9 || { log "otra sincronización en curso"; exit 0; }

# A tiene que ser la principal: si su app no corre, no hay nada bueno que copiar.
if [ -z "$(docker compose ps -q app 2>/dev/null)" ]; then
  log "la app no corre en A: no se sincroniza"; exit 0
fi

SNAP="data/ha-sync.db"
rm -f "$SNAP"
docker compose exec -T app radioweb backup /data/ha-sync.db >/dev/null
trap 'rm -f "$REPO_A/$SNAP"' EXIT

for nodo in "${NODOS[@]}"; do
  repo="$(repo_de "$nodo")"
  if stack_activo "$nodo"; then
    log "${nodo^^}: su stack corre (promovido): no se pisa"; continue
  fi
  if ! remoto "$nodo" "test -d $repo/data"; then
    log "${nodo^^}: sin $repo/data (falta desplegar-nodos.sh)"; continue
  fi
  copiar "$nodo" "$SNAP" "$repo/data/.radioclub25.db.nuevo" -a
  remoto "$nodo" "cd $repo/data && rm -f radioclub25.db-wal radioclub25.db-shm && mv .radioclub25.db.nuevo radioclub25.db && date -Is > .ha-sync"
  copiar "$nodo" "data/uploads/" "$repo/data/uploads/" -a
  log "${nodo^^}: base y archivos al día"
done
