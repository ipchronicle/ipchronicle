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
SELECT id, history_generation, pending_history_generation, history_reset_at
FROM system_state
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
    id, name, hostname, credential_digest, agent_version,
    operating_system, architecture, desired_configuration_revision, registered_at
) VALUES (?, ?, ?, ?, ?, ?, ?, 1, ?);

-- name: GetNodeByCredentialDigest :one
SELECT id, name, hostname, credential_digest, enabled, revoked_at,
       agent_version, operating_system, architecture,
       desired_configuration_revision, applied_configuration_revision,
       configuration_error, registered_at, last_seen_at,
       configuration_error_revision
FROM nodes
WHERE credential_digest = ?;

-- name: GetNodeByID :one
SELECT id, name, hostname, credential_digest, enabled, revoked_at,
       agent_version, operating_system, architecture,
       desired_configuration_revision, applied_configuration_revision,
       configuration_error, registered_at, last_seen_at,
       configuration_error_revision
FROM nodes
WHERE id = ?;

-- name: UpdateNodeHeartbeat :execrows
UPDATE nodes
SET hostname = ?, agent_version = ?, operating_system = ?, architecture = ?,
    applied_configuration_revision = ?, configuration_error = ?,
    configuration_error_revision = ?, last_seen_at = ?
WHERE id = ? AND revoked_at IS NULL;

-- name: ListNodes :many
SELECT id, name, hostname, credential_digest, enabled, revoked_at,
       agent_version, operating_system, architecture,
       desired_configuration_revision, applied_configuration_revision,
       configuration_error, registered_at, last_seen_at,
       configuration_error_revision
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
