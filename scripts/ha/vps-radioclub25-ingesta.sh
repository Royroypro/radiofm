#!/usr/bin/env bash
#
# vps-radioclub25-ingesta.sh — la ingesta del estudio también entra por radioclub25peru.com.
#
# Se ejecuta EN EL VPS:  ssh vps-restaurante 'sudo bash -s' < scripts/ha/vps-radioclub25-ingesta.sh
# Repetible. Sin esto el estudio depende de radioclub25.sehuacho.com (IP pública de A).
#
#   8010/tcp Icecast (audio, BUTT / Club25 Estudio)  → HAProxy mode tcp → A:8010 (B/C reserva)
#   1935/tcp RTMP (TV, OBS)                           → HAProxy mode tcp → A:1935 (B:1945, C:1935)
#   8890/udp SRT  (TV, OBS)                           → iptables DNAT    → A:8890 (HAProxy no hace UDP;
#                                                       tras un failover se repunta a mano, ver README)
#
# La red de Oracle ya deja pasar estos puertos (probado: el REJECT lo pone el iptables del VPS).
# HAProxy: copia previa y validación antes de recargar, como vps-radioclub25.sh.
# iptables: reglas marcadas "radioclub25" y guardadas con netfilter-persistent.
set -euo pipefail
[ "$(id -u)" = 0 ] || { echo "ejecutar con sudo"; exit 1; }

CFG="/etc/haproxy/haproxy.cfg"
STAMP="$(date +%Y%m%d-%H%M%S)"
WAN="$(ip route show default | awk '{print $5; exit}')"
A_WG="10.10.0.2"
log() { echo "== $*"; }

# ── HAProxy: audio y RTMP ─────────────────────────────────────────────────────
if ! grep -q "^listen radioclub25_icecast" "$CFG"; then
  cp -a "$CFG" "${CFG}.bak-rc25ingesta-${STAMP}"
  TMP="$(mktemp)"; cp "$CFG" "$TMP"
  cat >> "$TMP" <<'EOF'

# ── RadioClub25: ingesta del estudio (repo radioclub25, scripts/ha) ──────────
# El estudio emite a radioclub25peru.com; se pasa tal cual (TCP) al nodo activo.
# B y C tienen el stack parado: DOWN hasta que se promueven (como radioclub25_nodes).
listen radioclub25_icecast
    bind *:8010
    mode tcp
    option tcplog
    timeout client 24h
    timeout server 24h
    server rc25_a 10.10.0.2:8010 check inter 5s fall 3 rise 2
    server rc25_b 10.10.0.3:8010 check inter 5s fall 3 rise 2 backup
    server rc25_c 10.10.0.4:8010 check inter 5s fall 3 rise 2 backup

listen radioclub25_rtmp
    bind *:1935
    mode tcp
    option tcplog
    timeout client 24h
    timeout server 24h
    server rc25_a 10.10.0.2:1935 check inter 5s fall 3 rise 2
    server rc25_b 10.10.0.3:1945 check inter 5s fall 3 rise 2 backup
    server rc25_c 10.10.0.4:1935 check inter 5s fall 3 rise 2 backup
EOF
  if haproxy -c -f "$TMP" >/dev/null; then
    cp "$TMP" "$CFG"; rm -f "$TMP"; systemctl reload haproxy
    log "HAProxy recargado (copia: ${CFG}.bak-rc25ingesta-${STAMP})"
  else
    haproxy -c -f "$TMP" || true; rm -f "$TMP"
    echo "ERROR: la configuración no valida; haproxy.cfg NO se tocó"; exit 1
  fi
else
  log "HAProxy ya tiene la ingesta"
fi

# ── iptables ──────────────────────────────────────────────────────────────────
add() { local t="$1"; shift; iptables -t "$t" -C "$@" 2>/dev/null || iptables -t "$t" -I "$@"; }
# Entrada a HAProxy (antes del REJECT final de INPUT).
add filter INPUT -p tcp --dport 8010 -m comment --comment radioclub25 -j ACCEPT
add filter INPUT -p tcp --dport 1935 -m comment --comment radioclub25 -j ACCEPT
# SRT: DNAT a A por WireGuard; MASQUERADE para que la respuesta vuelva por el VPS.
add nat PREROUTING -i "$WAN" -p udp --dport 8890 -m comment --comment radioclub25 -j DNAT --to-destination "${A_WG}:8890"
add nat POSTROUTING -o wg0 -p udp -d "$A_WG" --dport 8890 -m comment --comment radioclub25 -j MASQUERADE
add filter FORWARD -i "$WAN" -o wg0 -p udp -d "$A_WG" --dport 8890 -m comment --comment radioclub25 -j ACCEPT
add filter FORWARD -i wg0 -o "$WAN" -p udp -s "$A_WG" --sport 8890 -m conntrack --ctstate RELATED,ESTABLISHED -m comment --comment radioclub25 -j ACCEPT
netfilter-persistent save >/dev/null 2>&1 && log "iptables guardado"

log "comprobación"
ss -ltn | grep -E ':(8010|1935) ' || echo "AVISO: HAProxy no escucha en 8010/1935"
echo "@1 show servers state" | socat stdio /run/haproxy-master.sock 2>/dev/null \
  | awk '$2 ~ /radioclub25_(icecast|rtmp)/ {print $2, $4, ($6==2?"UP":"DOWN")}' || true
