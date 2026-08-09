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
