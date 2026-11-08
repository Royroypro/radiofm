# ADR 0003 — Arquitectura en capas con raíz de composición

- Estado: aceptada
- Fecha: 2026-11-07

## Contexto
El código debe entenderlo cualquier programador de nivel medio y cualquier agente de IA.

## Decisión
- Capas `transport → service → domain ← repository`, con `platform` para infraestructura.
- Interfaces de repositorio definidas en `domain`, implementadas en `repository/sqlite`.
- Toda la construcción e inyección de dependencias en `internal/app` (sin globales).
- Todas las rutas en un único archivo (`httpserver/router.go`).
- Plantillas `html/template` + JS sin build (sin React ni bundlers).

## Consecuencias
- + Cada cosa tiene un único lugar evidente; `AGENTS.md` lo resume en recetas.
- + Los tests de punta a punta usan exactamente la misma construcción que producción.
- − Algo más de código "de paso" (constructores, interfaces) que una app plana.
