-- 0001_library: biblioteca musical e historial de emisiones.
-- Convenciones (las mismas del servidor):
--   * Fechas con hora: TEXT RFC3339 UTC.  * Duraciones: INTEGER en milisegundos.
--   * Booleanos: INTEGER 0/1.
-- Nunca editar una migración aplicada: crear 0002_...sql.

CREATE TABLE categories (
    id        INTEGER PRIMARY KEY AUTOINCREMENT,
    name      TEXT    NOT NULL,
    parent_id INTEGER NOT NULL DEFAULT 0,
    kind      TEXT    NOT NULL DEFAULT 'music',
    color     TEXT    NOT NULL DEFAULT '#223CB8',
    UNIQUE (parent_id, name)
);

CREATE TABLE tracks (
    id             INTEGER PRIMARY KEY AUTOINCREMENT,
    path           TEXT    NOT NULL UNIQUE COLLATE NOCASE, -- Windows no distingue mayúsculas
    title          TEXT    NOT NULL DEFAULT '',
    artist         TEXT    NOT NULL DEFAULT '',
    album          TEXT    NOT NULL DEFAULT '',
    genre          TEXT    NOT NULL DEFAULT '',
    year           TEXT    NOT NULL DEFAULT '',
    category_id    INTEGER NOT NULL DEFAULT 0,
    duration_ms    INTEGER NOT NULL DEFAULT 0,
    active         INTEGER NOT NULL DEFAULT 1,
    valid_from     TEXT,
    valid_to       TEXT,
    analyzed       INTEGER NOT NULL DEFAULT 0,
    loudness       REAL    NOT NULL DEFAULT 0,
    gain_db        REAL    NOT NULL DEFAULT 0,
    cue_in_ms      INTEGER NOT NULL DEFAULT 0,
    cue_out_ms     INTEGER NOT NULL DEFAULT 0,
    segue_ms       INTEGER NOT NULL DEFAULT 0,
    intro_ms       INTEGER NOT NULL DEFAULT 0,
    analyze_err    TEXT    NOT NULL DEFAULT '',
    missing        INTEGER NOT NULL DEFAULT 0,
    last_played_at TEXT,
    play_count     INTEGER NOT NULL DEFAULT 0,
    created_at     TEXT    NOT NULL
);
CREATE INDEX idx_tracks_category ON tracks(category_id, active);
CREATE INDEX idx_tracks_pending ON tracks(analyzed, missing);

-- Historial de lo emitido (as-run). Base de la rotación y de los reportes.
CREATE TABLE plays (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    track_id    INTEGER NOT NULL DEFAULT 0,
    spot_id     INTEGER NOT NULL DEFAULT 0,
    kind        TEXT    NOT NULL,
    artist      TEXT    NOT NULL DEFAULT '',
    title       TEXT    NOT NULL DEFAULT '',
    started_at  TEXT    NOT NULL,
    duration_ms INTEGER NOT NULL DEFAULT 0,
    complete    INTEGER NOT NULL DEFAULT 0,
    uuid        TEXT    NOT NULL UNIQUE
);
CREATE INDEX idx_plays_started ON plays(started_at);
CREATE INDEX idx_plays_track ON plays(track_id, started_at);
