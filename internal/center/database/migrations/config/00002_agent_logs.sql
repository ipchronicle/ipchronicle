-- +goose Up
ALTER TABLE nodes ADD COLUMN log_level TEXT NOT NULL DEFAULT 'info'
CHECK(log_level IN('error', 'warn', 'info', 'debug'));
ALTER TABLE nodes ADD COLUMN desired_configuration_updated_at INTEGER NOT NULL DEFAULT 0;
UPDATE nodes
SET desired_configuration_updated_at = registered_at
WHERE desired_configuration_updated_at = 0;

CREATE TABLE log_retention_settings(
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
  CHECK((mode = 'age') = (max_age_days IS NOT NULL)),
  CHECK((mode = 'size') = (max_logical_bytes IS NOT NULL))
);
INSERT INTO log_retention_settings(
  id, mode, max_age_days, max_logical_bytes, updated_at,
  last_cleanup_at, last_cleanup_deleted_items, last_cleanup_error
) VALUES (1, 'age', 7, NULL, unixepoch(), NULL, 0, NULL);

-- +goose Down
DROP TABLE log_retention_settings;
