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
       release_channel, external_origin
FROM system_state
WHERE id = 1;

-- name: GetExternalOrigin :one
SELECT external_origin
FROM system_state
WHERE id = 1;

-- name: SetExternalOrigin :execrows
UPDATE system_state
SET external_origin = ?
WHERE id = 1 AND external_origin != ?;

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
SELECT id, enabled, key_digest, key_encrypted, default_probe_timezone,
       created_at, rotated_at
FROM agent_enrollment
WHERE id = 1;

-- name: UpsertAgentEnrollmentKey :exec
INSERT INTO agent_enrollment (
    id, enabled, key_digest, key_encrypted, default_probe_timezone,
    created_at, rotated_at
) VALUES (1, 1, ?, ?, ?, ?, ?)
ON CONFLICT (id) DO UPDATE SET
    enabled = 1,
    key_digest = excluded.key_digest,
    key_encrypted = excluded.key_encrypted,
    default_probe_timezone = excluded.default_probe_timezone,
    rotated_at = excluded.rotated_at;

-- name: SetAgentEnrollmentEnabled :execrows
UPDATE agent_enrollment
SET enabled = ?
WHERE id = 1;

-- name: CreateNode :exec
INSERT INTO nodes (
    id, name, hostname, credential_digest, agent_version, agent_revision,
    operating_system, architecture, desired_configuration_revision,
    probe_schedule_timezone, registered_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, 1, ?, ?);

-- name: GetNodeByCredentialDigest :one
SELECT id, name, hostname, credential_digest, enabled, revoked_at,
       agent_version, agent_revision, operating_system, architecture,
       desired_configuration_revision, applied_configuration_revision,
       configuration_error, registered_at, last_seen_at,
       configuration_error_revision, physical_memory_bytes,
       probe_schedule_enabled, probe_schedule_cron, probe_schedule_timezone,
       probe_low_memory_override, probe_on_new_address
FROM nodes
WHERE credential_digest = ?;

-- name: GetNodeByID :one
SELECT id, name, hostname, credential_digest, enabled, revoked_at,
       agent_version, agent_revision, operating_system, architecture,
       desired_configuration_revision, applied_configuration_revision,
       configuration_error, registered_at, last_seen_at,
       configuration_error_revision, physical_memory_bytes,
       probe_schedule_enabled, probe_schedule_cron, probe_schedule_timezone,
       probe_low_memory_override, probe_on_new_address
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
       probe_low_memory_override, probe_on_new_address
FROM nodes
ORDER BY name COLLATE NOCASE, id;

-- name: ListNodePublicAddressSummaries :many
SELECT n.node_id, a.id, a.address, a.family, a.probe_enabled, a.last_seen_at,
       EXISTS (
           SELECT 1
           FROM public_address_paths p
           WHERE p.public_address_id = a.id
             AND p.node_id = n.node_id
             AND p.available = 1
       ) AS available
FROM public_address_nodes n
JOIN public_addresses a ON a.id = n.public_address_id
WHERE EXISTS (
    SELECT 1
    FROM public_address_paths p
    WHERE p.public_address_id = a.id
      AND p.node_id = n.node_id
      AND p.available = 1
)
ORDER BY n.node_id, a.family, a.address;

-- name: ListOverviewNodeProbeDetails :many
SELECT n.id AS node_id, n.physical_memory_bytes, n.probe_low_memory_override,
       s.next_scheduled_at
FROM nodes n
LEFT JOIN node_probe_status s ON s.node_id = n.id
ORDER BY n.name COLLATE NOCASE, n.id;

-- name: ListOverviewPublicAddressTraits :many
SELECT p.node_id, p.public_address_id,
       CAST(MAX(p.likely_nat) AS INTEGER) AS likely_nat,
       CAST(MAX(p.proxy_path) AS INTEGER) AS proxy_path
FROM public_address_paths p
WHERE p.available = 1
GROUP BY p.node_id, p.public_address_id
ORDER BY p.node_id, p.public_address_id;

-- name: ListOverviewActiveTasks :many
SELECT id, node_id, kind, status, created_at, expires_at, run_id
FROM probe_tasks
WHERE status IN('pending', 'acknowledged', 'running', 'verifying', 'installing', 'restarting')
ORDER BY created_at, id;

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

-- name: SetNodeName :execrows
UPDATE nodes
SET name = ?
WHERE id = ? AND name != ? AND revoked_at IS NULL
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
       created_at, updated_at
FROM network_egresses
WHERE node_id = ?
ORDER BY family, kind, created_at, id;

-- name: ListActiveNodeEgresses :many
SELECT e.id, e.node_id, e.name, e.kind, e.family, e.interface_name,
       e.source_address, e.proxy_id, e.enabled, e.available, e.automatic,
       e.lightweight_interval_seconds, e.created_at, e.updated_at
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
       created_at, updated_at
FROM network_egresses
WHERE node_id = ? AND id = ?;

-- name: GetNetworkEgressByID :one
SELECT id, node_id, name, kind, family, interface_name, source_address, proxy_id,
       enabled, available, automatic, lightweight_interval_seconds,
       created_at, updated_at
FROM network_egresses
WHERE id = ?;

-- name: GetDefaultNodeEgress :one
SELECT id, node_id, name, kind, family, interface_name, source_address, proxy_id,
       enabled, available, automatic, lightweight_interval_seconds,
       created_at, updated_at
FROM network_egresses
WHERE node_id = ? AND kind = 'default' AND family = ?;

-- name: CreateNodeEgress :exec
INSERT INTO network_egresses (
    id, node_id, name, kind, family, interface_name, source_address, proxy_id,
    enabled, available, automatic, lightweight_interval_seconds, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);

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

-- name: ListNodeNetworkProxies :many
SELECT id, node_id, name, scheme, host, port, username, password_encrypted,
       deletion_requested_at, created_at, updated_at
FROM network_proxies
WHERE node_id = ?
ORDER BY name COLLATE NOCASE, id;

-- name: ListActiveNodeNetworkProxies :many
SELECT id, node_id, name, scheme, host, port, username, password_encrypted,
       deletion_requested_at, created_at, updated_at
FROM network_proxies
WHERE node_id = ? AND deletion_requested_at IS NULL
ORDER BY name COLLATE NOCASE, id;

-- name: GetNodeNetworkProxy :one
SELECT id, node_id, name, scheme, host, port, username, password_encrypted,
       deletion_requested_at, created_at, updated_at
FROM network_proxies
WHERE node_id = ? AND id = ?;

-- name: GetNodeNetworkProxyByName :one
SELECT id, node_id, name, scheme, host, port, username, password_encrypted,
       deletion_requested_at, created_at, updated_at
FROM network_proxies
WHERE node_id = ? AND name = ? COLLATE NOCASE;

-- name: CreateNetworkProxy :exec
INSERT INTO network_proxies (
    id, node_id, name, scheme, host, port, username, password_encrypted,
    created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: UpdateNetworkProxy :execrows
UPDATE network_proxies
SET name = ?, scheme = ?, host = ?, port = ?, username = ?,
    password_encrypted = ?, updated_at = ?
WHERE node_id = ? AND id = ? AND deletion_requested_at IS NULL;

-- name: CountNodeNetworkProxies :one
SELECT count(*) FROM network_proxies WHERE node_id = ?;

-- name: ListNodeEgressesByProxy :many
SELECT id, node_id, name, kind, family, interface_name, source_address, proxy_id,
       enabled, available, automatic, lightweight_interval_seconds,
       created_at, updated_at
FROM network_egresses
WHERE node_id = ? AND proxy_id = ?
ORDER BY family, id;

-- name: MarkNetworkProxyDeletion :execrows
UPDATE network_proxies
SET deletion_requested_at = COALESCE(deletion_requested_at, ?)
WHERE node_id = ? AND id = ?;

-- name: DeleteMarkedNetworkProxyIfUnreferenced :execrows
DELETE FROM network_proxies
WHERE network_proxies.node_id = ? AND network_proxies.id = ?
  AND network_proxies.deletion_requested_at IS NOT NULL
  AND NOT EXISTS (
      SELECT 1 FROM network_egresses WHERE proxy_id = network_proxies.id
  );

-- name: GetNetworkObservationSettings :one
SELECT id, ipv4_services, ipv6_services, updated_at
FROM network_observation_settings
WHERE id = 1;

-- name: UpdateNetworkObservationSettings :exec
UPDATE network_observation_settings
SET ipv4_services = ?, ipv6_services = ?, updated_at = ?
WHERE id = 1;

-- name: UpsertPublicAddress :exec
INSERT INTO public_addresses (
    id, address, family, probe_enabled, first_seen_at, last_seen_at, updated_at
) VALUES (?, ?, ?, 1, ?, ?, ?)
ON CONFLICT (address) DO UPDATE SET
    family = excluded.family,
    last_seen_at = MAX(public_addresses.last_seen_at, excluded.last_seen_at),
    updated_at = MAX(public_addresses.updated_at, excluded.updated_at);

-- name: GetPublicAddressByAddress :one
SELECT id, address, family, probe_enabled,
       selected_path_id, first_seen_at, last_seen_at, updated_at
FROM public_addresses
WHERE address = ?;

-- name: GetPublicAddressByID :one
SELECT id, address, family, probe_enabled,
       selected_path_id, first_seen_at, last_seen_at, updated_at
FROM public_addresses
WHERE id = ?;

-- name: ListPublicAddresses :many
SELECT id, address, family, probe_enabled,
       selected_path_id, first_seen_at, last_seen_at, updated_at
FROM public_addresses
ORDER BY family, address;

-- name: ListNodePublicAddresses :many
SELECT DISTINCT a.id, a.address, a.family, a.probe_enabled,
       a.selected_path_id, a.first_seen_at, a.last_seen_at, a.updated_at
FROM public_addresses a
JOIN public_address_paths p ON p.public_address_id = a.id
WHERE p.node_id = ? AND p.available = 1
ORDER BY a.family, a.address;

-- name: UpsertPublicAddressNode :exec
INSERT INTO public_address_nodes (
    public_address_id, node_id, first_seen_at, last_seen_at
) VALUES (?, ?, ?, ?)
ON CONFLICT (public_address_id, node_id) DO UPDATE SET
    last_seen_at = MAX(public_address_nodes.last_seen_at, excluded.last_seen_at);

-- name: SetPublicAddressProbeSettings :execrows
UPDATE public_addresses
SET probe_enabled = ?, updated_at = ?
WHERE id = ? AND probe_enabled != ?;

-- name: UpsertPublicAddressPath :exec
INSERT INTO public_address_paths (
    public_address_id, path_id, node_id, local_interface, local_address,
    proxy_path, likely_nat, temporary, available, last_checked_at,
    last_succeeded_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (path_id) DO UPDATE SET
    public_address_id = excluded.public_address_id,
    node_id = excluded.node_id,
    local_interface = excluded.local_interface,
    local_address = excluded.local_address,
    proxy_path = excluded.proxy_path,
    likely_nat = excluded.likely_nat,
    temporary = excluded.temporary,
    available = excluded.available,
    last_checked_at = excluded.last_checked_at,
    last_succeeded_at = excluded.last_succeeded_at;

-- name: MarkNodePublicAddressPathsUnavailable :exec
UPDATE public_address_paths
SET available = 0
WHERE node_id = ?;

-- name: GetPublicAddressPathByPathID :one
SELECT public_address_id, path_id, node_id, local_interface, local_address,
       proxy_path, likely_nat, temporary, available, last_checked_at,
       last_succeeded_at
FROM public_address_paths
WHERE path_id = ?;

-- name: GetPublicAddressPathForNode :one
SELECT p.public_address_id, p.path_id, p.node_id, p.local_interface,
       p.local_address, p.proxy_path, p.likely_nat, p.temporary,
       p.available, p.last_checked_at, p.last_succeeded_at
FROM public_address_paths p
WHERE p.public_address_id = ? AND p.node_id = ?
ORDER BY p.available DESC, p.last_succeeded_at DESC, p.path_id
LIMIT 1;

-- name: ListPublicAddressPaths :many
SELECT public_address_id, path_id, node_id, local_interface, local_address,
       proxy_path, likely_nat, temporary, available, last_checked_at,
       last_succeeded_at
FROM public_address_paths
WHERE public_address_id = ?
ORDER BY available DESC, last_succeeded_at DESC, path_id;

-- name: ListNodePublicAddressPaths :many
SELECT public_address_id, path_id, node_id, local_interface, local_address,
       proxy_path, likely_nat, temporary, available, last_checked_at,
       last_succeeded_at
FROM public_address_paths
WHERE node_id = ?
ORDER BY path_id;

-- name: SelectPublicAddressPath :execrows
UPDATE public_addresses
SET selected_path_id = ?, updated_at = MAX(updated_at, last_seen_at)
WHERE id = ? AND COALESCE(selected_path_id, '') != COALESCE(?, '');

-- name: ClearUnavailablePublicAddressSelections :execrows
UPDATE public_addresses
SET selected_path_id = NULL, updated_at = MAX(updated_at, last_seen_at)
WHERE selected_path_id IS NOT NULL
  AND NOT EXISTS (
      SELECT 1 FROM public_address_paths p
      WHERE p.path_id = public_addresses.selected_path_id
        AND p.public_address_id = public_addresses.id AND p.available = 1
  );

-- name: ListPublicAddressesWithoutSelectedPath :many
SELECT id, address, family, probe_enabled,
       selected_path_id, first_seen_at, last_seen_at, updated_at
FROM public_addresses
WHERE selected_path_id IS NULL
  AND EXISTS (
      SELECT 1 FROM public_address_paths p
      WHERE p.public_address_id = public_addresses.id AND p.available = 1
  )
ORDER BY family, address;

-- name: GetPreferredPublicAddressPath :one
SELECT public_address_id, path_id, node_id, local_interface, local_address,
       proxy_path, likely_nat, temporary, available, last_checked_at,
       last_succeeded_at
FROM public_address_paths
WHERE public_address_id = ? AND available = 1
ORDER BY last_succeeded_at DESC, last_checked_at DESC, path_id
LIMIT 1;

-- name: ListNodeAvailablePublicAddressTargets :many
SELECT a.id, a.address, a.family, a.probe_enabled,
       a.selected_path_id, a.first_seen_at, a.last_seen_at, a.updated_at,
       e.node_id, e.name, e.kind, e.interface_name, e.source_address,
       e.proxy_id, e.lightweight_interval_seconds
FROM public_addresses a
JOIN network_egresses e ON e.id = a.selected_path_id
JOIN public_address_paths p ON p.path_id = e.id AND p.public_address_id = a.id
WHERE e.node_id = ? AND p.available = 1
ORDER BY a.family, a.address;

-- name: PublicAddressBelongsToNode :one
SELECT count(*)
FROM public_address_nodes
WHERE public_address_id = ? AND node_id = ?;

-- name: UpsertPendingPublicAddressProbe :exec
INSERT INTO pending_public_address_probes (
    public_address_id, node_id, required_configuration_revision, created_at
) VALUES (?, ?, ?, ?)
ON CONFLICT (node_id, public_address_id) DO UPDATE SET
    required_configuration_revision = excluded.required_configuration_revision,
    created_at = excluded.created_at;

-- name: ListReadyPublicAddressProbes :many
SELECT pending.public_address_id, pending.node_id,
       pending.required_configuration_revision, pending.created_at
FROM pending_public_address_probes AS pending
JOIN public_addresses AS address ON address.id = pending.public_address_id
JOIN public_address_paths AS path ON path.path_id = address.selected_path_id
WHERE pending.node_id = ?
  AND pending.required_configuration_revision <= ?
  AND address.probe_enabled = 1
  AND path.node_id = pending.node_id
  AND path.public_address_id = address.id
  AND path.available = 1
ORDER BY pending.created_at, pending.public_address_id
LIMIT 64;

-- name: DeletePendingPublicAddressProbes :exec
DELETE FROM pending_public_address_probes
WHERE node_id = ?;

-- name: DeletePendingPublicAddressProbeByAddress :exec
DELETE FROM pending_public_address_probes
WHERE public_address_id = ?;

-- name: DeleteUnavailablePendingPublicAddressProbes :exec
DELETE FROM pending_public_address_probes
WHERE pending_public_address_probes.node_id = ?
  AND NOT EXISTS (
      SELECT 1
      FROM public_addresses AS address
      JOIN public_address_paths AS path ON path.path_id = address.selected_path_id
      WHERE address.id = pending_public_address_probes.public_address_id
        AND address.probe_enabled = 1
        AND path.node_id = pending_public_address_probes.node_id
        AND path.public_address_id = address.id
        AND path.available = 1
  );

-- name: UpdateNodePhysicalMemory :execrows
UPDATE nodes
SET physical_memory_bytes = ?
WHERE id = ? AND revoked_at IS NULL;

-- name: GetNodeProbeSettings :one
SELECT id, enabled, revoked_at, last_seen_at, applied_configuration_revision,
       desired_configuration_revision, physical_memory_bytes,
       probe_schedule_enabled, probe_schedule_cron, probe_schedule_timezone,
       probe_low_memory_override, probe_on_new_address
FROM nodes
WHERE id = ?;

-- name: UpdateNodeProbeSettings :execrows
UPDATE nodes
SET probe_schedule_enabled = ?, probe_schedule_cron = ?,
    probe_schedule_timezone = ?, probe_low_memory_override = ?,
    probe_on_new_address = ?,
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
    id, node_id, kind, status, trigger, created_at, expires_at
) VALUES (?, ?, 'complete-probe', 'pending', 'manual', ?, ?);

-- name: CreateNewAddressProbeTask :exec
INSERT INTO probe_tasks (
    id, node_id, kind, status, trigger, created_at, expires_at
) VALUES (?, ?, 'complete-probe', 'pending', 'new-address', ?, ?);

-- name: CreateProbeTaskPublicAddress :exec
INSERT INTO probe_task_public_addresses (
    task_id, public_address_id, ordinal
) VALUES (?, ?, ?);

-- name: ListProbeTaskPublicAddresses :many
SELECT public_address_id
FROM probe_task_public_addresses
WHERE task_id = ?
ORDER BY ordinal;

-- name: CreateAgentUpdateTask :exec
INSERT INTO probe_tasks (
    id, node_id, kind, status, trigger, target_version, created_at, expires_at
) VALUES (?, ?, 'agent-update', 'pending', 'agent-update', ?, ?, ?);

-- name: GetProbeTask :one
SELECT *
FROM probe_tasks
WHERE id = ? AND node_id = ?;

-- name: GetActiveNodeTask :one
SELECT *
FROM probe_tasks
WHERE node_id = ? AND status IN (
    'pending', 'acknowledged', 'running', 'verifying', 'installing', 'restarting'
)
ORDER BY created_at, id
LIMIT 1;

-- name: GetLatestProbeTask :one
SELECT *
FROM probe_tasks
WHERE node_id = ? AND kind = 'complete-probe'
ORDER BY created_at DESC, id DESC
LIMIT 1;

-- name: GetLatestAgentUpdateTask :one
SELECT *
FROM probe_tasks
WHERE node_id = ? AND kind = 'agent-update'
ORDER BY created_at DESC, id DESC
LIMIT 1;

-- name: ListLatestAgentUpdateTasks :many
SELECT task.*
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
