-- +goose Up
ALTER TABLE notification_rules ADD COLUMN excluded_event_types_json TEXT NOT NULL DEFAULT '[]' CHECK(json_valid(excluded_event_types_json));
ALTER TABLE notification_rules ADD COLUMN excluded_field_prefixes_json TEXT NOT NULL DEFAULT '[]' CHECK(json_valid(excluded_field_prefixes_json));

-- +goose Down
-- SQLite cannot drop columns safely across the supported versions. This migration
-- is forward-only like the other released configuration migrations.
