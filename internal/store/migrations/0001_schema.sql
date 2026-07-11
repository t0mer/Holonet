-- HoloNet schema (design §3.6). Timestamps are RFC3339Nano UTC strings, which
-- sort lexicographically. Booleans are INTEGER 0/1. All *_sealed columns hold
-- AES-256-GCM base64 blobs (internal/crypto) — never plaintext.

CREATE TABLE settings (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
);

CREATE TABLE v2c_communities (
    id               INTEGER PRIMARY KEY AUTOINCREMENT,
    community_sealed TEXT NOT NULL,
    enabled          INTEGER NOT NULL DEFAULT 1,
    created_at       TEXT NOT NULL
);

CREATE TABLE snmpv3_users (
    id               INTEGER PRIMARY KEY AUTOINCREMENT,
    username         TEXT NOT NULL,
    security_level   TEXT NOT NULL,              -- authNoPriv | authPriv
    auth_protocol    TEXT NOT NULL DEFAULT '',
    auth_pass_sealed TEXT NOT NULL DEFAULT '',
    priv_protocol    TEXT NOT NULL DEFAULT '',
    priv_pass_sealed TEXT NOT NULL DEFAULT '',
    engine_id        TEXT NOT NULL DEFAULT '',
    enabled          INTEGER NOT NULL DEFAULT 1,
    created_at       TEXT NOT NULL
);

CREATE TABLE devices (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    source_ip  TEXT NOT NULL UNIQUE,
    name       TEXT NOT NULL,
    enabled    INTEGER NOT NULL DEFAULT 1,
    created_at TEXT NOT NULL
);

CREATE TABLE severity_levels (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    name       TEXT NOT NULL UNIQUE,
    rank       INTEGER NOT NULL UNIQUE,
    color      TEXT NOT NULL,
    emoji      TEXT NOT NULL,
    is_builtin INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE oid_map (
    id                  INTEGER PRIMARY KEY AUTOINCREMENT,
    oid                 TEXT NOT NULL UNIQUE,
    name                TEXT NOT NULL,
    description         TEXT NOT NULL DEFAULT '',
    default_severity_id INTEGER REFERENCES severity_levels(id),
    is_builtin          INTEGER NOT NULL DEFAULT 0,
    updated_at          TEXT NOT NULL
);

CREATE TABLE channels (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    name          TEXT NOT NULL,
    kind          TEXT NOT NULL,                 -- shoutrrr | whatsapp | webhook
    config_sealed TEXT NOT NULL,                 -- JSON, secrets sealed
    enabled       INTEGER NOT NULL DEFAULT 1,
    created_at    TEXT NOT NULL
);

CREATE TABLE rules (
    id                    INTEGER PRIMARY KEY AUTOINCREMENT,
    ord                   INTEGER NOT NULL UNIQUE,
    name                  TEXT NOT NULL,
    enabled               INTEGER NOT NULL DEFAULT 1,
    continue_on_match     INTEGER NOT NULL DEFAULT 0,
    bypass_flood_control  INTEGER NOT NULL DEFAULT 0,
    match_device_id       INTEGER REFERENCES devices(id),
    match_oid_glob        TEXT NOT NULL DEFAULT '*',
    match_varbind_regex   TEXT,
    severity_id           INTEGER REFERENCES severity_levels(id),
    created_at            TEXT NOT NULL
);

CREATE TABLE rule_channels (
    rule_id    INTEGER NOT NULL REFERENCES rules(id) ON DELETE CASCADE,
    channel_id INTEGER NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
    PRIMARY KEY (rule_id, channel_id)
);

CREATE TABLE default_routes (
    severity_id INTEGER NOT NULL REFERENCES severity_levels(id) ON DELETE CASCADE,
    channel_id  INTEGER NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
    PRIMARY KEY (severity_id, channel_id)
);

CREATE TABLE traps (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    received_at     TEXT NOT NULL,
    source_ip       TEXT NOT NULL,
    snmp_version    TEXT NOT NULL,
    trap_oid        TEXT NOT NULL,
    resolved_name   TEXT NOT NULL,
    severity_id     INTEGER REFERENCES severity_levels(id),
    matched_rule_id INTEGER REFERENCES rules(id),
    varbinds_json   TEXT NOT NULL DEFAULT '[]',
    aggregation_key TEXT NOT NULL DEFAULT '',
    suppressed      INTEGER NOT NULL DEFAULT 0,
    unmapped        INTEGER NOT NULL DEFAULT 0
);

CREATE INDEX idx_traps_received_at ON traps(received_at DESC);
CREATE INDEX idx_traps_source_ip ON traps(source_ip);

CREATE TABLE notifications (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    trap_id    INTEGER NOT NULL REFERENCES traps(id) ON DELETE CASCADE,
    channel_id INTEGER REFERENCES channels(id),
    status     TEXT NOT NULL,                    -- sent | failed | held
    attempts   INTEGER NOT NULL DEFAULT 0,
    last_error TEXT NOT NULL DEFAULT '',
    sent_at    TEXT
);

CREATE INDEX idx_notifications_trap_id ON notifications(trap_id);

CREATE TABLE admin (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    username   TEXT NOT NULL UNIQUE,
    pass_hash  TEXT NOT NULL,
    created_at TEXT NOT NULL
);
