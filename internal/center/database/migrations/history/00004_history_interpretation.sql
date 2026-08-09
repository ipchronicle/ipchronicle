-- +goose Up
CREATE TABLE probe_snapshot_stars (
    snapshot_id TEXT PRIMARY KEY REFERENCES probe_snapshots(id) ON DELETE CASCADE,
    starred_at INTEGER NOT NULL
);

CREATE TABLE probe_comparison_progress (
    egress_id TEXT PRIMARY KEY CHECK (length(egress_id) = 36),
    node_id TEXT NOT NULL CHECK (length(node_id) = 36),
    history_generation TEXT NOT NULL CHECK (length(history_generation) = 64),
    next_sequence INTEGER NOT NULL CHECK (next_sequence >= 1),
    last_success_snapshot_id TEXT REFERENCES probe_snapshots(id) ON DELETE SET NULL,
    updated_at INTEGER NOT NULL
);

CREATE TABLE probe_change_sets (
    id TEXT PRIMARY KEY CHECK (length(id) = 36),
    execution_id TEXT NOT NULL UNIQUE REFERENCES probe_executions(id) ON DELETE CASCADE,
    snapshot_id TEXT NOT NULL UNIQUE REFERENCES probe_snapshots(id) ON DELETE CASCADE,
    egress_id TEXT NOT NULL CHECK (length(egress_id) = 36),
    sequence INTEGER NOT NULL CHECK (sequence >= 1),
    previous_snapshot_id TEXT REFERENCES probe_snapshots(id) ON DELETE SET NULL,
    baseline INTEGER NOT NULL CHECK (baseline IN (0, 1)),
    change_count INTEGER NOT NULL CHECK (change_count >= 0),
    observed_at INTEGER NOT NULL,
    recorded_at INTEGER NOT NULL,
    CHECK (baseline = 0 OR previous_snapshot_id IS NULL),
    CHECK (baseline = 0 OR change_count = 0)
);

CREATE INDEX probe_change_sets_egress_order_idx
    ON probe_change_sets (egress_id, sequence DESC);

CREATE TABLE probe_field_changes (
    change_set_id TEXT NOT NULL REFERENCES probe_change_sets(id) ON DELETE CASCADE,
    field_id TEXT NOT NULL CHECK (length(field_id) BETWEEN 1 AND 256),
    group_name TEXT NOT NULL CHECK (length(group_name) BETWEEN 1 AND 64),
    json_path TEXT NOT NULL CHECK (length(json_path) BETWEEN 1 AND 256),
    value_type TEXT NOT NULL CHECK (value_type IN ('string', 'number', 'boolean', 'null')),
    before_value TEXT NOT NULL CHECK (length(CAST(before_value AS BLOB)) <= 65536),
    after_value TEXT NOT NULL CHECK (length(CAST(after_value AS BLOB)) <= 65536),
    PRIMARY KEY (change_set_id, field_id)
);

CREATE TABLE probe_snapshot_formats (
    snapshot_id TEXT PRIMARY KEY REFERENCES probe_snapshots(id) ON DELETE CASCADE,
    status TEXT NOT NULL CHECK (status IN ('compatible', 'mismatch')),
    signature TEXT NOT NULL CHECK (length(signature) = 64),
    issue_count INTEGER NOT NULL CHECK (issue_count >= 0),
    issues_json BLOB NOT NULL CHECK (length(issues_json) BETWEEN 2 AND 1048576),
    CHECK ((status = 'compatible') = (issue_count = 0))
);

CREATE TABLE probe_format_states (
    egress_id TEXT PRIMARY KEY CHECK (length(egress_id) = 36),
    snapshot_id TEXT NOT NULL UNIQUE REFERENCES probe_snapshots(id) ON DELETE CASCADE,
    sequence INTEGER NOT NULL CHECK (sequence >= 1),
    status TEXT NOT NULL CHECK (status IN ('compatible', 'mismatch')),
    signature TEXT NOT NULL CHECK (length(signature) = 64),
    issue_count INTEGER NOT NULL CHECK (issue_count >= 0),
    issues_json BLOB NOT NULL CHECK (length(issues_json) BETWEEN 2 AND 1048576),
    first_observed_at INTEGER NOT NULL,
    last_observed_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,
    CHECK ((status = 'compatible') = (issue_count = 0)),
    CHECK (last_observed_at >= first_observed_at)
);

CREATE TABLE probe_format_events (
    id TEXT PRIMARY KEY CHECK (length(id) = 36),
    execution_id TEXT NOT NULL UNIQUE REFERENCES probe_executions(id) ON DELETE CASCADE,
    snapshot_id TEXT NOT NULL UNIQUE REFERENCES probe_snapshots(id) ON DELETE CASCADE,
    egress_id TEXT NOT NULL CHECK (length(egress_id) = 36),
    sequence INTEGER NOT NULL CHECK (sequence >= 1),
    kind TEXT NOT NULL CHECK (kind IN ('mismatch', 'changed', 'recovered')),
    previous_signature TEXT CHECK (previous_signature IS NULL OR length(previous_signature) = 64),
    current_signature TEXT NOT NULL CHECK (length(current_signature) = 64),
    issue_count INTEGER NOT NULL CHECK (issue_count >= 0),
    issues_json BLOB NOT NULL CHECK (length(issues_json) BETWEEN 2 AND 1048576),
    observed_at INTEGER NOT NULL,
    recorded_at INTEGER NOT NULL
);

CREATE INDEX probe_format_events_egress_order_idx
    ON probe_format_events (egress_id, sequence DESC);
