# Despliegue

Servidor actual: `radioclub25.sehuacho.com` → 8.243.126.126 (Ubuntu, Docker, nginx del host con
certbot y otros sitios). Carpeta del proyecto: `/home/lima/sehuacho/radioclub25`.

## Puertos

| Puerto | Servicio | Expuesto | Para |
|---|---|---|---|
| 80/443 | nginx (host) | Internet | web, `/radio`, `/hls/` |
| 8120 | app Go | solo 127.0.0.1 | nginx |
| 8888 | MediaMTX HLS | solo 127.0.0.1 | nginx `/hls/` |
| 9997 | MediaMTX API | solo red Docker | app |
| 1935/tcp | MediaMTX RTMP | Internet | OBS |
| 8890/udp | MediaMTX SRT | Internet | OBS (alternativa) |
| 8010/tcp | Icecast | Internet | BUTT (fuente) y nginx |

El puerto 8000 del host ya estaba ocupado; por eso Icecast usa el 8010.

## 1. Contenedores

```bash
cd /home/lima/sehuacho/radioclub25
cp .env.example .env      # editar: todas las CAMBIAR → openssl rand -hex 24
mkdir -p data
docker compose up -d --build
docker compose ps         # app (healthy), mediamtx, icecast
```

El superadmin inicial se crea con `SUPERADMIN_EMAIL`/`SUPERADMIN_PASSWORD` del `.env` la primera
vez. Otra forma: `make superadmin EMAIL=... PASS=...`.

## 2. nginx + HTTPS (requiere sudo)

```bash
sudo cp deploy/nginx/radioclub25.sehuacho.com.conf /etc/nginx/sites-available/radioclub25.sehuacho.com
sudo ln -s /etc/nginx/sites-available/radioclub25.sehuacho.com /etc/nginx/sites-enabled/
sudo nginx -t && sudo systemctl reload nginx
sudo certbot --nginx -d radioclub25.sehuacho.com --redirect
```

certbot añade el bloque `listen 443 ssl` al mismo archivo y la redirección de HTTP a HTTPS.

## 3. Firewall y router (NAT)

El servidor **no tiene la IP pública**: su IP es `15.14.12.2` y sale por el router `15.14.12.1`,
que traduce 8.243.126.126 → 15.14.12.2. En el router hay que reenviar (dst-nat) hacia
`15.14.12.2` los mismos puertos: **1935/tcp, 8890/udp, 8010/tcp** (80/443 ya están).
Sin esto la web funciona, pero la PC emisora no puede conectarse desde fuera.

Docker publica sus puertos por iptables aunque ufw esté activo; aun así quedan abiertos en ufw
(ya aplicado el 2026-11-07):

```bash
sudo ufw allow 1935/tcp
sudo ufw allow 8890/udp
sudo ufw allow 8010/tcp
```

## 4. Comprobación

```bash
curl -I https://radioclub25.sehuacho.com/healthz            # 200
curl -s https://radioclub25.sehuacho.com/api/status
make simulate TV_PASS=<clave del panel>                       # y abrir la web
```

## Actualizar

```bash
git pull
docker compose up -d --build app      # las migraciones se aplican solas al arrancar
```

## Copias de seguridad

```bash
make backup          # backups/radioclub25-AAAAMMDD-HHMMSS.tar.gz (base + uploads)
```
Programar a diario con cron (`crontab -e`):
```
30 3 * * * cd /home/lima/sehuacho/radioclub25 && make backup >/dev/null 2>&1
```
Restaurar: detener la app, descomprimir en `data/` renombrando `backup-*.db` a
`radioclub25.db`, y arrancar.

## Cambiar a un dominio propio (cuando el cliente lo contrate)

1. Apuntar el DNS del dominio (registro A) a 8.243.126.126.
2. Copiar el vhost con el nuevo nombre:
   ```bash
   sed 's/radioclub25.sehuacho.com/www.dominio.pe dominio.pe/' deploy/nginx/radioclub25.sehuacho.com.conf \
     | sudo tee /etc/nginx/sites-available/dominio.pe
   sudo ln -s /etc/nginx/sites-available/dominio.pe /etc/nginx/sites-enabled/
   sudo nginx -t && sudo systemctl reload nginx
   sudo certbot --nginx -d dominio.pe -d www.dominio.pe --redirect
   ```
3. En `.env`: `BASE_URL=https://dominio.pe` e `INGEST_HOST=dominio.pe`; luego
   `docker compose up -d app icecast`.
4. Opcional: dejar el subdominio redirigiendo con un `return 301 https://dominio.pe$request_uri;`
   en su bloque `server`.
5. Actualizar BUTT y OBS con el nuevo host (los puertos y claves no cambian).

## Logs

```bash
docker compose logs -f app          # JSON: peticiones, autorizaciones de streams, cambios de licencia
docker compose logs -f mediamtx
docker compose exec icecast tail -f /var/log/icecast/error.log
sudo tail -f /var/log/nginx/radioclub25.error.log
```
