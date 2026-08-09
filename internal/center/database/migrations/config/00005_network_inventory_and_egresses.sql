-- +goose Up
CREATE TABLE node_network_inventories (
    node_id TEXT PRIMARY KEY REFERENCES nodes(id) ON DELETE CASCADE,
    payload TEXT,
    captured_at INTEGER,
    received_at INTEGER NOT NULL,
    last_error TEXT CHECK (last_error IS NULL OR length(last_error) BETWEEN 1 AND 1024),
    CHECK ((payload IS NULL) = (captured_at IS NULL))
);

CREATE TABLE network_egresses (
    id TEXT PRIMARY KEY CHECK (length(id) = 36),
    node_id TEXT NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
    name TEXT NOT NULL CHECK (length(name) BETWEEN 1 AND 128),
    kind TEXT NOT NULL CHECK (kind IN ('default', 'interface', 'source')),
    family TEXT NOT NULL CHECK (family IN ('ipv4', 'ipv6')),
    interface_name TEXT CHECK (interface_name IS NULL OR length(interface_name) BETWEEN 1 AND 64),
    source_address TEXT CHECK (source_address IS NULL OR length(source_address) BETWEEN 2 AND 45),
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
        (kind = 'default' AND interface_name IS NULL AND source_address IS NULL AND automatic = 1)
        OR (kind = 'interface' AND interface_name IS NOT NULL AND source_address IS NULL AND automatic = 0)
        OR (kind = 'source' AND interface_name IS NOT NULL AND source_address IS NOT NULL AND automatic = 0)
    )
);

CREATE UNIQUE INDEX network_egresses_default_idx
    ON network_egresses (node_id, family)
    WHERE kind = 'default';

CREATE UNIQUE INDEX network_egresses_selector_idx
    ON network_egresses (node_id, kind, family, interface_name, COALESCE(source_address, ''))
    WHERE kind != 'default';

CREATE INDEX network_egresses_node_idx
    ON network_egresses (node_id, family, kind, created_at, id);
