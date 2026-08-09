-- +goose Up
CREATE TABLE history_retention_settings (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    mode TEXT NOT NULL CHECK (mode IN ('indefinite', 'age', 'size')),
    max_age_days INTEGER CHECK (max_age_days IS NULL OR max_age_days BETWEEN 1 AND 36500),
    max_logical_bytes INTEGER CHECK (
        max_logical_bytes IS NULL OR
        max_logical_bytes BETWEEN 1048576 AND 1099511627776
    ),
    updated_at INTEGER NOT NULL,
    last_cleanup_at INTEGER,
    last_cleanup_deleted_items INTEGER NOT NULL DEFAULT 0
        CHECK (last_cleanup_deleted_items >= 0),
    last_cleanup_error TEXT CHECK (
        last_cleanup_error IS NULL OR
        length(CAST(last_cleanup_error AS BLOB)) BETWEEN 1 AND 4096
    ),
    CHECK ((mode = 'age') = (max_age_days IS NOT NULL)),
    CHECK ((mode = 'size') = (max_logical_bytes IS NOT NULL))
);

INSERT INTO history_retention_settings (
    id, mode, max_age_days, max_logical_bytes, updated_at,
    last_cleanup_at, last_cleanup_deleted_items, last_cleanup_error
) VALUES (
    1, 'indefinite', NULL, NULL, CAST(strftime('%s', 'now') AS INTEGER),
    NULL, 0, NULL
);
