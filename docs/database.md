# Base de datos

SQLite en `<DATA_DIR>/radioclub25.db` (en Docker: `./data/radioclub25.db`), modo WAL.
Esquema en `internal/repository/sqlite/migrations/`. Se aplica solo al arrancar
(`radioweb migrate` lo hace sin arrancar el servidor). La tabla `schema_migrations` registra
las aplicadas.

Convenciones: fechas con hora en RFC3339 UTC; fechas sin hora `YYYY-MM-DD`; importes en céntimos;
booleanos 0/1.

## Tablas

| Tabla | Para qué | Notas |
|---|---|---|
| `users` | personas que entran a los paneles | `role`: superadmin, admin, editor. Email único sin distinguir mayúsculas |
| `sessions` | sesiones abiertas | se guarda el SHA-256 del token, no el token |
| `settings` | ajustes de la radio (clave/valor) | claves en `domain.SiteSettings` |
| `programs` | programación semanal | `weekday` 0=domingo; horas `HH:MM`; puede cruzar medianoche |
| `ads` | anuncios | `type`: banner, preroll_video, audio_spot; `position` solo banners |
| `ad_events` | medición | `type`: impression, click, complete, skip; `ip_hash` diario |
| `client` | datos del cliente (fila única id=1) | razón social, RUC, contacto |
| `plans` | membresías | precio en céntimos, límites (0 = sin límite) |
| `contracts` | contratos con un plan | el "vigente" más reciente controla la licencia |
| `payments` | pagos / cuotas | solo los `paid` cuentan para "pagado hasta" (`period_end` máximo) |
| `license` | política de bloqueo (fila única id=1) | gracia, nivel automático, bloqueo manual, último nivel |
| `audit_log` | auditoría | `user_id` 0 = sistema |

## Relaciones

```
users 1─* sessions
ads 1─* ad_events                (borrar anuncio borra sus eventos)
plans 1─* contracts               (no se puede borrar un plan con contratos)
contracts 1─* payments            (borrar contrato borra sus pagos)
```

## Copias de seguridad

`make backup` crea `backups/radioclub25-FECHA.tar.gz` con una copia consistente de la base
(`radioweb backup` usa `VACUUM INTO`) y la carpeta `uploads/`. Ver `docs/deployment.md`.
