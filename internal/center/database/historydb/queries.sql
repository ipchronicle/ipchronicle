-- name: GetHistoryMetadata :one
SELECT id, generation, created_at
FROM history_metadata
WHERE id = 1;

-- name: CreateHistoryMetadata :exec
INSERT INTO history_metadata (id, generation, created_at)
VALUES (1, ?, ?);

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
    id, egress_id, node_id, history_generation, sequence, kind, family,
    previous_address, public_address, local_interface, local_address,
    proxy_path, likely_nat, temporary, failure_reason, observed_at, received_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (id) DO NOTHING;

-- name: GetAddressEvent :one
SELECT id, egress_id, node_id, history_generation, sequence, kind, family,
       previous_address, public_address, local_interface, local_address,
       proxy_path, likely_nat, temporary, failure_reason, observed_at,
       received_at
FROM address_events
WHERE id = ?;

-- name: GetAddressEventBySequence :one
SELECT id, egress_id, node_id, history_generation, sequence, kind, family,
       previous_address, public_address, local_interface, local_address,
       proxy_path, likely_nat, temporary, failure_reason, observed_at,
       received_at
FROM address_events
WHERE egress_id = ? AND history_generation = ? AND sequence = ?;

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
SELECT id, egress_id, node_id, history_generation, sequence, kind, family,
       previous_address, public_address, local_interface, local_address,
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

-- name: DeleteEgressAddressEvents :exec
DELETE FROM address_events WHERE egress_id = ?;

-- name: DeleteEgressAddressGaps :exec
DELETE FROM history_gaps WHERE egress_id = ?;

-- name: UpdateHistoryGeneration :execrows
UPDATE history_metadata
SET generation = ?, created_at = ?
WHERE id = 1;

-- name: CreateProbeRun :execrows
INSERT INTO probe_runs (
    id, node_id, history_generation, configuration_revision, trigger,
    task_id, triggering_egress_id, status, expected_executions,
    started_at, completed_at, received_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (id) DO NOTHING;

-- name: GetProbeRun :one
SELECT id, node_id, history_generation, configuration_revision, trigger,
       task_id, triggering_egress_id, status, expected_executions,
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

-- name: ListNodeProbeRuns :many
SELECT id, node_id, history_generation, configuration_revision, trigger,
       task_id, triggering_egress_id, status, expected_executions,
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

-- name: DeleteEgressProbeSnapshots :exec
DELETE FROM probe_snapshots WHERE egress_id = ?;

-- name: DeleteEgressProbeExecutions :exec
DELETE FROM probe_executions WHERE egress_id = ?;

-- name: DeleteEgressProbeGaps :exec
DELETE FROM probe_gaps WHERE egress_id = ?;

-- name: DeleteEmptyProbeRuns :exec
DELETE FROM probe_runs
WHERE NOT EXISTS (
    SELECT 1 FROM probe_executions e WHERE e.run_id = probe_runs.id
);

-- name: ResetProbeHistory :exec
DELETE FROM probe_runs;

-- name: ResetProbeGaps :exec
DELETE FROM probe_gaps;

-- name: ResetAddressHistory :exec
DELETE FROM address_events;

-- name: ResetAddressStates :exec
DELETE FROM address_states;

-- name: ResetAddressGaps :exec
DELETE FROM history_gaps;
