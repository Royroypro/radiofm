-- 0005_campaigns: publicidad FM profesional (tráfico).
--
-- Anunciante (con su rubro = grupo de competencia) → campañas (las reglas:
-- vigencia, veces por día, franja, días de la semana, posición pedida en la
-- tanda, prioridad, frecuencia) → versiones (los audios; las filas de fm_spots).
--
-- Las reglas de la campaña se COPIAN a cada versión al guardar la campaña
-- (desnormalizado a propósito): así la API del estudio, el comprobante y las
-- emisiones siguen leyendo fm_spots como antes, y solo se agregan campos.

CREATE TABLE fm_advertisers (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    name       TEXT NOT NULL UNIQUE COLLATE NOCASE,
    sector     TEXT NOT NULL DEFAULT '',  -- rubro: dos del mismo rubro no van en la misma tanda
    created_at TEXT NOT NULL
);

CREATE TABLE fm_campaigns (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    advertiser_id INTEGER NOT NULL REFERENCES fm_advertisers(id),
    name          TEXT    NOT NULL,
    starts_at     TEXT,
    ends_at       TEXT,
    plays_per_day INTEGER NOT NULL DEFAULT 1 CHECK (plays_per_day BETWEEN 1 AND 96),
    hour_from     INTEGER NOT NULL DEFAULT 0  CHECK (hour_from BETWEEN 0 AND 23),
    hour_to       INTEGER NOT NULL DEFAULT 24 CHECK (hour_to BETWEEN 1 AND 24),
    days_mask     INTEGER NOT NULL DEFAULT 127 CHECK (days_mask BETWEEN 1 AND 127), -- bit 0 = domingo … bit 6 = sábado
    position      TEXT    NOT NULL DEFAULT 'any' CHECK (position IN ('any', 'first', 'last')),
    priority      INTEGER NOT NULL DEFAULT 2 CHECK (priority BETWEEN 1 AND 3),         -- 1 alta, 2 normal, 3 baja
    frequency_id  INTEGER REFERENCES frequencies(id),
    active        INTEGER NOT NULL DEFAULT 1,
    created_at    TEXT    NOT NULL,
    updated_at    TEXT    NOT NULL,
    CHECK (hour_from < hour_to)
);
CREATE INDEX idx_fm_campaigns_advertiser ON fm_campaigns(advertiser_id);

ALTER TABLE fm_spots ADD COLUMN campaign_id INTEGER REFERENCES fm_campaigns(id);
ALTER TABLE fm_spots ADD COLUMN weight INTEGER NOT NULL DEFAULT 1;
ALTER TABLE fm_spots ADD COLUMN days_mask INTEGER NOT NULL DEFAULT 127;
ALTER TABLE fm_spots ADD COLUMN position TEXT NOT NULL DEFAULT 'any';
ALTER TABLE fm_spots ADD COLUMN priority INTEGER NOT NULL DEFAULT 2;
ALTER TABLE fm_spots ADD COLUMN sector TEXT NOT NULL DEFAULT '';
CREATE INDEX idx_fm_spots_campaign ON fm_spots(campaign_id);

-- Datos existentes: un anunciante por nombre y una campaña por cuña (con el mismo id).
INSERT INTO fm_advertisers (name, created_at)
SELECT advertiser, MIN(created_at) FROM fm_spots GROUP BY advertiser COLLATE NOCASE;

INSERT INTO fm_campaigns (id, advertiser_id, name, starts_at, ends_at, plays_per_day, hour_from, hour_to,
                          frequency_id, active, created_at, updated_at)
SELECT s.id, (SELECT a.id FROM fm_advertisers a WHERE a.name = s.advertiser COLLATE NOCASE), s.name,
       s.starts_at, s.ends_at, s.plays_per_day, s.hour_from, s.hour_to, s.frequency_id, s.active,
       s.created_at, s.updated_at
  FROM fm_spots s;

UPDATE fm_spots SET campaign_id = id;
