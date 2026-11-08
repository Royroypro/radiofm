-- 0004_carts: botonera (páginas de 24 botones).

CREATE TABLE carts (
    page         INTEGER NOT NULL CHECK (page BETWEEN 0 AND 9),
    slot         INTEGER NOT NULL CHECK (slot BETWEEN 0 AND 23),
    label        TEXT    NOT NULL DEFAULT '',
    color        TEXT    NOT NULL DEFAULT '#223CB8',
    path         TEXT    NOT NULL,
    stop_on_push INTEGER NOT NULL DEFAULT 1,
    PRIMARY KEY (page, slot)
);
