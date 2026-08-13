-- +goose Up
CREATE TABLE address_states (
    egress_id TEXT PRIMARY KEY,
    node_id TEXT NOT NULL,
    history_generation TEXT NOT NULL CHECK (length(history_generation) = 64),
    family TEXT NOT NULL CHECK (family IN ('ipv4', 'ipv6')),
    status TEXT NOT NULL CHECK (status IN ('confirmed', 'failed')),
    sequence INTEGER NOT NULL CHECK (sequence >= 0),
    public_address TEXT,
    local_interface TEXT,
    local_address TEXT,
    proxy_path INTEGER NOT NULL CHECK (proxy_path IN (0, 1)),
    likely_nat INTEGER NOT NULL CHECK (likely_nat IN (0, 1)),
    temporary INTEGER NOT NULL CHECK (temporary IN (0, 1)),
    failure_reason TEXT CHECK (failure_reason IN (
        'selector-unavailable', 'no-valid-response',
        'confirmation-unavailable', 'conflicting-responses'
    )),
    last_checked_at INTEGER NOT NULL,
    last_succeeded_at INTEGER,
    last_changed_at INTEGER,
    received_at INTEGER NOT NULL
);

CREATE INDEX address_states_node_id_idx
    ON address_states (node_id, egress_id);

CREATE TABLE address_events (
    id TEXT PRIMARY KEY,
    public_address_id TEXT NOT NULL,
    source_path_id TEXT NOT NULL,
    node_id TEXT NOT NULL,
    history_generation TEXT NOT NULL CHECK (length(history_generation) = 64),
    sequence INTEGER NOT NULL CHECK (sequence >= 1),
    kind TEXT NOT NULL CHECK (kind IN (
        'first-observation', 'address-change', 'check-failure', 'recovery'
    )),
    family TEXT NOT NULL CHECK (family IN ('ipv4', 'ipv6')),
    previous_address TEXT,
    public_address TEXT,
    local_interface TEXT,
    local_address TEXT,
    proxy_path INTEGER NOT NULL CHECK (proxy_path IN (0, 1)),
    likely_nat INTEGER NOT NULL CHECK (likely_nat IN (0, 1)),
    temporary INTEGER NOT NULL CHECK (temporary IN (0, 1)),
    failure_reason TEXT CHECK (failure_reason IN (
        'selector-unavailable', 'no-valid-response',
        'confirmation-unavailable', 'conflicting-responses'
    )),
    observed_at INTEGER NOT NULL,
    received_at INTEGER NOT NULL,
    UNIQUE (source_path_id, history_generation, sequence)
);

CREATE INDEX address_events_node_time_idx
    ON address_events (node_id, observed_at DESC, sequence DESC);

CREATE INDEX address_events_public_address_time_idx
    ON address_events (public_address_id, observed_at DESC, sequence DESC);

CREATE TABLE history_gaps (
    id TEXT PRIMARY KEY,
    egress_id TEXT NOT NULL,
    node_id TEXT NOT NULL,
    history_generation TEXT NOT NULL CHECK (length(history_generation) = 64),
    kind TEXT NOT NULL CHECK (kind = 'address'),
    dropped_count INTEGER NOT NULL CHECK (dropped_count >= 1),
    first_sequence INTEGER NOT NULL CHECK (first_sequence >= 1),
    last_sequence INTEGER NOT NULL CHECK (last_sequence >= first_sequence),
    first_observed_at INTEGER NOT NULL,
    last_observed_at INTEGER NOT NULL,
    received_at INTEGER NOT NULL
);

CREATE INDEX history_gaps_node_time_idx
    ON history_gaps (node_id, last_observed_at DESC);
