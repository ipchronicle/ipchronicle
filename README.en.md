# IPChronicle

[简体中文](README.md) | English

[![CI](https://github.com/ipchronicle/ipchronicle/actions/workflows/ci.yml/badge.svg)](https://github.com/ipchronicle/ipchronicle/actions/workflows/ci.yml)
[![Latest release](https://img.shields.io/github/v/release/ipchronicle/ipchronicle?display_name=tag&sort=semver)](https://github.com/ipchronicle/ipchronicle/releases/latest)
[![License: AGPL-3.0-only](https://img.shields.io/badge/license-AGPL--3.0--only-0f766e)](LICENSE)

IPChronicle is a self-hosted IP address and IP quality history product for one
administrator managing Linux nodes under their control. It discovers durable
public IPv4 and IPv6 addresses through managed nodes, records confirmed address
changes, runs the built-in complete IP-quality probe on demand or on a local
schedule, compares retained reports,
and delivers change notifications.

The product is one repository containing the Go Center, root Linux Agent,
React web interface, OpenAPI contract, Docker Compose deployment, tests, and
release tooling. The Center runs on Linux with Docker Compose. Agents make only
outbound connections and require no inbound management port.

IPChronicle is licensed under `AGPL-3.0-only`. Product scope and architecture
decisions are maintained in the
[IPChronicle workspace](https://github.com/ipchronicle/workspace/tree/main/docs).

## Highlights

- Discover distinct public IPv4 and IPv6 egresses across direct and node-scoped
  proxy paths without exposing interface-level topology as the product model.
- Run complete probes manually, on local schedules, or when an established node
  discovers a new public address.
- Retain address changes and report history, compare snapshots, and export or
  copy report images.
- Deliver configurable Telegram, webhook, and JavaScript notifications in text
  or image form.
- Use the administrator interface in Simplified Chinese or English with
  optional TOTP two-factor authentication.
- Run outbound-only AMD64 or ARM64 Linux Agents under systemd or Alpine/OpenRC.

## Getting Started

Use the [latest stable release](https://github.com/ipchronicle/ipchronicle/releases/latest)
and follow the [online deployment
guide](https://ipchronicle.github.io/en/guide/deploy-center) to deploy the Center
with Docker Compose. After signing in, the Nodes page generates a one-line
command that enrolls and starts each root Agent. The corresponding Release also
provides a frozen offline operator guide.

The Center does not terminate TLS or manage a reverse proxy. HTTPS is
recommended, while certificate and reverse-proxy operation remain under the
self-hosting administrator's control.

## Documentation

- [Online documentation](https://ipchronicle.github.io/en/) · [简体中文](https://ipchronicle.github.io/)
- [Operator guide frozen with the current source](OPERATOR_GUIDE.en.md) · [简体中文](OPERATOR_GUIDE.md)
- [Notification guide frozen with the current source](NOTIFICATIONS.en.md) · [简体中文](NOTIFICATIONS.md)
- [Release notes](RELEASE_NOTES.en.md) · [简体中文](RELEASE_NOTES.md)
- [Release readiness](RELEASE_READINESS.en.md) · [简体中文](RELEASE_READINESS.md)
- [Product and architecture decisions](https://github.com/ipchronicle/workspace/tree/main/docs)

The online documentation is the starting point for installation, reverse
proxying, Agent enrollment, public-IP discovery, probing, history, upgrades,
recovery, and removal for the latest stable release. Every release includes
matching frozen documentation, the Compose file, Agent installer, checksums,
manifest, SBOMs, build metadata, and release-readiness report.

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

Direct Center builds must set `GOEXPERIMENT=nogreenteagc`; the Make and Docker
builds apply it automatically to preserve the JavaScript worker memory-limit
boundary under Go 1.26.

- `make generate` regenerates the Go and TypeScript OpenAPI bindings and the
  separate `config.db` and `history.db` sqlc packages.
- `make check` validates generated drift, formatting, lint, types, unit and
  race tests, the native Center, no-CGO AMD64/ARM64 Agents, and the production
  web build.
- `make compose-smoke` validates the production image and Compose boundary.
- `make browser-test` validates desktop and mobile Chromium workflows in
  Simplified Chinese and English.

See [AGENTS.md](AGENTS.md) for repository ownership and engineering rules.

## Contributing And Security

Read the organization-wide
[contribution guide](https://github.com/ipchronicle/.github/blob/main/CONTRIBUTING.en.md)
before opening a pull request. Use a structured
[issue](https://github.com/ipchronicle/ipchronicle/issues/new/choose) for bugs
and feature requests. Report suspected vulnerabilities privately through the
[security policy](https://github.com/ipchronicle/.github/blob/main/SECURITY.en.md).
