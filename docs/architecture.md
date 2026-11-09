# Arquitectura

## Visión general

```
                    ┌──────────────── servidor (Docker Compose) ───────────────┐
 PC emisora ─ radioclub25peru.com :8010/:1935/:8890 ─ VPS (HAProxy tcp / DNAT) ─┐
                    │                                                          │
  BUTT ──audio────────► icecast :8000 (host :8010) ◄──── stream_auth ──┐       │
  OBS ──video+audio───► mediamtx :1935 RTMP / :8890 SRT ◄─ auth http ──┤       │
                    │        │ HLS :8888 (127.0.0.1)                   │       │
                    │        │                                   app (Go) :8120│
                    │        │                     API :9997 ◄─────────┤       │
                    │        │                     status-json ◄───────┤       │
                    │        │                              SQLite + uploads   │
                    └────────┼─────────────────────────────────────────┼───────┘
                             │                                         │
 Navegador ◄─ HTTPS radioclub25peru.com ─ VPS (HAProxy) ─ edge nginx :8125 ─┘
                         /radio  → Icecast   (auth_request → app /internal/gate/radio)
                         /hls/   → MediaMTX
                         /       → app
```

- **Icecast** y **MediaMTX** reciben y distribuyen los medios. No se modificó su código.
- La **app Go** sirve la web y los paneles, guarda los datos en **SQLite** y es la autoridad
  que decide quién puede publicar o escuchar (licencia y claves).
- Todo entra por **radioclub25peru.com**: el **HAProxy del VPS** termina HTTPS y pasa la web al
  nginx **edge** del nodo activo (A, con B y C en reserva), que reparte las rutas; la ingesta del
  estudio pasa por TCP/UDP. Ver [scripts/ha/README.md](../scripts/ha/README.md).

## Capas de la app

```
transport/httpserver   handlers, middlewares, plantillas      ← HTTP entra aquí
        │ llama a
        ▼
service/<caso>         reglas de negocio, validación, auditoría
        │ usa interfaces de
        ▼
domain                 entidades + interfaces de repositorio + errores (Go puro)
        ▲ implementa
repository/sqlite      SQL
platform/*             config, db, logger, clientes Icecast/MediaMTX, storage, scheduler
app                    construye todo e inyecta dependencias (composition root)
```

Regla: las flechas no se invierten. Un handler no conoce SQL; un servicio no conoce HTTP.

## Flujo de una petición (ejemplo: guardar un anuncio)

1. nginx → `POST /admin/ads` (multipart).
2. `middleware.Recover → RealIP → Logger → SecurityHeaders → BodyLimit → CSRF → LoadUser`.
3. Grupo `/admin`: `RequireRole(editor)` y `AdminWriteGuard` (bloquea POST si la licencia
   está en solo lectura).
4. `admin.Handler.AdSave` lee el formulario (`form.*`), sube el archivo (`storage.Store.Save`
   valida tipo por contenido y tamaño) y llama a `ads.Service.Create`.
5. `ads.Service` valida, comprueba el límite de anuncios activos del plan
   (`license.Current().Limits`), guarda con `AdRepository` y registra auditoría.
6. Si hay error de validación, el handler vuelve a dibujar el formulario con el mensaje; si no,
   redirige con un mensaje flash.

## Licencia y bloqueos

- `domain.EvaluateLicense` (función pura, con tests) decide el nivel:
  `active → overdue (gracia) → panel_only | no_ads | full`.
- `license.Service` guarda el estado en memoria; `Current()` se consulta en cada petición sin
  tocar la base. `Refresh()` se ejecuta al arrancar, cada hora y tras cambios en
  contratos/pagos/política.
- Efectos de cada nivel:

| Nivel | Web pública | Publicidad | Panel admin | Streams |
|---|---|---|---|---|
| active / overdue | sí | sí | sí (overdue muestra aviso) | sí |
| panel_only | sí | sí | solo lectura | sí |
| no_ads | sí | **no** | solo lectura | sí |
| full | "servicio suspendido" | no | solo lectura | **cortados** |

- Corte de streams en `full`: `media.CutStreams` expulsa a OBS (API de MediaMTX) y a la fuente
  de Icecast (`/admin/killsource`). Las reconexiones se rechazan porque MediaMTX
  (`authMethod: http`) e Icecast (`stream_auth`) preguntan a la app antes de aceptarlas.
  Los oyentes que entran por nginx pasan por `auth_request`.

## Publicidad

- **Banners**: el servidor elige uno por posición (`header`, `sidebar`, `between`, `footer`)
  al azar ponderado por peso. La impresión la cuenta el navegador cuando el banner es visible
  (IntersectionObserver → `POST /api/ads/event`). El clic pasa por `/ad/click/{id}`.
- **Pre-roll de video / cuña de audio**: al pulsar reproducir, `site.js` pide
  `/api/ads/preroll?kind=video|audio`, lo reproduce (con botón "saltar" tras N segundos) y luego
  conecta el directo. El intervalo mínimo entre anuncios por oyente se configura en el panel.
- Las IP nunca se guardan: solo un hash diario con sal (`IP_HASH_SALT`).

## Decisiones

Ver [docs/adr/](adr/). Resumen:
- 0001 Icecast para radio y MediaMTX para TV, en lugar de un servidor propio.
- 0002 SQLite (modernc, sin CGO) en lugar de PostgreSQL.
- 0003 Arquitectura en capas con composition root.
- 0004 Usar el nginx del host (ya existente) en lugar de Caddy.
- 0005 "Club25 Estudio" (automatización en la PC) + API `/api/studio/*`; cuñas FM en tablas propias.
