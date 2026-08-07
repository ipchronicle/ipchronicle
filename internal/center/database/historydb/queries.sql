-- name: GetHistoryMetadata :one
SELECT id, generation, created_at
FROM history_metadata
WHERE id = 1;

-- name: CreateHistoryMetadata :exec
INSERT INTO history_metadata (id, generation, created_at)
VALUES (1, ?, ?);
