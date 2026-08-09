-- +goose Up
CREATE TABLE node_sync_sessions (
    node_id TEXT PRIMARY KEY REFERENCES nodes(id) ON DELETE CASCADE,
    session_id TEXT NOT NULL UNIQUE CHECK (length(session_id) = 36),
    requested_at INTEGER NOT NULL,
    expires_at INTEGER NOT NULL CHECK (expires_at > requested_at),
    delivered_at INTEGER,
    CHECK (delivered_at IS NULL OR (
        delivered_at >= requested_at AND delivered_at < expires_at
    ))
);

CREATE INDEX node_sync_sessions_expires_at_idx
    ON node_sync_sessions (expires_at);
