# comun.sh — funciones compartidas por los scripts de alta disponibilidad de RadioClub25.
# Se usa con `source` desde el Nodo A; no se ejecuta solo.
#
# Mismo acceso que los scripts de Kipu (repo kipu, scripts/ha): A llega a B
# directo por WireGuard con ~/.ssh/nodo_b y a C a través del VPS.

REPO_A="${REPO_A:-/home/lima/sehuacho/radioclub25}"

SSH_OPTS=(-o BatchMode=yes -o ConnectTimeout=10)
SSH_B=(ssh "${SSH_OPTS[@]}" -i "$HOME/.ssh/nodo_b" -p 1522 oyon@10.10.0.3)
SSH_VPS=(ssh "${SSH_OPTS[@]}" -i "$HOME/.ssh/vps_restaurante" ubuntu@10.10.0.1)
SSH_C_DESDE_VPS="ssh -o BatchMode=yes -o ConnectTimeout=10 -i ~/.ssh/id_ed25519 server3@10.10.0.4"

log() { echo "[$(date '+%Y-%m-%d %H:%M:%S')] $*"; }

ip_de()   { case "$1" in a) echo 10.10.0.2 ;; b) echo 10.10.0.3 ;; c) echo 10.10.0.4 ;; esac; }
repo_de() { case "$1" in a) echo "$REPO_A" ;; b) echo /home/oyon/sehuacho/radioclub25 ;; c) echo /home/server3/sehuacho/radioclub25 ;; esac; }

# Puertos públicos por nodo. B ya tiene otro MediaMTX en 1935/tcp, 8888/tcp y 8890/udp.
puertos_de() {
  case "$1" in
    b) echo "RTMP_PUBLIC_PORT=1945 SRT_PUBLIC_PORT=8900 HLS_LOCAL_PORT=8898" ;;
    *) echo "RTMP_PUBLIC_PORT=1935 SRT_PUBLIC_PORT=8890 HLS_LOCAL_PORT=8888" ;;
  esac
}

# remoto NODO "comando" — ejecuta un comando de shell en B o C.
remoto() {
  local nodo="$1"; shift
  case "$nodo" in
    b) "${SSH_B[@]}" "$*" ;;
    c) "${SSH_VPS[@]}" "$SSH_C_DESDE_VPS $(printf '%q' "$*")" ;;
    *) echo "nodo inválido: $nodo" >&2; return 2 ;;
  esac
}

# copiar NODO ORIGEN DESTINO_REMOTO [opciones de rsync…] — rsync hacia B o C.
copiar() {
  local nodo="$1" src="$2" dst="$3"; shift 3
  case "$nodo" in
    b) rsync "$@" -e "ssh ${SSH_OPTS[*]} -i $HOME/.ssh/nodo_b -p 1522" "$src" "oyon@10.10.0.3:$dst" ;;
    c) rsync "$@" -e "ssh ${SSH_OPTS[*]} -i $HOME/.ssh/vps_restaurante ubuntu@10.10.0.1 ssh -o BatchMode=yes -o ConnectTimeout=10 -i ~/.ssh/id_ed25519" \
         "$src" "server3@10.10.0.4:$dst" ;;
  esac
}

# stack_activo NODO — 0 si el contenedor app del nodo está corriendo (nodo promovido).
stack_activo() {
  local n
  n="$(remoto "$1" "docker ps -q --filter label=com.docker.compose.project=radioclub25 --filter label=com.docker.compose.service=app" 2>/dev/null)" || return 2
  [ -n "$n" ]
}
