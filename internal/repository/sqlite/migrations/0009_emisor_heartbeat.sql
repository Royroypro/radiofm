-- 0009_emisor_heartbeat: señal de Club25 Emisor (emisor/) por frecuencia.
-- Usa el mismo token que Club25 Estudio; su estado se guarda aparte para que el
-- panel muestre qué programa está dando señal (Estudio, Emisor o ambos).
ALTER TABLE studio_stations ADD COLUMN emisor_last_seen TEXT;
ALTER TABLE studio_stations ADD COLUMN emisor_version TEXT NOT NULL DEFAULT '';
ALTER TABLE studio_stations ADD COLUMN emisor_status_json TEXT NOT NULL DEFAULT '{}';
