-- +goose Up
CREATE TABLE agent_enrollment (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    enabled INTEGER NOT NULL CHECK (enabled IN (0, 1)),
    key_digest BLOB NOT NULL CHECK (length(key_digest) = 32),
    key_encrypted BLOB NOT NULL,
    created_at INTEGER NOT NULL,
    rotated_at INTEGER NOT NULL CHECK (rotated_at >= created_at)
);

CREATE TABLE nodes (
    id TEXT PRIMARY KEY CHECK (length(id) = 36),
    name TEXT NOT NULL CHECK (length(name) BETWEEN 1 AND 64),
    hostname TEXT NOT NULL CHECK (length(hostname) BETWEEN 1 AND 253),
    credential_digest BLOB NOT NULL UNIQUE CHECK (length(credential_digest) = 32),
    enabled INTEGER NOT NULL DEFAULT 1 CHECK (enabled IN (0, 1)),
    revoked_at INTEGER,
    agent_version TEXT NOT NULL CHECK (length(agent_version) BETWEEN 1 AND 64),
    operating_system TEXT NOT NULL CHECK (operating_system = 'linux'),
    architecture TEXT NOT NULL CHECK (architecture IN ('amd64', 'arm64')),
    desired_configuration_revision INTEGER NOT NULL DEFAULT 0
        CHECK (desired_configuration_revision >= 0),
    applied_configuration_revision INTEGER NOT NULL DEFAULT 0
        CHECK (applied_configuration_revision >= 0),
    configuration_error TEXT
        CHECK (configuration_error IS NULL OR length(configuration_error) BETWEEN 1 AND 1024),
    registered_at INTEGER NOT NULL,
    last_seen_at INTEGER
);

CREATE INDEX nodes_name_idx ON nodes (name COLLATE NOCASE, id);
CREATE INDEX nodes_last_seen_idx ON nodes (last_seen_at);

CREATE TABLE node_capabilities (
    node_id TEXT NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
    capability TEXT NOT NULL CHECK (length(capability) BETWEEN 1 AND 64),
    PRIMARY KEY (node_id, capability)
);
