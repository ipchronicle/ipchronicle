-- +goose Up
CREATE TABLE egress_deletion_operations (
    egress_id TEXT PRIMARY KEY CHECK (length(egress_id) = 36),
    node_id TEXT NOT NULL CHECK (length(node_id) = 36),
    status TEXT NOT NULL CHECK (status IN ('pending', 'failed', 'completed')),
    requested_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,
    last_error TEXT CHECK (last_error IS NULL OR length(last_error) BETWEEN 1 AND 1024)
);

CREATE INDEX egress_deletion_operations_status_idx
    ON egress_deletion_operations (status, requested_at, egress_id);

CREATE INDEX egress_deletion_operations_node_idx
    ON egress_deletion_operations (node_id, status, egress_id);
