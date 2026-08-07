-- +goose Up
CREATE TABLE administrators (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    username TEXT NOT NULL COLLATE NOCASE UNIQUE
        CHECK (length(username) BETWEEN 1 AND 64),
    password_hash TEXT NOT NULL,
    locale TEXT NOT NULL DEFAULT 'en'
        CHECK (locale IN ('zh-CN', 'en')),
    uses_default_credentials INTEGER NOT NULL DEFAULT 0
        CHECK (uses_default_credentials IN (0, 1)),
    totp_secret_encrypted BLOB,
    totp_enabled INTEGER NOT NULL DEFAULT 0
        CHECK (totp_enabled IN (0, 1)),
    totp_last_used_step INTEGER NOT NULL DEFAULT -1,
    created_at INTEGER NOT NULL,
    credentials_updated_at INTEGER NOT NULL
);

CREATE TABLE administrator_sessions (
    token_digest BLOB PRIMARY KEY CHECK (length(token_digest) = 32),
    created_at INTEGER NOT NULL,
    expires_at INTEGER NOT NULL,
    client_address TEXT NOT NULL,
    user_agent TEXT NOT NULL,
    CHECK (expires_at > created_at)
);

CREATE INDEX administrator_sessions_expires_at_idx
    ON administrator_sessions (expires_at);

CREATE TABLE system_state (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    history_generation TEXT NOT NULL CHECK (length(history_generation) = 64),
    pending_history_generation TEXT
        CHECK (pending_history_generation IS NULL OR length(pending_history_generation) = 64),
    history_reset_at INTEGER,
    CHECK (pending_history_generation IS NULL OR pending_history_generation != history_generation)
);
