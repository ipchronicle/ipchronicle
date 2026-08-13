-- +goose Up
ALTER TABLE system_state ADD COLUMN external_origin TEXT NOT NULL DEFAULT ''
    CHECK (length(external_origin) <= 2048);
