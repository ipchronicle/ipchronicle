-- +goose Up
CREATE TABLE public_addresses (
    id TEXT PRIMARY KEY CHECK (length(id) = 36),
    address TEXT NOT NULL UNIQUE CHECK (length(address) BETWEEN 2 AND 45),
    family TEXT NOT NULL CHECK (family IN ('ipv4', 'ipv6')),
    probe_enabled INTEGER NOT NULL DEFAULT 0 CHECK (probe_enabled IN (0, 1)),
    probe_on_rediscovery INTEGER NOT NULL DEFAULT 1
        CHECK (probe_on_rediscovery IN (0, 1)),
    selected_path_id TEXT REFERENCES network_egresses(id) ON DELETE SET NULL,
    first_seen_at INTEGER NOT NULL,
    last_seen_at INTEGER NOT NULL CHECK (last_seen_at >= first_seen_at),
    updated_at INTEGER NOT NULL CHECK (updated_at >= first_seen_at)
);

CREATE TABLE public_address_paths (
    public_address_id TEXT NOT NULL REFERENCES public_addresses(id) ON DELETE CASCADE,
    path_id TEXT NOT NULL UNIQUE REFERENCES network_egresses(id) ON DELETE CASCADE,
    node_id TEXT NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
    local_interface TEXT,
    local_address TEXT,
    proxy_path INTEGER NOT NULL CHECK (proxy_path IN (0, 1)),
    likely_nat INTEGER NOT NULL CHECK (likely_nat IN (0, 1)),
    temporary INTEGER NOT NULL CHECK (temporary IN (0, 1)),
    available INTEGER NOT NULL CHECK (available IN (0, 1)),
    last_checked_at INTEGER NOT NULL,
    last_succeeded_at INTEGER,
    PRIMARY KEY (public_address_id, path_id)
);

CREATE INDEX public_addresses_selected_path_idx
    ON public_addresses (selected_path_id);

CREATE INDEX public_address_paths_node_idx
    ON public_address_paths (node_id, available, public_address_id, path_id);

CREATE INDEX public_address_paths_selection_idx
    ON public_address_paths (public_address_id, available, last_succeeded_at DESC, path_id);
