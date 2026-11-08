-- 0001_init: esquema inicial de RadioClub25.
-- Convenciones:
--   * Fechas con hora: TEXT en RFC3339 UTC (2026-01-31T15:04:05Z).
--   * Fechas sin hora: TEXT 'YYYY-MM-DD'.
--   * Importes: INTEGER en céntimos.
--   * Booleanos: INTEGER 0/1.
-- Nunca editar esta migración si ya se aplicó en producción: crear 0002_...sql.

CREATE TABLE users (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    email         TEXT    NOT NULL UNIQUE COLLATE NOCASE,
    name          TEXT    NOT NULL DEFAULT '',
    password_hash TEXT    NOT NULL,
    role          TEXT    NOT NULL CHECK (role IN ('superadmin','admin','editor')),
    active        INTEGER NOT NULL DEFAULT 1,
    created_at    TEXT    NOT NULL,
    last_login_at TEXT
);

CREATE TABLE sessions (
    token_hash TEXT PRIMARY KEY,
    user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    ip         TEXT NOT NULL DEFAULT '',
    user_agent TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL,
    expires_at TEXT NOT NULL
);
CREATE INDEX idx_sessions_user ON sessions(user_id);
CREATE INDEX idx_sessions_expires ON sessions(expires_at);

-- Ajustes del sitio como clave/valor (ver domain.SiteSettings).
CREATE TABLE settings (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
);

CREATE TABLE programs (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    title       TEXT    NOT NULL,
    host        TEXT    NOT NULL DEFAULT '',
    description TEXT    NOT NULL DEFAULT '',
    image_url   TEXT    NOT NULL DEFAULT '',
    weekday     INTEGER NOT NULL CHECK (weekday BETWEEN 0 AND 6),
    start_time  TEXT    NOT NULL, -- HH:MM
    end_time    TEXT    NOT NULL, -- HH:MM
    active      INTEGER NOT NULL DEFAULT 1
);

CREATE TABLE ads (
    id             INTEGER PRIMARY KEY AUTOINCREMENT,
    name           TEXT    NOT NULL,
    type           TEXT    NOT NULL CHECK (type IN ('banner','preroll_video','audio_spot')),
    position       TEXT    NOT NULL DEFAULT '',
    file_url       TEXT    NOT NULL,
    link_url       TEXT    NOT NULL DEFAULT '',
    starts_at      TEXT,
    ends_at        TEXT,
    weight         INTEGER NOT NULL DEFAULT 1,
    skip_after_sec INTEGER NOT NULL DEFAULT 5,
    active         INTEGER NOT NULL DEFAULT 1,
    created_at     TEXT    NOT NULL,
    updated_at     TEXT    NOT NULL
);
CREATE INDEX idx_ads_type ON ads(type, position, active);

CREATE TABLE ad_events (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    ad_id      INTEGER NOT NULL REFERENCES ads(id) ON DELETE CASCADE,
    type       TEXT    NOT NULL CHECK (type IN ('impression','click','complete','skip')),
    ip_hash    TEXT    NOT NULL DEFAULT '',
    created_at TEXT    NOT NULL
);
CREATE INDEX idx_ad_events_ad_date ON ad_events(ad_id, created_at);
CREATE INDEX idx_ad_events_date ON ad_events(created_at);

-- Cliente único (id siempre = 1).
CREATE TABLE client (
    id            INTEGER PRIMARY KEY CHECK (id = 1),
    business_name TEXT NOT NULL DEFAULT '',
    tax_id        TEXT NOT NULL DEFAULT '',
    contact_name  TEXT NOT NULL DEFAULT '',
    phone         TEXT NOT NULL DEFAULT '',
    email         TEXT NOT NULL DEFAULT '',
    address       TEXT NOT NULL DEFAULT '',
    notes         TEXT NOT NULL DEFAULT ''
);
INSERT INTO client (id) VALUES (1);

CREATE TABLE plans (
    id             INTEGER PRIMARY KEY AUTOINCREMENT,
    name           TEXT    NOT NULL,
    price_cents    INTEGER NOT NULL DEFAULT 0,
    currency       TEXT    NOT NULL DEFAULT 'PEN',
    period         TEXT    NOT NULL DEFAULT 'monthly' CHECK (period IN ('monthly','yearly')),
    max_active_ads INTEGER NOT NULL DEFAULT 0,
    max_upload_mb  INTEGER NOT NULL DEFAULT 0,
    tv_enabled     INTEGER NOT NULL DEFAULT 1,
    stats_enabled  INTEGER NOT NULL DEFAULT 1,
    active         INTEGER NOT NULL DEFAULT 1
);

CREATE TABLE contracts (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    plan_id      INTEGER NOT NULL REFERENCES plans(id) ON DELETE RESTRICT,
    start_date   TEXT    NOT NULL,
    end_date     TEXT,
    amount_cents INTEGER NOT NULL DEFAULT 0,
    currency     TEXT    NOT NULL DEFAULT 'PEN',
    auto_renew   INTEGER NOT NULL DEFAULT 1,
    status       TEXT    NOT NULL DEFAULT 'active' CHECK (status IN ('active','expired','cancelled')),
    document_url TEXT    NOT NULL DEFAULT '',
    notes        TEXT    NOT NULL DEFAULT '',
    created_at   TEXT    NOT NULL
);

CREATE TABLE payments (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    contract_id  INTEGER NOT NULL REFERENCES contracts(id) ON DELETE CASCADE,
    period_start TEXT    NOT NULL,
    period_end   TEXT    NOT NULL,
    amount_cents INTEGER NOT NULL DEFAULT 0,
    currency     TEXT    NOT NULL DEFAULT 'PEN',
    status       TEXT    NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','paid','void')),
    method       TEXT    NOT NULL DEFAULT '',
    reference    TEXT    NOT NULL DEFAULT '',
    paid_at      TEXT,
    receipt_url  TEXT    NOT NULL DEFAULT '',
    notes        TEXT    NOT NULL DEFAULT '',
    created_at   TEXT    NOT NULL
);
CREATE INDEX idx_payments_contract ON payments(contract_id, status, period_end);

-- Política de licencia y último nivel aplicado (fila única id = 1).
CREATE TABLE license (
    id                INTEGER PRIMARY KEY CHECK (id = 1),
    grace_days        INTEGER NOT NULL DEFAULT 5,
    auto_block_level  TEXT    NOT NULL DEFAULT 'panel_only',
    manual_level      TEXT    NOT NULL DEFAULT '',
    manual_reason     TEXT    NOT NULL DEFAULT '',
    suspended_message TEXT    NOT NULL DEFAULT 'Servicio temporalmente suspendido. Volveremos pronto.',
    last_level        TEXT    NOT NULL DEFAULT 'active'
);
INSERT INTO license (id) VALUES (1);

CREATE TABLE audit_log (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id    INTEGER NOT NULL DEFAULT 0,
    user_email TEXT    NOT NULL DEFAULT '',
    action     TEXT    NOT NULL,
    entity     TEXT    NOT NULL DEFAULT '',
    entity_id  INTEGER NOT NULL DEFAULT 0,
    details    TEXT    NOT NULL DEFAULT '',
    ip         TEXT    NOT NULL DEFAULT '',
    created_at TEXT    NOT NULL
);
CREATE INDEX idx_audit_date ON audit_log(created_at);
