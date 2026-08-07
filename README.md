# IPChronicle

IPChronicle is a self-hosted IP quality monitoring product for an individual
owner managing Linux nodes under their control.

The first release is being built as one product repository containing the Go
center, root Linux Agent, React web interface, OpenAPI contract, deployment
assets, tests, and release tooling. The center is deployed with Docker Compose;
Agents use outbound-only communication and require no inbound management port.

The product source is licensed under `AGPL-3.0-only`. Product scope and
architecture decisions are maintained in the
[IPChronicle workspace](https://github.com/ipchronicle/workspace/tree/main/docs).

## Repository Status

Phase 0 provides a reproducible end-to-end foundation: a real Center health
and status API, a generated typed client, a bilingual status interface, an
embedded production web build, a no-CGO Agent build, and a supported Compose
topology. Probe, persistence, enrollment, and authentication behavior belong
to later vertical slices and are not represented by placeholder success paths.

## Run The Center

```sh
docker compose -f deploy/compose.yaml up -d --build
```

The Center is then available at <http://127.0.0.1:8080>. Set
`IPCHRONICLE_HTTP_PORT` to publish another host port. The application stores
future durable state at `/var/lib/ipchronicle` in the `center-data` volume.
Phase 0 does not provide a built-in backup command; external volume copies are
an operator responsibility.

## Development

Prerequisites:

- Docker with Docker Compose and GNU Make for repository-level commands
- `curl` and `jq` for the Compose smoke test
- Node.js 24.19.0 and npm 11.17.0 for direct frontend work
- Go 1.26.5 for direct Go work

Repository-level commands use pinned containers and project dependencies:

```sh
make generate
make check
make compose-smoke
make browser-test
```

- `make generate` regenerates committed Go and TypeScript OpenAPI bindings.
- `make check` validates generated drift, formatting, lint, types, unit and race
  tests, native Center build, no-CGO Agent builds for AMD64 and ARM64, and the
  production web build.
- `make compose-smoke` builds the production image and checks health, API, web,
  and SPA/API routing boundaries.
- `make browser-test` checks the bilingual and theme workflows in desktop and
  mobile Chromium.

Dependabot proposes Go, npm, Docker, and GitHub Actions updates weekly. Updates
are reviewed and validated through the same checks; they are not merged
automatically.

See [AGENTS.md](AGENTS.md) for ownership and engineering rules.
