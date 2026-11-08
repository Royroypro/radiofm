-- 0002_studio: integración con "Club25 Estudio", el programa de automatización
-- de radio que corre en la PC de la emisora (ver docs/adr/0005-estudio-windows.md).
--
-- Las cuñas FM van en tablas propias y NO como un tipo más de `ads`:
--   * `ads.type` tiene un CHECK que SQLite no permite modificar sin reconstruir
--     la tabla, y reconstruirla dentro de la transacción de la migración (con
--     foreign_keys=ON) borraría en cascada `ad_events`.
--   * una cuña FM necesita datos que un anuncio web no tiene (anunciante,
--     sha256, duración, veces por día, franja horaria).

-- Cuñas para la emisión FM. La PC las descarga y las programa en las tandas.
CREATE TABLE fm_spots (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    advertiser    TEXT    NOT NULL,              -- anunciante (agrupa el comprobante)
    name          TEXT    NOT NULL,              -- nombre de la cuña ("Verano 2027 — 20 s")
    file_url      TEXT    NOT NULL,              -- /uploads/fm/...
    sha256        TEXT    NOT NULL DEFAULT '',   -- hash del archivo, la PC lo verifica al descargar
    duration_ms   INTEGER NOT NULL DEFAULT 0,    -- 0 = desconocida (la mide la PC)
    starts_at     TEXT,                          -- NULL = sin fecha de inicio
    ends_at       TEXT,                          -- NULL = sin fecha de fin
    plays_per_day INTEGER NOT NULL DEFAULT 1 CHECK (plays_per_day BETWEEN 1 AND 96),
    hour_from     INTEGER NOT NULL DEFAULT 0  CHECK (hour_from BETWEEN 0 AND 23),
    hour_to       INTEGER NOT NULL DEFAULT 24 CHECK (hour_to BETWEEN 1 AND 24), -- exclusiva
    active        INTEGER NOT NULL DEFAULT 1,
    created_at    TEXT    NOT NULL,
    updated_at    TEXT    NOT NULL,
    CHECK (hour_from < hour_to)
);
CREATE INDEX idx_fm_spots_active ON fm_spots(active, ends_at);

-- Emisiones reales (as-run) que reporta la PC. Es la base del comprobante para
-- el anunciante, así que guarda una copia del anunciante y del nombre: si la
-- cuña se borra, el comprobante sigue siendo válido (spot_id queda en NULL).
CREATE TABLE fm_spot_airplays (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    spot_id     INTEGER REFERENCES fm_spots(id) ON DELETE SET NULL,
    advertiser  TEXT    NOT NULL,
    spot_name   TEXT    NOT NULL,
    aired_at    TEXT    NOT NULL,              -- RFC3339 UTC, hora de inicio de la emisión
    duration_ms INTEGER NOT NULL DEFAULT 0,    -- lo que sonó realmente
    complete    INTEGER NOT NULL DEFAULT 0,    -- 1 = sonó entera
    play_uuid   TEXT    NOT NULL UNIQUE,       -- generado por la PC: reenviar no duplica
    created_at  TEXT    NOT NULL
);
CREATE INDEX idx_fm_airplays_spot_date ON fm_spot_airplays(spot_id, aired_at);
CREATE INDEX idx_fm_airplays_date ON fm_spot_airplays(aired_at);

-- Estado de la PC del estudio (fila única id = 1).
CREATE TABLE studio_state (
    id               INTEGER PRIMARY KEY CHECK (id = 1),
    token_hash       TEXT NOT NULL DEFAULT '',   -- SHA-256 hex del token; '' = sin token
    token_created_at TEXT,
    last_seen        TEXT,                       -- último heartbeat
    version          TEXT NOT NULL DEFAULT '',
    mode             TEXT NOT NULL DEFAULT '',   -- auto, assisted, live
    encoder_json     TEXT NOT NULL DEFAULT '{}', -- estado del codificador tal cual lo envía la PC
    now_playing_json TEXT NOT NULL DEFAULT '',   -- último "lo que suena"
    now_playing_at   TEXT
);
INSERT INTO studio_state (id) VALUES (1);
