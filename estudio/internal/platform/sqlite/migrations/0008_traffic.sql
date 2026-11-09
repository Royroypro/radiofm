-- 0008_traffic: reglas de campaña de las cuñas (Estudio 1.2, campañas del panel web).
-- Cada cuña es una versión de una campaña; las veces por día son de la campaña.
ALTER TABLE spots ADD COLUMN campaign_id INTEGER NOT NULL DEFAULT 0;
ALTER TABLE spots ADD COLUMN weight INTEGER NOT NULL DEFAULT 1;
ALTER TABLE spots ADD COLUMN days_mask INTEGER NOT NULL DEFAULT 0;
ALTER TABLE spots ADD COLUMN position TEXT NOT NULL DEFAULT 'any';
ALTER TABLE spots ADD COLUMN priority INTEGER NOT NULL DEFAULT 2;
ALTER TABLE spots ADD COLUMN sector TEXT NOT NULL DEFAULT '';
