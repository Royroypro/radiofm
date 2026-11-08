-- 0006_envelope: envolvente de sonoridad de cada pista (forma de onda en Al aire).
-- 200 puntos codificados como un carácter base 36 cada uno ("0" = silencio, "z" = máximo).
-- Las pistas ya analizadas se vuelven a analizar para calcularla.
ALTER TABLE tracks ADD COLUMN envelope TEXT NOT NULL DEFAULT '';
UPDATE tracks SET analyzed = 0 WHERE envelope = '';
