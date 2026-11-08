-- 0002_schedule: relojes, parrilla semanal, excepciones y playlist del día.

CREATE TABLE clocks (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    name       TEXT    NOT NULL UNIQUE,
    color      TEXT    NOT NULL DEFAULT '#223CB8',
    slots_json TEXT    NOT NULL DEFAULT '[]' -- []domain.ClockSlot
);

CREATE TABLE grid (
    weekday  INTEGER NOT NULL CHECK (weekday BETWEEN 0 AND 6),
    hour     INTEGER NOT NULL CHECK (hour BETWEEN 0 AND 23),
    clock_id INTEGER NOT NULL,
    PRIMARY KEY (weekday, hour)
);

CREATE TABLE grid_exceptions (
    date     TEXT    NOT NULL, -- YYYY-MM-DD
    hour     INTEGER NOT NULL CHECK (hour BETWEEN 0 AND 23),
    clock_id INTEGER NOT NULL,
    PRIMARY KEY (date, hour)
);

CREATE TABLE log_items (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    day         TEXT    NOT NULL, -- YYYY-MM-DD (hora de la radio)
    hour        INTEGER NOT NULL,
    seq         INTEGER NOT NULL,
    kind        TEXT    NOT NULL,
    track_id    INTEGER NOT NULL DEFAULT 0,
    category_id INTEGER NOT NULL DEFAULT 0,
    hard        INTEGER NOT NULL DEFAULT 0,
    at          TEXT    NOT NULL,
    length_ms   INTEGER NOT NULL DEFAULT 0,
    optional    INTEGER NOT NULL DEFAULT 0,
    locked      INTEGER NOT NULL DEFAULT 0,
    status      TEXT    NOT NULL DEFAULT 'pending',
    title       TEXT    NOT NULL DEFAULT '',
    artist      TEXT    NOT NULL DEFAULT '',
    mode        TEXT    NOT NULL DEFAULT '',
    seconds     INTEGER NOT NULL DEFAULT 0,
    note        TEXT    NOT NULL DEFAULT ''
);
CREATE INDEX idx_log_day ON log_items(day, seq);
