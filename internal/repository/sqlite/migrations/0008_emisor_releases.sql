-- 0008_emisor_releases: versiones publicadas del programa Club25 Emisor (emisor/).
-- El superadmin sube el .exe; las PC autenticadas con su token lo descargan para
-- actualizarse solas. El archivo vive en DATA_DIR/emisor/<id>/ (no es público).
CREATE TABLE emisor_releases (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    version    TEXT    NOT NULL UNIQUE,
    file_name  TEXT    NOT NULL,
    sha256     TEXT    NOT NULL,
    size       INTEGER NOT NULL,
    notes      TEXT    NOT NULL DEFAULT '',
    created_at TEXT    NOT NULL,
    created_by TEXT    NOT NULL DEFAULT ''
);
