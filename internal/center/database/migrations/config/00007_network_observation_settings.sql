-- +goose Up
CREATE TABLE network_observation_settings (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    ipv4_services TEXT NOT NULL CHECK (length(ipv4_services) BETWEEN 2 AND 16384),
    ipv6_services TEXT NOT NULL CHECK (length(ipv6_services) BETWEEN 2 AND 16384),
    updated_at INTEGER NOT NULL
);

INSERT INTO network_observation_settings (id, ipv4_services, ipv6_services, updated_at)
VALUES (
    1,
    '["https://api.ipify.org","https://ipv4.icanhazip.com","https://v4.ident.me"]',
    '["https://api6.ipify.org","https://ipv6.icanhazip.com","https://v6.ident.me"]',
    unixepoch()
);
