-- Flood-control tunables (design §3.4). INSERT OR IGNORE so existing operator
-- values are never clobbered on re-run.
INSERT OR IGNORE INTO settings (key, value) VALUES
    ('flood.dedupe_window',   '30s'),
    ('flood.rate_n',          '5'),
    ('flood.rate_window',     '1m'),
    ('flood.digest_interval', '5m');
