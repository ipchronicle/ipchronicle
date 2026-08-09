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

Phase 1 provides the Center persistence and administrator boundary: separate
configuration and history databases, embedded migrations, an installation
master key, administrator bootstrap and recovery, persistent sessions, CSRF
and origin enforcement, TOTP, account settings, and an authenticated bilingual
status interface.

Phase 2 now provides automatic Agent enrollment, root-only encrypted bbolt
identity storage, 30-second authenticated control polling, two-minute online
state, complete desired-configuration snapshots with atomic revision
convergence, node disable/revocation/permanent deletion, a systemd/OpenRC
installer, the bilingual node inventory, and optional ten-minute WebSocket
sessions that wake the ordinary HTTP synchronization path while an
administrator is editing a node. Network egress discovery and complete probes
remain later slices; the current control API does not expose placeholder
success for them.

## Run The Center

```sh
docker compose -f deploy/compose.yaml up -d --build
```

The Center is then available at <http://127.0.0.1:8080>. Set
`IPCHRONICLE_HTTP_PORT` to publish another host port. The initial administrator
username and password are both `admin`; the interface warns while those
defaults remain active but does not force a change.

Compose stores `config.db` and the installation master key in the
`center-config` volume, and `history.db` in the independent `center-history`
volume. Deliberately removing history must not require removing account or
configuration state. The first release does not provide a built-in backup
command; external volume copies are an operator responsibility.

## Enroll A Node

Sign in, open **Nodes**, and generate the automatic-registration key. The page
shows one command to run as root on a supported Linux node. The command checks
the operating system, CPU architecture, and init system before changing the
host; installs the documented IPQuality dependencies; verifies the matching
Agent version from the official GitHub Release checksums; registers the node;
and starts a systemd
or OpenRC service.

The shared registration key is used only during enrollment. The Agent stores
its node-specific credential encrypted in `/var/lib/ipchronicle-agent/state.db`
using a separate root-only local master key, and the installed service contains
neither credential. Re-running the command preserves an existing valid local
identity. Disabling automatic enrollment prevents new nodes without
disconnecting registered Agents.

The pre-release installer recognizes Debian, Ubuntu, RHEL, Rocky Linux,
AlmaLinux, CentOS, and Alpine on Linux AMD64 or ARM64. The exact supported
distribution-version matrix is fixed and tested for each public release.
Until the repository publishes its first GitHub Release assets, the generated
installation command is a release-path preview and cannot download an Agent
binary from GitHub.

Bootstrap credentials can be supplied before the first start. They are read
only when `config.db` has no administrator account:

```sh
IPCHRONICLE_ADMIN_USERNAME=owner \
IPCHRONICLE_ADMIN_PASSWORD='choose-a-strong-password' \
docker compose -f deploy/compose.yaml up -d --build
```

For reverse-proxy deployments, set `IPCHRONICLE_EXTERNAL_URL` to the exact
browser-facing HTTP or HTTPS origin. Set `IPCHRONICLE_TRUSTED_PROXIES` to the
comma-separated proxy CIDRs that may supply `X-Forwarded-For` and
`X-Forwarded-Proto`. IPChronicle does not terminate TLS; HTTPS is recommended
at the operator-managed reverse proxy, while intentional HTTP remains usable
with a visible warning. Configure the reverse proxy to forward WebSocket
Upgrade requests under `/api/v1/agent/sync/` to use temporary sync sessions.
Without WebSocket support, Agents continue normal 30-second HTTP polling and
the node page reports the temporary session as degraded.

## Local Administrator Recovery

These commands are intended for a server operator with local Compose access.
Both revoke every administrator session:

```sh
read -r -s IPCHRONICLE_RECOVERY_PASSWORD
printf '%s\n' "$IPCHRONICLE_RECOVERY_PASSWORD" | \
  docker compose -f deploy/compose.yaml exec -T center \
    /usr/local/bin/ipchronicle-center admin reset-password --password-stdin
unset IPCHRONICLE_RECOVERY_PASSWORD

docker compose -f deploy/compose.yaml exec -T center \
  /usr/local/bin/ipchronicle-center admin disable-totp
```

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
- `make generate` also runs pinned `sqlc` generation independently for
  `config.db` and `history.db`.
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
