-- 0007_studio_commands: control remoto de la PC del estudio desde el panel.
-- El panel encola órdenes (siguiente, iniciar, detener, modo, mover, quitar, TV); la PC
-- las pide cada pocos segundos, las ejecuta y responde el resultado.
CREATE TABLE studio_commands (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    frequency_id INTEGER NOT NULL REFERENCES frequencies(id) ON DELETE CASCADE,
    kind         TEXT    NOT NULL,
    arg          TEXT    NOT NULL DEFAULT '',
    created_at   TEXT    NOT NULL,
    created_by   TEXT    NOT NULL DEFAULT '',
    taken_at     TEXT,
    done_at      TEXT,
    ok           INTEGER NOT NULL DEFAULT 0,
    result       TEXT    NOT NULL DEFAULT ''
);
CREATE INDEX idx_studio_commands_pending ON studio_commands(frequency_id, taken_at, created_at);
