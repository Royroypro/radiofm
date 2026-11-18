#!/usr/bin/env bash
#
# recargar-streams.sh — aplica cambios de configuración de Icecast, MediaMTX o el
# edge (nginx) SIN cortar la radio, la TV ni a los oyentes. Se ejecuta en el
# nodo, en la carpeta del proyecto, después de editar deploy/*.
#
#   scripts/recargar-streams.sh            recarga los tres
#   scripts/recargar-streams.sh icecast    solo Icecast (deploy/icecast.xml)
#   scripts/recargar-streams.sh edge       solo nginx (deploy/nginx/edge.conf)
#   scripts/recargar-streams.sh mediamtx   solo MediaMTX (deploy/mediamtx.yml)
#
# Ojo: deploy/*.xml|.conf|.yml están montados como ARCHIVOS sueltos. Cuando git
# pull (o una copia) los reemplaza, el contenedor sigue viendo el archivo viejo.
# Por eso a Icecast se le pasa el contenido del repositorio por la entrada, y
# para nginx y MediaMTX se compara antes: si difieren, avisa (hace falta
# `docker compose up -d --force-recreate <servicio>`, que sí corta unos segundos).
#
# Cómo lo hace:
#   - Icecast: vuelve a armar /etc/icecast.xml igual que al arrancar (plantilla +
#     token interno + contraseñas del .env con el docker-entrypoint de la imagen)
#     y le manda HUP: relee la configuración (límites, montajes, autenticación) y
#     la PC emisora y los oyentes siguen conectados. Lo que Icecast NO cambia con
#     HUP (los puertos de <listen-socket>) sí necesita reiniciar el contenedor.
#   - edge (nginx): `nginx -t` y `nginx -s reload` (recarga ordenada).
#   - MediaMTX: relee solo su archivo cuando cambia (no hace falta nada); aquí
#     solo se confirma que el archivo montado es el del repositorio.
#
# Si la configuración nueva tiene un error, Icecast conserva la anterior y nginx
# no recarga: nada se corta. Revise el registro con `docker compose logs icecast`.
set -euo pipefail
cd "$(dirname "$0")/.."

que="${1:-todo}"

recargar_icecast() {
  echo "== Icecast: rearmar /etc/icecast.xml y recargar (HUP)"
  # El contenido sale de deploy/icecast.xml del repositorio (por la entrada), no del
  # archivo montado, que puede ser el viejo (ver arriba).
  docker compose exec -T icecast sh -c '
    set -e
    # El contenedor corre como el usuario icecast: no puede crear archivos en /etc,
    # pero /etc/icecast.xml es suyo. Se arma en /tmp, se revisa y se sobrescribe.
    sed "s|@INTERNAL_TOKEN@|$INTERNAL_TOKEN|g" > /tmp/icecast.xml.nuevo
    grep -q "</icecast>" /tmp/icecast.xml.nuevo || { echo "configuración incompleta: no se toca nada" >&2; exit 1; }
    cp /etc/icecast.xml /tmp/icecast.xml.anterior
    cat /tmp/icecast.xml.nuevo > /etc/icecast.xml
    docker-entrypoint true          # escribe las contraseñas del .env, como al arrancar
    kill -HUP 1                     # Icecast es el proceso 1 del contenedor
  ' < deploy/icecast.xml
  echo "   listo: Icecast releyó la configuración sin soltar la fuente ni a los oyentes"
}

igual_que_repo() { # servicio ruta-en-contenedor archivo-del-repo
  local dentro fuera
  dentro="$(docker compose exec -T "$1" sha256sum "$2" 2>/dev/null | cut -d" " -f1)"
  fuera="$(sha256sum "$3" | cut -d" " -f1)"
  [ -n "$dentro" ] && [ "$dentro" = "$fuera" ]
}

recargar_edge() {
  echo "== edge (nginx): probar y recargar"
  if ! igual_que_repo edge /etc/nginx/conf.d/default.conf deploy/nginx/edge.conf; then
    echo "   el contenedor ve otra versión de edge.conf (archivo reemplazado): hace falta"
    echo "   docker compose up -d --force-recreate edge   (corta la web y los oyentes unos segundos)"
    return 1
  fi
  docker compose exec -T edge nginx -t
  docker compose exec -T edge nginx -s reload
}

revisar_mediamtx() {
  echo "== MediaMTX: relee solo su configuración cuando el archivo montado cambia"
  echo "   (la imagen no trae shell: si deploy/mediamtx.yml se reemplazó en lugar de editarse,"
  echo "    hace falta docker compose up -d --force-recreate mediamtx, que corta la TV unos segundos)"
}

case "$que" in
  icecast)  recargar_icecast ;;
  edge)     recargar_edge ;;
  mediamtx) revisar_mediamtx ;;
  todo)     recargar_icecast; recargar_edge; revisar_mediamtx ;;
  *) echo "uso: $0 [icecast|edge|mediamtx]"; exit 2 ;;
esac
