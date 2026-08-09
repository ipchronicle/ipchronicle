-- +goose Up
CREATE TABLE notification_senders (
    id TEXT PRIMARY KEY CHECK (length(id) = 36),
    name TEXT NOT NULL CHECK (length(name) BETWEEN 1 AND 128),
    kind TEXT NOT NULL CHECK (kind IN ('telegram', 'webhook', 'javascript')),
    enabled INTEGER NOT NULL DEFAULT 1 CHECK (enabled IN (0, 1)),
    configuration_encrypted BLOB NOT NULL
        CHECK (length(configuration_encrypted) BETWEEN 30 AND 1048608),
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL CHECK (updated_at >= created_at)
);

CREATE UNIQUE INDEX notification_senders_name_idx
    ON notification_senders (name COLLATE NOCASE);

CREATE TABLE notification_rules (
    id TEXT PRIMARY KEY CHECK (length(id) = 36),
    name TEXT NOT NULL CHECK (length(name) BETWEEN 1 AND 128),
    enabled INTEGER NOT NULL DEFAULT 1 CHECK (enabled IN (0, 1)),
    sender_id TEXT NOT NULL REFERENCES notification_senders(id) ON DELETE RESTRICT,
    event_type TEXT NOT NULL CHECK (event_type IN (
        'probe-field-change', 'address-change', 'address-check-failure',
        'address-check-recovery', 'probe-failure', 'probe-recovery',
        'address-gap', 'probe-gap', 'format-mismatch', 'format-changed',
        'format-recovery'
    )),
    field_id TEXT CHECK (field_id IS NULL OR length(field_id) BETWEEN 1 AND 256),
    node_id TEXT REFERENCES nodes(id) ON DELETE CASCADE,
    egress_id TEXT REFERENCES network_egresses(id) ON DELETE CASCADE,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL CHECK (updated_at >= created_at),
    CHECK (field_id IS NULL OR event_type = 'probe-field-change')
);

CREATE UNIQUE INDEX notification_rules_name_idx
    ON notification_rules (name COLLATE NOCASE);

CREATE INDEX notification_rules_sender_idx
    ON notification_rules (sender_id, enabled, event_type, id);

CREATE INDEX notification_rules_scope_idx
    ON notification_rules (node_id, egress_id, event_type, enabled, id);
