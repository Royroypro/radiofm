-- 0009_attributes: atributos musicales para las reglas de flujo de la rotación (Estudio 1.3).
--   bpm (estimado en segundo plano), energy 1–5 (estimada, corregible), voice, language, mood.
--   tempo_done = ya se intentó estimar el tempo (no reintentar sin fin).
ALTER TABLE tracks ADD COLUMN bpm REAL NOT NULL DEFAULT 0;
ALTER TABLE tracks ADD COLUMN energy INTEGER NOT NULL DEFAULT 0;
ALTER TABLE tracks ADD COLUMN voice TEXT NOT NULL DEFAULT '';
ALTER TABLE tracks ADD COLUMN language TEXT NOT NULL DEFAULT '';
ALTER TABLE tracks ADD COLUMN mood TEXT NOT NULL DEFAULT '';
ALTER TABLE tracks ADD COLUMN tempo_done INTEGER NOT NULL DEFAULT 0;
