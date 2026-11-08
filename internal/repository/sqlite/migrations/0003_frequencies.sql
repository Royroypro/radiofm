-- 0003_frequencies: frecuencias de la radio por ciudad (93.3 FM Huacho, 103.2 FM Barranca…).
-- Se administran en Admin → Frecuencias y se muestran en la web. La primera activa
-- (menor sort_order) es la principal: reemplaza a settings.frequency en títulos y logos.

CREATE TABLE frequencies (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    city       TEXT    NOT NULL,                -- Huacho, Barranca, Huaral…
    frequency  TEXT    NOT NULL,                -- 93.3
    band       TEXT    NOT NULL DEFAULT 'FM' CHECK (band IN ('FM', 'AM')),
    sort_order INTEGER NOT NULL DEFAULT 0,
    active     INTEGER NOT NULL DEFAULT 1,
    created_at TEXT    NOT NULL
);
CREATE INDEX idx_frequencies_order ON frequencies(active, sort_order, id);

-- La frecuencia que ya estaba en Ajustes ("93.3 FM") pasa a ser la primera, en Huacho.
INSERT INTO frequencies (city, frequency, band, sort_order, active, created_at)
SELECT 'Huacho',
       TRIM(REPLACE(REPLACE(value, 'FM', ''), 'AM', '')),
       CASE WHEN value LIKE '%AM%' THEN 'AM' ELSE 'FM' END,
       0, 1, strftime('%Y-%m-%dT%H:%M:%SZ', 'now')
FROM settings WHERE key = 'frequency' AND TRIM(value) <> '';
