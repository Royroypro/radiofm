# RadioClub25 en la infraestructura HA (VPS + nodos A, B y C)

Mismo esquema que el resto de sistemas del VPS (legado, Kipu): el VPS termina TLS
en HAProxy y reparte por WireGuard a los nodos.

```
Internet ── radioclub25peru.com (DNS → VPS 146.181.41.183)
   └─ VPS · HAProxy https_in · backend radioclub25_nodes · chequeo GET /healthz
        ├─ rc25_a 10.10.0.2:8125   PRINCIPAL: app + icecast + mediamtx + edge
        ├─ rc25_b 10.10.0.3:8125   reserva: stack PARADO, copia de data/ cada 5 min
        └─ rc25_c 10.10.0.4:8125   reserva: stack PARADO, copia de data/ cada 5 min

PC del estudio ── radioclub25.sehuacho.com (8.243.126.126 → Nodo A directo)
   ├─ Icecast 8010/tcp (audio)      └─ MediaMTX 1935/tcp RTMP · 8890/udp SRT (TV)
```

- **edge**: servicio `edge` del `docker-compose.yml` (nginx, `deploy/nginx/edge.conf`).
  Hace lo mismo que el vhost del nginx del host (web, `/radio*` con su puerta,
  `/hls/`) pero en un solo puerto y en la IP de WireGuard (`EDGE_BIND`). Así los
  tres nodos son idénticos sin tocar el nginx de B ni de C.
- **Datos**: todo está en `data/` (SQLite + `uploads/`). `sincronizar-nodos.sh`
  (cron en A) hace una copia consistente con `radioweb backup` y la deja en B y C;
  los archivos subidos solo se añaden, nunca se borran. RPO ≈ 5 min.
- **Reserva = parada**: B y C no corren la app, así que figuran DOWN en HAProxy y es
  lo correcto (dos apps con SQLite serían dos bases separadas).
- **Mismos secretos** en los tres `.env` (los copia `desplegar-nodos.sh`): las
  sesiones, los tokens del estudio y las claves de Icecast siguen valiendo tras un failover.
- **Ingesta directa a A**: la PC del estudio no pasa por el VPS (latencia y puertos
  de Oracle). Tras un failover la web vuelve sola; la emisión en vivo requiere
  cambiar el host en el estudio (ver abajo).
- Los servers de HAProxy se llaman `rc25_*`, no `nodo_*`: el panel del legado
  deshabilita todo server `nodo_<letra>` al desacoplar un nodo.
- `radioclub25.sehuacho.com` (nginx del host de A, certbot) sigue funcionando igual.

## Montaje (una vez, desde A)

```bash
cd /home/lima/sehuacho/radioclub25

# 1. Edge de A en su IP de WireGuard
grep -q '^EDGE_BIND=' .env || printf 'EDGE_BIND=10.10.0.2\nEDGE_PORT=8125\n' >> .env
docker compose up -d edge
curl -s http://10.10.0.2:8125/healthz          # ok

# 2. B y C en reserva (copia código y .env, construye imágenes, NO arranca, sincroniza)
scripts/ha/desplegar-nodos.sh

# 3. Cron de copia
(crontab -l; echo '*/5 * * * * /home/lima/sehuacho/radioclub25/scripts/ha/sincronizar-nodos.sh >> /home/lima/logs/radioclub25-sync.log 2>&1') | crontab -

# 4. VPS: certificado + HAProxy (repetible; copia previa y validación antes de recargar)
ssh vps-restaurante 'sudo bash -s' < scripts/ha/vps-radioclub25.sh

# 5. URL pública de la app
sed -i 's|^BASE_URL=.*|BASE_URL=https://radioclub25peru.com|' .env
docker compose up -d app && scripts/ha/desplegar-nodos.sh   # el .env nuevo llega a B y C
```

Puertos por nodo: B ya tiene otro MediaMTX en 1935/tcp, 8888/tcp y 8890/udp, así
que allí la radio usa RTMP 1945, SRT 8900/udp y HLS local 8898 (`comun.sh` → `puertos_de`).

## Panel HA (`/panel-ha` → RadioClub25)

El panel del VPS tiene un menú lateral con un apartado por sistema; el de
RadioClub25 muestra el rol de cada nodo, si su edge atiende, la antigüedad de la
última copia, lo que está al aire, y los botones:

| Botón | Qué hace |
|---|---|
| Verificar | solo lectura en A: código, contenedores, edge, última copia, disco, sintaxis de scripts |
| Sincronizar B y C ahora | `sincronizar-nodos.sh` |
| Redesplegar RadioClub25 (`RADIO`) | en A `docker compose up -d --build` (copia de trabajo, sin git pull) y luego `desplegar-nodos.sh` |
| Promover → Nodo B/C (`PROMOVER`) | `promover.sh` en ese nodo |

El parche del panel está en `panel-ha/` (ver su README).

## Failover (A cayó)

Desde el panel (**Promover RadioClub25 → Nodo B**) o en el nodo:

```bash
CONFIRMAR=si ~/sehuacho/radioclub25/scripts/ha/promover.sh
```

Arranca el stack con la última copia; en ~10 s HAProxy marca `rc25_b` UP. El
script se niega si A sigue sirviendo (`FORZAR=si` para saltarlo, sabiendo que
entonces habrá que detener A).

**Volver al aire**: en Club25 Estudio / BUTT / OBS cambiar el host al del nodo
promovido. B y C no tienen IP pública propia hoy: hace falta reenviar en su router
los puertos de ingesta (B: 8010/tcp, 1945/tcp, 8900/udp; C: 8010/tcp, 1935/tcp,
8890/udp). Mientras tanto la web, la programación y el archivo funcionan.

## Failback (A vuelve)

A tiene su base vieja y, si arranca, HAProxy la prefiere (es la principal). Antes
de levantar A hay que traer los datos del nodo promovido:

```bash
# en A, con su stack PARADO (docker compose stop)
source scripts/ha/comun.sh
remoto b "cd /home/oyon/sehuacho/radioclub25 && docker compose exec -T app radioweb backup /data/failback.db"
rsync -a -e "ssh -i ~/.ssh/nodo_b -p 1522" oyon@10.10.0.3:/home/oyon/sehuacho/radioclub25/data/failback.db data/radioclub25.db.failback
rsync -a -e "ssh -i ~/.ssh/nodo_b -p 1522" oyon@10.10.0.3:/home/oyon/sehuacho/radioclub25/data/uploads/ data/uploads/
mv data/radioclub25.db data/radioclub25.db.antes-failback && rm -f data/radioclub25.db-wal data/radioclub25.db-shm
mv data/radioclub25.db.failback data/radioclub25.db
remoto b "cd /home/oyon/sehuacho/radioclub25 && docker compose down && rm -f data/.ha-principal data/failback.db"
docker compose up -d
```

## Comprobar

```bash
curl -s https://radioclub25peru.com/healthz                  # ok
tail /home/lima/logs/radioclub25-sync.log                     # copias cada 5 min
ssh vps-restaurante 'echo "@1 show servers state radioclub25_nodes" | sudo socat stdio /run/haproxy-master.sock'
```
