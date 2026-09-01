-- +goose Up
CREATE TABLE history_metadata(
  id INTEGER PRIMARY KEY CHECK(id = 1),
  generation TEXT NOT NULL UNIQUE CHECK(length(generation) = 64),
  created_at INTEGER NOT NULL
);
CREATE TABLE history_nodes(
  node_id TEXT PRIMARY KEY CHECK(length(node_id) = 36),
  node_name TEXT NOT NULL CHECK(length(node_name) BETWEEN 1 AND 128),
  recorded_at INTEGER NOT NULL
);
CREATE TABLE address_states(
  egress_id TEXT PRIMARY KEY,
  node_id TEXT NOT NULL,
  history_generation TEXT NOT NULL CHECK(length(history_generation) = 64),
  family TEXT NOT NULL CHECK(family IN('ipv4', 'ipv6')),
  status TEXT NOT NULL CHECK(status IN('confirmed', 'failed')),
  sequence INTEGER NOT NULL CHECK(sequence >= 0),
  public_address TEXT,
  local_interface TEXT,
  local_address TEXT,
  proxy_path INTEGER NOT NULL CHECK(proxy_path IN(0, 1)),
  likely_nat INTEGER NOT NULL CHECK(likely_nat IN(0, 1)),
  temporary INTEGER NOT NULL CHECK(temporary IN(0, 1)),
  failure_reason TEXT CHECK(failure_reason IN('selector-unavailable', 'no-valid-response',
'confirmation-unavailable', 'conflicting-responses')),
  last_checked_at INTEGER NOT NULL,
  last_succeeded_at INTEGER,
  last_changed_at INTEGER,
  received_at INTEGER NOT NULL
);
CREATE INDEX address_states_node_id_idx
ON address_states(node_id, egress_id);
CREATE TABLE address_events(
  id TEXT PRIMARY KEY,
  public_address_id TEXT NOT NULL,
  source_path_id TEXT NOT NULL,
  node_id TEXT NOT NULL,
  history_generation TEXT NOT NULL CHECK(length(history_generation) = 64),
  sequence INTEGER NOT NULL CHECK(sequence >= 1),
  kind TEXT NOT NULL CHECK(kind IN('first-observation', 'address-added', 'address-removed', 'check-failure', 'recovery')),
  family TEXT NOT NULL CHECK(family IN('ipv4', 'ipv6')),
  public_address TEXT,
  local_interface TEXT,
  local_address TEXT,
  proxy_path INTEGER NOT NULL CHECK(proxy_path IN(0, 1)),
  likely_nat INTEGER NOT NULL CHECK(likely_nat IN(0, 1)),
  temporary INTEGER NOT NULL CHECK(temporary IN(0, 1)),
  failure_reason TEXT CHECK(failure_reason IN('selector-unavailable', 'no-valid-response',
'confirmation-unavailable', 'conflicting-responses')),
  observed_at INTEGER NOT NULL,
  received_at INTEGER NOT NULL,
  UNIQUE(source_path_id, history_generation, sequence)
);
CREATE INDEX address_events_node_time_idx
ON address_events(
  node_id,
  observed_at DESC,
  sequence DESC
);
CREATE INDEX address_events_public_address_time_idx
ON address_events(
  public_address_id,
  observed_at DESC,
  sequence DESC
);
CREATE TABLE history_gaps(
  id TEXT PRIMARY KEY,
  egress_id TEXT NOT NULL,
  node_id TEXT NOT NULL,
  history_generation TEXT NOT NULL CHECK(length(history_generation) = 64),
  kind TEXT NOT NULL CHECK(kind = 'address'),
  dropped_count INTEGER NOT NULL CHECK(dropped_count >= 1),
  first_sequence INTEGER NOT NULL CHECK(first_sequence >= 1),
  last_sequence INTEGER NOT NULL CHECK(last_sequence >= first_sequence),
  first_observed_at INTEGER NOT NULL,
  last_observed_at INTEGER NOT NULL,
  received_at INTEGER NOT NULL
);
CREATE INDEX history_gaps_node_time_idx
ON history_gaps(
  node_id,
  last_observed_at DESC
);
CREATE TABLE probe_runs(
  id TEXT PRIMARY KEY CHECK(length(id) = 36),
  node_id TEXT NOT NULL CHECK(length(node_id) = 36),
  history_generation TEXT NOT NULL CHECK(length(history_generation) = 64),
  configuration_revision INTEGER NOT NULL CHECK(configuration_revision >= 1),
  trigger TEXT NOT NULL CHECK(trigger IN('manual', 'schedule', 'new-address')),
  task_id TEXT CHECK(task_id IS NULL OR length(task_id) = 36),
  status TEXT NOT NULL CHECK(status IN('running', 'succeeded', 'partial', 'failed')),
  expected_executions INTEGER NOT NULL CHECK(expected_executions BETWEEN 1 AND 64),
  started_at INTEGER NOT NULL,
  completed_at INTEGER,
  received_at INTEGER NOT NULL,
  CHECK((status = 'running') =(completed_at IS NULL))
);
CREATE INDEX probe_runs_node_time_idx
ON probe_runs(
  node_id,
  started_at DESC,
  id
);
CREATE TABLE probe_executions(
  id TEXT PRIMARY KEY CHECK(length(id) = 36),
  run_id TEXT NOT NULL REFERENCES probe_runs(id) ON DELETE CASCADE,
  egress_id TEXT NOT NULL CHECK(length(egress_id) = 36),
  ordinal INTEGER NOT NULL CHECK(ordinal BETWEEN 0 AND 63),
  sequence INTEGER NOT NULL CHECK(sequence >= 1),
  status TEXT NOT NULL CHECK(status IN('pending', 'running', 'succeeded', 'failed', 'interrupted', 'skipped')),
  started_at INTEGER,
  completed_at INTEGER,
  failure_stage TEXT CHECK(failure_stage IS NULL OR
failure_stage IN('download', 'selector', 'adapter', 'process', 'timeout', 'output', 'restart')),
  diagnostic TEXT CHECK(diagnostic IS NULL OR length(CAST(diagnostic AS BLOB)) <= 65536),
  received_at INTEGER NOT NULL,
  UNIQUE(run_id, ordinal),
  UNIQUE(egress_id, sequence)
);
CREATE INDEX probe_executions_egress_order_idx
ON probe_executions(
  egress_id,
  sequence DESC
);
CREATE TABLE probe_snapshots(
  id TEXT PRIMARY KEY CHECK(length(id) = 36),
  execution_id TEXT NOT NULL UNIQUE REFERENCES probe_executions(id) ON DELETE CASCADE,
  egress_id TEXT NOT NULL CHECK(length(egress_id) = 36),
  sequence INTEGER NOT NULL CHECK(sequence >= 1),
  observed_at INTEGER NOT NULL,
  raw_result BLOB NOT NULL CHECK(length(raw_result) BETWEEN 1 AND 1048576),
  encoded_size INTEGER NOT NULL CHECK(encoded_size BETWEEN 1 AND 1048576),
  received_at INTEGER NOT NULL,
  CHECK(length(raw_result) = encoded_size),
  UNIQUE(egress_id, sequence)
);
CREATE INDEX probe_snapshots_egress_time_idx
ON probe_snapshots(
  egress_id,
  sequence DESC
);
CREATE TABLE current_probe_snapshots(
  egress_id TEXT PRIMARY KEY CHECK(length(egress_id) = 36),
  execution_id TEXT NOT NULL UNIQUE REFERENCES probe_executions(id) ON DELETE CASCADE,
  snapshot_id TEXT NOT NULL UNIQUE REFERENCES probe_snapshots(id) ON DELETE CASCADE,
  sequence INTEGER NOT NULL CHECK(sequence >= 1),
  observed_at INTEGER NOT NULL,
  received_at INTEGER NOT NULL
);
CREATE TABLE probe_gaps(
  id TEXT PRIMARY KEY CHECK(length(id) = 36),
  egress_id TEXT NOT NULL CHECK(length(egress_id) = 36),
  node_id TEXT NOT NULL CHECK(length(node_id) = 36),
  history_generation TEXT NOT NULL CHECK(length(history_generation) = 64),
  dropped_count INTEGER NOT NULL CHECK(dropped_count >= 1),
  first_sequence INTEGER NOT NULL CHECK(first_sequence >= 1),
  last_sequence INTEGER NOT NULL CHECK(last_sequence >= first_sequence),
  first_observed_at INTEGER NOT NULL,
  last_observed_at INTEGER NOT NULL,
  received_at INTEGER NOT NULL
);
CREATE INDEX probe_gaps_node_time_idx
ON probe_gaps(
  node_id,
  last_observed_at DESC,
  id
);
CREATE TABLE probe_snapshot_stars(
  snapshot_id TEXT PRIMARY KEY REFERENCES probe_snapshots(id) ON DELETE CASCADE,
  starred_at INTEGER NOT NULL
);
CREATE TABLE probe_comparison_progress(
  egress_id TEXT PRIMARY KEY CHECK(length(egress_id) = 36),
  node_id TEXT NOT NULL CHECK(length(node_id) = 36),
  history_generation TEXT NOT NULL CHECK(length(history_generation) = 64),
  next_sequence INTEGER NOT NULL CHECK(next_sequence >= 1),
  last_success_snapshot_id TEXT REFERENCES probe_snapshots(id) ON DELETE SET NULL,
  updated_at INTEGER NOT NULL
);
CREATE TABLE probe_change_sets(
  id TEXT PRIMARY KEY CHECK(length(id) = 36),
  execution_id TEXT NOT NULL UNIQUE REFERENCES probe_executions(id) ON DELETE CASCADE,
  snapshot_id TEXT NOT NULL UNIQUE REFERENCES probe_snapshots(id) ON DELETE CASCADE,
  egress_id TEXT NOT NULL CHECK(length(egress_id) = 36),
  sequence INTEGER NOT NULL CHECK(sequence >= 1),
  previous_snapshot_id TEXT REFERENCES probe_snapshots(id) ON DELETE SET NULL,
  baseline INTEGER NOT NULL CHECK(baseline IN(0, 1)),
  change_count INTEGER NOT NULL CHECK(change_count >= 0),
  observed_at INTEGER NOT NULL,
  recorded_at INTEGER NOT NULL,
  CHECK(baseline = 0 OR previous_snapshot_id IS NULL),
  CHECK(baseline = 0 OR change_count = 0)
);
CREATE INDEX probe_change_sets_egress_order_idx
ON probe_change_sets(
  egress_id,
  sequence DESC
);
CREATE TABLE probe_field_changes(
  change_set_id TEXT NOT NULL REFERENCES probe_change_sets(id) ON DELETE CASCADE,
  field_id TEXT NOT NULL CHECK(length(field_id) BETWEEN 1 AND 256),
  group_name TEXT NOT NULL CHECK(length(group_name) BETWEEN 1 AND 64),
  json_path TEXT NOT NULL CHECK(length(json_path) BETWEEN 1 AND 256),
  value_type TEXT NOT NULL CHECK(value_type IN('string', 'number', 'boolean', 'null')),
  before_value TEXT NOT NULL CHECK(length(CAST(before_value AS BLOB)) <= 65536),
  after_value TEXT NOT NULL CHECK(length(CAST(after_value AS BLOB)) <= 65536),
  PRIMARY KEY(change_set_id, field_id)
);
CREATE TABLE probe_snapshot_formats(
  snapshot_id TEXT PRIMARY KEY REFERENCES probe_snapshots(id) ON DELETE CASCADE,
  status TEXT NOT NULL CHECK(status IN('compatible', 'mismatch')),
  signature TEXT NOT NULL CHECK(length(signature) = 64),
  issue_count INTEGER NOT NULL CHECK(issue_count >= 0),
  issues_json BLOB NOT NULL CHECK(length(issues_json) BETWEEN 2 AND 1048576),
  CHECK((status = 'compatible') =(issue_count = 0))
);
CREATE TABLE probe_format_states(
  egress_id TEXT PRIMARY KEY CHECK(length(egress_id) = 36),
  snapshot_id TEXT NOT NULL UNIQUE REFERENCES probe_snapshots(id) ON DELETE CASCADE,
  sequence INTEGER NOT NULL CHECK(sequence >= 1),
  status TEXT NOT NULL CHECK(status IN('compatible', 'mismatch')),
  signature TEXT NOT NULL CHECK(length(signature) = 64),
  issue_count INTEGER NOT NULL CHECK(issue_count >= 0),
  issues_json BLOB NOT NULL CHECK(length(issues_json) BETWEEN 2 AND 1048576),
  first_observed_at INTEGER NOT NULL,
  last_observed_at INTEGER NOT NULL,
  updated_at INTEGER NOT NULL,
  CHECK((status = 'compatible') =(issue_count = 0)),
  CHECK(last_observed_at >= first_observed_at)
);
CREATE TABLE probe_format_events(
  id TEXT PRIMARY KEY CHECK(length(id) = 36),
  execution_id TEXT NOT NULL UNIQUE REFERENCES probe_executions(id) ON DELETE CASCADE,
  snapshot_id TEXT NOT NULL UNIQUE REFERENCES probe_snapshots(id) ON DELETE CASCADE,
  egress_id TEXT NOT NULL CHECK(length(egress_id) = 36),
  sequence INTEGER NOT NULL CHECK(sequence >= 1),
  kind TEXT NOT NULL CHECK(kind IN('mismatch', 'changed', 'recovered')),
  previous_signature TEXT CHECK(previous_signature IS NULL OR length(previous_signature) = 64),
  current_signature TEXT NOT NULL CHECK(length(current_signature) = 64),
  issue_count INTEGER NOT NULL CHECK(issue_count >= 0),
  issues_json BLOB NOT NULL CHECK(length(issues_json) BETWEEN 2 AND 1048576),
  observed_at INTEGER NOT NULL,
  recorded_at INTEGER NOT NULL
);
CREATE INDEX probe_format_events_egress_order_idx
ON probe_format_events(
  egress_id,
  sequence DESC
);
CREATE TABLE probe_outcome_states(
  egress_id TEXT PRIMARY KEY CHECK(length(egress_id) = 36),
  node_id TEXT NOT NULL CHECK(length(node_id) = 36),
  history_generation TEXT NOT NULL CHECK(length(history_generation) = 64),
  execution_id TEXT NOT NULL UNIQUE REFERENCES probe_executions(id) ON DELETE CASCADE,
  sequence INTEGER NOT NULL CHECK(sequence >= 1),
  status TEXT NOT NULL CHECK(status IN('healthy', 'failed')),
  first_observed_at INTEGER NOT NULL,
  last_observed_at INTEGER NOT NULL,
  updated_at INTEGER NOT NULL,
  CHECK(last_observed_at >= first_observed_at)
);
CREATE TABLE notification_events(
  id TEXT PRIMARY KEY CHECK(length(id) = 36),
  event_type TEXT NOT NULL CHECK(event_type IN('probe-field-change', 'address-change', 'address-check-failure',
'address-check-recovery', 'probe-failure', 'probe-recovery',
'address-gap', 'probe-gap', 'format-mismatch', 'format-changed',
'format-recovery', 'test')),
  source_kind TEXT NOT NULL CHECK(source_kind IN('probe-change-set', 'address-event', 'probe-execution',
'address-gap', 'probe-gap', 'format-event', 'test')),
  source_id TEXT NOT NULL CHECK(length(source_id) BETWEEN 1 AND 64),
  node_id TEXT CHECK(node_id IS NULL OR length(node_id) = 36),
  egress_id TEXT CHECK(egress_id IS NULL OR length(egress_id) = 36),
  payload_json BLOB NOT NULL CHECK(json_valid(payload_json) AND
length(payload_json) BETWEEN 2 AND 1048576),
  observed_at INTEGER NOT NULL,
  recorded_at INTEGER NOT NULL,
  processed_at INTEGER,
  CHECK(event_type = 'test' OR(node_id IS NOT NULL AND(egress_id IS NOT NULL OR event_type = 'address-gap'))),
  UNIQUE(source_kind, source_id, event_type)
);
CREATE INDEX notification_events_processing_idx
ON notification_events(
  processed_at,
  recorded_at,
  id
);
CREATE INDEX notification_events_node_idx
ON notification_events(
  node_id,
  observed_at DESC,
  id
);
CREATE INDEX notification_events_egress_idx
ON notification_events(
  egress_id,
  observed_at DESC,
  id
);
CREATE TABLE notification_deliveries(
  id TEXT PRIMARY KEY CHECK(length(id) = 36),
  event_id TEXT NOT NULL REFERENCES notification_events(id) ON DELETE CASCADE,
  sender_id TEXT NOT NULL CHECK(length(sender_id) = 36),
  sender_name TEXT NOT NULL CHECK(length(sender_name) BETWEEN 1 AND 128),
  sender_kind TEXT NOT NULL CHECK(sender_kind IN('telegram', 'webhook', 'javascript')),
  event_type TEXT NOT NULL,
  node_id TEXT CHECK(node_id IS NULL OR length(node_id) = 36),
  egress_id TEXT CHECK(egress_id IS NULL OR length(egress_id) = 36),
  is_test INTEGER NOT NULL DEFAULT 0 CHECK(is_test IN(0, 1)),
  status TEXT NOT NULL CHECK(status IN('pending', 'running', 'retrying', 'succeeded', 'failed')),
  attempt_count INTEGER NOT NULL DEFAULT 0 CHECK(attempt_count BETWEEN 0 AND 4),
  next_attempt_at INTEGER,
  last_attempt_at INTEGER,
  completed_at INTEGER,
  error_code TEXT CHECK(error_code IS NULL OR length(error_code) BETWEEN 1 AND 64),
  matched_rule_ids_json BLOB NOT NULL CHECK(json_valid(matched_rule_ids_json) AND
length(matched_rule_ids_json) BETWEEN 2 AND 65536),
  event_json BLOB NOT NULL CHECK(json_valid(event_json) AND
length(event_json) BETWEEN 2 AND 1048576),
  title TEXT NOT NULL CHECK(length(CAST(title AS BLOB)) BETWEEN 1 AND 8192),
  body TEXT NOT NULL CHECK(length(CAST(body AS BLOB)) BETWEEN 1 AND 65536),
  created_at INTEGER NOT NULL,
  updated_at INTEGER NOT NULL,
  CHECK((status IN('pending', 'retrying')) =(next_attempt_at IS NOT NULL)),
  CHECK((status IN('succeeded', 'failed')) =(completed_at IS NOT NULL)),
  CHECK((status = 'failed') =(error_code IS NOT NULL)),
  UNIQUE(event_id, sender_id)
);
CREATE INDEX notification_deliveries_work_idx
ON notification_deliveries(
  sender_kind,
  status,
  next_attempt_at,
  created_at,
  id
);
CREATE INDEX notification_deliveries_sender_active_idx
ON notification_deliveries(
  sender_id,
  status,
  id
);
CREATE INDEX notification_deliveries_history_idx
ON notification_deliveries(
  created_at DESC,
  id DESC
);
