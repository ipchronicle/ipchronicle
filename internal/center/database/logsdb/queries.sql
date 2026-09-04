-- name: InsertLogEvent :execrows
INSERT INTO log_events(
  id, source, node_id, occurred_at, received_at, level, component, event_type,
  message, public_address_id, public_address, family, task_id, proxy_id,
  configuration_revision, discovery_path, failure_category, request_method,
  request_target, http_status, duration_milliseconds, response_content_type,
  rate_limit_headers, response_body, response_truncated, dropped_count,
  dropped_from, dropped_to, logical_bytes
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO NOTHING;

-- name: GetLogEvent :one
SELECT * FROM log_events WHERE id = ?;

-- name: ListLogEvents :many
SELECT id, source, node_id, occurred_at, received_at, level, component,
       event_type, message, public_address_id, public_address, family,
       task_id, proxy_id, configuration_revision, failure_category,
       request_method, request_target, http_status, duration_milliseconds,
       response_content_type,
       CAST(COALESCE(length(response_body), 0) AS INTEGER) AS response_body_bytes,
       response_truncated, dropped_count,
       dropped_from, dropped_to, logical_bytes
FROM log_events
WHERE (sqlc.narg(from_time) IS NULL OR occurred_at >= sqlc.narg(from_time))
  AND (sqlc.narg(to_time) IS NULL OR occurred_at <= sqlc.narg(to_time))
  AND (sqlc.narg(node_id) IS NULL OR node_id = sqlc.narg(node_id))
  AND (sqlc.narg(level) IS NULL OR level = sqlc.narg(level))
  AND (sqlc.narg(component) IS NULL OR component = sqlc.narg(component))
  AND (sqlc.narg(event_type) IS NULL OR event_type = sqlc.narg(event_type))
  AND (sqlc.narg(public_address) IS NULL OR public_address = sqlc.narg(public_address))
  AND (sqlc.narg(task_id) IS NULL OR task_id = sqlc.narg(task_id))
  AND (sqlc.narg(proxy_id) IS NULL OR proxy_id = sqlc.narg(proxy_id))
  AND (sqlc.narg(keyword) IS NULL OR instr(message, sqlc.narg(keyword)) > 0)
  AND (sqlc.narg(cursor_time) IS NULL OR occurred_at < sqlc.narg(cursor_time)
       OR (occurred_at = sqlc.narg(cursor_time) AND id < sqlc.narg(cursor_id)))
ORDER BY occurred_at DESC, id DESC
LIMIT sqlc.arg(page_size);

-- name: SumLogLogicalBytes :one
SELECT CAST(COALESCE(SUM(logical_bytes), 0) AS INTEGER) FROM log_events;

-- name: CountLogEvents :one
SELECT COUNT(*) FROM log_events;

-- name: DeleteLogEventsOlderThan :execrows
DELETE FROM log_events WHERE id IN (
  SELECT candidate.id FROM log_events AS candidate
  WHERE candidate.occurred_at < ?
  ORDER BY candidate.occurred_at, candidate.id
  LIMIT ?
);

-- name: DeleteOldestLogEvents :execrows
DELETE FROM log_events WHERE id IN (
  SELECT candidate.id FROM log_events AS candidate
  ORDER BY candidate.occurred_at, candidate.id
  LIMIT ?
);

-- name: ListOldestLogEventSizes :many
SELECT logical_bytes
FROM log_events
ORDER BY occurred_at, id
LIMIT ?;
