-- 0004_stations: cada frecuencia es una emisora con su propio audio, su propia
-- PC del estudio y, si hace falta, su propia programación y sus propias cuñas.
--
-- Audio de cada frecuencia (source):
--   local    → un montaje propio en nuestro Icecast (mount /radio, /radio-barranca…).
--              nginx publica /radio y /radio-* en el mismo dominio.
--   external → la URL de otro servidor de streaming (stream_url).
--
-- programs.frequency_id y fm_spots.frequency_id: NULL = todas las frecuencias.

ALTER TABLE frequencies ADD COLUMN source TEXT NOT NULL DEFAULT 'local' CHECK (source IN ('local', 'external'));
ALTER TABLE frequencies ADD COLUMN mount TEXT NOT NULL DEFAULT '';
ALTER TABLE frequencies ADD COLUMN stream_url TEXT NOT NULL DEFAULT '';

-- La frecuencia principal (la primera) se queda con el montaje de siempre.
UPDATE frequencies SET mount = '/radio'
 WHERE id = (SELECT id FROM frequencies ORDER BY sort_order, id LIMIT 1);
-- Las demás: /radio-<ciudad> si la ciudad es simple (letras sin tilde, números, espacios)
-- y no se repite; si no, /radio-<id>. Se pueden cambiar luego en Admin → Frecuencias.
UPDATE frequencies SET mount = '/radio-' || lower(replace(trim(city), ' ', '-'))
 WHERE mount = '' AND trim(city) <> '' AND length(trim(city)) <= 40
   AND trim(city) NOT GLOB '*[^A-Za-z0-9 ]*' AND trim(city) NOT GLOB '*  *'
   AND (SELECT COUNT(*) FROM frequencies o WHERE lower(trim(o.city)) = lower(trim(frequencies.city))) = 1
   AND lower(trim(city)) <> 'radio';
UPDATE frequencies SET mount = '/radio-' || id WHERE mount = '';
CREATE UNIQUE INDEX idx_frequencies_mount ON frequencies(mount) WHERE source = 'local';

-- PC del estudio por frecuencia (reemplaza a la fila única de studio_state).
CREATE TABLE studio_stations (
    frequency_id     INTEGER PRIMARY KEY REFERENCES frequencies(id) ON DELETE CASCADE,
    token_hash       TEXT NOT NULL DEFAULT '',   -- SHA-256 hex del token; '' = sin token
    token_created_at TEXT,
    last_seen        TEXT,
    version          TEXT NOT NULL DEFAULT '',
    mode             TEXT NOT NULL DEFAULT '',
    encoder_json     TEXT NOT NULL DEFAULT '{}',
    now_playing_json TEXT NOT NULL DEFAULT '',
    now_playing_at   TEXT
);
CREATE INDEX idx_studio_stations_token ON studio_stations(token_hash) WHERE token_hash <> '';

-- La PC que ya estaba configurada queda asignada a la frecuencia principal.
INSERT INTO studio_stations (frequency_id, token_hash, token_created_at, last_seen, version, mode,
                             encoder_json, now_playing_json, now_playing_at)
SELECT (SELECT id FROM frequencies ORDER BY sort_order, id LIMIT 1), token_hash, token_created_at, last_seen,
       version, mode, encoder_json, now_playing_json, now_playing_at
  FROM studio_state WHERE id = 1 AND EXISTS (SELECT 1 FROM frequencies);
DROP TABLE studio_state;

ALTER TABLE programs ADD COLUMN frequency_id INTEGER REFERENCES frequencies(id);
ALTER TABLE fm_spots ADD COLUMN frequency_id INTEGER REFERENCES frequencies(id);
-- Dónde sonó cada cuña (el comprobante se conserva aunque se borre la frecuencia).
ALTER TABLE fm_spot_airplays ADD COLUMN frequency_id INTEGER REFERENCES frequencies(id) ON DELETE SET NULL;
CREATE INDEX idx_programs_frequency ON programs(frequency_id);
CREATE INDEX idx_fm_spots_frequency ON fm_spots(frequency_id);
