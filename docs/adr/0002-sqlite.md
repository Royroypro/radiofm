# ADR 0002 — SQLite (modernc.org/sqlite) como base de datos

- Estado: aceptada
- Fecha: 2026-11-07

## Contexto
Una instalación por cliente, pocos usuarios de panel, escrituras moderadas (eventos de anuncios).

## Decisión
SQLite en modo WAL con el driver **modernc.org/sqlite** (Go puro, sin CGO). Migraciones SQL
propias embebidas en el binario (`internal/repository/sqlite/migrations`).

## Consecuencias
- + Cero servicios extra, copia de seguridad = un archivo (`VACUUM INTO`).
- + Binario estático, compila en cualquier sitio.
- − Escrituras serializadas: suficiente para miles de eventos por minuto. Si algún día se
  necesitara más, el acceso a datos está aislado en `repository/` y se puede cambiar a
  PostgreSQL implementando las mismas interfaces de `domain`.
