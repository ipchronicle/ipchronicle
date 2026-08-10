-- +goose Up
ALTER TABLE system_state ADD COLUMN release_channel TEXT NOT NULL DEFAULT 'stable'
    CHECK (release_channel IN ('stable', 'rc'));

ALTER TABLE nodes ADD COLUMN agent_revision TEXT
    CHECK (agent_revision IS NULL OR length(agent_revision) BETWEEN 1 AND 64);

ALTER TABLE probe_tasks RENAME TO probe_tasks_before_agent_updates;

CREATE TABLE probe_tasks (
    id TEXT PRIMARY KEY CHECK (length(id) = 36),
    node_id TEXT NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
    kind TEXT NOT NULL CHECK (kind IN ('complete-probe', 'agent-update')),
    status TEXT NOT NULL CHECK (status IN (
        'pending', 'acknowledged', 'running', 'verifying', 'installing',
        'restarting', 'succeeded', 'partial', 'failed', 'rolled-back',
        'rejected', 'expired'
    )),
    created_at INTEGER NOT NULL,
    expires_at INTEGER NOT NULL CHECK (expires_at > created_at),
    acknowledged_at INTEGER,
    started_at INTEGER,
    completed_at INTEGER,
    run_id TEXT CHECK (run_id IS NULL OR length(run_id) = 36),
    rejection_reason TEXT CHECK (
        rejection_reason IS NULL OR
        rejection_reason IN ('busy', 'disabled', 'low-memory', 'no-egress', 'missed')
    ),
    target_version TEXT CHECK (target_version IS NULL OR length(target_version) BETWEEN 5 AND 64),
    previous_version TEXT CHECK (previous_version IS NULL OR length(previous_version) BETWEEN 1 AND 64),
    result_version TEXT CHECK (result_version IS NULL OR length(result_version) BETWEEN 1 AND 64),
    failure_code TEXT CHECK (failure_code IS NULL OR length(failure_code) BETWEEN 1 AND 64),
    diagnostic TEXT CHECK (diagnostic IS NULL OR length(diagnostic) BETWEEN 1 AND 4096),
    terminal_confirmed_at INTEGER,
    CHECK ((kind = 'agent-update') = (target_version IS NOT NULL)),
    CHECK (kind = 'complete-probe' OR (run_id IS NULL AND rejection_reason IS NULL)),
    CHECK (started_at IS NULL OR acknowledged_at IS NOT NULL),
    CHECK (completed_at IS NULL OR acknowledged_at IS NOT NULL OR status = 'expired')
);

INSERT INTO probe_tasks (
    id, node_id, kind, status, created_at, expires_at, acknowledged_at,
    started_at, completed_at, run_id, rejection_reason, terminal_confirmed_at
)
SELECT id, node_id, kind, status, created_at, expires_at, acknowledged_at,
       started_at, completed_at, run_id, rejection_reason, terminal_confirmed_at
FROM probe_tasks_before_agent_updates;

DROP TABLE probe_tasks_before_agent_updates;

CREATE UNIQUE INDEX probe_tasks_active_node_idx
    ON probe_tasks (node_id)
    WHERE status IN (
        'pending', 'acknowledged', 'running', 'verifying', 'installing', 'restarting'
    );

CREATE INDEX probe_tasks_node_created_idx
    ON probe_tasks (node_id, created_at DESC, id);

CREATE INDEX probe_tasks_terminal_cleanup_idx
    ON probe_tasks (completed_at)
    WHERE status IN (
        'succeeded', 'partial', 'failed', 'rolled-back', 'rejected', 'expired'
    );
