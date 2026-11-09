-- 0007_autocue: análisis de finales para las transiciones (Estudio 1.1).
--   ending       'fade' (fundido) o 'cold' (final seco); '' = sin analizar
--   fade_out_ms  duración del fundido final propio de la pista (0 = seco)
--   marks_manual 1 = el operador ajustó cue-in/intro/mezcla/cue-out: el análisis no los pisa
-- Las pistas se vuelven a analizar en segundo plano para calcular el tipo de final.
ALTER TABLE tracks ADD COLUMN ending TEXT NOT NULL DEFAULT '';
ALTER TABLE tracks ADD COLUMN fade_out_ms INTEGER NOT NULL DEFAULT 0;
ALTER TABLE tracks ADD COLUMN marks_manual INTEGER NOT NULL DEFAULT 0;
UPDATE tracks SET analyzed = 0;
