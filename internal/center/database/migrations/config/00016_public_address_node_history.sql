-- +goose Up
CREATE TABLE public_address_nodes (
    public_address_id TEXT NOT NULL REFERENCES public_addresses(id) ON DELETE CASCADE,
    node_id TEXT NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
    first_seen_at INTEGER NOT NULL,
    last_seen_at INTEGER NOT NULL CHECK (last_seen_at >= first_seen_at),
    PRIMARY KEY (public_address_id, node_id)
);

CREATE INDEX public_address_nodes_node_idx
    ON public_address_nodes (node_id, last_seen_at DESC, public_address_id);
