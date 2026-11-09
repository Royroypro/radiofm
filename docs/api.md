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
  "on_air": {"title": "Mañanas Club", "host": "DJ", "start": "06:00", "end": "09:00", "image": ""},
  "stations": [
    {"id": 1, "city": "Huacho", "label": "93.3 FM", "external": false, "online": true, "listeners": 12,
     "title": "Artista - Canción", "on_air": {"title": "Mañanas Club", "host": "DJ", "start": "06:00", "end": "09:00", "image": ""}},
    {"id": 3, "city": "Huaral", "label": "105.4 FM", "external": true, "online": null, "listeners": null,
     "title": "", "on_air": null}
  ]
}
```
`stations` tiene una entrada por frecuencia activa, en orden (la web muestra un reproductor por
cada una). De un servidor externo no se sabe si está al aire ni cuántos escuchan: `null`.
`radio` y `on_air` son los de la frecuencia principal (compatibilidad). `on_air` es `null` si no
hay programa. Se cachea 5 s en el servidor. Si la PC del estudio de una frecuencia informó lo que
suena hace menos de 2 minutos, su `title` es ese título (ver API del estudio).

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

## API del estudio (`/api/studio/*`, la usa la PC con Club25 Estudio)

Contrato entre el servidor y el programa de automatización de la PC de la radio (`estudio/`).
Handlers en `internal/transport/httpserver/studioapi`, lógica en `internal/service/studio`.
**Si cambia algo aquí, actualice también el cliente de `estudio/` y sus tests.**

- **Una PC por frecuencia:** cada frecuencia tiene su token. El token identifica la frecuencia:
  `config` devuelve su ciudad y su montaje, `spots` y `programs` traen los suyos más los de
  «todas», y `heartbeat`, `nowplaying` y `airplays` se guardan en esa frecuencia.
- **Autenticación:** cabecera `Authorization: Bearer c25s_<64 hex>`. El token se genera en
  Admin → Estudio (PC) para cada frecuencia, se muestra una sola vez y el servidor solo guarda su SHA-256.
  Sin token válido: `401 {"error":"unauthorized"}` con `WWW-Authenticate: Bearer realm="studio"`.
- **Licencia:** con bloqueo total responde `503 {"error":"suspended"}` (Icecast también rechaza
  la fuente). La PC debe seguir sonando por FM y mostrar "servicio web suspendido".
- **Formato:** JSON UTF-8; fechas RFC 3339 en UTC; duraciones en milisegundos. Sin CSRF.
- **Errores:** `400 {"error":"invalid","field":"mode","message":"Modo inválido"}`,
  `400 {"error":"bad_json",…}`, `415 {"error":"content_type"}` (si se envía un Content-Type que
  no es `application/json`), `500 {"error":"internal"}`.

### `GET /api/studio/config`
```json
{
  "station": {"name": "Radio Club 25", "frequency": "103.2 FM", "frequency_id": 2, "city": "Barranca",
              "frequencies": [{"id": 1, "city": "Huacho", "frequency": "93.3", "band": "FM", "label": "93.3 FM"},
                              {"id": 2, "city": "Barranca", "frequency": "103.2", "band": "FM", "label": "103.2 FM"}],
              "slogan": "La música que va contigo",
              "timezone": "America/Lima", "latitude": -11.1067, "longitude": -77.605},
  "icecast": {"host": "radioclub25.sehuacho.com", "port": 8010, "mount": "/radio-barranca",
              "user": "source", "radio_enabled": true, "source": "local"},
  "server_time": "2026-11-08T17:00:00Z"
}
```
`station.frequency`, `city` y `icecast.mount` son los de la frecuencia del token; `frequencies` son
todas las activas en orden. Si el audio de esa frecuencia sale de un servidor externo
(`icecast.source = "external"`), `host` y `mount` van vacíos y la PC conserva el servidor que tenga
configurado.
La contraseña de fuente **no** se envía: se escribe una vez en la PC (que la guarda cifrada).
Coordenadas: variables `STATION_LAT` / `STATION_LON`.

### `GET /api/studio/spots`
Cuñas FM activas y no vencidas (incluye las que empiezan en el futuro, para programar con
antelación). Responde `ETag`; con `If-None-Match` igual devuelve `304` sin cuerpo.
```json
{"spots": [{
  "id": 7, "advertiser": "Ferretería El Sol", "name": "Verano 20 s",
  "url": "https://radioclub25.sehuacho.com/uploads/fm/3f9a….mp3",
  "sha256": "e3b0c442…", "duration_ms": 20000,
  "starts_at": null, "ends_at": "2026-12-31T05:00:00Z",
  "plays_per_day": 6, "hour_from": 7, "hour_to": 13,
  "updated_at": "2026-11-08T16:55:10Z",
  "campaign_id": 3, "weight": 2, "days": 62, "position": "first", "priority": 1, "sector": "ferretería"
}]}
```
- Cada entrada es una **versión** (un audio) de una **campaña**; las reglas (veces por día, franja,
  `days` = máscara bit 0 domingo … bit 6 sábado, `position` any/first/last, `priority` 1 alta … 3 baja,
  `sector` = rubro del anunciante) son las de su campaña. Las veces por día son **de la campaña**:
  la PC agrupa por `campaign_id` y rota las versiones según `weight`.
- **Compatibilidad:** sin `?v=2` se envía **una versión por campaña** (la de más peso), para que un
  Club25 Estudio anterior a 1.2 no duplique las emisiones. Estudio 1.2+ pide `?v=2`.
- `hour_from`/`hour_to`: franja en hora local de la radio, `[hour_from, hour_to)`; `0`–`24` = todo el día.
- `duration_ms` = 0 si no se conoce (la PC la mide al descargar).
- La PC descarga `url` (pública), verifica `sha256` y descarta el archivo si no coincide.

### `GET /api/studio/programs`
```json
{"programs": [{"id": 1, "title": "Mañanas Club", "host": "DJ", "weekday": 1, "start": "06:00", "end": "09:00"}]}
```
`weekday` 0 = domingo. Solo programas activos. Un programa puede cruzar la medianoche (`end` < `start`).

### `POST /api/studio/heartbeat`
Cada ~60 s. Cuerpo: `{"version": "0.1.0", "mode": "auto" | "assisted" | "live", "encoder": {…}}`
(`encoder` es libre, máx. 2 KB; p. ej. `{"connected": true, "kbps": 128, "retries": 0, "dropped": 0}`).
Respuesta: `{"ok": true, "server_time": "…"}`. El panel muestra la PC "Sin conexión" tras 3 min sin señal.

### `POST /api/studio/nowplaying`
Cuerpo: `{"artist": "Grupo 5", "title": "Motor y motivo", "kind": "music", "ends_at": "…"}` → `204`.
`kind`: `music`, `spot`, `jingle`, `voice`, `live`… (texto libre, máx. 20). `ends_at` es opcional.
`/api/status` usa este título en lugar del de Icecast durante 2 minutos (si la radio está en línea).

### `POST /api/studio/airplays`
Emisiones reales de cuñas (as-run). Lote de hasta 500. La PC guarda una cola local si no hay red
y la reenvía; reenviar no duplica (`play_uuid` único).
```json
{"airplays": [{"spot_id": 7, "aired_at": "2026-11-08T15:00:03Z", "duration_ms": 20010,
               "complete": true, "play_uuid": "5f0c2b9e-1d2a-4c3b-9e8f-000000000001"}]}
```
Respuesta:
```json
{"received": 1, "inserted": 1, "ignored": 0, "rejected": []}
```
- `ignored`: ya registradas o de cuñas que ya no existen. La PC puede borrarlas de su cola.
- `rejected`: `play_uuid` con datos inválidos (uuid fuera de `[A-Za-z0-9-]{8,64}`, `aired_at` más de
  10 min en el futuro o de hace más de un año, duración fuera de 0–3 600 000). No deben reenviarse.
- `complete`: lo decide la PC (sonó ≥ duración − 0,5 s).

## Internas (nginx devuelve 404 desde Internet)

| Método | Ruta | Quién la llama | Respuesta |
|---|---|---|---|
| POST | `/internal/mediamtx/auth?token=…` | MediaMTX (authMethod http) | 200 permitido / 401 denegado |
| POST | `/internal/icecast/source?token=…` | Icecast (stream_auth) | cabecera `icecast-auth-user: 1` si se permite |
| GET | `/internal/gate/radio` | nginx (auth_request) | 204 permitido / 403 denegado |

`token` = `INTERNAL_TOKEN` del `.env`.

## Paneles (HTML, sesión + CSRF)

- `/login`, `/logout`
- `/admin/…` (editor+): resumen, `ads`, `stats`, `stats.csv`, `fm-spots` (cuñas FM),
  `fm-spots/report` y `fm-spots/report.csv` (comprobante de emisión), `programs`, `password`;
  (admin+): `settings`, `streams`, `studio` (token y estado de la PC de cada frecuencia:
  `POST /admin/studio/{frecuencia}/token` y `/token/revoke`), `users`. Editor+: `frequencies`.
- `/superadmin/…` (superadmin): `client`, `plans`, `contracts`, `payments`, `license`, `audit`.

## Archivos

- `GET /static/…` archivos del repositorio (`web/static`), caché 1 día.
- `GET /uploads/…` archivos subidos (`<DATA_DIR>/uploads`), sin listado de carpetas.
- `GET /healthz` → `ok` (healthcheck de Docker).
