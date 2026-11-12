-- 0006_studio_status: estado completo que envía la PC del estudio en cada heartbeat
-- (lo que suena, problemas recientes, web, TV, grabador, RDS, actualización, respaldo).
-- El panel arma las alarmas a partir de él (domain.StudioState.Alarms).
ALTER TABLE studio_stations ADD COLUMN status_json TEXT NOT NULL DEFAULT '{}';
