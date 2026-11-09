-- 0010_relay_composer (Estudio 1.4):
--   log_items.url: enlace a cadena (elemento «relay»: un stream externo durante N segundos).
--   tracks.composer: compositor (etiqueta del archivo), para el informe musical (APDAYC).
ALTER TABLE log_items ADD COLUMN url TEXT NOT NULL DEFAULT '';
ALTER TABLE tracks ADD COLUMN composer TEXT NOT NULL DEFAULT '';
