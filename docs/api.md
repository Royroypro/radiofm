# Rutas HTTP

Todas se registran en `internal/transport/httpserver/router.go`.

## Web pública (se cortan con bloqueo total → 503 "servicio suspendido")

| Método | Ruta | Descripción |
|---|---|---|
| GET | `/` | portada: reproductores de radio y TV, programación de hoy, banners |
| GET | `/programacion` | programación semanal |
| GET | `/contacto` | datos de contacto y mapa |
| GET | `/ad/click/{id}` | registra clic y redirige al anunciante |

## API JSON (usada por `web/static/js/site.js`, sin CSRF)

### `GET /api/status`
```json
{
  "radio": {"online": true, "listeners": 12, "title": "Artista - Canción", "bitrate": 128},
  "tv": {"online": false, "viewers": 0},
  "on_air": {"title": "Mañanas Club", "host": "DJ", "start": "06:00", "end": "09:00", "image": ""}
}
```
`on_air` es `null` si no hay programa. Se cachea 5 s en el servidor.

### `GET /api/ads/preroll?kind=video|audio`
- `204` si no hay anuncio (o la licencia desactivó la publicidad).
- `200`:
```json
{"id": 3, "url": "/uploads/ads/ab12.mp4", "skip_after": 5, "has_link": true,
 "click_url": "/ad/click/3", "interval_min": 30}
```

### `POST /api/ads/event`
Cuerpo JSON (acepta `text/plain` para `navigator.sendBeacon`):
`{"ad_id": 3, "type": "impression" | "complete" | "skip"}` → `204`.
Los clics se registran solo en `/ad/click/{id}`.

## Internas (nginx devuelve 404 desde Internet)

| Método | Ruta | Quién la llama | Respuesta |
|---|---|---|---|
| POST | `/internal/mediamtx/auth?token=…` | MediaMTX (authMethod http) | 200 permitido / 401 denegado |
| POST | `/internal/icecast/source?token=…` | Icecast (stream_auth) | cabecera `icecast-auth-user: 1` si se permite |
| GET | `/internal/gate/radio` | nginx (auth_request) | 204 permitido / 403 denegado |

`token` = `INTERNAL_TOKEN` del `.env`.

## Paneles (HTML, sesión + CSRF)

- `/login`, `/logout`
- `/admin/…` (editor+): resumen, `ads`, `stats`, `stats.csv`, `programs`, `password`;
  (admin+): `settings`, `streams`, `users`.
- `/superadmin/…` (superadmin): `client`, `plans`, `contracts`, `payments`, `license`, `audit`.

## Archivos

- `GET /static/…` archivos del repositorio (`web/static`), caché 1 día.
- `GET /uploads/…` archivos subidos (`<DATA_DIR>/uploads`), sin listado de carpetas.
- `GET /healthz` → `ok` (healthcheck de Docker).
