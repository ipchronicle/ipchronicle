-- +goose Up
CREATE TABLE probe_runs (
    id TEXT PRIMARY KEY CHECK (length(id) = 36),
    node_id TEXT NOT NULL CHECK (length(node_id) = 36),
    history_generation TEXT NOT NULL CHECK (length(history_generation) = 64),
    configuration_revision INTEGER NOT NULL CHECK (configuration_revision >= 1),
    trigger TEXT NOT NULL CHECK (trigger IN ('manual', 'schedule', 'address-change')),
    task_id TEXT CHECK (task_id IS NULL OR length(task_id) = 36),
    triggering_egress_id TEXT CHECK (
        triggering_egress_id IS NULL OR length(triggering_egress_id) = 36
    ),
    status TEXT NOT NULL CHECK (status IN ('running', 'succeeded', 'partial', 'failed')),
    expected_executions INTEGER NOT NULL CHECK (expected_executions BETWEEN 1 AND 64),
    started_at INTEGER NOT NULL,
    completed_at INTEGER,
    received_at INTEGER NOT NULL,
    CHECK ((status = 'running') = (completed_at IS NULL))
);

CREATE INDEX probe_runs_node_time_idx
    ON probe_runs (node_id, started_at DESC, id);

CREATE TABLE probe_executions (
    id TEXT PRIMARY KEY CHECK (length(id) = 36),
    run_id TEXT NOT NULL REFERENCES probe_runs(id) ON DELETE CASCADE,
    egress_id TEXT NOT NULL CHECK (length(egress_id) = 36),
    ordinal INTEGER NOT NULL CHECK (ordinal BETWEEN 0 AND 63),
    sequence INTEGER NOT NULL CHECK (sequence >= 1),
    status TEXT NOT NULL CHECK (status IN (
        'pending', 'running', 'succeeded', 'failed', 'interrupted', 'skipped'
    )),
    started_at INTEGER,
    completed_at INTEGER,
    failure_stage TEXT CHECK (
        failure_stage IS NULL OR
        failure_stage IN ('download', 'selector', 'adapter', 'process', 'timeout', 'output', 'restart')
    ),
    diagnostic TEXT CHECK (diagnostic IS NULL OR length(CAST(diagnostic AS BLOB)) <= 65536),
    received_at INTEGER NOT NULL,
    UNIQUE (run_id, ordinal),
    UNIQUE (egress_id, sequence)
);

CREATE INDEX probe_executions_egress_order_idx
    ON probe_executions (egress_id, sequence DESC);

CREATE TABLE probe_snapshots (
    id TEXT PRIMARY KEY CHECK (length(id) = 36),
    execution_id TEXT NOT NULL UNIQUE REFERENCES probe_executions(id) ON DELETE CASCADE,
    egress_id TEXT NOT NULL CHECK (length(egress_id) = 36),
    sequence INTEGER NOT NULL CHECK (sequence >= 1),
    observed_at INTEGER NOT NULL,
    raw_result BLOB NOT NULL CHECK (length(raw_result) BETWEEN 1 AND 1048576),
    encoded_size INTEGER NOT NULL CHECK (encoded_size BETWEEN 1 AND 1048576),
    received_at INTEGER NOT NULL,
    CHECK (length(raw_result) = encoded_size),
    UNIQUE (egress_id, sequence)
);

CREATE INDEX probe_snapshots_egress_time_idx
    ON probe_snapshots (egress_id, sequence DESC);

CREATE TABLE current_probe_snapshots (
    egress_id TEXT PRIMARY KEY CHECK (length(egress_id) = 36),
    execution_id TEXT NOT NULL UNIQUE REFERENCES probe_executions(id) ON DELETE CASCADE,
    snapshot_id TEXT NOT NULL UNIQUE REFERENCES probe_snapshots(id) ON DELETE CASCADE,
    sequence INTEGER NOT NULL CHECK (sequence >= 1),
    observed_at INTEGER NOT NULL,
    received_at INTEGER NOT NULL
);

CREATE TABLE probe_gaps (
    id TEXT PRIMARY KEY CHECK (length(id) = 36),
    egress_id TEXT NOT NULL CHECK (length(egress_id) = 36),
    node_id TEXT NOT NULL CHECK (length(node_id) = 36),
    history_generation TEXT NOT NULL CHECK (length(history_generation) = 64),
    dropped_count INTEGER NOT NULL CHECK (dropped_count >= 1),
    first_sequence INTEGER NOT NULL CHECK (first_sequence >= 1),
    last_sequence INTEGER NOT NULL CHECK (last_sequence >= first_sequence),
    first_observed_at INTEGER NOT NULL,
    last_observed_at INTEGER NOT NULL,
    received_at INTEGER NOT NULL
);

CREATE INDEX probe_gaps_node_time_idx
    ON probe_gaps (node_id, last_observed_at DESC, id);
