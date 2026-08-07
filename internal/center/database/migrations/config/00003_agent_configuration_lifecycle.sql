-- +goose Up
ALTER TABLE nodes ADD COLUMN configuration_error_revision INTEGER
    CHECK (configuration_error_revision IS NULL OR configuration_error_revision >= 1);

UPDATE nodes
SET desired_configuration_revision = 1
WHERE desired_configuration_revision = 0;

CREATE TABLE revoked_agent_credentials (
    credential_digest BLOB PRIMARY KEY CHECK (length(credential_digest) = 32),
    revoked_at INTEGER NOT NULL,
    reason TEXT NOT NULL CHECK (reason IN ('revoked', 'deleted'))
);

CREATE TABLE node_deletion_operations (
    node_id TEXT PRIMARY KEY CHECK (length(node_id) = 36),
    credential_digest BLOB NOT NULL CHECK (length(credential_digest) = 32),
    status TEXT NOT NULL CHECK (status IN ('pending', 'failed', 'completed')),
    requested_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,
    last_error TEXT CHECK (last_error IS NULL OR length(last_error) BETWEEN 1 AND 1024)
);

CREATE INDEX node_deletion_operations_status_idx
    ON node_deletion_operations (status, requested_at);
