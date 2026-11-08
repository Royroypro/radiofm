-- 0005_sync: cola local de emisiones de cuñas del panel web que faltan enviar
-- (si no hay red se acumulan y se envían al volver; reenviar no duplica por play_uuid).

CREATE TABLE airplay_queue (
    play_uuid   TEXT    PRIMARY KEY,
    spot_id     INTEGER NOT NULL, -- id de la cuña en el panel web (remote_id)
    aired_at    TEXT    NOT NULL,
    duration_ms INTEGER NOT NULL DEFAULT 0,
    complete    INTEGER NOT NULL DEFAULT 0,
    attempts    INTEGER NOT NULL DEFAULT 0,
    created_at  TEXT    NOT NULL
);

-- Valores pequeños de estado (ETag de cuñas, última sincronización…).
CREATE TABLE kv (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
);
