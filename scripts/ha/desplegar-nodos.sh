#!/usr/bin/env bash
#
# desplegar-nodos.sh — prepara B y C como reserva de RadioClub25 (se ejecuta en A).
#
# Para cada nodo:
#   1. copia el código de A (sin data/, backups/, estudio/, .git ni .env);
#   2. escribe su .env: el de A (mismos secretos, sesiones válidas tras un failover)
#      con EDGE_BIND = su IP de WireGuard, sus puertos y su UID;
#   3. construye/descarga las imágenes, SIN arrancar el stack (queda en espera:
#      HAProxy lo ve DOWN y no le manda tráfico);
#   4. llama a sincronizar-nodos.sh para dejarle la base y los archivos al día.
#
# Si el nodo está PROMOVIDO (su app corre) no se toca: sería pisar la principal.
# Repetible. Uso: scripts/ha/desplegar-nodos.sh [b] [c]
set -euo pipefail
source "$(dirname "$0")/comun.sh"

NODOS=("$@"); [ ${#NODOS[@]} -gt 0 ] || NODOS=(b c)
cd "$REPO_A"

for nodo in "${NODOS[@]}"; do
  ip="$(ip_de "$nodo")"; repo="$(repo_de "$nodo")"
  log "== Nodo ${nodo^^} ($ip) → $repo"

  if stack_activo "$nodo"; then
    log "el stack de ${nodo^^} está corriendo (¿promovido?): no se toca"; continue
  fi

  remoto "$nodo" "mkdir -p $repo/data/uploads"
  copiar "$nodo" "$REPO_A/" "$repo/" -az --delete \
    --exclude=/data --exclude=/backups --exclude=/bin --exclude=/estudio --exclude=/.git \
    --exclude=/imagenes_referecia --exclude=/.env --exclude='*.bak*'

  uid="$(remoto "$nodo" 'id -u')"; gid="$(remoto "$nodo" 'id -g')"
  tmp="$(mktemp)"
  grep -vE '^(EDGE_BIND|EDGE_PORT|APP_UID|APP_GID|RTMP_PUBLIC_PORT|SRT_PUBLIC_PORT|HLS_LOCAL_PORT|SUPERADMIN_EMAIL|SUPERADMIN_PASSWORD|CLAVE_TEMP)=' .env > "$tmp"
  {
    echo
    echo "# --- Nodo ${nodo^^} (escrito por scripts/ha/desplegar-nodos.sh desde A) ---"
    echo "EDGE_BIND=$ip"
    echo "EDGE_PORT=8125"
    echo "APP_UID=$uid"
    echo "APP_GID=$gid"
    for kv in $(puertos_de "$nodo"); do echo "$kv"; done
  } >> "$tmp"
  copiar "$nodo" "$tmp" "$repo/.env" -a --chmod=F600
  rm -f "$tmp"

  log "imágenes en ${nodo^^} (build de la app; puede tardar)"
  remoto "$nodo" "cd $repo && docker compose build -q app && docker compose pull -q mediamtx icecast edge"
done

"$(dirname "$0")/sincronizar-nodos.sh" "${NODOS[@]}"
log "listo: B y C en espera (stack parado)."
