-- 0011_video (Estudio 1.8, modo TV fase B):
--   tracks.video_path: video de la pista (videoclip, promo). Si el archivo de la pista ya es un
--   video, es la misma ruta: su audio suena en la radio y su imagen sale en la TV.
ALTER TABLE tracks ADD COLUMN video_path TEXT NOT NULL DEFAULT '';
