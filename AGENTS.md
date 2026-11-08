# AGENTS.md — guía para agentes de IA y programadores

Lea esto antes de tocar el código. Es corto a propósito; los detalles están en `docs/`.

## Qué es

App Go que sirve la web de una radio (radio + TV en vivo), un panel admin (publicidad,
programación, marca) y un panel superadmin (contratos, pagos, bloqueos). Los streams los
manejan **Icecast** (audio) y **MediaMTX** (video); la app solo consulta su estado y decide
quién puede publicar/escuchar. Una instalación = un cliente (no es multi-tenant).

## Comandos

```bash
make dev     # ejecutar en local (http://127.0.0.1:8120; admin@local.test / cambiar123)
make test    # go test ./...
make lint    # gofmt, go vet, staticcheck — debe pasar antes de terminar
make up      # docker compose up -d --build (producción)
make logs    # logs de los contenedores
```

Si `go` no está en el PATH: `export PATH=$HOME/.local/go/bin:$PATH`.

## Mapa del repositorio

```
cmd/radioweb/            main: CLI (serve, migrate, create-superadmin, backup) y ciclo de vida HTTP
internal/app/            raíz de composición: construye repos → servicios → router (también la usan los tests)
internal/domain/         entidades, interfaces de repositorio, errores. Go puro, sin dependencias
internal/service/<x>/    casos de uso (auth, ads, billing, license, settings, programs, media, gate, audit)
internal/repository/sqlite/  ÚNICO lugar con SQL; migrations/NNNN_*.sql embebidas
internal/platform/       infraestructura: config (env), db, logger, icecast, mediamtx, storage, scheduler
internal/transport/httpserver/
    router.go            TODAS las rutas
    middleware/          recover, logs, IP, CSRF, sesión, roles, licencia
    public/ admin/ superadmin/   handlers por área
    views/               render de plantillas, funciones de plantilla, flash
    form/                lectura de formularios
internal/transport/httpserver/studioapi/  API /api/studio/* para la PC del estudio (token Bearer)
internal/testutil/       base temporal, config y fuentes de medios falsas para tests
web/templates/           layouts/, partials/, public/, admin/, superadmin/, auth/, errors/
web/static/              css/site.css (web), css/panel.css (paneles), js/site.js (reproductor), js/panel.js
deploy/                  mediamtx.yml, icecast.xml, nginx/
docs/                    documentación (ver README)
estudio/                 programa de Windows "Club25 Estudio" (módulo Go aparte; ver estudio/AGENTS.md)
```

## Integración con la PC del estudio

- Contrato de la API en `docs/api.md` → "API del estudio". Si cambia un campo, actualice el
  handler (`studioapi`), el documento y el cliente de `estudio/` en el mismo cambio.
- Lógica en `internal/service/studio` (token, cuñas FM, emisiones idempotentes por `play_uuid`).
- Cuñas FM = tablas `fm_spots` / `fm_spot_airplays` (no `ads`; ver `docs/adr/0005-estudio-windows.md`).

## Reglas (no romper)

1. **Capas**: `transport → service → domain ← repository`. Un handler nunca usa SQL ni un
   repositorio directamente; un servicio nunca importa `net/http`; `domain` no importa nada interno.
2. **SQL solo en `internal/repository/sqlite`**. Cambios de esquema = migración nueva
   `NNNN_descripcion.sql`. **Nunca** edite una migración existente.
3. **Configuración solo en `internal/platform/config`** (variables de entorno). Documente cada
   variable nueva en `.env.example`. Nada de dominios, puertos ni claves fijos en el código.
4. **Validación en los servicios**: devuelven `domain.Invalid("campo", "mensaje para el usuario")`.
   Los handlers la muestran en el formulario con `formError`.
5. **Auditoría**: toda acción que cambie datos llama a `audit.Log` en el servicio.
6. **Formularios POST** llevan `{{template "csrf" .}}`. Las rutas `/api/` e `/internal/` están exentas.
7. **Licencia**: el estado se lee con `license.Current()` (memoria). Tras cambiar contratos/pagos
   se llama a `Refresh` (ya lo hace `billing` con `OnChange`).
8. Identificadores en inglés; comentarios, documentación y textos de la interfaz en **español**.
9. Cada paquete tiene comentario de paquete explicando qué hace y qué no.

## Recetas

### Añadir una página al panel admin
1. Handler en `internal/transport/httpserver/admin/<tema>.go` (método de `*Handler`).
2. Ruta en `router.go`, dentro de `/admin` (y del grupo `RequireRole(domain.RoleAdmin)` si es solo para admin).
3. Plantilla `web/templates/admin/<pagina>.html` con `{{define "content"}}…{{end}}`;
   se renderiza con `h.Views.Render(w, r, 200, "admin/<pagina>", "Título", data)`.
4. Enlace en el menú: `web/templates/layouts/panel.html`.
5. Test en `internal/app/app_test.go` (la lista de páginas de `TestPanelsRequireLoginAndRoles`).

### Añadir una tabla / entidad
1. `internal/repository/sqlite/migrations/000N_<algo>.sql`.
2. Entidad + interfaz `XRepository` en `internal/domain/<algo>.go`.
3. Implementación en `internal/repository/sqlite/<algo>.go` + test en `sqlite_test.go`.
4. Servicio en `internal/service/<algo>/` (validación + auditoría).
5. Construirlo en `internal/app/app.go` y pasarlo al router en `httpserver.Deps`.
6. Documentar la tabla en `docs/database.md`.

### Añadir un ajuste editable de la radio
Siga el comentario de `domain.SiteSettings` (campo + clave + valor por defecto + input en
`admin/settings.html` + lectura en `admin/settings.go`). `TestSiteSettingsRoundTrip` avisa si olvida algo.

### Añadir una variable de entorno
Campo en `config.Config`, lectura en `Load`, validación en `validate`, línea en `.env.example`
y, si la usa un contenedor, en `docker-compose.yml`.

## Cómo verificar un cambio

1. `make lint && make test`.
2. `make dev` y abrir las páginas afectadas (plantillas se recargan solas con `WEB_DIR`).
3. Si toca streams: `docker compose up -d --build` y `make simulate TV_PASS=<clave de Admin → Streams>`.

## Qué no tocar sin motivo

- `deploy/icecast.xml` → `stream_auth` y `deploy/mediamtx.yml` → `authMethod: http`:
  son los que permiten bloquear la emisión desde el superadmin.
- El hash de tokens de sesión (`auth.hashToken`) y el patrón CSRF (`middleware.CSRF`).
- `.env` (secretos de producción; nunca al repositorio).
