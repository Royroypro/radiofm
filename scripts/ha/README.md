# RadioClub25 en la infraestructura HA (VPS + nodos A, B y C)

Mismo esquema que el resto de sistemas del VPS (legado, Kipu): el VPS termina TLS
en HAProxy y reparte por WireGuard a los nodos.

```
Internet ── radioclub25peru.com (DNS → VPS 146.181.41.183)
   └─ VPS · HAProxy https_in · backend radioclub25_nodes · chequeo GET /healthz
        ├─ rc25_a 10.10.0.2:8125   PRINCIPAL: app + icecast + mediamtx + edge
        ├─ rc25_b 10.10.0.3:8125   reserva: stack PARADO, copia de data/ cada 5 min
        └─ rc25_c 10.10.0.4:8125   reserva: stack PARADO, copia de data/ cada 5 min

PC del estudio ── radioclub25peru.com (el mismo VPS)
   ├─ 8010/tcp Icecast (audio) ─ HAProxy listen radioclub25_icecast (tcp) → A:8010 (B/C reserva)
   ├─ 1935/tcp RTMP (TV, OBS)  ─ HAProxy listen radioclub25_rtmp (tcp)    → A:1935 (B:1945, C:1935)
   └─ 8890/udp SRT (TV, OBS)   ─ iptables DNAT del VPS                    → A:8890
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
- **Ingesta por el VPS**: la PC del estudio emite a `radioclub25peru.com`
  (`INGEST_HOST`; el panel se lo envía al Estudio, que lo guarda solo). Audio y RTMP
  siguen solos al nodo activo tras un failover; el SRT (UDP) hay que repuntarlo.
  Se monta con `vps-radioclub25-ingesta.sh`. La red de Oracle ya deja pasar los puertos.
- Los servers de HAProxy se llaman `rc25_*`, no `nodo_*`: el panel del legado
  deshabilita todo server `nodo_<letra>` al desacoplar un nodo.
- `radioclub25.sehuacho.com` (subdominio anterior, nginx del host de A) **se desactiva**:
  nada depende de él (web, API del estudio, ingesta y archivos van por el dominio nuevo).
  Solo queda como email del superadmin inicial, que es un identificador, no tráfico.

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

# 5. VPS: ingesta del estudio (8010/tcp, 1935/tcp, 8890/udp) por el dominio
ssh vps-restaurante 'sudo bash -s' < scripts/ha/vps-radioclub25-ingesta.sh

# 6. URL pública e ingesta de la app (INGEST_HOST solo DESPUÉS del paso 5)
sed -i 's|^BASE_URL=.*|BASE_URL=https://radioclub25peru.com|; s|^INGEST_HOST=.*|INGEST_HOST=radioclub25peru.com|' .env
docker compose up -d app icecast && scripts/ha/desplegar-nodos.sh   # el .env nuevo llega a B y C
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

**Al aire**: el audio (8010) y el RTMP (1935) los pasa HAProxy al nodo promovido
solos; el estudio solo tiene que reconectar. El SRT es un DNAT fijo a A: para
usarlo tras un failover, en el VPS cambiar el destino de la regla `radioclub25` de
`iptables -t nat` a `10.10.0.3:8900` (B) o `10.10.0.4:8890` (C).

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
