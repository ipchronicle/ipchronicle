-- +goose Up
CREATE TABLE probe_outcome_states (
    egress_id TEXT PRIMARY KEY CHECK (length(egress_id) = 36),
    node_id TEXT NOT NULL CHECK (length(node_id) = 36),
    history_generation TEXT NOT NULL CHECK (length(history_generation) = 64),
    execution_id TEXT NOT NULL UNIQUE REFERENCES probe_executions(id) ON DELETE CASCADE,
    sequence INTEGER NOT NULL CHECK (sequence >= 1),
    status TEXT NOT NULL CHECK (status IN ('healthy', 'failed')),
    first_observed_at INTEGER NOT NULL,
    last_observed_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,
    CHECK (last_observed_at >= first_observed_at)
);

CREATE TABLE notification_events (
    id TEXT PRIMARY KEY CHECK (length(id) = 36),
    event_type TEXT NOT NULL CHECK (event_type IN (
        'probe-field-change', 'address-change', 'address-check-failure',
        'address-check-recovery', 'probe-failure', 'probe-recovery',
        'address-gap', 'probe-gap', 'format-mismatch', 'format-changed',
        'format-recovery', 'test'
    )),
    source_kind TEXT NOT NULL CHECK (source_kind IN (
        'probe-change-set', 'address-event', 'probe-execution',
        'address-gap', 'probe-gap', 'format-event', 'test'
    )),
    source_id TEXT NOT NULL CHECK (length(source_id) BETWEEN 1 AND 64),
    node_id TEXT CHECK (node_id IS NULL OR length(node_id) = 36),
    egress_id TEXT CHECK (egress_id IS NULL OR length(egress_id) = 36),
    payload_json BLOB NOT NULL CHECK (
        json_valid(payload_json) AND
        length(payload_json) BETWEEN 2 AND 1048576
    ),
    observed_at INTEGER NOT NULL,
    recorded_at INTEGER NOT NULL,
    processed_at INTEGER,
    CHECK (
        event_type = 'test' OR
        (node_id IS NOT NULL AND (egress_id IS NOT NULL OR event_type = 'address-gap'))
    ),
    UNIQUE (source_kind, source_id, event_type)
);

CREATE INDEX notification_events_processing_idx
    ON notification_events (processed_at, recorded_at, id);

CREATE INDEX notification_events_node_idx
    ON notification_events (node_id, observed_at DESC, id);

CREATE INDEX notification_events_egress_idx
    ON notification_events (egress_id, observed_at DESC, id);

CREATE TABLE notification_deliveries (
    id TEXT PRIMARY KEY CHECK (length(id) = 36),
    event_id TEXT NOT NULL REFERENCES notification_events(id) ON DELETE CASCADE,
    sender_id TEXT NOT NULL CHECK (length(sender_id) = 36),
    sender_name TEXT NOT NULL CHECK (length(sender_name) BETWEEN 1 AND 128),
    sender_kind TEXT NOT NULL CHECK (sender_kind IN ('telegram', 'webhook', 'javascript')),
    event_type TEXT NOT NULL,
    node_id TEXT CHECK (node_id IS NULL OR length(node_id) = 36),
    egress_id TEXT CHECK (egress_id IS NULL OR length(egress_id) = 36),
    is_test INTEGER NOT NULL DEFAULT 0 CHECK (is_test IN (0, 1)),
    status TEXT NOT NULL CHECK (status IN (
        'pending', 'running', 'retrying', 'succeeded', 'failed'
    )),
    attempt_count INTEGER NOT NULL DEFAULT 0 CHECK (attempt_count BETWEEN 0 AND 4),
    next_attempt_at INTEGER,
    last_attempt_at INTEGER,
    completed_at INTEGER,
    error_code TEXT CHECK (
        error_code IS NULL OR length(error_code) BETWEEN 1 AND 64
    ),
    matched_rule_ids_json BLOB NOT NULL CHECK (
        json_valid(matched_rule_ids_json) AND
        length(matched_rule_ids_json) BETWEEN 2 AND 65536
    ),
    event_json BLOB NOT NULL CHECK (
        json_valid(event_json) AND
        length(event_json) BETWEEN 2 AND 1048576
    ),
    title TEXT NOT NULL CHECK (length(CAST(title AS BLOB)) BETWEEN 1 AND 8192),
    body TEXT NOT NULL CHECK (length(CAST(body AS BLOB)) BETWEEN 1 AND 65536),
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,
    CHECK ((status IN ('pending', 'retrying')) = (next_attempt_at IS NOT NULL)),
    CHECK ((status IN ('succeeded', 'failed')) = (completed_at IS NOT NULL)),
    CHECK ((status = 'failed') = (error_code IS NOT NULL)),
    UNIQUE (event_id, sender_id)
);

CREATE INDEX notification_deliveries_work_idx
    ON notification_deliveries (sender_kind, status, next_attempt_at, created_at, id);

CREATE INDEX notification_deliveries_sender_active_idx
    ON notification_deliveries (sender_id, status, id);

CREATE INDEX notification_deliveries_history_idx
    ON notification_deliveries (created_at DESC, id DESC);
