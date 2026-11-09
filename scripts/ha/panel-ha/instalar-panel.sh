#!/usr/bin/env bash
# instalar-panel.sh — instala app.py e index.html de esta carpeta en el panel HA del VPS.
# Se ejecuta desde A. Copia previa, prueba de compilación y vuelta atrás si no arranca.
set -euo pipefail
cd "$(dirname "$0")"
VPS=(ssh -o BatchMode=yes -i "$HOME/.ssh/vps_restaurante" ubuntu@10.10.0.1)
STAMP="$(date +%Y%m%d-%H%M%S)"

scp -q -i "$HOME/.ssh/vps_restaurante" app.py index.html ubuntu@10.10.0.1:/tmp/
"${VPS[@]}" "set -e
  python3 -m py_compile /tmp/app.py
  cd /opt/ha-panel
  sudo cp -a backend/app.py backend/app.py.bak-radioclub25-$STAMP
  sudo cp -a frontend/index.html frontend/index.html.bak-radioclub25-$STAMP
  sudo install -m 644 /tmp/app.py backend/app.py
  sudo install -m 644 /tmp/index.html frontend/index.html
  sudo systemctl restart ha-panel
  sleep 3
  if curl -sf -o /dev/null http://127.0.0.1:9099/panel-ha/api/overview; then
    echo 'panel OK (copias: *.bak-radioclub25-$STAMP)'
  else
    echo 'el panel no responde: restaurando'
    sudo cp -a backend/app.py.bak-radioclub25-$STAMP backend/app.py
    sudo cp -a frontend/index.html.bak-radioclub25-$STAMP frontend/index.html
    sudo systemctl restart ha-panel; exit 1
  fi
  rm -f /tmp/app.py /tmp/index.html"
