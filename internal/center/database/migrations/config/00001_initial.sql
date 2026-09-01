-- +goose Up
CREATE TABLE administrators(
  id INTEGER PRIMARY KEY CHECK(id = 1),
  username TEXT NOT NULL COLLATE NOCASE UNIQUE
  CHECK(length(username) BETWEEN 1 AND 64),
  password_hash TEXT NOT NULL,
  locale TEXT NOT NULL DEFAULT 'en'
  CHECK(locale IN('zh-CN', 'en')),
  uses_default_credentials INTEGER NOT NULL DEFAULT 0
  CHECK(uses_default_credentials IN(0, 1)),
  totp_secret_encrypted BLOB,
  totp_enabled INTEGER NOT NULL DEFAULT 0
  CHECK(totp_enabled IN(0, 1)),
  totp_last_used_step INTEGER NOT NULL DEFAULT -1,
  created_at INTEGER NOT NULL,
  credentials_updated_at INTEGER NOT NULL
);
CREATE TABLE administrator_sessions(
  token_digest BLOB PRIMARY KEY CHECK(length(token_digest) = 32),
  created_at INTEGER NOT NULL,
  expires_at INTEGER NOT NULL,
  client_address TEXT NOT NULL,
  user_agent TEXT NOT NULL,
  CHECK(expires_at > created_at)
);
CREATE INDEX administrator_sessions_expires_at_idx
ON administrator_sessions(
  expires_at
);
CREATE TABLE system_state(
  id INTEGER PRIMARY KEY CHECK(id = 1),
  history_generation TEXT NOT NULL CHECK(length(history_generation) = 64),
  pending_history_generation TEXT
  CHECK(pending_history_generation IS NULL OR length(pending_history_generation) = 64),
  history_reset_at INTEGER,
  release_channel TEXT NOT NULL DEFAULT 'stable'
  CHECK(release_channel IN('stable', 'rc')),
  external_origin TEXT NOT NULL DEFAULT ''
  CHECK(length(external_origin) <= 2048),
  CHECK(pending_history_generation IS NULL OR pending_history_generation != history_generation)
);
CREATE TABLE agent_enrollment(
  id INTEGER PRIMARY KEY CHECK(id = 1),
  enabled INTEGER NOT NULL CHECK(enabled IN(0, 1)),
  key_digest BLOB NOT NULL CHECK(length(key_digest) = 32),
  key_encrypted BLOB NOT NULL,
  default_probe_timezone TEXT NOT NULL
  CHECK(length(default_probe_timezone) BETWEEN 1 AND 128),
  created_at INTEGER NOT NULL,
  rotated_at INTEGER NOT NULL CHECK(rotated_at >= created_at)
);
CREATE TABLE nodes(
  id TEXT PRIMARY KEY CHECK(length(id) = 36),
  name TEXT NOT NULL CHECK(length(name) BETWEEN 1 AND 64),
  hostname TEXT NOT NULL CHECK(length(hostname) BETWEEN 1 AND 253),
  credential_digest BLOB NOT NULL UNIQUE CHECK(length(credential_digest) = 32),
  enabled INTEGER NOT NULL DEFAULT 1 CHECK(enabled IN(0, 1)),
  revoked_at INTEGER,
  agent_version TEXT NOT NULL CHECK(length(agent_version) BETWEEN 1 AND 64),
  operating_system TEXT NOT NULL CHECK(operating_system = 'linux'),
  architecture TEXT NOT NULL CHECK(architecture IN('amd64', 'arm64')),
  desired_configuration_revision INTEGER NOT NULL DEFAULT 0
  CHECK(desired_configuration_revision >= 0),
  applied_configuration_revision INTEGER NOT NULL DEFAULT 0
  CHECK(applied_configuration_revision >= 0),
  configuration_error TEXT
  CHECK(configuration_error IS NULL OR length(configuration_error) BETWEEN 1 AND 1024),
  registered_at INTEGER NOT NULL,
  last_seen_at INTEGER
  ,
  configuration_error_revision INTEGER
  CHECK(configuration_error_revision IS NULL OR configuration_error_revision >= 1),
  physical_memory_bytes INTEGER
  CHECK(physical_memory_bytes IS NULL OR physical_memory_bytes >= 1),
  probe_schedule_enabled INTEGER NOT NULL DEFAULT 1
  CHECK(probe_schedule_enabled IN(0, 1)),
  probe_schedule_cron TEXT NOT NULL DEFAULT '0 0 0 * * *'
  CHECK(length(probe_schedule_cron) BETWEEN 9 AND 128),
  probe_schedule_timezone TEXT NOT NULL DEFAULT 'UTC'
  CHECK(length(probe_schedule_timezone) BETWEEN 1 AND 128),
  probe_low_memory_override INTEGER NOT NULL DEFAULT 0
  CHECK(probe_low_memory_override IN(0, 1)),
  probe_on_new_address INTEGER NOT NULL DEFAULT 1
  CHECK(probe_on_new_address IN(0, 1)),
  agent_revision TEXT
  CHECK(agent_revision IS NULL OR length(agent_revision) BETWEEN 1 AND 64)
);
CREATE INDEX nodes_name_idx ON nodes(name COLLATE NOCASE, id);
CREATE INDEX nodes_last_seen_idx ON nodes(last_seen_at);
CREATE TABLE node_capabilities(
  node_id TEXT NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
  capability TEXT NOT NULL CHECK(length(capability) BETWEEN 1 AND 64),
  PRIMARY KEY(node_id, capability)
);
CREATE TABLE revoked_agent_credentials(
  credential_digest BLOB PRIMARY KEY CHECK(length(credential_digest) = 32),
  revoked_at INTEGER NOT NULL,
  reason TEXT NOT NULL CHECK(reason IN('revoked', 'deleted'))
);
CREATE TABLE node_deletion_operations(
  node_id TEXT PRIMARY KEY CHECK(length(node_id) = 36),
  credential_digest BLOB NOT NULL CHECK(length(credential_digest) = 32),
  status TEXT NOT NULL CHECK(status IN('pending', 'failed', 'completed')),
  requested_at INTEGER NOT NULL,
  updated_at INTEGER NOT NULL,
  last_error TEXT CHECK(last_error IS NULL OR length(last_error) BETWEEN 1 AND 1024)
);
CREATE INDEX node_deletion_operations_status_idx
ON node_deletion_operations(
  status,
  requested_at
);
CREATE TABLE node_sync_sessions(
  node_id TEXT PRIMARY KEY REFERENCES nodes(id) ON DELETE CASCADE,
  session_id TEXT NOT NULL UNIQUE CHECK(length(session_id) = 36),
  requested_at INTEGER NOT NULL,
  expires_at INTEGER NOT NULL CHECK(expires_at > requested_at),
  delivered_at INTEGER,
  CHECK(delivered_at IS NULL OR(delivered_at >= requested_at AND delivered_at < expires_at))
);
CREATE INDEX node_sync_sessions_expires_at_idx
ON node_sync_sessions(
  expires_at
);
CREATE TABLE node_network_inventories(
  node_id TEXT PRIMARY KEY REFERENCES nodes(id) ON DELETE CASCADE,
  payload TEXT,
  captured_at INTEGER,
  received_at INTEGER NOT NULL,
  last_error TEXT CHECK(last_error IS NULL OR length(last_error) BETWEEN 1 AND 1024),
  CHECK((payload IS NULL) =(captured_at IS NULL))
);
CREATE TABLE network_proxies(
  id TEXT PRIMARY KEY CHECK(length(id) = 36),
  node_id TEXT NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
  name TEXT NOT NULL CHECK(length(name) BETWEEN 1 AND 128),
  scheme TEXT NOT NULL CHECK(scheme IN('http', 'https', 'socks5')),
  host TEXT NOT NULL CHECK(length(host) BETWEEN 1 AND 253),
  port INTEGER NOT NULL CHECK(port BETWEEN 1 AND 65535),
  username TEXT CHECK(username IS NULL OR length(username) BETWEEN 1 AND 512),
  password_encrypted BLOB,
  enabled INTEGER NOT NULL DEFAULT 1 CHECK(enabled IN(0, 1)),
  deletion_requested_at INTEGER,
  created_at INTEGER NOT NULL,
  updated_at INTEGER NOT NULL CHECK(updated_at >= created_at),
  CHECK(deletion_requested_at IS NULL OR deletion_requested_at >= created_at)
);
CREATE UNIQUE INDEX network_proxies_node_name_idx
ON network_proxies(
  node_id,
  name COLLATE NOCASE
);
CREATE INDEX network_proxies_node_idx
ON network_proxies(
  node_id,
  deletion_requested_at,
  name
);
CREATE TABLE network_observation_settings(
  id INTEGER PRIMARY KEY CHECK(id = 1),
  ipv4_services TEXT NOT NULL CHECK(length(ipv4_services) BETWEEN 2 AND 16384),
  ipv6_services TEXT NOT NULL CHECK(length(ipv6_services) BETWEEN 2 AND 16384),
  updated_at INTEGER NOT NULL
);
CREATE TABLE egress_deletion_operations(
  egress_id TEXT PRIMARY KEY CHECK(length(egress_id) = 36),
  node_id TEXT NOT NULL CHECK(length(node_id) = 36),
  status TEXT NOT NULL CHECK(status IN('pending', 'failed', 'completed')),
  requested_at INTEGER NOT NULL,
  updated_at INTEGER NOT NULL,
  last_error TEXT CHECK(last_error IS NULL OR length(last_error) BETWEEN 1 AND 1024)
);
CREATE INDEX egress_deletion_operations_status_idx
ON egress_deletion_operations(
  status,
  requested_at,
  egress_id
);
CREATE INDEX egress_deletion_operations_node_idx
ON egress_deletion_operations(
  node_id,
  status,
  egress_id
);
CREATE TABLE node_probe_status(
  node_id TEXT PRIMARY KEY REFERENCES nodes(id) ON DELETE CASCADE,
  active_run_id TEXT CHECK(active_run_id IS NULL OR length(active_run_id) = 36),
  next_scheduled_at INTEGER,
  last_occurrence_at INTEGER,
  last_occurrence_trigger TEXT CHECK(last_occurrence_trigger IS NULL OR
last_occurrence_trigger IN('manual', 'schedule', 'new-address')),
  last_occurrence_status TEXT CHECK(last_occurrence_status IS NULL OR
last_occurrence_status IN('started', 'skipped')),
  last_skip_reason TEXT CHECK(last_skip_reason IS NULL OR
last_skip_reason IN('busy', 'disabled', 'low-memory', 'no-egress', 'missed')),
  history_reset_generation TEXT CHECK(history_reset_generation IS NULL OR
length(history_reset_generation) = 64),
  history_reset_at INTEGER,
  history_reset_discarded_address_items INTEGER NOT NULL DEFAULT 0
  CHECK(history_reset_discarded_address_items >= 0),
  history_reset_discarded_probe_items INTEGER NOT NULL DEFAULT 0
  CHECK(history_reset_discarded_probe_items >= 0),
  reported_at INTEGER NOT NULL,
  CHECK((last_occurrence_at IS NULL) =(last_occurrence_status IS NULL)),
  CHECK((last_occurrence_status = 'skipped') =(last_skip_reason IS NOT NULL)),
  CHECK((history_reset_generation IS NULL) =(history_reset_at IS NULL)),
  CHECK(history_reset_generation IS NOT NULL OR(history_reset_discarded_address_items = 0 AND history_reset_discarded_probe_items = 0))
);
CREATE TABLE history_retention_settings(
  id INTEGER PRIMARY KEY CHECK(id = 1),
  mode TEXT NOT NULL CHECK(mode IN('indefinite', 'age', 'size')),
  max_age_days INTEGER CHECK(max_age_days IS NULL OR max_age_days BETWEEN 1 AND 36500),
  max_logical_bytes INTEGER CHECK(max_logical_bytes IS NULL OR
max_logical_bytes BETWEEN 1048576 AND 1099511627776),
  updated_at INTEGER NOT NULL,
  last_cleanup_at INTEGER,
  last_cleanup_deleted_items INTEGER NOT NULL DEFAULT 0
  CHECK(last_cleanup_deleted_items >= 0),
  last_cleanup_error TEXT CHECK(last_cleanup_error IS NULL OR
length(CAST(last_cleanup_error AS BLOB)) BETWEEN 1 AND 4096),
  CHECK((mode = 'age') =(max_age_days IS NOT NULL)),
  CHECK((mode = 'size') =(max_logical_bytes IS NOT NULL))
);
CREATE TABLE notification_senders(
  id TEXT PRIMARY KEY CHECK(length(id) = 36),
  name TEXT NOT NULL CHECK(length(name) BETWEEN 1 AND 128),
  kind TEXT NOT NULL CHECK(kind IN('telegram', 'webhook', 'javascript')),
  enabled INTEGER NOT NULL DEFAULT 1 CHECK(enabled IN(0, 1)),
  configuration_encrypted BLOB NOT NULL
  CHECK(length(configuration_encrypted) BETWEEN 30 AND 1048608),
  created_at INTEGER NOT NULL,
  updated_at INTEGER NOT NULL CHECK(updated_at >= created_at)
);
CREATE UNIQUE INDEX notification_senders_name_idx
ON notification_senders(
  name COLLATE NOCASE
);
CREATE TABLE probe_tasks(
  id TEXT PRIMARY KEY CHECK(length(id) = 36),
  node_id TEXT NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
  kind TEXT NOT NULL CHECK(kind IN('complete-probe', 'agent-update')),
  status TEXT NOT NULL CHECK(status IN('pending', 'acknowledged', 'running', 'verifying', 'installing',
'restarting', 'succeeded', 'partial', 'failed', 'rolled-back',
'rejected', 'expired')),
  created_at INTEGER NOT NULL,
  expires_at INTEGER NOT NULL CHECK(expires_at > created_at),
  acknowledged_at INTEGER,
  started_at INTEGER,
  completed_at INTEGER,
  run_id TEXT CHECK(run_id IS NULL OR length(run_id) = 36),
  rejection_reason TEXT CHECK(rejection_reason IS NULL OR
rejection_reason IN('busy', 'disabled', 'low-memory', 'no-egress', 'missed')),
  target_version TEXT CHECK(target_version IS NULL OR length(target_version) BETWEEN 5 AND 64),
  previous_version TEXT CHECK(previous_version IS NULL OR length(previous_version) BETWEEN 1 AND 64),
  result_version TEXT CHECK(result_version IS NULL OR length(result_version) BETWEEN 1 AND 64),
  failure_code TEXT CHECK(failure_code IS NULL OR length(failure_code) BETWEEN 1 AND 64),
  diagnostic TEXT CHECK(diagnostic IS NULL OR length(diagnostic) BETWEEN 1 AND 4096),
  terminal_confirmed_at INTEGER,
  trigger TEXT NOT NULL DEFAULT 'manual'
  CHECK(trigger IN('manual', 'new-address', 'agent-update')),
  CHECK((kind = 'agent-update') =(target_version IS NOT NULL)),
  CHECK(kind = 'complete-probe' OR(run_id IS NULL AND rejection_reason IS NULL)),
  CHECK(started_at IS NULL OR acknowledged_at IS NOT NULL),
  CHECK(completed_at IS NULL OR acknowledged_at IS NOT NULL OR status = 'expired')
);
CREATE UNIQUE INDEX probe_tasks_active_node_idx
ON probe_tasks(
  node_id
)
WHERE status IN(
  'pending',
  'acknowledged',
  'running',
  'verifying',
  'installing',
  'restarting'
);
CREATE INDEX probe_tasks_node_created_idx
ON probe_tasks(
  node_id,
  created_at DESC,
  id
);
CREATE INDEX probe_tasks_terminal_cleanup_idx
ON probe_tasks(
  completed_at
)
WHERE status IN(
  'succeeded',
  'partial',
  'failed',
  'rolled-back',
  'rejected',
  'expired'
);
CREATE TABLE public_address_paths(
  public_address_id TEXT NOT NULL REFERENCES public_addresses(id) ON DELETE CASCADE,
  path_id TEXT NOT NULL UNIQUE REFERENCES network_egresses(id) ON DELETE CASCADE,
  node_id TEXT NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
  local_interface TEXT,
  local_address TEXT,
  proxy_path INTEGER NOT NULL CHECK(proxy_path IN(0, 1)),
  likely_nat INTEGER NOT NULL CHECK(likely_nat IN(0, 1)),
  temporary INTEGER NOT NULL CHECK(temporary IN(0, 1)),
  available INTEGER NOT NULL CHECK(available IN(0, 1)),
  last_checked_at INTEGER NOT NULL,
  last_succeeded_at INTEGER,
  PRIMARY KEY(public_address_id, path_id)
);
CREATE INDEX public_address_paths_node_idx
ON public_address_paths(
  node_id,
  available,
  public_address_id,
  path_id
);
CREATE INDEX public_address_paths_selection_idx
ON public_address_paths(
  public_address_id,
  available,
  last_succeeded_at DESC,
  path_id
);
CREATE TABLE notification_rules(
  id TEXT PRIMARY KEY CHECK(length(id) = 36),
  name TEXT NOT NULL CHECK(length(name) BETWEEN 1 AND 128),
  enabled INTEGER NOT NULL DEFAULT 1 CHECK(enabled IN(0, 1)),
  sender_id TEXT NOT NULL REFERENCES notification_senders(id) ON DELETE RESTRICT,
  event_type TEXT NOT NULL CHECK(event_type IN('probe-field-change', 'address-change', 'address-check-failure',
'address-check-recovery', 'probe-failure', 'probe-recovery',
'address-gap', 'probe-gap', 'format-mismatch', 'format-changed',
'format-recovery')),
  field_id TEXT CHECK(field_id IS NULL OR length(field_id) BETWEEN 1 AND 256),
  node_id TEXT REFERENCES nodes(id) ON DELETE CASCADE,
  egress_id TEXT REFERENCES public_addresses(id) ON DELETE CASCADE,
  created_at INTEGER NOT NULL,
  updated_at INTEGER NOT NULL CHECK(updated_at >= created_at),
  CHECK(field_id IS NULL OR event_type = 'probe-field-change')
);
CREATE UNIQUE INDEX notification_rules_name_idx
ON notification_rules(
  name COLLATE NOCASE
);
CREATE INDEX notification_rules_sender_idx
ON notification_rules(
  sender_id,
  enabled,
  event_type,
  id
);
CREATE INDEX notification_rules_scope_idx
ON notification_rules(
  node_id,
  egress_id,
  event_type,
  enabled,
  id
);
CREATE TABLE public_address_nodes(
  public_address_id TEXT NOT NULL REFERENCES public_addresses(id) ON DELETE CASCADE,
  node_id TEXT NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
  first_seen_at INTEGER NOT NULL,
  last_seen_at INTEGER NOT NULL CHECK(last_seen_at >= first_seen_at),
  PRIMARY KEY(public_address_id, node_id)
);
CREATE INDEX public_address_nodes_node_idx
ON public_address_nodes(
  node_id,
  last_seen_at DESC,
  public_address_id
);
CREATE TABLE pending_public_address_probes(
  node_id TEXT NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
  public_address_id TEXT NOT NULL
  REFERENCES public_addresses(id) ON DELETE CASCADE,
  required_configuration_revision INTEGER NOT NULL CHECK(required_configuration_revision >= 1),
  created_at INTEGER NOT NULL,
  PRIMARY KEY(node_id, public_address_id)
);
CREATE INDEX pending_public_address_probes_ready_idx
ON pending_public_address_probes(
  required_configuration_revision,
  created_at
);
CREATE TABLE network_egresses(
  id TEXT PRIMARY KEY CHECK(length(id) = 36),
  node_id TEXT NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
  name TEXT NOT NULL CHECK(length(name) BETWEEN 1 AND 128),
  kind TEXT NOT NULL CHECK(kind IN('default', 'interface', 'source', 'proxy')),
  family TEXT NOT NULL CHECK(family IN('ipv4', 'ipv6')),
  interface_name TEXT CHECK(interface_name IS NULL OR length(interface_name) BETWEEN 1 AND 64),
  source_address TEXT CHECK(source_address IS NULL OR length(source_address) BETWEEN 2 AND 45),
  proxy_id TEXT REFERENCES network_proxies(id) ON DELETE CASCADE,
  enabled INTEGER NOT NULL DEFAULT 1 CHECK(enabled IN(0, 1)),
  available INTEGER NOT NULL DEFAULT 1 CHECK(available IN(0, 1)),
  automatic INTEGER NOT NULL DEFAULT 0 CHECK(automatic IN(0, 1)),
  lightweight_interval_seconds INTEGER NOT NULL DEFAULT 600
  CHECK(lightweight_interval_seconds >= 1),
  created_at INTEGER NOT NULL,
  updated_at INTEGER NOT NULL CHECK(updated_at >= created_at),
  CHECK((kind = 'default' AND interface_name IS NULL AND source_address IS NULL AND proxy_id IS NULL AND automatic = 1)
OR(kind = 'interface' AND interface_name IS NOT NULL AND source_address IS NULL AND proxy_id IS NULL AND automatic = 0)
OR(kind = 'source' AND interface_name IS NOT NULL AND source_address IS NOT NULL AND proxy_id IS NULL)
OR(kind = 'proxy' AND interface_name IS NULL AND source_address IS NULL AND proxy_id IS NOT NULL AND automatic = 0))
);
CREATE UNIQUE INDEX network_egresses_default_idx
ON network_egresses(
  node_id,
  family
)
WHERE kind = 'default';
CREATE UNIQUE INDEX network_egresses_selector_idx
ON network_egresses(
  node_id,
  kind,
  family,
  interface_name,
  COALESCE(source_address, '')
)
WHERE kind IN(
  'interface',
  'source'
);
CREATE UNIQUE INDEX network_egresses_proxy_idx
ON network_egresses(
  node_id,
  proxy_id,
  family
)
WHERE kind = 'proxy';
CREATE INDEX network_egresses_node_idx
ON network_egresses(
  node_id,
  family,
  kind,
  created_at,
  id
);
CREATE INDEX network_egresses_proxy_reference_idx
ON network_egresses(
  proxy_id,
  node_id
)
WHERE proxy_id IS NOT NULL;
CREATE TABLE public_addresses(
  id TEXT PRIMARY KEY CHECK(length(id) = 36),
  address TEXT NOT NULL UNIQUE CHECK(length(address) BETWEEN 2 AND 45),
  family TEXT NOT NULL CHECK(family IN('ipv4', 'ipv6')),
  probe_enabled INTEGER NOT NULL DEFAULT 1 CHECK(probe_enabled IN(0, 1)),
  selected_path_id TEXT REFERENCES network_egresses(id) ON DELETE SET NULL,
  first_seen_at INTEGER NOT NULL,
  last_seen_at INTEGER NOT NULL CHECK(last_seen_at >= first_seen_at),
  updated_at INTEGER NOT NULL CHECK(updated_at >= first_seen_at)
);
CREATE INDEX public_addresses_selected_path_idx
ON public_addresses(
  selected_path_id
);
CREATE TABLE probe_task_public_addresses(
  task_id TEXT NOT NULL REFERENCES probe_tasks(id) ON DELETE CASCADE,
  public_address_id TEXT NOT NULL REFERENCES public_addresses(id) ON DELETE CASCADE,
  ordinal INTEGER NOT NULL CHECK(ordinal BETWEEN 0 AND 63),
  PRIMARY KEY(task_id, public_address_id),
  UNIQUE(task_id, ordinal)
);
CREATE INDEX probe_task_public_addresses_address_idx
ON probe_task_public_addresses(
  public_address_id,
  task_id
);

INSERT INTO network_observation_settings (
  id,
  ipv4_services,
  ipv6_services,
  updated_at
) VALUES (
  1,
  '["https://api.ipify.org","https://ipv4.icanhazip.com","https://v4.ident.me"]',
  '["https://api6.ipify.org","https://ipv6.icanhazip.com","https://v6.ident.me"]',
  unixepoch()
);

INSERT INTO history_retention_settings (
  id,
  mode,
  max_age_days,
  max_logical_bytes,
  updated_at,
  last_cleanup_at,
  last_cleanup_deleted_items,
  last_cleanup_error
) VALUES (
  1,
  'indefinite',
  NULL,
  NULL,
  unixepoch(),
  NULL,
  0,
  NULL
);
