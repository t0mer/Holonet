-- Seed data (design §3.3 severities, §4 generic OID table, §6 default settings).

-- Five built-in severity levels. Rank 1 = most severe; ranks stay unique/orderable.
INSERT INTO severity_levels (name, rank, color, emoji, is_builtin) VALUES
    ('Critical', 1, '#dc2626', '🔴', 1),
    ('High',     2, '#ea580c', '🟠', 1),
    ('Medium',   3, '#d97706', '🟡', 1),
    ('Low',      4, '#2563eb', '🔵', 1),
    ('Info',     5, '#6b7280', '⚪', 1);

-- RFC-standard generic notifications Sophos may emit (design §4).
INSERT INTO oid_map (oid, name, description, default_severity_id, is_builtin, updated_at) VALUES
    ('1.3.6.1.6.3.1.1.5.1', 'coldStart',             'Agent reinitializing; configuration may have changed', (SELECT id FROM severity_levels WHERE name='High'),   1, strftime('%Y-%m-%dT%H:%M:%SZ','now')),
    ('1.3.6.1.6.3.1.1.5.2', 'warmStart',             'Agent reinitializing without configuration change',    (SELECT id FROM severity_levels WHERE name='Medium'), 1, strftime('%Y-%m-%dT%H:%M:%SZ','now')),
    ('1.3.6.1.6.3.1.1.5.3', 'linkDown',              'A communication link has failed',                      (SELECT id FROM severity_levels WHERE name='High'),   1, strftime('%Y-%m-%dT%H:%M:%SZ','now')),
    ('1.3.6.1.6.3.1.1.5.4', 'linkUp',                'A communication link has come up',                     (SELECT id FROM severity_levels WHERE name='Low'),    1, strftime('%Y-%m-%dT%H:%M:%SZ','now')),
    ('1.3.6.1.6.3.1.1.5.5', 'authenticationFailure', 'An SNMP authentication failure occurred',              (SELECT id FROM severity_levels WHERE name='High'),   1, strftime('%Y-%m-%dT%H:%M:%SZ','now'));

-- Default operational settings (§6). Bind address lives in SQLite, not env.
INSERT INTO settings (key, value) VALUES
    ('snmp.bind_addr',              '0.0.0.0:1162'),
    ('flood.strategy',             'none'),
    ('unknown_default_severity_id', (SELECT CAST(id AS TEXT) FROM severity_levels WHERE name='Info')),
    ('auth.enabled',               'true');
