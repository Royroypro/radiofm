# ADR 0003 — Licencia GPL-3.0-or-later para Club25 Estudio

- Estado: aceptada
- Fecha: 2026-11-08

## Contexto
Los mejores proyectos libres de radio (Mixxx, Liquidsoap, Rivendell) son GPL. Con una licencia
privada solo se podrían usar como referencia de diseño; con GPL se puede portar su código.

## Decisión
- `estudio/` se publica bajo **GPL-3.0-or-later** (`estudio/LICENSE`). El servidor de la raíz del
  repositorio conserva su licencia.
- Se puede reutilizar o portar código GPL-2.0-or-later, GPL-3.0, LGPL, MIT, BSD, Apache-2.0 y dominio
  público. No se copia código solo GPL-2.0 ni AGPL (incompatibles o con obligaciones de red): de esos
  solo se toman ideas.
- Todo lo portado se registra en `docs/third-party.md` y con un comentario en el archivo.

## Consecuencias
- + Se aprovechan algoritmos probados (crossfade, ducking, fallback por silencio…).
- − Si se distribuye el programa a terceros hay que ofrecer el código fuente (el repositorio).
- − Hay que revisar la cabecera de licencia de cada archivo antes de portarlo.
