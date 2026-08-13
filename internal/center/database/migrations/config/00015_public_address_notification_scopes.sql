-- +goose Up
DROP INDEX notification_rules_scope_idx;
DROP INDEX notification_rules_sender_idx;
DROP INDEX notification_rules_name_idx;

ALTER TABLE notification_rules RENAME TO notification_rules_path_scoped;

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
    egress_id TEXT REFERENCES public_addresses(id) ON DELETE CASCADE,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL CHECK (updated_at >= created_at),
    CHECK (field_id IS NULL OR event_type = 'probe-field-change')
);

-- Path-scoped rules cannot be mapped safely until that path observes an IP.
-- Preserve the rule configuration but leave it unmatched and disabled for the
-- administrator to review instead of silently widening its scope.
INSERT INTO notification_rules (
    id, name, enabled, sender_id, event_type, field_id,
    node_id, egress_id, created_at, updated_at
)
SELECT id, name, CASE WHEN egress_id IS NULL THEN enabled ELSE 0 END,
       sender_id, event_type, field_id, node_id, NULL, created_at, updated_at
FROM notification_rules_path_scoped;

DROP TABLE notification_rules_path_scoped;

CREATE UNIQUE INDEX notification_rules_name_idx
    ON notification_rules (name COLLATE NOCASE);

CREATE INDEX notification_rules_sender_idx
    ON notification_rules (sender_id, enabled, event_type, id);

CREATE INDEX notification_rules_scope_idx
    ON notification_rules (node_id, egress_id, event_type, enabled, id);
