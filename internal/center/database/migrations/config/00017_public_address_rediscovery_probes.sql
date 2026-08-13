-- +goose Up
ALTER TABLE probe_tasks ADD COLUMN trigger TEXT NOT NULL DEFAULT 'manual'
    CHECK (trigger IN ('manual', 'address-change', 'agent-update'));

UPDATE probe_tasks SET trigger = 'agent-update' WHERE kind = 'agent-update';

ALTER TABLE probe_tasks ADD COLUMN triggering_public_address_id TEXT
    REFERENCES public_addresses(id) ON DELETE SET NULL;

CREATE TABLE pending_public_address_probes (
    node_id TEXT PRIMARY KEY REFERENCES nodes(id) ON DELETE CASCADE,
    public_address_id TEXT NOT NULL
        REFERENCES public_addresses(id) ON DELETE CASCADE,
    required_configuration_revision INTEGER NOT NULL CHECK (required_configuration_revision >= 1),
    created_at INTEGER NOT NULL
);

CREATE INDEX pending_public_address_probes_ready_idx
    ON pending_public_address_probes (required_configuration_revision, created_at);
