-- +goose Up
CREATE TABLE network_proxies (
    id TEXT PRIMARY KEY CHECK (length(id) = 36),
    name TEXT NOT NULL CHECK (length(name) BETWEEN 1 AND 128),
    scheme TEXT NOT NULL CHECK (scheme IN ('http', 'https', 'socks5')),
    host TEXT NOT NULL CHECK (length(host) BETWEEN 1 AND 253),
    port INTEGER NOT NULL CHECK (port BETWEEN 1 AND 65535),
    username TEXT CHECK (username IS NULL OR length(username) BETWEEN 1 AND 512),
    password_encrypted BLOB,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL CHECK (updated_at >= created_at)
);

CREATE UNIQUE INDEX network_proxies_name_idx
    ON network_proxies (name COLLATE NOCASE);

DROP INDEX network_egresses_default_idx;
DROP INDEX network_egresses_selector_idx;
DROP INDEX network_egresses_node_idx;

ALTER TABLE network_egresses RENAME TO network_egresses_v5;

CREATE TABLE network_egresses (
    id TEXT PRIMARY KEY CHECK (length(id) = 36),
    node_id TEXT NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
    name TEXT NOT NULL CHECK (length(name) BETWEEN 1 AND 128),
    kind TEXT NOT NULL CHECK (kind IN ('default', 'interface', 'source', 'proxy')),
    family TEXT NOT NULL CHECK (family IN ('ipv4', 'ipv6')),
    interface_name TEXT CHECK (interface_name IS NULL OR length(interface_name) BETWEEN 1 AND 64),
    source_address TEXT CHECK (source_address IS NULL OR length(source_address) BETWEEN 2 AND 45),
    proxy_id TEXT REFERENCES network_proxies(id) ON DELETE RESTRICT,
    enabled INTEGER NOT NULL DEFAULT 1 CHECK (enabled IN (0, 1)),
    available INTEGER NOT NULL DEFAULT 1 CHECK (available IN (0, 1)),
    automatic INTEGER NOT NULL DEFAULT 0 CHECK (automatic IN (0, 1)),
    lightweight_interval_seconds INTEGER NOT NULL DEFAULT 600
        CHECK (lightweight_interval_seconds >= 1),
    probe_on_address_change INTEGER NOT NULL DEFAULT 1
        CHECK (probe_on_address_change IN (0, 1)),
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL CHECK (updated_at >= created_at),
    CHECK (
        (kind = 'default' AND interface_name IS NULL AND source_address IS NULL AND proxy_id IS NULL AND automatic = 1)
        OR (kind = 'interface' AND interface_name IS NOT NULL AND source_address IS NULL AND proxy_id IS NULL AND automatic = 0)
        OR (kind = 'source' AND interface_name IS NOT NULL AND source_address IS NOT NULL AND proxy_id IS NULL AND automatic = 0)
        OR (kind = 'proxy' AND interface_name IS NULL AND source_address IS NULL AND proxy_id IS NOT NULL AND automatic = 0)
    )
);

INSERT INTO network_egresses (
    id, node_id, name, kind, family, interface_name, source_address, proxy_id,
    enabled, available, automatic, lightweight_interval_seconds,
    probe_on_address_change, created_at, updated_at
)
SELECT id, node_id, name, kind, family, interface_name, source_address, NULL,
       enabled, available, automatic, lightweight_interval_seconds,
       probe_on_address_change, created_at, updated_at
FROM network_egresses_v5;

DROP TABLE network_egresses_v5;

CREATE UNIQUE INDEX network_egresses_default_idx
    ON network_egresses (node_id, family)
    WHERE kind = 'default';

CREATE UNIQUE INDEX network_egresses_selector_idx
    ON network_egresses (node_id, kind, family, interface_name, COALESCE(source_address, ''))
    WHERE kind IN ('interface', 'source');

CREATE UNIQUE INDEX network_egresses_proxy_idx
    ON network_egresses (node_id, proxy_id, family)
    WHERE kind = 'proxy';

CREATE INDEX network_egresses_node_idx
    ON network_egresses (node_id, family, kind, created_at, id);

CREATE INDEX network_egresses_proxy_reference_idx
    ON network_egresses (proxy_id, node_id)
    WHERE proxy_id IS NOT NULL;
