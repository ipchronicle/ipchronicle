# IPChronicle

IPChronicle is a self-hosted IP address and IP quality history product for one
administrator managing Linux nodes under their control. It discovers durable
network egresses, records confirmed public-address changes, runs the official
IPQuality probe on demand or on a local schedule, compares retained reports,
and delivers change notifications.

The product is one repository containing the Go Center, root Linux Agent,
React web interface, OpenAPI contract, Docker Compose deployment, tests, and
release tooling. The Center runs on Linux with Docker Compose. Agents make only
outbound connections and require no inbound management port.

IPChronicle is licensed under `AGPL-3.0-only`. Product scope and architecture
decisions are maintained in the
[IPChronicle workspace](https://github.com/ipchronicle/workspace/tree/main/docs).

## Release Documentation

- [Operator guide](OPERATOR_GUIDE.md)
- [Notification senders and delivery behavior](NOTIFICATIONS.md)
- [Release readiness](RELEASE_READINESS.md)

The operator guide is the starting point for installation, reverse proxying,
Agent enrollment, network egresses, probes, history, updates, recovery, and
uninstallation. Every published release contains these documents together
with its versioned Compose file, Agent installer, checksums, manifest, SBOMs,
build metadata, and release-readiness report.

## Development

Repository-level development requires Docker with Docker Compose, GNU Make,
`curl`, and `jq`. Direct frontend work uses Node.js 24.19.0 and npm 11.17.0;
direct Go work uses Go 1.26.5. The standard checks use pinned containers:

```sh
make generate
make check
make compose-smoke
make browser-test
```

- `make generate` regenerates the Go and TypeScript OpenAPI bindings and the
  separate `config.db` and `history.db` sqlc packages.
- `make check` validates generated drift, formatting, lint, types, unit and
  race tests, the native Center, no-CGO AMD64/ARM64 Agents, and the production
  web build.
- `make compose-smoke` validates the production image and Compose boundary.
- `make browser-test` validates desktop and mobile Chromium workflows in
  Simplified Chinese and English.

See [AGENTS.md](AGENTS.md) for repository ownership and engineering rules.
