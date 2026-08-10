-- name: GetAdministrator :one
SELECT id, username, password_hash, locale, uses_default_credentials,
       totp_secret_encrypted, totp_enabled, totp_last_used_step,
       created_at, credentials_updated_at
FROM administrators
WHERE id = 1;

-- name: CreateAdministrator :exec
INSERT INTO administrators (
    id, username, password_hash, locale, uses_default_credentials,
    created_at, credentials_updated_at
) VALUES (1, ?, ?, ?, ?, ?, ?);

-- name: GetAdministratorByUsername :one
SELECT id, username, password_hash, locale, uses_default_credentials,
       totp_secret_encrypted, totp_enabled, totp_last_used_step,
       created_at, credentials_updated_at
FROM administrators
WHERE username = ? COLLATE NOCASE;

-- name: UpdateAdministratorUsername :exec
UPDATE administrators
SET username = ?, uses_default_credentials = ?,
    credentials_updated_at = ?
WHERE id = 1;

-- name: UpdateAdministratorPassword :exec
UPDATE administrators
SET password_hash = ?, uses_default_credentials = ?,
    credentials_updated_at = ?
WHERE id = 1;

-- name: RehashAdministratorPassword :exec
UPDATE administrators
SET password_hash = ?
WHERE id = 1;

-- name: UpdateAdministratorLocale :exec
UPDATE administrators
SET locale = ?
WHERE id = 1;

-- name: SetTOTPEnrollment :exec
UPDATE administrators
SET totp_secret_encrypted = ?, totp_enabled = 0,
    totp_last_used_step = -1
WHERE id = 1;

-- name: EnableTOTP :exec
UPDATE administrators
SET totp_enabled = 1, totp_last_used_step = ?
WHERE id = 1 AND totp_secret_encrypted IS NOT NULL;

-- name: UseTOTPStep :execrows
UPDATE administrators
SET totp_last_used_step = ?
WHERE id = 1 AND totp_enabled = 1 AND totp_last_used_step < ?;

-- name: DisableTOTP :exec
UPDATE administrators
SET totp_secret_encrypted = NULL, totp_enabled = 0,
    totp_last_used_step = -1
WHERE id = 1;

-- name: CreateAdministratorSession :exec
INSERT INTO administrator_sessions (
    token_digest, created_at, expires_at, client_address, user_agent
) VALUES (?, ?, ?, ?, ?);

-- name: GetAdministratorSession :one
SELECT token_digest, created_at, expires_at, client_address, user_agent
FROM administrator_sessions
WHERE token_digest = ? AND expires_at > ?;

-- name: DeleteAdministratorSession :exec
DELETE FROM administrator_sessions
WHERE token_digest = ?;

-- name: DeleteAllAdministratorSessions :exec
DELETE FROM administrator_sessions;

-- name: DeleteExpiredAdministratorSessions :exec
DELETE FROM administrator_sessions
WHERE expires_at <= ?;

-- name: GetSystemState :one
SELECT id, history_generation, pending_history_generation, history_reset_at,
       release_channel
FROM system_state
WHERE id = 1;

-- name: SetReleaseChannel :execrows
UPDATE system_state
SET release_channel = ?
WHERE id = 1 AND release_channel != ?;

-- name: GetHistoryRetentionSettings :one
SELECT id, mode, max_age_days, max_logical_bytes, updated_at
       , last_cleanup_at, last_cleanup_deleted_items, last_cleanup_error
FROM history_retention_settings
WHERE id = 1;

-- name: UpdateHistoryRetentionSettings :execrows
UPDATE history_retention_settings
SET mode = ?, max_age_days = ?, max_logical_bytes = ?, updated_at = ?
WHERE id = 1;

-- name: RecordHistoryRetentionCleanup :exec
UPDATE history_retention_settings
SET last_cleanup_at = ?, last_cleanup_deleted_items = ?, last_cleanup_error = ?
WHERE id = 1;

-- name: CreateSystemState :exec
INSERT INTO system_state (id, history_generation)
VALUES (1, ?);

-- name: SetPendingHistoryGeneration :exec
UPDATE system_state
SET pending_history_generation = ?
WHERE id = 1 AND pending_history_generation IS NULL;

-- name: PromotePendingHistoryGeneration :execrows
UPDATE system_state
SET history_generation = pending_history_generation,
    pending_history_generation = NULL,
    history_reset_at = ?
WHERE id = 1 AND pending_history_generation = ?;

-- name: GetAgentEnrollment :one
SELECT id, enabled, key_digest, key_encrypted, created_at, rotated_at
FROM agent_enrollment
WHERE id = 1;

-- name: UpsertAgentEnrollmentKey :exec
INSERT INTO agent_enrollment (
    id, enabled, key_digest, key_encrypted, created_at, rotated_at
) VALUES (1, 1, ?, ?, ?, ?)
ON CONFLICT (id) DO UPDATE SET
    enabled = 1,
    key_digest = excluded.key_digest,
    key_encrypted = excluded.key_encrypted,
    rotated_at = excluded.rotated_at;

-- name: SetAgentEnrollmentEnabled :execrows
UPDATE agent_enrollment
SET enabled = ?
WHERE id = 1;

-- name: CreateNode :exec
INSERT INTO nodes (
    id, name, hostname, credential_digest, agent_version, agent_revision,
    operating_system, architecture, desired_configuration_revision, registered_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, 1, ?);

-- name: GetNodeByCredentialDigest :one
SELECT id, name, hostname, credential_digest, enabled, revoked_at,
       agent_version, agent_revision, operating_system, architecture,
       desired_configuration_revision, applied_configuration_revision,
       configuration_error, registered_at, last_seen_at,
       configuration_error_revision, physical_memory_bytes,
       probe_schedule_enabled, probe_schedule_cron, probe_schedule_timezone,
       probe_low_memory_override
FROM nodes
WHERE credential_digest = ?;

-- name: GetNodeByID :one
SELECT id, name, hostname, credential_digest, enabled, revoked_at,
       agent_version, agent_revision, operating_system, architecture,
       desired_configuration_revision, applied_configuration_revision,
       configuration_error, registered_at, last_seen_at,
       configuration_error_revision, physical_memory_bytes,
       probe_schedule_enabled, probe_schedule_cron, probe_schedule_timezone,
       probe_low_memory_override
FROM nodes
WHERE id = ?;

-- name: UpdateNodeHeartbeat :execrows
UPDATE nodes
SET hostname = ?, agent_version = ?, agent_revision = ?, operating_system = ?, architecture = ?,
    applied_configuration_revision = ?, configuration_error = ?,
    configuration_error_revision = ?, last_seen_at = ?
WHERE id = ? AND revoked_at IS NULL;

-- name: ListNodes :many
SELECT id, name, hostname, credential_digest, enabled, revoked_at,
       agent_version, agent_revision, operating_system, architecture,
       desired_configuration_revision, applied_configuration_revision,
       configuration_error, registered_at, last_seen_at,
       configuration_error_revision, physical_memory_bytes,
       probe_schedule_enabled, probe_schedule_cron, probe_schedule_timezone,
       probe_low_memory_override
FROM nodes
ORDER BY name COLLATE NOCASE, id;

-- name: SetNodeEnabled :execrows
UPDATE nodes
SET enabled = ?,
    desired_configuration_revision = desired_configuration_revision + 1,
    configuration_error = NULL,
    configuration_error_revision = NULL
WHERE id = ? AND enabled != ? AND revoked_at IS NULL
  AND NOT EXISTS (
      SELECT 1 FROM node_deletion_operations
      WHERE node_id = nodes.id AND status != 'completed'
  );

-- name: RevokeNode :execrows
UPDATE nodes
SET enabled = 0, revoked_at = ?, configuration_error = NULL,
    configuration_error_revision = NULL
WHERE id = ? AND revoked_at IS NULL;

-- name: IncrementAllNodeDesiredConfigurationRevisions :exec
UPDATE nodes
SET desired_configuration_revision = desired_configuration_revision + 1,
    configuration_error = NULL,
    configuration_error_revision = NULL
WHERE revoked_at IS NULL;

-- name: GetRevokedAgentCredential :one
SELECT credential_digest, revoked_at, reason
FROM revoked_agent_credentials
WHERE credential_digest = ?;

-- name: UpsertRevokedAgentCredential :exec
INSERT INTO revoked_agent_credentials (credential_digest, revoked_at, reason)
VALUES (?, ?, ?)
ON CONFLICT (credential_digest) DO UPDATE SET
    revoked_at = excluded.revoked_at,
    reason = excluded.reason;

-- name: CreateNodeDeletion :exec
INSERT INTO node_deletion_operations (
    node_id, credential_digest, status, requested_at, updated_at
) VALUES (?, ?, 'pending', ?, ?)
ON CONFLICT (node_id) DO UPDATE SET
    status = CASE
        WHEN node_deletion_operations.status = 'completed' THEN 'completed'
        ELSE 'pending'
    END,
    updated_at = excluded.updated_at,
    last_error = NULL;

-- name: GetNodeDeletion :one
SELECT node_id, credential_digest, status, requested_at, updated_at, last_error
FROM node_deletion_operations
WHERE node_id = ?;

-- name: ListActiveNodeDeletions :many
SELECT node_id, credential_digest, status, requested_at, updated_at, last_error
FROM node_deletion_operations
WHERE status IN ('pending', 'failed')
ORDER BY requested_at, node_id
LIMIT ?;

-- name: RetryNodeDeletion :exec
UPDATE node_deletion_operations
SET status = 'pending', updated_at = ?, last_error = NULL
WHERE node_id = ? AND status = 'failed';

-- name: FailNodeDeletion :exec
UPDATE node_deletion_operations
SET status = 'failed', updated_at = ?, last_error = ?
WHERE node_id = ? AND status != 'completed';

-- name: CompleteNodeDeletion :exec
UPDATE node_deletion_operations
SET status = 'completed', updated_at = ?, last_error = NULL
WHERE node_id = ?;

-- name: CreateEgressDeletion :exec
INSERT INTO egress_deletion_operations (
    egress_id, node_id, status, requested_at, updated_at
) VALUES (?, ?, 'pending', ?, ?)
ON CONFLICT (egress_id) DO UPDATE SET
    status = CASE
        WHEN egress_deletion_operations.status = 'completed' THEN 'completed'
        ELSE 'pending'
    END,
    updated_at = excluded.updated_at,
    last_error = NULL;

-- name: GetEgressDeletion :one
SELECT egress_id, node_id, status, requested_at, updated_at, last_error
FROM egress_deletion_operations
WHERE egress_id = ? AND node_id = ?;

-- name: ListActiveEgressDeletions :many
SELECT egress_id, node_id, status, requested_at, updated_at, last_error
FROM egress_deletion_operations
WHERE status IN ('pending', 'failed')
ORDER BY requested_at, egress_id
LIMIT ?;

-- name: ListActiveNodeEgressDeletions :many
SELECT egress_id, node_id, status, requested_at, updated_at, last_error
FROM egress_deletion_operations
WHERE node_id = ? AND status IN ('pending', 'failed')
ORDER BY requested_at, egress_id;

-- name: RetryEgressDeletion :exec
UPDATE egress_deletion_operations
SET status = 'pending', updated_at = ?, last_error = NULL
WHERE egress_id = ? AND node_id = ? AND status = 'failed';

-- name: FailEgressDeletion :exec
UPDATE egress_deletion_operations
SET status = 'failed', updated_at = ?, last_error = ?
WHERE egress_id = ? AND node_id = ? AND status != 'completed';

-- name: CompleteEgressDeletion :exec
UPDATE egress_deletion_operations
SET status = 'completed', updated_at = ?, last_error = NULL
WHERE egress_id = ? AND node_id = ?;

-- name: DeleteNodeEgressDeletionOperations :exec
DELETE FROM egress_deletion_operations
WHERE node_id = ?;

-- name: DeleteNode :exec
DELETE FROM nodes
WHERE id = ?;

-- name: DeleteNodeCapabilities :exec
DELETE FROM node_capabilities
WHERE node_id = ?;

-- name: CreateNodeCapability :exec
INSERT INTO node_capabilities (node_id, capability)
VALUES (?, ?);

-- name: ListNodeCapabilities :many
SELECT node_id, capability
FROM node_capabilities
ORDER BY node_id, capability;

-- name: GetNodeCapability :one
SELECT capability
FROM node_capabilities
WHERE node_id = ? AND capability = ?;

-- name: UpsertNodeSyncSession :exec
INSERT INTO node_sync_sessions (
    node_id, session_id, requested_at, expires_at
) VALUES (?, ?, ?, ?)
ON CONFLICT (node_id) DO UPDATE SET
    session_id = excluded.session_id,
    requested_at = excluded.requested_at,
    expires_at = excluded.expires_at,
    delivered_at = NULL;

-- name: GetActiveNodeSyncSession :one
SELECT node_id, session_id, requested_at, expires_at, delivered_at
FROM node_sync_sessions
WHERE node_id = ? AND expires_at > ?;

-- name: GetActiveNodeSyncSessionByID :one
SELECT node_id, session_id, requested_at, expires_at, delivered_at
FROM node_sync_sessions
WHERE node_id = ? AND session_id = ? AND expires_at > ?;

-- name: ListNodeSyncSessions :many
SELECT node_id, session_id, requested_at, expires_at, delivered_at
FROM node_sync_sessions
WHERE expires_at > ?
ORDER BY node_id;

-- name: MarkNodeSyncSessionDelivered :execrows
UPDATE node_sync_sessions
SET delivered_at = COALESCE(delivered_at, ?)
WHERE node_id = ? AND session_id = ? AND expires_at > ?;

-- name: DeleteNodeSyncSession :exec
DELETE FROM node_sync_sessions
WHERE node_id = ?;

-- name: GetNodeNetworkInventory :one
SELECT node_id, payload, captured_at, received_at, last_error
FROM node_network_inventories
WHERE node_id = ?;

-- name: UpsertNodeNetworkInventory :exec
INSERT INTO node_network_inventories (
    node_id, payload, captured_at, received_at, last_error
) VALUES (?, ?, ?, ?, NULL)
ON CONFLICT (node_id) DO UPDATE SET
    payload = excluded.payload,
    captured_at = excluded.captured_at,
    received_at = excluded.received_at,
    last_error = NULL;

-- name: RecordNodeNetworkInventoryError :exec
INSERT INTO node_network_inventories (
    node_id, payload, captured_at, received_at, last_error
) VALUES (?, NULL, NULL, ?, ?)
ON CONFLICT (node_id) DO UPDATE SET
    received_at = excluded.received_at,
    last_error = excluded.last_error;

-- name: ListNodeEgresses :many
SELECT id, node_id, name, kind, family, interface_name, source_address, proxy_id,
       enabled, available, automatic, lightweight_interval_seconds,
       probe_on_address_change, created_at, updated_at
FROM network_egresses
WHERE node_id = ?
ORDER BY family, kind, created_at, id;

-- name: ListActiveNodeEgresses :many
SELECT e.id, e.node_id, e.name, e.kind, e.family, e.interface_name,
       e.source_address, e.proxy_id, e.enabled, e.available, e.automatic,
       e.lightweight_interval_seconds, e.probe_on_address_change,
       e.created_at, e.updated_at
FROM network_egresses e
WHERE e.node_id = ?
  AND NOT EXISTS (
      SELECT 1
      FROM egress_deletion_operations d
      WHERE d.egress_id = e.id AND d.status IN ('pending', 'failed')
  )
ORDER BY e.family, e.kind, e.created_at, e.id;

-- name: GetNodeEgress :one
SELECT id, node_id, name, kind, family, interface_name, source_address, proxy_id,
       enabled, available, automatic, lightweight_interval_seconds,
       probe_on_address_change, created_at, updated_at
FROM network_egresses
WHERE node_id = ? AND id = ?;

-- name: GetNetworkEgressByID :one
SELECT id, node_id, name, kind, family, interface_name, source_address, proxy_id,
       enabled, available, automatic, lightweight_interval_seconds,
       probe_on_address_change, created_at, updated_at
FROM network_egresses
WHERE id = ?;

-- name: GetDefaultNodeEgress :one
SELECT id, node_id, name, kind, family, interface_name, source_address, proxy_id,
       enabled, available, automatic, lightweight_interval_seconds,
       probe_on_address_change, created_at, updated_at
FROM network_egresses
WHERE node_id = ? AND kind = 'default' AND family = ?;

-- name: GetNodeEgressBySelector :one
SELECT id, node_id, name, kind, family, interface_name, source_address, proxy_id,
       enabled, available, automatic, lightweight_interval_seconds,
       probe_on_address_change, created_at, updated_at
FROM network_egresses
WHERE node_id = ? AND kind = ? AND family = ? AND interface_name = ?
  AND COALESCE(source_address, '') = COALESCE(?, '');

-- name: GetNodeEgressByProxy :one
SELECT id, node_id, name, kind, family, interface_name, source_address, proxy_id,
       enabled, available, automatic, lightweight_interval_seconds,
       probe_on_address_change, created_at, updated_at
FROM network_egresses
WHERE node_id = ? AND kind = 'proxy' AND family = ? AND proxy_id = ?;

-- name: CreateNodeEgress :exec
INSERT INTO network_egresses (
    id, node_id, name, kind, family, interface_name, source_address, proxy_id,
    enabled, available, automatic, lightweight_interval_seconds,
    probe_on_address_change, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: SetNodeEgressAvailability :execrows
UPDATE network_egresses
SET available = ?, updated_at = ?
WHERE id = ? AND node_id = ? AND available != ?;

-- name: SetNodeEgressEnabled :execrows
UPDATE network_egresses
SET enabled = ?, updated_at = ?
WHERE id = ? AND node_id = ? AND enabled != ?;

-- name: DeleteNodeEgress :execrows
DELETE FROM network_egresses
WHERE id = ? AND node_id = ?;

-- name: IncrementNodeDesiredConfigurationRevision :execrows
UPDATE nodes
SET desired_configuration_revision = desired_configuration_revision + 1,
    configuration_error = NULL,
    configuration_error_revision = NULL
WHERE id = ? AND revoked_at IS NULL;

-- name: ListNetworkProxies :many
SELECT id, name, scheme, host, port, username, password_encrypted, created_at, updated_at
FROM network_proxies
ORDER BY name COLLATE NOCASE, id;

-- name: GetNetworkProxy :one
SELECT id, name, scheme, host, port, username, password_encrypted, created_at, updated_at
FROM network_proxies
WHERE id = ?;

-- name: GetNetworkProxyByName :one
SELECT id, name, scheme, host, port, username, password_encrypted, created_at, updated_at
FROM network_proxies
WHERE name = ? COLLATE NOCASE;

-- name: ListNodeNetworkProxies :many
SELECT DISTINCT p.id, p.name, p.scheme, p.host, p.port, p.username,
       p.password_encrypted, p.created_at, p.updated_at
FROM network_proxies p
JOIN network_egresses e ON e.proxy_id = p.id
WHERE e.node_id = ?
  AND NOT EXISTS (
      SELECT 1
      FROM egress_deletion_operations d
      WHERE d.egress_id = e.id AND d.status IN ('pending', 'failed')
  )
ORDER BY p.name COLLATE NOCASE, p.id;

-- name: CreateNetworkProxy :exec
INSERT INTO network_proxies (
    id, name, scheme, host, port, username, password_encrypted, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: UpdateNetworkProxy :execrows
UPDATE network_proxies
SET name = ?, scheme = ?, host = ?, port = ?, username = ?,
    password_encrypted = ?, updated_at = ?
WHERE id = ?;

-- name: CountNetworkProxies :one
SELECT count(*) FROM network_proxies;

-- name: ListNodeIDsReferencingNetworkProxy :many
SELECT DISTINCT node_id
FROM network_egresses
WHERE proxy_id = ?
ORDER BY node_id;

-- name: CountNetworkProxyReferences :one
SELECT count(*) FROM network_egresses WHERE proxy_id = ?;

-- name: DeleteNetworkProxy :execrows
DELETE FROM network_proxies WHERE id = ?;

-- name: GetNetworkObservationSettings :one
SELECT id, ipv4_services, ipv6_services, updated_at
FROM network_observation_settings
WHERE id = 1;

-- name: UpdateNetworkObservationSettings :exec
UPDATE network_observation_settings
SET ipv4_services = ?, ipv6_services = ?, updated_at = ?
WHERE id = 1;

-- name: UpdateNodeEgressSettings :execrows
UPDATE network_egresses
SET enabled = ?, lightweight_interval_seconds = ?,
    probe_on_address_change = ?, updated_at = ?
WHERE id = ? AND node_id = ?
  AND (
      enabled != ? OR lightweight_interval_seconds != ? OR
      probe_on_address_change != ?
  );

-- name: UpdateNodePhysicalMemory :execrows
UPDATE nodes
SET physical_memory_bytes = ?
WHERE id = ? AND revoked_at IS NULL;

-- name: GetNodeProbeSettings :one
SELECT id, enabled, revoked_at, last_seen_at, applied_configuration_revision,
       desired_configuration_revision, physical_memory_bytes,
       probe_schedule_enabled, probe_schedule_cron, probe_schedule_timezone,
       probe_low_memory_override
FROM nodes
WHERE id = ?;

-- name: UpdateNodeProbeSettings :execrows
UPDATE nodes
SET probe_schedule_enabled = ?, probe_schedule_cron = ?,
    probe_schedule_timezone = ?, probe_low_memory_override = ?,
    desired_configuration_revision = desired_configuration_revision + 1,
    configuration_error = NULL, configuration_error_revision = NULL
WHERE id = ? AND revoked_at IS NULL
  AND NOT EXISTS (
      SELECT 1 FROM node_deletion_operations
      WHERE node_id = nodes.id AND status != 'completed'
  );

-- name: UpsertNodeProbeStatus :exec
INSERT INTO node_probe_status (
    node_id, active_run_id, next_scheduled_at, last_occurrence_at,
    last_occurrence_trigger, last_occurrence_status, last_skip_reason,
    history_reset_generation, history_reset_at,
    history_reset_discarded_address_items, history_reset_discarded_probe_items,
    reported_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (node_id) DO UPDATE SET
    active_run_id = excluded.active_run_id,
    next_scheduled_at = excluded.next_scheduled_at,
    last_occurrence_at = excluded.last_occurrence_at,
    last_occurrence_trigger = excluded.last_occurrence_trigger,
    last_occurrence_status = excluded.last_occurrence_status,
    last_skip_reason = excluded.last_skip_reason,
    history_reset_generation = excluded.history_reset_generation,
    history_reset_at = excluded.history_reset_at,
    history_reset_discarded_address_items = excluded.history_reset_discarded_address_items,
    history_reset_discarded_probe_items = excluded.history_reset_discarded_probe_items,
    reported_at = excluded.reported_at;

-- name: GetNodeProbeStatus :one
SELECT node_id, active_run_id, next_scheduled_at, last_occurrence_at,
       last_occurrence_trigger, last_occurrence_status, last_skip_reason,
       history_reset_generation, history_reset_at,
       history_reset_discarded_address_items, history_reset_discarded_probe_items,
       reported_at
FROM node_probe_status
WHERE node_id = ?;

-- name: CreateProbeTask :exec
INSERT INTO probe_tasks (
    id, node_id, kind, status, created_at, expires_at
) VALUES (?, ?, 'complete-probe', 'pending', ?, ?);

-- name: CreateAgentUpdateTask :exec
INSERT INTO probe_tasks (
    id, node_id, kind, status, target_version, created_at, expires_at
) VALUES (?, ?, 'agent-update', 'pending', ?, ?, ?);

-- name: GetProbeTask :one
SELECT id, node_id, kind, status, created_at, expires_at, acknowledged_at,
       started_at, completed_at, run_id, rejection_reason,
       target_version, previous_version, result_version, failure_code, diagnostic,
       terminal_confirmed_at
FROM probe_tasks
WHERE id = ? AND node_id = ?;

-- name: GetActiveNodeTask :one
SELECT id, node_id, kind, status, created_at, expires_at, acknowledged_at,
       started_at, completed_at, run_id, rejection_reason,
       target_version, previous_version, result_version, failure_code, diagnostic,
       terminal_confirmed_at
FROM probe_tasks
WHERE node_id = ? AND status IN (
    'pending', 'acknowledged', 'running', 'verifying', 'installing', 'restarting'
)
ORDER BY created_at, id
LIMIT 1;

-- name: GetLatestProbeTask :one
SELECT id, node_id, kind, status, created_at, expires_at, acknowledged_at,
       started_at, completed_at, run_id, rejection_reason,
       target_version, previous_version, result_version, failure_code, diagnostic,
       terminal_confirmed_at
FROM probe_tasks
WHERE node_id = ? AND kind = 'complete-probe'
ORDER BY created_at DESC, id DESC
LIMIT 1;

-- name: GetLatestAgentUpdateTask :one
SELECT id, node_id, kind, status, created_at, expires_at, acknowledged_at,
       started_at, completed_at, run_id, rejection_reason,
       target_version, previous_version, result_version, failure_code, diagnostic,
       terminal_confirmed_at
FROM probe_tasks
WHERE node_id = ? AND kind = 'agent-update'
ORDER BY created_at DESC, id DESC
LIMIT 1;

-- name: ListLatestAgentUpdateTasks :many
SELECT task.id, task.node_id, task.kind, task.status, task.created_at,
       task.expires_at, task.acknowledged_at, task.started_at,
       task.completed_at, task.run_id, task.rejection_reason,
       task.target_version, task.previous_version, task.result_version,
       task.failure_code, task.diagnostic, task.terminal_confirmed_at
FROM probe_tasks AS task
WHERE task.kind = 'agent-update'
  AND task.id = (
      SELECT latest.id
      FROM probe_tasks AS latest
      WHERE latest.node_id = task.node_id AND latest.kind = 'agent-update'
      ORDER BY latest.created_at DESC, latest.id DESC
      LIMIT 1
  )
ORDER BY task.created_at DESC, task.id DESC
LIMIT 70;

-- name: ExpireProbeTask :execrows
UPDATE probe_tasks
SET status = 'expired', completed_at = ?
WHERE id = ? AND node_id = ? AND status = 'pending' AND expires_at <= ?;

-- name: UpdateProbeTaskReport :execrows
UPDATE probe_tasks
SET status = ?, acknowledged_at = ?, started_at = ?, completed_at = ?,
    run_id = ?, rejection_reason = ?, previous_version = ?, result_version = ?,
    failure_code = ?, diagnostic = ?, terminal_confirmed_at = ?
WHERE id = ? AND node_id = ?
  AND status IN (
      'pending', 'acknowledged', 'running', 'verifying', 'installing', 'restarting'
  );

-- name: DeleteTerminalProbeTasksBefore :exec
DELETE FROM probe_tasks
WHERE completed_at IS NOT NULL AND completed_at < ?
  AND status IN ('succeeded', 'partial', 'failed', 'rolled-back', 'rejected', 'expired');

-- name: CreateNotificationSender :exec
INSERT INTO notification_senders (
    id, name, kind, enabled, configuration_encrypted, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?);

-- name: GetNotificationSender :one
SELECT id, name, kind, enabled, configuration_encrypted, created_at, updated_at
FROM notification_senders
WHERE id = ?;

-- name: ListNotificationSenders :many
SELECT id, name, kind, enabled, configuration_encrypted, created_at, updated_at
FROM notification_senders
ORDER BY name COLLATE NOCASE, id;

-- name: UpdateNotificationSender :execrows
UPDATE notification_senders
SET name = ?, enabled = ?, configuration_encrypted = ?, updated_at = ?
WHERE id = ? AND kind = ?;

-- name: DeleteNotificationSender :execrows
DELETE FROM notification_senders
WHERE notification_senders.id = ?
  AND NOT EXISTS (
      SELECT 1 FROM notification_rules WHERE sender_id = notification_senders.id
  );

-- name: CreateNotificationRule :exec
INSERT INTO notification_rules (
    id, name, enabled, sender_id, event_type, field_id,
    node_id, egress_id, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: GetNotificationRule :one
SELECT id, name, enabled, sender_id, event_type, field_id,
       node_id, egress_id, created_at, updated_at
FROM notification_rules
WHERE id = ?;

-- name: ListNotificationRules :many
SELECT id, name, enabled, sender_id, event_type, field_id,
       node_id, egress_id, created_at, updated_at
FROM notification_rules
ORDER BY name COLLATE NOCASE, id;

-- name: ListEnabledNotificationRules :many
SELECT r.id, r.name, r.sender_id, r.event_type, r.field_id,
       r.node_id, r.egress_id, s.name AS sender_name, s.kind AS sender_kind
FROM notification_rules r
JOIN notification_senders s ON s.id = r.sender_id
WHERE r.enabled = 1 AND s.enabled = 1
ORDER BY r.sender_id, r.id;

-- name: UpdateNotificationRule :execrows
UPDATE notification_rules
SET name = ?, enabled = ?, sender_id = ?, event_type = ?, field_id = ?,
    node_id = ?, egress_id = ?, updated_at = ?
WHERE id = ?;

-- name: DeleteNotificationRule :execrows
DELETE FROM notification_rules WHERE id = ?;
