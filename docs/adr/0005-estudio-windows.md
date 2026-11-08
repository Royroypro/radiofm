# ADR 0005 — "Club25 Estudio": automatización de radio en la PC, sincronizada con la web

- Estado: aceptada
- Fecha: 2026-11-08

## Contexto
La PC de la radio solo emitía con BUTT (no programa nada). Se necesita un programa tipo
automatizador de radio (biblioteca, relojes, parrilla, tandas de cuñas, jingles, hora locutada)
que saque el audio a la tarjeta de sonido (consola/transmisor 93.3 FM) y a Icecast, y que las
cuñas se gestionen y se comprueben desde el panel web. Plan completo: [docs/estudio/plan.md](../estudio/plan.md).

## Decisión
1. Programa de escritorio para Windows en **Go + Wails v2** (miniaudio/malgo para el audio,
   ffmpeg como subproceso), en la carpeta `estudio/` de este repositorio como **módulo Go aparte**
   (el servidor sigue compilando con `CGO_ENABLED=0`). Licencia de `estudio/`: GPL-3.0-or-later.
2. El servidor expone una **API propia** `/api/studio/*` (token Bearer generado en el panel, solo se
   guarda su SHA-256) para: configuración, cuñas FM, programación, heartbeat, "lo que suena" y
   emisiones (as-run) idempotentes por `play_uuid`.
3. Las **cuñas FM van en tablas propias** (`fm_spots`, `fm_spot_airplays`) y no como un tipo más en
   `ads`: `ads.type` tiene un `CHECK` que SQLite no deja cambiar sin reconstruir la tabla, y hacerlo
   dentro de la transacción de migración con `foreign_keys=ON` borraría en cascada `ad_events`.
   Además una cuña FM necesita datos propios (anunciante, sha256, duración, veces/día, franja).
4. Las emisiones guardan una copia del anunciante y del nombre de la cuña y `spot_id` pasa a `NULL`
   al borrar la cuña: el comprobante para el anunciante no se pierde.
5. La API sigue la licencia como la web pública: con bloqueo total responde 503. La emisión FM no
   depende del servidor.
6. No hace falta otro servidor: se reutilizan la app, Icecast (8010) y `/uploads`.

## Consecuencias
- + El personal sube las cuñas y descarga el comprobante desde el panel que ya conoce.
- + La PC funciona sin internet (cola local de emisiones) y se resincroniza sola.
- + La web muestra el título exacto aunque Icecast no actualice la metadata.
- − Hay dos módulos Go en el repo; `go.work` los une para el editor.
- − Las cuñas FM no cuentan para el límite `max_active_ads` del plan (decisión pendiente si se quiere limitar).
