# Convenciones

## Idioma
- Identificadores (paquetes, tipos, funciones, variables, columnas SQL): **inglés**.
- Comentarios, documentación, mensajes de error para el usuario y textos de la interfaz: **español**.

## Código Go
- `gofmt` siempre; `make lint` debe pasar (gofmt + go vet + staticcheck).
- Cada paquete empieza con un comentario de paquete: qué hace y qué no debe hacer.
- Todo tipo y función exportados llevan comentario godoc.
- Sin estado global ni `init()` con efectos. Las dependencias se pasan por constructor (`New...`)
  y se conectan en `internal/app`.
- `context.Context` es el primer parámetro de todo lo que hace E/S.

## Errores
- Envuelva con contexto: `fmt.Errorf("ads: create: %w", err)`.
- Errores de negocio tipados en `internal/domain/errors.go` (`ErrNotFound`, `ErrForbidden`,
  `ErrPlanLimit`…). Compare con `errors.Is`.
- Validación: `domain.Invalid("campo", "Mensaje claro para el usuario")`. El campo coincide con el
  `name` del input para resaltarlo en el formulario.
- `views.ErrorMessage` traduce errores a código HTTP y mensaje. Los 500 se registran en el log.

## Logs
- `log/slog`: texto en desarrollo, JSON en producción.
- Mensajes cortos en español + atributos: `log.Error("recalcular licencia", "err", err)`.
- Nunca registrar contraseñas, tokens ni la clave de publicación.

## Base de datos
- SQL solo en `internal/repository/sqlite`.
- Fechas con hora: TEXT RFC3339 en UTC. Fechas sin hora: TEXT `YYYY-MM-DD`. Importes: INTEGER
  en céntimos (`domain.Money`). Booleanos: INTEGER 0/1.
- Migraciones nuevas: `NNNN_descripcion.sql`, nunca editar una aplicada.

## HTTP y plantillas
- Todas las rutas en `internal/transport/httpserver/router.go`.
- Patrón POST → redirect → GET con mensaje flash (`views.Redirect`).
- Plantillas: cada página define `content` (y opcionalmente `head`/`scripts`). Comentario al
  inicio indicando qué recibe en `.Data`.
- Todo formulario POST incluye `{{template "csrf" .}}`.

## CSS y JS
- Colores como variables CSS (`--navy`, `--royal`, `--red`, `--orange`); los toma
  `partials/common.html` (`theme`) de los ajustes de marca.
- JS sin frameworks ni build: `site.js` (web pública) y `panel.js` (paneles), en módulos con
  comentarios. `localStorage` siempre con try/catch.
- Librerías de terceros en `web/static/js/vendor/` con versión fija (hls.js 1.7.4).

## Tests
- Dominio: tests unitarios puros (`internal/domain/domain_test.go`).
- Repositorios: SQLite temporal (`testutil.DB`).
- Servicios: repos reales sobre SQLite temporal y fakes para Icecast/MediaMTX.
- HTTP de punta a punta: `internal/app/app_test.go` con `httptest`.
- Nombre de los casos en español describiendo el comportamiento esperado.

## Git
- Ramas cortas; mensajes de commit en español, en imperativo: "Añadir límite de anuncios por plan".
- Nunca subir `.env`, `data/` ni `backups/`.
