-- +goose Up
ALTER TABLE nodes ADD COLUMN physical_memory_bytes INTEGER
    CHECK (physical_memory_bytes IS NULL OR physical_memory_bytes >= 1);
ALTER TABLE nodes ADD COLUMN probe_schedule_enabled INTEGER NOT NULL DEFAULT 1
    CHECK (probe_schedule_enabled IN (0, 1));
ALTER TABLE nodes ADD COLUMN probe_schedule_cron TEXT NOT NULL DEFAULT '0 0 0 * * *'
    CHECK (length(probe_schedule_cron) BETWEEN 9 AND 128);
ALTER TABLE nodes ADD COLUMN probe_schedule_timezone TEXT NOT NULL DEFAULT 'agent-local'
    CHECK (length(probe_schedule_timezone) BETWEEN 1 AND 128);
ALTER TABLE nodes ADD COLUMN probe_low_memory_override INTEGER NOT NULL DEFAULT 0
    CHECK (probe_low_memory_override IN (0, 1));

CREATE TABLE node_probe_status (
    node_id TEXT PRIMARY KEY REFERENCES nodes(id) ON DELETE CASCADE,
    active_run_id TEXT CHECK (active_run_id IS NULL OR length(active_run_id) = 36),
    next_scheduled_at INTEGER,
    last_occurrence_at INTEGER,
    last_occurrence_trigger TEXT CHECK (
        last_occurrence_trigger IS NULL OR
        last_occurrence_trigger IN ('manual', 'schedule', 'address-change')
    ),
    last_occurrence_status TEXT CHECK (
        last_occurrence_status IS NULL OR
        last_occurrence_status IN ('started', 'skipped')
    ),
    last_skip_reason TEXT CHECK (
        last_skip_reason IS NULL OR
        last_skip_reason IN ('busy', 'disabled', 'low-memory', 'no-egress', 'missed')
    ),
    history_reset_generation TEXT CHECK (
        history_reset_generation IS NULL OR
        length(history_reset_generation) = 64
    ),
    history_reset_at INTEGER,
    history_reset_discarded_address_items INTEGER NOT NULL DEFAULT 0
        CHECK (history_reset_discarded_address_items >= 0),
    history_reset_discarded_probe_items INTEGER NOT NULL DEFAULT 0
        CHECK (history_reset_discarded_probe_items >= 0),
    reported_at INTEGER NOT NULL,
    CHECK ((last_occurrence_at IS NULL) = (last_occurrence_status IS NULL)),
    CHECK ((last_occurrence_status = 'skipped') = (last_skip_reason IS NOT NULL)),
    CHECK ((history_reset_generation IS NULL) = (history_reset_at IS NULL)),
    CHECK (
        history_reset_generation IS NOT NULL OR
        (history_reset_discarded_address_items = 0 AND history_reset_discarded_probe_items = 0)
    )
);

CREATE TABLE probe_tasks (
    id TEXT PRIMARY KEY CHECK (length(id) = 36),
    node_id TEXT NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
    kind TEXT NOT NULL CHECK (kind = 'complete-probe'),
    status TEXT NOT NULL CHECK (status IN (
        'pending', 'acknowledged', 'running', 'succeeded', 'partial',
        'failed', 'rejected', 'expired'
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
    terminal_confirmed_at INTEGER,
    CHECK (started_at IS NULL OR acknowledged_at IS NOT NULL),
    CHECK (completed_at IS NULL OR acknowledged_at IS NOT NULL OR status = 'expired')
);

CREATE UNIQUE INDEX probe_tasks_active_node_idx
    ON probe_tasks (node_id)
    WHERE status IN ('pending', 'acknowledged', 'running');

CREATE INDEX probe_tasks_node_created_idx
    ON probe_tasks (node_id, created_at DESC, id);

CREATE INDEX probe_tasks_terminal_cleanup_idx
    ON probe_tasks (completed_at)
    WHERE status IN ('succeeded', 'partial', 'failed', 'rejected', 'expired');
