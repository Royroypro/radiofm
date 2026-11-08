# RadioClub25

Web de **Radio Club 25 (93.3 FM)** con radio y TV en vivo, panel de administración de la radio
(publicidad, programación, marca) y panel **superadmin** del proveedor (contratos, pagos,
membresías y bloqueos).

- **Web pública**: reproductor de radio (audio) y de TV (video con audio), programación,
  contacto y banners publicitarios. Responsive.
- **Servidores de medios** (proyectos libres, no hechos desde cero):
  - **Icecast 2** recibe el audio de la radio desde la PC emisora (BUTT, Mixxx, RadioDJ…).
  - **MediaMTX** recibe el video+audio desde OBS (RTMP o SRT) y lo publica como HLS.
- **Panel admin** (`/admin`): banners, pre-roll de video, cuñas de audio, estadísticas (CSV),
  programación, marca/colores/logo, contacto, redes, streams y usuarios.
- **Panel superadmin** (`/superadmin`): cliente, planes, contratos, pagos (Soles), bloqueo
  manual y automático por falta de pago, auditoría.

Hecho en **Go** con arquitectura en capas. Ver [docs/architecture.md](docs/architecture.md).

```
PC emisora ── audio (BUTT) ─────────► Icecast :8010 ───┐
           └─ video+audio (OBS) ────► MediaMTX :1935 ──┤
                                                        ▼
Navegador ◄── HTTPS ── nginx ──► /radio (Icecast) · /hls/ (MediaMTX) · / (app Go :8120)
```

## Arranque rápido (producción, Docker)

```bash
cp .env.example .env        # y cambie TODAS las contraseñas (openssl rand -hex 24)
mkdir -p data
docker compose up -d --build
```

Luego configure nginx y HTTPS: [docs/deployment.md](docs/deployment.md).
Para conectar la PC emisora: [docs/streaming.md](docs/streaming.md).

## Desarrollo local

Requisitos: Go 1.27+ (o use `~/.local/go`), ffmpeg y Docker para probar los streams.

```bash
make dev        # app en http://127.0.0.1:8120, plantillas recargadas desde ./web
                # usuario: admin@local.test / cambiar123 (superadmin de desarrollo)
make test       # todos los tests
make lint       # gofmt + go vet + staticcheck
```

Sin Icecast/MediaMTX la web funciona igual y muestra "Fuera del aire".

## Documentación

| Documento | Para quién | Contenido |
|---|---|---|
| [AGENTS.md](AGENTS.md) | agentes de IA y programadores | mapa del repo, reglas, recetas paso a paso |
| [docs/architecture.md](docs/architecture.md) | programadores | capas, flujo de una petición, decisiones |
| [docs/conventions.md](docs/conventions.md) | programadores | estilo, errores, logs, tests |
| [docs/database.md](docs/database.md) | programadores | tablas y migraciones |
| [docs/api.md](docs/api.md) | programadores | rutas HTTP y API JSON |
| [docs/streaming.md](docs/streaming.md) | técnico de la radio | configurar BUTT y OBS |
| [docs/deployment.md](docs/deployment.md) | administrador del servidor | Docker, nginx, certbot, firewall, backups, dominio propio |
| [docs/admin-guide.md](docs/admin-guide.md) | personal de la radio | uso del panel admin |
| [docs/superadmin-guide.md](docs/superadmin-guide.md) | proveedor | contratos, pagos y bloqueos |
| [docs/branding.md](docs/branding.md) | diseño | paleta, tipografía, logo |
| [docs/adr/](docs/adr/) | todos | registro de decisiones de arquitectura |
| [estudio/README.md](estudio/README.md) | programadores (Windows) | programa de automatización "Club25 Estudio": instalación, AGENTS.md y documentación propia |
| [docs/estudio/plan.md](docs/estudio/plan.md) | programadores (Windows) | plan del programa de automatización "Club25 Estudio" y la API que lo conecta |

## Proyectos libres utilizados

| Proyecto | Licencia | Uso |
|---|---|---|
| [MediaMTX](https://github.com/bluenviron/mediamtx) | MIT | servidor de TV (RTMP/SRT → HLS) |
| [Icecast 2](https://icecast.org) (imagen `libretime/icecast`) | GPL-2.0 | servidor de radio |
| [hls.js](https://github.com/video-dev/hls.js) | Apache-2.0 | reproducción HLS en el navegador |
| [chi](https://github.com/go-chi/chi) | MIT | router HTTP |
| [modernc.org/sqlite](https://gitlab.com/cznic/sqlite) | BSD-3 | SQLite en Go puro |
| golang.org/x/crypto (bcrypt) | BSD-3 | contraseñas |
