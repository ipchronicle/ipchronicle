-- +goose Up
CREATE TABLE node_recovery_credentials(
  node_id TEXT PRIMARY KEY REFERENCES nodes(id) ON DELETE CASCADE,
  key_digest BLOB NOT NULL UNIQUE CHECK(length(key_digest) = 32),
  key_encrypted BLOB NOT NULL,
  rotated_at INTEGER NOT NULL
);

-- +goose Down
DROP TABLE node_recovery_credentials;
