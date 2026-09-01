-- name: GetHistoryMetadata :one
SELECT id, generation, created_at
FROM history_metadata
WHERE id = 1;

-- name: CreateHistoryMetadata :exec
INSERT INTO history_metadata (id, generation, created_at)
VALUES (1, ?, ?);

-- name: UpsertHistoryNode :exec
INSERT INTO history_nodes (node_id, node_name, recorded_at)
VALUES (?, ?, ?)
ON CONFLICT (node_id) DO UPDATE SET
    node_name = excluded.node_name,
    recorded_at = excluded.recorded_at
WHERE history_nodes.node_name != excluded.node_name;

-- name: GetHistoryNode :one
SELECT node_id, node_name, recorded_at
FROM history_nodes
WHERE node_id = ?;

-- name: UpsertAddressState :exec
INSERT INTO address_states (
    egress_id, node_id, history_generation, family, status, sequence,
    public_address, local_interface, local_address, proxy_path, likely_nat,
    temporary, failure_reason, last_checked_at, last_succeeded_at,
    last_changed_at, received_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (egress_id) DO UPDATE SET
    node_id = excluded.node_id,
    history_generation = excluded.history_generation,
    family = excluded.family,
    status = excluded.status,
    sequence = excluded.sequence,
    public_address = excluded.public_address,
    local_interface = excluded.local_interface,
    local_address = excluded.local_address,
    proxy_path = excluded.proxy_path,
    likely_nat = excluded.likely_nat,
    temporary = excluded.temporary,
    failure_reason = excluded.failure_reason,
    last_checked_at = excluded.last_checked_at,
    last_succeeded_at = excluded.last_succeeded_at,
    last_changed_at = excluded.last_changed_at,
    received_at = excluded.received_at
WHERE excluded.history_generation != address_states.history_generation
   OR excluded.sequence > address_states.sequence
   OR (
       excluded.sequence = address_states.sequence AND
       excluded.last_checked_at >= address_states.last_checked_at
   );

-- name: CreateAddressEvent :execrows
INSERT INTO address_events (
    id, public_address_id, source_path_id, node_id, history_generation, sequence, kind, family,
    public_address, local_interface, local_address,
    proxy_path, likely_nat, temporary, failure_reason, observed_at, received_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (id) DO NOTHING;

-- name: GetAddressEvent :one
SELECT id, public_address_id, source_path_id, node_id, history_generation, sequence, kind, family,
       public_address, local_interface, local_address,
       proxy_path, likely_nat, temporary, failure_reason, observed_at,
       received_at
FROM address_events
WHERE id = ?;

-- name: GetAddressEventBySequence :one
SELECT id, public_address_id, source_path_id, node_id, history_generation, sequence, kind, family,
       public_address, local_interface, local_address,
       proxy_path, likely_nat, temporary, failure_reason, observed_at,
       received_at
FROM address_events
WHERE source_path_id = ? AND history_generation = ? AND sequence = ?;

-- name: UpsertAddressGap :execrows
INSERT INTO history_gaps (
    id, egress_id, node_id, history_generation, kind, dropped_count,
    first_sequence, last_sequence, first_observed_at, last_observed_at,
    received_at
) VALUES (?, ?, ?, ?, 'address', ?, ?, ?, ?, ?, ?)
ON CONFLICT (id) DO UPDATE SET
    dropped_count = MAX(history_gaps.dropped_count, excluded.dropped_count),
    last_sequence = MAX(history_gaps.last_sequence, excluded.last_sequence),
    last_observed_at = MAX(history_gaps.last_observed_at, excluded.last_observed_at),
    received_at = excluded.received_at
WHERE history_gaps.egress_id = excluded.egress_id
  AND history_gaps.node_id = excluded.node_id
  AND history_gaps.history_generation = excluded.history_generation
  AND history_gaps.first_sequence = excluded.first_sequence
  AND history_gaps.first_observed_at = excluded.first_observed_at;

-- name: ListNodeAddressStates :many
SELECT egress_id, node_id, history_generation, family, status, sequence,
       public_address, local_interface, local_address, proxy_path, likely_nat,
       temporary, failure_reason, last_checked_at, last_succeeded_at,
       last_changed_at, received_at
FROM address_states
WHERE node_id = ?
ORDER BY egress_id;

-- name: ListNodeAddressEvents :many
SELECT id, public_address_id, source_path_id, node_id, history_generation, sequence, kind, family,
       public_address, local_interface, local_address,
       proxy_path, likely_nat, temporary, failure_reason, observed_at,
       received_at
FROM address_events
WHERE node_id = ?
ORDER BY observed_at DESC, sequence DESC, id
LIMIT ?;

-- name: ListNodeAddressGaps :many
SELECT id, egress_id, node_id, history_generation, kind, dropped_count,
       first_sequence, last_sequence, first_observed_at, last_observed_at,
       received_at
FROM history_gaps
WHERE node_id = ?
ORDER BY last_observed_at DESC, id
LIMIT ?;

-- name: DeleteNodeAddressStates :exec
DELETE FROM address_states WHERE node_id = ?;

-- name: DeleteNodeAddressEvents :exec
DELETE FROM address_events WHERE node_id = ?;

-- name: DeleteNodeAddressGaps :exec
DELETE FROM history_gaps WHERE node_id = ?;

-- name: DeleteEgressAddressStates :exec
DELETE FROM address_states WHERE egress_id = ?;

-- name: DeleteEgressAddressGaps :exec
DELETE FROM history_gaps WHERE egress_id = ?;

-- name: UpdateHistoryGeneration :execrows
UPDATE history_metadata
SET generation = ?, created_at = ?
WHERE id = 1;

-- name: CreateProbeRun :execrows
INSERT INTO probe_runs (
    id, node_id, history_generation, configuration_revision, trigger,
    task_id, status, expected_executions,
    started_at, completed_at, received_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (id) DO NOTHING;

-- name: GetProbeRun :one
SELECT id, node_id, history_generation, configuration_revision, trigger,
       task_id, status, expected_executions,
       started_at, completed_at, received_at
FROM probe_runs
WHERE id = ?;

-- name: CompleteProbeRun :execrows
UPDATE probe_runs
SET status = ?, completed_at = ?, received_at = ?
WHERE id = ? AND status = 'running';

-- name: CreateProbeExecution :execrows
INSERT INTO probe_executions (
    id, run_id, egress_id, ordinal, sequence, status, started_at,
    completed_at, failure_stage, diagnostic, received_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (id) DO NOTHING;

-- name: GetProbeExecution :one
SELECT id, run_id, egress_id, ordinal, sequence, status, started_at,
       completed_at, failure_stage, diagnostic, received_at
FROM probe_executions
WHERE id = ?;

-- name: UpdateProbeExecution :execrows
UPDATE probe_executions
SET status = ?, started_at = ?, completed_at = ?, failure_stage = ?,
    diagnostic = ?, received_at = ?
WHERE id = ? AND status IN ('pending', 'running');

-- name: CreateProbeSnapshot :execrows
INSERT INTO probe_snapshots (
    id, execution_id, egress_id, sequence, observed_at, raw_result,
    encoded_size, received_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (id) DO NOTHING;

-- name: GetProbeSnapshotByExecution :one
SELECT id, execution_id, egress_id, sequence, observed_at, raw_result,
       encoded_size, received_at
FROM probe_snapshots
WHERE execution_id = ?;

-- name: GetProbeSnapshot :one
SELECT id, execution_id, egress_id, sequence, observed_at, raw_result,
       encoded_size, received_at
FROM probe_snapshots
WHERE id = ?;

-- name: GetPreviousProbeSnapshotID :one
SELECT id
FROM probe_snapshots
WHERE egress_id = ? AND sequence < ?
ORDER BY sequence DESC
LIMIT 1;

-- name: GetProbeExecutionByEgressSequence :one
SELECT id, run_id, egress_id, ordinal, sequence, status, started_at,
       completed_at, failure_stage, diagnostic, received_at
FROM probe_executions
WHERE egress_id = ? AND sequence = ?;

-- name: GetProbeGapCoveringSequence :one
SELECT id, egress_id, node_id, history_generation, dropped_count,
       first_sequence, last_sequence, first_observed_at, last_observed_at,
       received_at
FROM probe_gaps
WHERE egress_id = ? AND history_generation = ?
  AND first_sequence <= ? AND last_sequence >= ?
ORDER BY last_sequence DESC, id
LIMIT 1;

-- name: UpsertCurrentProbeSnapshot :exec
INSERT INTO current_probe_snapshots (
    egress_id, execution_id, snapshot_id, sequence, observed_at, received_at
) VALUES (?, ?, ?, ?, ?, ?)
ON CONFLICT (egress_id) DO UPDATE SET
    execution_id = excluded.execution_id,
    snapshot_id = excluded.snapshot_id,
    sequence = excluded.sequence,
    observed_at = excluded.observed_at,
    received_at = excluded.received_at
WHERE excluded.sequence > current_probe_snapshots.sequence;

-- name: GetProbeComparisonProgress :one
SELECT egress_id, node_id, history_generation, next_sequence,
       last_success_snapshot_id, updated_at
FROM probe_comparison_progress
WHERE egress_id = ?;

-- name: ListCurrentProbeSnapshots :many
SELECT egress_id, snapshot_id, observed_at
FROM current_probe_snapshots
ORDER BY egress_id;

-- name: ListOverviewCurrentProbeStates :many
SELECT identity.egress_id, current.snapshot_id, current.observed_at,
       format.status AS format_status,
       outcome.status AS latest_probe_outcome,
       outcome.last_observed_at AS latest_probe_at,
       outcome_execution.run_id AS latest_probe_run_id
FROM (
    SELECT egress_id FROM current_probe_snapshots
    UNION
    SELECT egress_id FROM probe_outcome_states
) identity
LEFT JOIN current_probe_snapshots current ON current.egress_id = identity.egress_id
LEFT JOIN probe_format_states format ON format.egress_id = identity.egress_id
LEFT JOIN probe_outcome_states outcome ON outcome.egress_id = identity.egress_id
LEFT JOIN probe_executions outcome_execution ON outcome_execution.id = outcome.execution_id
ORDER BY identity.egress_id;

-- name: ListOverviewLatestNodeProbeRuns :many
SELECT run.id, run.node_id, run.trigger, run.started_at, run.completed_at,
       run.status, run.expected_executions,
       CAST(SUM(CASE WHEN execution.status NOT IN('pending', 'running') THEN 1 ELSE 0 END) AS INTEGER) AS completed_executions
FROM probe_runs run
LEFT JOIN probe_executions execution ON execution.run_id = run.id
WHERE NOT EXISTS (
    SELECT 1
    FROM probe_runs newer
    WHERE newer.node_id = run.node_id
      AND(newer.started_at > run.started_at OR(newer.started_at = run.started_at AND newer.id > run.id))
)
GROUP BY run.id
ORDER BY run.node_id;

-- name: ListOverviewRecentProbeRuns :many
SELECT run.id, run.node_id, run.trigger, run.started_at, run.completed_at,
       run.status, run.expected_executions,
       CAST(SUM(CASE WHEN execution.status NOT IN('pending', 'running') THEN 1 ELSE 0 END) AS INTEGER) AS completed_executions
FROM probe_runs run
LEFT JOIN probe_executions execution ON execution.run_id = run.id
GROUP BY run.id
ORDER BY run.started_at DESC, run.id DESC
LIMIT 8;

-- name: UpsertProbeComparisonProgress :exec
INSERT INTO probe_comparison_progress (
    egress_id, node_id, history_generation, next_sequence,
    last_success_snapshot_id, updated_at
) VALUES (?, ?, ?, ?, ?, ?)
ON CONFLICT (egress_id) DO UPDATE SET
    node_id = excluded.node_id,
    history_generation = excluded.history_generation,
    next_sequence = excluded.next_sequence,
    last_success_snapshot_id = excluded.last_success_snapshot_id,
    updated_at = excluded.updated_at;

-- name: CreateProbeChangeSet :execrows
INSERT INTO probe_change_sets (
    id, execution_id, snapshot_id, egress_id, sequence,
    previous_snapshot_id, baseline, change_count, observed_at, recorded_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (execution_id) DO NOTHING;

-- name: GetProbeChangeSetBySnapshot :one
SELECT id, execution_id, snapshot_id, egress_id, sequence,
       previous_snapshot_id, baseline, change_count, observed_at, recorded_at
FROM probe_change_sets
WHERE snapshot_id = ?;

-- name: CreateProbeFieldChange :execrows
INSERT INTO probe_field_changes (
    change_set_id, field_id, group_name, json_path, value_type,
    before_value, after_value
) VALUES (?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (change_set_id, field_id) DO NOTHING;

-- name: ListProbeFieldChanges :many
SELECT change_set_id, field_id, group_name, json_path, value_type,
       before_value, after_value
FROM probe_field_changes
WHERE change_set_id = ?
ORDER BY group_name, json_path;

-- name: CreateProbeSnapshotFormat :execrows
INSERT INTO probe_snapshot_formats (
    snapshot_id, status, signature, issue_count, issues_json
) VALUES (?, ?, ?, ?, ?)
ON CONFLICT (snapshot_id) DO NOTHING;

-- name: GetProbeSnapshotFormat :one
SELECT snapshot_id, status, signature, issue_count, issues_json
FROM probe_snapshot_formats
WHERE snapshot_id = ?;

-- name: GetProbeFormatState :one
SELECT egress_id, snapshot_id, sequence, status, signature, issue_count,
       issues_json, first_observed_at, last_observed_at, updated_at
FROM probe_format_states
WHERE egress_id = ?;

-- name: UpsertProbeFormatState :exec
INSERT INTO probe_format_states (
    egress_id, snapshot_id, sequence, status, signature, issue_count,
    issues_json, first_observed_at, last_observed_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (egress_id) DO UPDATE SET
    snapshot_id = excluded.snapshot_id,
    sequence = excluded.sequence,
    status = excluded.status,
    signature = excluded.signature,
    issue_count = excluded.issue_count,
    issues_json = excluded.issues_json,
    first_observed_at = excluded.first_observed_at,
    last_observed_at = excluded.last_observed_at,
    updated_at = excluded.updated_at
WHERE excluded.sequence > probe_format_states.sequence;

-- name: CreateProbeFormatEvent :execrows
INSERT INTO probe_format_events (
    id, execution_id, snapshot_id, egress_id, sequence, kind,
    previous_signature, current_signature, issue_count, issues_json,
    observed_at, recorded_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (execution_id) DO NOTHING;

-- name: StarProbeSnapshot :execrows
INSERT INTO probe_snapshot_stars (snapshot_id, starred_at)
VALUES (?, ?)
ON CONFLICT (snapshot_id) DO NOTHING;

-- name: UnstarProbeSnapshot :execrows
DELETE FROM probe_snapshot_stars WHERE snapshot_id = ?;

-- name: IsProbeSnapshotStarred :one
SELECT EXISTS(
    SELECT 1 FROM probe_snapshot_stars WHERE snapshot_id = ?
) AS starred;

-- name: ListProbeSnapshots :many
SELECT s.id, s.execution_id, s.egress_id, s.sequence, s.observed_at,
       s.encoded_size, s.received_at, e.run_id, r.node_id, r.trigger, r.status AS run_status,
       CAST(COALESCE((
           SELECT previous.id
           FROM probe_snapshots previous
           WHERE previous.egress_id = s.egress_id AND previous.sequence < s.sequence
           ORDER BY previous.sequence DESC
           LIMIT 1
       ), '') AS TEXT) AS previous_snapshot_id,
       EXISTS(SELECT 1 FROM probe_snapshot_stars star WHERE star.snapshot_id = s.id) AS starred,
       EXISTS(
           SELECT 1 FROM current_probe_snapshots current_snapshot
           WHERE current_snapshot.snapshot_id = s.id
       ) AS is_current,
       changes.id IS NOT NULL AS processed,
       COALESCE(changes.baseline, 0) AS baseline,
       COALESCE(changes.change_count, 0) AS change_count,
       COALESCE(format.status, 'mismatch') AS format_status,
       COALESCE(format.issue_count, 1) AS format_issue_count
FROM probe_snapshots s
JOIN probe_executions e ON e.id = s.execution_id
JOIN probe_runs r ON r.id = e.run_id
LEFT JOIN probe_change_sets changes ON changes.snapshot_id = s.id
LEFT JOIN probe_snapshot_formats format ON format.snapshot_id = s.id
WHERE (sqlc.arg(node_id) = '' OR r.node_id = sqlc.arg(node_id))
  AND (sqlc.arg(egress_id) = '' OR s.egress_id = sqlc.arg(egress_id))
  AND (sqlc.narg(from_observed_at) IS NULL OR s.observed_at >= sqlc.narg(from_observed_at))
  AND (sqlc.narg(to_observed_at) IS NULL OR s.observed_at <= sqlc.narg(to_observed_at))
  AND (sqlc.arg(run_status) = '' OR r.status = sqlc.arg(run_status))
  AND (sqlc.arg(trigger) = '' OR r.trigger = sqlc.arg(trigger))
  AND (
      sqlc.narg(changed) IS NULL OR
      (COALESCE(changes.change_count, 0) > 0) = sqlc.narg(changed)
  )
  AND (
      sqlc.arg(format_status) = '' OR
      COALESCE(format.status, 'mismatch') = sqlc.arg(format_status)
  )
ORDER BY s.observed_at DESC, s.id DESC
LIMIT sqlc.arg(page_size) OFFSET sqlc.arg(page_offset);

-- name: CountProbeSnapshots :one
SELECT COUNT(*)
FROM probe_snapshots s
JOIN probe_executions e ON e.id = s.execution_id
JOIN probe_runs r ON r.id = e.run_id
LEFT JOIN probe_change_sets changes ON changes.snapshot_id = s.id
LEFT JOIN probe_snapshot_formats format ON format.snapshot_id = s.id
WHERE (sqlc.arg(node_id) = '' OR r.node_id = sqlc.arg(node_id))
  AND (sqlc.arg(egress_id) = '' OR s.egress_id = sqlc.arg(egress_id))
  AND (sqlc.narg(from_observed_at) IS NULL OR s.observed_at >= sqlc.narg(from_observed_at))
  AND (sqlc.narg(to_observed_at) IS NULL OR s.observed_at <= sqlc.narg(to_observed_at))
  AND (sqlc.arg(run_status) = '' OR r.status = sqlc.arg(run_status))
  AND (sqlc.arg(trigger) = '' OR r.trigger = sqlc.arg(trigger))
  AND (
      sqlc.narg(changed) IS NULL OR
      (COALESCE(changes.change_count, 0) > 0) = sqlc.narg(changed)
  )
  AND (
      sqlc.arg(format_status) = '' OR
      COALESCE(format.status, 'mismatch') = sqlc.arg(format_status)
  );

-- name: ListGlobalAddressEvents :many
SELECT id, public_address_id, source_path_id, node_id, history_generation, sequence, kind, family,
       public_address, local_interface, local_address,
       proxy_path, likely_nat, temporary, failure_reason, observed_at,
       received_at
FROM address_events
WHERE (sqlc.arg(node_id) = '' OR node_id = sqlc.arg(node_id))
  AND (sqlc.arg(egress_id) = '' OR public_address_id = sqlc.arg(egress_id))
  AND (sqlc.narg(from_observed_at) IS NULL OR observed_at >= sqlc.narg(from_observed_at))
  AND (sqlc.narg(to_observed_at) IS NULL OR observed_at <= sqlc.narg(to_observed_at))
  AND (sqlc.arg(event_kind) = '' OR kind = sqlc.arg(event_kind))
  AND (sqlc.arg(family) = '' OR family = sqlc.arg(family))
ORDER BY observed_at DESC, id DESC
LIMIT sqlc.arg(page_size) OFFSET sqlc.arg(page_offset);

-- name: CountGlobalAddressEvents :one
SELECT COUNT(*) FROM address_events
WHERE (sqlc.arg(node_id) = '' OR node_id = sqlc.arg(node_id))
  AND (sqlc.arg(egress_id) = '' OR public_address_id = sqlc.arg(egress_id))
  AND (sqlc.narg(from_observed_at) IS NULL OR observed_at >= sqlc.narg(from_observed_at))
  AND (sqlc.narg(to_observed_at) IS NULL OR observed_at <= sqlc.narg(to_observed_at))
  AND (sqlc.arg(event_kind) = '' OR kind = sqlc.arg(event_kind))
  AND (sqlc.arg(family) = '' OR family = sqlc.arg(family));

-- name: ListGlobalProbeGaps :many
SELECT id, egress_id, node_id, history_generation, dropped_count,
       first_sequence, last_sequence, first_observed_at, last_observed_at,
       received_at
FROM probe_gaps
WHERE (sqlc.arg(node_id) = '' OR node_id = sqlc.arg(node_id))
  AND (sqlc.arg(egress_id) = '' OR egress_id = sqlc.arg(egress_id))
  AND (sqlc.narg(from_observed_at) IS NULL OR last_observed_at >= sqlc.narg(from_observed_at))
  AND (sqlc.narg(to_observed_at) IS NULL OR last_observed_at <= sqlc.narg(to_observed_at))
ORDER BY last_observed_at DESC, id DESC
LIMIT sqlc.arg(page_size) OFFSET sqlc.arg(page_offset);

-- name: CountGlobalProbeGaps :one
SELECT COUNT(*) FROM probe_gaps
WHERE (sqlc.arg(node_id) = '' OR node_id = sqlc.arg(node_id))
  AND (sqlc.arg(egress_id) = '' OR egress_id = sqlc.arg(egress_id))
  AND (sqlc.narg(from_observed_at) IS NULL OR last_observed_at >= sqlc.narg(from_observed_at))
  AND (sqlc.narg(to_observed_at) IS NULL OR last_observed_at <= sqlc.narg(to_observed_at));

-- name: ListGlobalFormatEvents :many
SELECT f.id, f.execution_id, f.snapshot_id, f.egress_id, f.sequence, f.kind,
       f.previous_signature, f.current_signature, f.issue_count, f.issues_json,
       f.observed_at, f.recorded_at, r.node_id
FROM probe_format_events f
JOIN probe_executions e ON e.id = f.execution_id
JOIN probe_runs r ON r.id = e.run_id
WHERE (sqlc.arg(node_id) = '' OR r.node_id = sqlc.arg(node_id))
  AND (sqlc.arg(egress_id) = '' OR f.egress_id = sqlc.arg(egress_id))
  AND (sqlc.narg(from_observed_at) IS NULL OR f.observed_at >= sqlc.narg(from_observed_at))
  AND (sqlc.narg(to_observed_at) IS NULL OR f.observed_at <= sqlc.narg(to_observed_at))
ORDER BY f.observed_at DESC, f.id DESC
LIMIT sqlc.arg(page_size) OFFSET sqlc.arg(page_offset);

-- name: CountGlobalFormatEvents :one
SELECT COUNT(*)
FROM probe_format_events f
JOIN probe_executions e ON e.id = f.execution_id
JOIN probe_runs r ON r.id = e.run_id
WHERE (sqlc.arg(node_id) = '' OR r.node_id = sqlc.arg(node_id))
  AND (sqlc.arg(egress_id) = '' OR f.egress_id = sqlc.arg(egress_id))
  AND (sqlc.narg(from_observed_at) IS NULL OR f.observed_at >= sqlc.narg(from_observed_at))
  AND (sqlc.narg(to_observed_at) IS NULL OR f.observed_at <= sqlc.narg(to_observed_at));

-- name: ListGlobalAddressGaps :many
SELECT id, egress_id, node_id, history_generation, kind, dropped_count,
       first_sequence, last_sequence, first_observed_at, last_observed_at,
       received_at
FROM history_gaps
WHERE (sqlc.arg(node_id) = '' OR node_id = sqlc.arg(node_id))
  AND (sqlc.arg(egress_id) = '' OR egress_id = sqlc.arg(egress_id))
  AND (sqlc.narg(from_observed_at) IS NULL OR last_observed_at >= sqlc.narg(from_observed_at))
  AND (sqlc.narg(to_observed_at) IS NULL OR last_observed_at <= sqlc.narg(to_observed_at))
ORDER BY last_observed_at DESC, id DESC
LIMIT sqlc.arg(page_size) OFFSET sqlc.arg(page_offset);

-- name: CountGlobalAddressGaps :one
SELECT COUNT(*) FROM history_gaps
WHERE (sqlc.arg(node_id) = '' OR node_id = sqlc.arg(node_id))
  AND (sqlc.arg(egress_id) = '' OR egress_id = sqlc.arg(egress_id))
  AND (sqlc.narg(from_observed_at) IS NULL OR last_observed_at >= sqlc.narg(from_observed_at))
  AND (sqlc.narg(to_observed_at) IS NULL OR last_observed_at <= sqlc.narg(to_observed_at));

-- name: GetHistoryLogicalUsage :one
SELECT
    CAST(COALESCE(SUM(logical_bytes), 0) AS INTEGER) AS logical_bytes,
    CAST(COALESCE(SUM(record_count), 0) AS INTEGER) AS record_count
FROM (
    SELECT SUM(encoded_size + 160) AS logical_bytes, COUNT(*) AS record_count FROM probe_snapshots
    UNION ALL SELECT SUM(
        256 + length(id) + length(run_id) + length(egress_id) +
        COALESCE(length(failure_stage), 0) + COALESCE(length(CAST(diagnostic AS BLOB)), 0)
    ), COUNT(*) FROM probe_executions
    UNION ALL SELECT SUM(
        256 + length(id) + length(node_id) + length(history_generation) +
        length(trigger) + COALESCE(length(task_id), 0)
    ), COUNT(*) FROM probe_runs
    UNION ALL SELECT SUM(
        256 + length(id) + length(public_address_id) + length(source_path_id) + length(node_id) +
        COALESCE(length(public_address), 0) +
        COALESCE(length(local_interface), 0) + COALESCE(length(local_address), 0) +
        COALESCE(length(proxy_path), 0) + COALESCE(length(failure_reason), 0)
    ), COUNT(*) FROM address_events
    UNION ALL SELECT COUNT(*) * 224, COUNT(*) FROM history_gaps
    UNION ALL SELECT COUNT(*) * 224, COUNT(*) FROM probe_gaps
    UNION ALL SELECT SUM(192 + length(issues_json)), COUNT(*) FROM probe_snapshot_formats
    UNION ALL SELECT SUM(256 + length(issues_json)), COUNT(*) FROM probe_format_states
    UNION ALL SELECT SUM(256 + length(issues_json)), COUNT(*) FROM probe_format_events
    UNION ALL SELECT COUNT(*) * 256, COUNT(*) FROM probe_change_sets
    UNION ALL SELECT SUM(
        160 + length(field_id) + length(group_name) + length(json_path) +
        length(CAST(before_value AS BLOB)) + length(CAST(after_value AS BLOB))
    ), COUNT(*) FROM probe_field_changes
    UNION ALL SELECT COUNT(*) * 96, COUNT(*) FROM probe_snapshot_stars
    UNION ALL SELECT COUNT(*) * 192, COUNT(*) FROM probe_comparison_progress
    UNION ALL SELECT COUNT(*) * 160, COUNT(*) FROM current_probe_snapshots
    UNION ALL SELECT COUNT(*) * 224, COUNT(*) FROM probe_outcome_states
    UNION ALL SELECT SUM(
        256 + length(id) + length(event_type) + length(source_kind) + length(source_id) +
        COALESCE(length(node_id), 0) + COALESCE(length(egress_id), 0) + length(payload_json)
    ), COUNT(*) FROM notification_events
    UNION ALL SELECT SUM(
        512 + length(id) + length(event_id) + length(sender_id) + length(sender_name) +
        length(sender_kind) + length(event_type) + length(matched_rule_ids_json) +
        length(event_json) + length(CAST(title AS BLOB)) + length(CAST(body AS BLOB)) +
        COALESCE(length(error_code), 0)
    ), COUNT(*) FROM notification_deliveries
    UNION ALL SELECT SUM(
        256 + COALESCE(length(public_address), 0) + COALESCE(length(local_interface), 0) +
        COALESCE(length(local_address), 0) + COALESCE(length(proxy_path), 0) +
        COALESCE(length(failure_reason), 0)
    ), COUNT(*) FROM address_states
);

-- name: GetProtectedHistoryLogicalBytes :one
WITH protected_snapshots AS (
    SELECT s.id, s.execution_id, s.encoded_size
    FROM probe_snapshots s
    JOIN probe_executions e ON e.id = s.execution_id
    JOIN probe_runs r ON r.id = e.run_id
    WHERE r.status = 'running'
       OR e.status IN ('pending', 'running')
       OR EXISTS(SELECT 1 FROM probe_snapshot_stars star WHERE star.snapshot_id = s.id)
       OR EXISTS(SELECT 1 FROM current_probe_snapshots current WHERE current.snapshot_id = s.id)
       OR EXISTS(SELECT 1 FROM probe_format_states state WHERE state.snapshot_id = s.id)
       OR EXISTS(
           SELECT 1 FROM probe_comparison_progress progress
           WHERE progress.last_success_snapshot_id = s.id
       )
), protected_executions AS (
    SELECT e.id, e.run_id, e.egress_id, e.failure_stage, e.diagnostic
    FROM probe_executions e
    JOIN probe_runs r ON r.id = e.run_id
    WHERE r.status = 'running'
       OR e.status IN ('pending', 'running')
       OR EXISTS(SELECT 1 FROM protected_snapshots snapshot WHERE snapshot.execution_id = e.id)
       OR EXISTS(SELECT 1 FROM probe_outcome_states outcome WHERE outcome.execution_id = e.id)
), protected_runs AS (
    SELECT r.id, r.node_id, r.history_generation, r.trigger, r.task_id
    FROM probe_runs r
    WHERE r.status = 'running'
       OR EXISTS(SELECT 1 FROM protected_executions execution WHERE execution.run_id = r.id)
)
SELECT CAST(COALESCE(SUM(logical_bytes), 0) AS INTEGER) AS logical_bytes
FROM (
    SELECT SUM(encoded_size + 160) AS logical_bytes FROM protected_snapshots
    UNION ALL
    SELECT SUM(
        256 + length(id) + length(run_id) + length(egress_id) +
        COALESCE(length(failure_stage), 0) + COALESCE(length(CAST(diagnostic AS BLOB)), 0)
    ) FROM protected_executions
    UNION ALL
    SELECT SUM(
        256 + length(id) + length(node_id) + length(history_generation) +
        length(trigger) + COALESCE(length(task_id), 0)
    ) FROM protected_runs
    UNION ALL
    SELECT SUM(192 + length(format.issues_json))
    FROM probe_snapshot_formats format
    JOIN protected_snapshots snapshot ON snapshot.id = format.snapshot_id
    UNION ALL
    SELECT SUM(256 + length(event.issues_json))
    FROM probe_format_events event
    JOIN protected_snapshots snapshot ON snapshot.id = event.snapshot_id
    UNION ALL
    SELECT COUNT(*) * 256
    FROM probe_change_sets change_set
    JOIN protected_snapshots snapshot ON snapshot.id = change_set.snapshot_id
    UNION ALL
    SELECT SUM(
        160 + length(change.field_id) + length(change.group_name) + length(change.json_path) +
        length(CAST(change.before_value AS BLOB)) + length(CAST(change.after_value AS BLOB))
    )
    FROM probe_field_changes change
    JOIN probe_change_sets change_set ON change_set.id = change.change_set_id
    JOIN protected_snapshots snapshot ON snapshot.id = change_set.snapshot_id
    UNION ALL
    SELECT COUNT(*) * 96 FROM probe_snapshot_stars
    UNION ALL
    SELECT COUNT(*) * 160 FROM current_probe_snapshots
    UNION ALL
    SELECT SUM(256 + length(issues_json)) FROM probe_format_states
    UNION ALL
    SELECT SUM(
        256 + COALESCE(length(public_address), 0) + COALESCE(length(local_interface), 0) +
        COALESCE(length(local_address), 0) + COALESCE(length(proxy_path), 0) +
        COALESCE(length(failure_reason), 0)
    ) FROM address_states
    UNION ALL
    SELECT COUNT(*) * 192 FROM probe_comparison_progress
    UNION ALL
    SELECT COUNT(*) * 224 FROM probe_outcome_states
    UNION ALL
    SELECT SUM(
        256 + length(event.id) + length(event.event_type) + length(event.source_kind) +
        length(event.source_id) + COALESCE(length(event.node_id), 0) +
        COALESCE(length(event.egress_id), 0) + length(event.payload_json)
    )
    FROM notification_events event
    WHERE event.processed_at IS NULL OR EXISTS(
        SELECT 1 FROM notification_deliveries delivery
        WHERE delivery.event_id = event.id
          AND delivery.status IN ('pending', 'running', 'retrying')
    )
    UNION ALL
    SELECT SUM(
        512 + length(id) + length(event_id) + length(sender_id) + length(sender_name) +
        length(sender_kind) + length(event_type) + length(matched_rule_ids_json) +
        length(event_json) + length(CAST(title AS BLOB)) + length(CAST(body AS BLOB)) +
        COALESCE(length(error_code), 0)
    )
    FROM notification_deliveries
    WHERE status IN ('pending', 'running', 'retrying')
);

-- name: ListRetentionCandidates :many
SELECT category, id, observed_at, logical_bytes
FROM (
    SELECT 'execution' AS category, e.id AS id,
           COALESCE(e.completed_at, e.started_at, e.received_at) AS observed_at,
           256 + length(e.id) + length(e.run_id) + length(e.egress_id) +
           COALESCE(length(e.failure_stage), 0) + COALESCE(length(CAST(e.diagnostic AS BLOB)), 0) +
           COALESCE(s.encoded_size + 160, 0) AS logical_bytes
    FROM probe_executions e
    JOIN probe_runs r ON r.id = e.run_id
    LEFT JOIN probe_snapshots s ON s.execution_id = e.id
    WHERE e.status NOT IN ('pending', 'running') AND r.status != 'running'
      AND NOT EXISTS(SELECT 1 FROM probe_snapshot_stars star WHERE star.snapshot_id = s.id)
      AND NOT EXISTS(SELECT 1 FROM current_probe_snapshots current WHERE current.snapshot_id = s.id)
      AND NOT EXISTS(SELECT 1 FROM probe_format_states state WHERE state.snapshot_id = s.id)
      AND NOT EXISTS(
          SELECT 1 FROM probe_comparison_progress progress
          WHERE progress.last_success_snapshot_id = s.id
      )
      AND NOT EXISTS(
          SELECT 1 FROM probe_outcome_states outcome
          WHERE outcome.execution_id = e.id
      )
    UNION ALL
    SELECT 'address-event', id, observed_at,
           256 + length(id) + length(public_address_id) + length(source_path_id) + length(node_id) +
           COALESCE(length(public_address), 0) +
           COALESCE(length(local_interface), 0) + COALESCE(length(local_address), 0) +
           COALESCE(length(proxy_path), 0) + COALESCE(length(failure_reason), 0)
    FROM address_events
    UNION ALL SELECT 'address-gap', id, last_observed_at, 224 FROM history_gaps
    UNION ALL SELECT 'probe-gap', id, last_observed_at, 224 FROM probe_gaps
    UNION ALL
    SELECT 'notification-event', event.id, event.observed_at,
           256 + length(event.id) + length(event.event_type) + length(event.source_kind) +
           length(event.source_id) + COALESCE(length(event.node_id), 0) +
           COALESCE(length(event.egress_id), 0) + length(event.payload_json) +
           COALESCE((
               SELECT SUM(
                   512 + length(delivery.id) + length(delivery.event_id) +
                   length(delivery.sender_id) + length(delivery.sender_name) +
                   length(delivery.sender_kind) + length(delivery.event_type) +
                   length(delivery.matched_rule_ids_json) + length(delivery.event_json) +
                   length(CAST(delivery.title AS BLOB)) + length(CAST(delivery.body AS BLOB)) +
                   COALESCE(length(delivery.error_code), 0)
               )
               FROM notification_deliveries delivery
               WHERE delivery.event_id = event.id
           ), 0)
    FROM notification_events event
    WHERE event.processed_at IS NOT NULL
      AND NOT EXISTS(
          SELECT 1 FROM notification_deliveries delivery
          WHERE delivery.event_id = event.id
            AND delivery.status IN ('pending', 'running', 'retrying')
      )
)
WHERE (sqlc.narg(older_than) IS NULL OR observed_at < sqlc.narg(older_than))
ORDER BY observed_at, category, id
LIMIT sqlc.arg(page_size);

-- name: DeleteRetentionExecution :execrows
DELETE FROM probe_executions
WHERE probe_executions.id = ? AND probe_executions.status NOT IN ('pending', 'running')
  AND EXISTS(
      SELECT 1 FROM probe_runs run
      WHERE run.id = probe_executions.run_id AND run.status != 'running'
  )
  AND NOT EXISTS(
      SELECT 1 FROM probe_snapshots s
      JOIN probe_snapshot_stars star ON star.snapshot_id = s.id
      WHERE s.execution_id = probe_executions.id
  )
  AND NOT EXISTS(
      SELECT 1 FROM probe_snapshots s
      JOIN current_probe_snapshots current ON current.snapshot_id = s.id
      WHERE s.execution_id = probe_executions.id
  )
  AND NOT EXISTS(
      SELECT 1 FROM probe_snapshots s
      JOIN probe_format_states state ON state.snapshot_id = s.id
      WHERE s.execution_id = probe_executions.id
  )
  AND NOT EXISTS(
      SELECT 1 FROM probe_snapshots s
      JOIN probe_comparison_progress progress ON progress.last_success_snapshot_id = s.id
      WHERE s.execution_id = probe_executions.id
  )
  AND NOT EXISTS(
      SELECT 1 FROM probe_outcome_states outcome
      WHERE outcome.execution_id = probe_executions.id
  );

-- name: DeleteRetentionAddressEvent :execrows
DELETE FROM address_events WHERE id = ?;

-- name: DeleteRetentionAddressGap :execrows
DELETE FROM history_gaps WHERE id = ?;

-- name: DeleteRetentionProbeGap :execrows
DELETE FROM probe_gaps WHERE id = ?;

-- name: DeleteRetentionNotificationEvent :execrows
DELETE FROM notification_events
WHERE notification_events.id = ? AND processed_at IS NOT NULL
  AND NOT EXISTS(
      SELECT 1 FROM notification_deliveries delivery
      WHERE delivery.event_id = notification_events.id
        AND delivery.status IN ('pending', 'running', 'retrying')
  );

-- name: DeleteNodeProbeComparisonProgress :exec
DELETE FROM probe_comparison_progress WHERE node_id = ?;

-- name: ResetProbeComparisonProgress :exec
DELETE FROM probe_comparison_progress;

-- name: ListNodeProbeRuns :many
SELECT id, node_id, history_generation, configuration_revision, trigger,
       task_id, status, expected_executions,
       started_at, completed_at, received_at
FROM probe_runs
WHERE node_id = ?
ORDER BY started_at DESC, id DESC
LIMIT ?;

-- name: ListProbeRunExecutions :many
SELECT id, run_id, egress_id, ordinal, sequence, status, started_at,
       completed_at, failure_stage, diagnostic, received_at
FROM probe_executions
WHERE run_id = ?
ORDER BY ordinal, id;

-- name: UpsertProbeGap :execrows
INSERT INTO probe_gaps (
    id, egress_id, node_id, history_generation, dropped_count,
    first_sequence, last_sequence, first_observed_at, last_observed_at,
    received_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (id) DO UPDATE SET
    dropped_count = MAX(probe_gaps.dropped_count, excluded.dropped_count),
    last_sequence = MAX(probe_gaps.last_sequence, excluded.last_sequence),
    last_observed_at = MAX(probe_gaps.last_observed_at, excluded.last_observed_at),
    received_at = excluded.received_at
WHERE probe_gaps.egress_id = excluded.egress_id
  AND probe_gaps.node_id = excluded.node_id
  AND probe_gaps.history_generation = excluded.history_generation
  AND probe_gaps.first_sequence = excluded.first_sequence
  AND probe_gaps.first_observed_at = excluded.first_observed_at;

-- name: DeleteNodeProbeHistory :exec
DELETE FROM probe_runs WHERE node_id = ?;

-- name: DeleteNodeProbeGaps :exec
DELETE FROM probe_gaps WHERE node_id = ?;

-- name: DeleteEmptyProbeRuns :exec
DELETE FROM probe_runs
WHERE NOT EXISTS (
    SELECT 1 FROM probe_executions e WHERE e.run_id = probe_runs.id
);

-- name: ResetProbeHistory :exec
DELETE FROM probe_runs;

-- name: ResetHistoryNodes :exec
DELETE FROM history_nodes;

-- name: ResetProbeGaps :exec
DELETE FROM probe_gaps;

-- name: ResetAddressHistory :exec
DELETE FROM address_events;

-- name: ResetAddressStates :exec
DELETE FROM address_states;

-- name: ResetAddressGaps :exec
DELETE FROM history_gaps;

-- name: ResetNotificationHistory :exec
DELETE FROM notification_events;

-- name: DeleteOrphanHistoryNodes :exec
DELETE FROM history_nodes
WHERE NOT EXISTS(SELECT 1 FROM address_states WHERE node_id = history_nodes.node_id)
  AND NOT EXISTS(SELECT 1 FROM address_events WHERE node_id = history_nodes.node_id)
  AND NOT EXISTS(SELECT 1 FROM history_gaps WHERE node_id = history_nodes.node_id)
  AND NOT EXISTS(SELECT 1 FROM probe_runs WHERE node_id = history_nodes.node_id)
  AND NOT EXISTS(SELECT 1 FROM probe_gaps WHERE node_id = history_nodes.node_id)
  AND NOT EXISTS(SELECT 1 FROM probe_comparison_progress WHERE node_id = history_nodes.node_id)
  AND NOT EXISTS(SELECT 1 FROM probe_outcome_states WHERE node_id = history_nodes.node_id)
  AND NOT EXISTS(SELECT 1 FROM notification_events WHERE node_id = history_nodes.node_id);

-- name: GetProbeOutcomeState :one
SELECT egress_id, node_id, history_generation, execution_id, sequence,
       status, first_observed_at, last_observed_at, updated_at
FROM probe_outcome_states
WHERE egress_id = ?;

-- name: UpsertProbeOutcomeState :exec
INSERT INTO probe_outcome_states (
    egress_id, node_id, history_generation, execution_id, sequence,
    status, first_observed_at, last_observed_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (egress_id) DO UPDATE SET
    node_id = excluded.node_id,
    history_generation = excluded.history_generation,
    execution_id = excluded.execution_id,
    sequence = excluded.sequence,
    status = excluded.status,
    first_observed_at = excluded.first_observed_at,
    last_observed_at = excluded.last_observed_at,
    updated_at = excluded.updated_at
WHERE excluded.history_generation != probe_outcome_states.history_generation
   OR excluded.sequence > probe_outcome_states.sequence;

-- name: CreateNotificationEvent :execrows
INSERT INTO notification_events (
    id, event_type, source_kind, source_id, node_id, egress_id,
    payload_json, observed_at, recorded_at, processed_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (source_kind, source_id, event_type) DO NOTHING;

-- name: GetNotificationEvent :one
SELECT id, event_type, source_kind, source_id, node_id, egress_id,
       payload_json, observed_at, recorded_at, processed_at
FROM notification_events
WHERE id = ?;

-- name: ListPendingNotificationEvents :many
SELECT id, event_type, source_kind, source_id, node_id, egress_id,
       payload_json, observed_at, recorded_at, processed_at
FROM notification_events
WHERE processed_at IS NULL
ORDER BY recorded_at, id
LIMIT ?;

-- name: MarkNotificationEventProcessed :execrows
UPDATE notification_events
SET processed_at = ?
WHERE id = ? AND processed_at IS NULL;

-- name: CreateNotificationDelivery :execrows
INSERT INTO notification_deliveries (
    id, event_id, sender_id, sender_name, sender_kind, event_type,
    node_id, egress_id, is_test, status, attempt_count, next_attempt_at,
    last_attempt_at, completed_at, error_code, matched_rule_ids_json,
    event_json, title, body, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (event_id, sender_id) DO NOTHING;

-- name: CountActiveNotificationDeliveriesForSender :one
SELECT COUNT(*)
FROM notification_deliveries
WHERE sender_id = ? AND status IN ('pending', 'running', 'retrying');

-- name: RecoverRunningNotificationDeliveries :exec
UPDATE notification_deliveries
SET status = CASE WHEN attempt_count >= 4 THEN 'failed' ELSE 'retrying' END,
    next_attempt_at = CASE WHEN attempt_count >= 4 THEN NULL ELSE ? END,
    completed_at = CASE WHEN attempt_count >= 4 THEN ? ELSE NULL END,
    error_code = CASE WHEN attempt_count >= 4 THEN 'center-restarted' ELSE NULL END,
    updated_at = ?
WHERE status = 'running';

-- name: ClaimNotificationDelivery :execrows
UPDATE notification_deliveries
SET status = 'running', attempt_count = attempt_count + 1,
    next_attempt_at = NULL, last_attempt_at = ?, updated_at = ?
WHERE id = ?
  AND status IN ('pending', 'retrying')
  AND next_attempt_at <= ?
  AND attempt_count < 4;

-- name: GetNotificationDelivery :one
SELECT id, event_id, sender_id, sender_name, sender_kind, event_type,
       node_id, egress_id, is_test, status, attempt_count, next_attempt_at,
       last_attempt_at, completed_at, error_code, matched_rule_ids_json,
       event_json, title, body, created_at, updated_at
FROM notification_deliveries
WHERE id = ?;

-- name: ListReadyFixedNotificationDeliveries :many
SELECT id, event_id, sender_id, sender_name, sender_kind, event_type,
       node_id, egress_id, is_test, status, attempt_count, next_attempt_at,
       last_attempt_at, completed_at, error_code, matched_rule_ids_json,
       event_json, title, body, created_at, updated_at
FROM notification_deliveries
WHERE sender_kind IN ('telegram', 'webhook')
  AND status IN ('pending', 'retrying')
  AND next_attempt_at <= ?
ORDER BY next_attempt_at, created_at, id
LIMIT ?;

-- name: ListReadyJavaScriptNotificationDeliveries :many
SELECT id, event_id, sender_id, sender_name, sender_kind, event_type,
       node_id, egress_id, is_test, status, attempt_count, next_attempt_at,
       last_attempt_at, completed_at, error_code, matched_rule_ids_json,
       event_json, title, body, created_at, updated_at
FROM notification_deliveries
WHERE sender_kind = 'javascript'
  AND status IN ('pending', 'retrying')
  AND next_attempt_at <= ?
ORDER BY next_attempt_at, created_at, id
LIMIT 1;

-- name: CompleteNotificationDelivery :execrows
UPDATE notification_deliveries
SET status = 'succeeded', completed_at = ?, error_code = NULL, updated_at = ?
WHERE id = ? AND status = 'running';

-- name: RetryNotificationDelivery :execrows
UPDATE notification_deliveries
SET status = 'retrying', next_attempt_at = ?, error_code = NULL, updated_at = ?
WHERE id = ? AND status = 'running' AND attempt_count < 4;

-- name: FailNotificationDelivery :execrows
UPDATE notification_deliveries
SET status = 'failed', next_attempt_at = NULL, completed_at = ?,
    error_code = ?, updated_at = ?
WHERE id = ? AND status = 'running';

-- name: ListNotificationDeliveries :many
SELECT id, event_id, sender_id, sender_name, sender_kind, event_type,
       node_id, egress_id, is_test, status, attempt_count, next_attempt_at,
       last_attempt_at, completed_at, error_code, matched_rule_ids_json,
       event_json, title, body, created_at, updated_at
FROM notification_deliveries
WHERE (sqlc.arg(sender_id) = '' OR sender_id = sqlc.arg(sender_id))
  AND (sqlc.arg(delivery_status) = '' OR status = sqlc.arg(delivery_status))
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg(page_size) OFFSET sqlc.arg(page_offset);

-- name: CountNotificationDeliveries :one
SELECT COUNT(*)
FROM notification_deliveries
WHERE (sqlc.arg(sender_id) = '' OR sender_id = sqlc.arg(sender_id))
  AND (sqlc.arg(delivery_status) = '' OR status = sqlc.arg(delivery_status));

-- name: DeleteNodeNotificationHistory :exec
DELETE FROM notification_events WHERE node_id = ?;

-- name: DeleteNodeAddressNotificationHistory :exec
DELETE FROM notification_events
WHERE node_id = ? AND egress_id IS NULL;

-- name: DeleteProbeOutcomeState :exec
DELETE FROM probe_outcome_states WHERE egress_id = ?;

-- name: DeleteNodeProbeOutcomeStates :exec
DELETE FROM probe_outcome_states WHERE node_id = ?;
