#!/usr/bin/env bash
#
# vps-radioclub25.sh — publica radioclub25peru.com en el HAProxy del VPS.
#
# Se ejecuta EN EL VPS (o desde A: ssh vps-restaurante 'sudo bash -s' < scripts/ha/vps-radioclub25.sh).
# Repetible: si algo ya está hecho, lo salta.
#
#   1. Certificado Let's Encrypt (standalone en 127.0.0.1:8888, el backend acme_certbot
#      de HAProxy; igual que asistencia.sehuacho.com) para radioclub25peru.com y www.
#   2. PEM para HAProxy + caso en el hook de renovación.
#   3. HAProxy: backend radioclub25_nodes (A principal, B y C en espera), ACL por host
#      en https_in y redirección HTTP→HTTPS en http_in. Solo AÑADE bloques: el legado
#      y Kipu no cambian. Copia previa haproxy.cfg.bak-radioclub25-<fecha>; si
#      `haproxy -c` falla, se restaura y no se recarga.
#
# Los servers se llaman rc25_a/b/c (no nodo_*): el panel HA del legado deshabilita
# todo server nodo_<letra> al desacoplar un nodo y no debe arrastrar a la radio.
set -euo pipefail

DOM="radioclub25peru.com"
CFG="/etc/haproxy/haproxy.cfg"
PEM="/etc/haproxy/certs/${DOM}.pem"
HOOK="/etc/letsencrypt/renewal-hooks/deploy/update-haproxy-certificates.sh"
EDGE_PORT="${EDGE_PORT:-8125}"
STAMP="$(date +%Y%m%d-%H%M%S)"

[ "$(id -u)" = 0 ] || { echo "ejecutar con sudo"; exit 1; }
log() { echo "== $*"; }

# ── 1. Certificado ────────────────────────────────────────────────────────────
if [ ! -d "/etc/letsencrypt/live/${DOM}" ]; then
  log "certificado de ${DOM} (standalone :8888)"
  certbot certonly --standalone --http-01-port 8888 --non-interactive --agree-tos \
    --register-unsafely-without-email -d "${DOM}" -d "www.${DOM}"
else
  log "certificado ya existe"
fi

# ── 2. PEM + hook de renovación ──────────────────────────────────────────────
cat "/etc/letsencrypt/live/${DOM}/fullchain.pem" "/etc/letsencrypt/live/${DOM}/privkey.pem" > "${PEM}.new"
chmod 600 "${PEM}.new"; mv "${PEM}.new" "${PEM}"
if ! grep -q "live/${DOM})" "$HOOK"; then
  log "hook de renovación"
  cp -a "$HOOK" "${HOOK}.bak-radioclub25-${STAMP}"
  sed -i "/^  \*)$/i\\  /etc/letsencrypt/live/${DOM})\\n    TARGET=\"/etc/haproxy/certs/${DOM}.pem\"\\n    ;;" "$HOOK"
  bash -n "$HOOK"
fi

# ── 3. HAProxy ───────────────────────────────────────────────────────────────
cp -a "$CFG" "${CFG}.bak-radioclub25-${STAMP}"
TMP="$(mktemp)"; cp "$CFG" "$TMP"

if ! grep -q "crt ${PEM}" "$TMP"; then
  sed -i "s|^\(    bind \*:443 ssl .*\) alpn h2,http/1.1|\1 crt ${PEM} alpn h2,http/1.1|" "$TMP"
fi

if ! grep -q "is_radioclub25_http" "$TMP"; then
  # http_in: redirección a HTTPS (el reto ACME sigue yendo a certbot: va antes).
  sed -i "/^    use_backend acme_certbot if is_acme$/a\\
\\
    # RadioClub25 (radioclub25peru.com): siempre por HTTPS\\
    acl is_radioclub25_http hdr(host) -i ${DOM} www.${DOM}\\
    http-request redirect scheme https code 301 if is_radioclub25_http !is_acme" "$TMP"
fi

if ! grep -q "is_radioclub25 " "$TMP"; then
  # https_in: justo después de Kipu, antes de las reglas por ruta del legado
  # (/app, /api, /storage… existen también en la radio y no deben ir al legado).
  sed -i "/^    use_backend kipu_frontend_nodes if is_estacion24v1$/a\\
\\
    # RadioClub25 (repo radioclub25, scripts/ha): A principal, B y C en espera.\\
    # www → dominio sin www. El audio (/radio*) es un stream largo: timeouts propios.\\
    acl is_radioclub25 hdr(host) -i ${DOM} www.${DOM}\\
    http-request redirect prefix https://${DOM} code 301 if { hdr(host) -i www.${DOM} }\\
    use_backend radioclub25_nodes if is_radioclub25" "$TMP"
fi

if ! grep -q "^backend radioclub25_nodes" "$TMP"; then
  # Backend antes de la sección de reverb del legado.
  sed -i "/^backend laravel_nodes$/i\\
backend radioclub25_nodes\\
    # nginx \"edge\" del compose de radioclub25 en la IP de WireGuard de cada nodo.\\
    # B y C tienen el stack PARADO (copia de datos cada 5 min desde A): figuran\\
    # DOWN hasta que se ejecuta scripts/ha/promover.sh en ellos.\\
    option  httpchk GET /healthz\\
    http-check expect status 200\\
    timeout server  24h\\
    timeout tunnel  24h\\
    server rc25_a 10.10.0.2:${EDGE_PORT} check inter 5s fall 3 rise 2\\
    server rc25_b 10.10.0.3:${EDGE_PORT} check inter 5s fall 3 rise 2 backup\\
    server rc25_c 10.10.0.4:${EDGE_PORT} check inter 5s fall 3 rise 2 backup\\
" "$TMP"
fi

if haproxy -c -f "$TMP" >/dev/null; then
  cp "$TMP" "$CFG"; rm -f "$TMP"
  systemctl reload haproxy
  log "HAProxy recargado (copia: ${CFG}.bak-radioclub25-${STAMP})"
else
  haproxy -c -f "$TMP" || true
  rm -f "$TMP"
  echo "ERROR: la configuración nueva no valida; haproxy.cfg NO se tocó"; exit 1
fi

log "comprobación"
sleep 3
echo "@1 show servers state radioclub25_nodes" | socat stdio /run/haproxy-master.sock 2>/dev/null | awk 'NR>2{print $4, ($6==2?"UP":"DOWN")}' || true
curl -s -o /dev/null -w "https://${DOM}/healthz → %{http_code}\n" --resolve "${DOM}:443:127.0.0.1" "https://${DOM}/healthz"
