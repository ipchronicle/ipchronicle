-- +goose Up
CREATE TABLE history_metadata (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    generation TEXT NOT NULL UNIQUE CHECK (length(generation) = 64),
    created_at INTEGER NOT NULL
);
