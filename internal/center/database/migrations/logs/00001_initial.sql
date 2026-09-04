-- +goose Up
CREATE TABLE log_events(
  id TEXT PRIMARY KEY CHECK(length(id) = 36),
  source TEXT NOT NULL CHECK(source IN('agent', 'center')),
  node_id TEXT CHECK(node_id IS NULL OR length(node_id) = 36),
  occurred_at INTEGER NOT NULL,
  received_at INTEGER NOT NULL,
  level TEXT NOT NULL CHECK(level IN('error', 'warn', 'info', 'debug')),
  component TEXT NOT NULL CHECK(length(component) BETWEEN 1 AND 64),
  event_type TEXT NOT NULL CHECK(length(event_type) BETWEEN 1 AND 96),
  message TEXT NOT NULL CHECK(length(CAST(message AS BLOB)) BETWEEN 1 AND 4096),
  public_address_id TEXT CHECK(public_address_id IS NULL OR length(public_address_id) = 36),
  public_address TEXT CHECK(public_address IS NULL OR length(public_address) BETWEEN 2 AND 45),
  family TEXT CHECK(family IS NULL OR family IN('ipv4', 'ipv6')),
  task_id TEXT CHECK(task_id IS NULL OR length(task_id) = 36),
  proxy_id TEXT CHECK(proxy_id IS NULL OR length(proxy_id) = 36),
  configuration_revision INTEGER CHECK(configuration_revision IS NULL OR configuration_revision >= 0),
  discovery_path TEXT CHECK(discovery_path IS NULL OR length(CAST(discovery_path AS BLOB)) BETWEEN 1 AND 2048),
  failure_category TEXT CHECK(failure_category IS NULL OR failure_category IN(
    'dns', 'connect', 'tls', 'timeout', 'rate-limit', 'http-status',
    'response-too-large', 'invalid-response', 'internal'
  )),
  request_method TEXT CHECK(request_method IS NULL OR length(request_method) BETWEEN 3 AND 16),
  request_target TEXT CHECK(request_target IS NULL OR length(CAST(request_target AS BLOB)) BETWEEN 1 AND 2048),
  http_status INTEGER CHECK(http_status IS NULL OR http_status BETWEEN 100 AND 599),
  duration_milliseconds INTEGER CHECK(duration_milliseconds IS NULL OR duration_milliseconds >= 0),
  response_content_type TEXT CHECK(response_content_type IS NULL OR
  length(CAST(response_content_type AS BLOB)) BETWEEN 1 AND 256),
  rate_limit_headers TEXT CHECK(rate_limit_headers IS NULL OR
  length(CAST(rate_limit_headers AS BLOB)) BETWEEN 2 AND 17408),
  response_body BLOB CHECK(response_body IS NULL OR length(response_body) <= 4194304),
  response_truncated INTEGER NOT NULL DEFAULT 0 CHECK(response_truncated IN(0, 1)),
  dropped_count INTEGER CHECK(dropped_count IS NULL OR dropped_count >= 1),
  dropped_from INTEGER,
  dropped_to INTEGER,
  logical_bytes INTEGER NOT NULL CHECK(logical_bytes >= 0),
  CHECK((dropped_count IS NULL) = (dropped_from IS NULL)),
  CHECK((dropped_count IS NULL) = (dropped_to IS NULL))
);
CREATE INDEX log_events_time_idx ON log_events(occurred_at DESC, id);
CREATE INDEX log_events_node_time_idx ON log_events(node_id, occurred_at DESC, id);
CREATE INDEX log_events_level_time_idx ON log_events(level, occurred_at DESC, id);
CREATE INDEX log_events_component_time_idx ON log_events(component, occurred_at DESC, id);
CREATE INDEX log_events_type_time_idx ON log_events(event_type, occurred_at DESC, id);
CREATE INDEX log_events_address_time_idx ON log_events(public_address, occurred_at DESC, id);
CREATE INDEX log_events_task_time_idx ON log_events(task_id, occurred_at DESC, id);
CREATE INDEX log_events_proxy_time_idx ON log_events(proxy_id, occurred_at DESC, id);

-- +goose Down
DROP TABLE log_events;
