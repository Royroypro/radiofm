# ADR 0004 — Monorepo: `estudio/` como módulo Go aparte dentro de `radiofm`

- Estado: aceptada
- Fecha: 2026-11-08

## Contexto
El programa de la PC y el servidor comparten un contrato (la API `/api/studio/*`) que debe cambiar
en ambos lados a la vez. Un agente de IA o un programador trabaja mejor si ve las dos partes.

## Decisión
- El estudio vive en `radiofm/estudio/` con su propio `go.mod` (`radioclub25/estudio`).
- Así el servidor sigue compilando con `CGO_ENABLED=0` en Linux sin arrastrar Wails ni malgo, y
  `estudio/` se excluye del contexto de Docker (`.dockerignore`).
- El contrato vive en un solo lugar: `docs/api.md` (sección "API del estudio").
- `go.work` **no se versiona**: cada programador lo crea en local (`go work init . ./estudio`) para
  que el editor vea ambos módulos sin alterar la compilación del servidor.
- Una rama y un PR por fase; los cambios de contrato tocan servidor, documento y cliente en el mismo PR.

## Consecuencias
- + Un cambio de API se revisa entero en un PR.
- + Documentación compartida (plan, ADRs, AGENTS.md raíz que apunta a `estudio/AGENTS.md`).
- − Dos módulos Go: los comandos se ejecutan desde la carpeta correspondiente.
