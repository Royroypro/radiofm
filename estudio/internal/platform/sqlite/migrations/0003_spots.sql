-- 0003_spots: cuñas (del panel web o locales) y contenido planificado de las tandas.

CREATE TABLE spots (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    remote_id     INTEGER NOT NULL DEFAULT 0, -- id en el panel web (0 = cuña local)
    advertiser    TEXT    NOT NULL,
    name          TEXT    NOT NULL,
    path          TEXT    NOT NULL DEFAULT '',
    sha256        TEXT    NOT NULL DEFAULT '',
    duration_ms   INTEGER NOT NULL DEFAULT 0,
    starts_at     TEXT,
    ends_at       TEXT,
    plays_per_day INTEGER NOT NULL DEFAULT 1,
    hour_from     INTEGER NOT NULL DEFAULT 0,
    hour_to       INTEGER NOT NULL DEFAULT 24,
    active        INTEGER NOT NULL DEFAULT 1,
    updated_at    TEXT    NOT NULL
);
CREATE INDEX idx_spots_remote ON spots(remote_id);

CREATE TABLE tanda_spots (
    log_item_id INTEGER NOT NULL,
    position    INTEGER NOT NULL,
    spot_id     INTEGER NOT NULL,
    PRIMARY KEY (log_item_id, position)
);
