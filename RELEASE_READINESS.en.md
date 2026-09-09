# IPChronicle v0.1.2 Release Readiness

[简体中文](RELEASE_READINESS.md) | English

Status: Pre-release validation in progress

This report defines the scope, artifacts, and validation gates for `v0.1.2`.
The final candidate's `release-manifest.json` and `checksums.txt` record the
exact source revision and artifact digests.

## Release Identity

- Version: `0.1.2`
- Tag: `v0.1.2`
- Channel: `stable`
- License: `AGPL-3.0-only`
- Source: <https://github.com/ipchronicle/ipchronicle/tree/v0.1.2>
- Release: <https://github.com/ipchronicle/ipchronicle/releases/tag/v0.1.2>
- Center image: `ghcr.io/ipchronicle/ipchronicle-center:v0.1.2`

## Release Scope

This release delivers:

- single-administrator authentication, sessions, TOTP, and server-local account
  recovery;
- root Agent enrollment, persistent identity, 30-second polling, temporary
  WebSocket sync, and atomic updates;
- Linux AMD64/ARM64 public-egress discovery, node-scoped proxies, NAT markers,
  and address-change history;
- manual, scheduled, and new-public-IP complete probes, structured and raw
  results, and snapshot comparison;
- Telegram text or image, Webhook, and isolated JavaScript notifications;
- a bilingual administrator interface;
- separate configuration, history, and Agent operational log databases;
- node log levels, offline log uploads, filters, and bounded request retries;
- node identity recovery, batch operations, report semantics, and PNG fixes; and
- Docker Compose examples for a conventional reverse proxy and Cloudflare
  Tunnel.

## Validation Gates

Ordinary CI checks generated files, formatting, linting, types, ordinary unit
tests, static analysis, committed secrets, and the production Web build. The
release-candidate workflow always builds and verifies both Agent architectures,
Center images, SBOMs, manifests, checksums, version metadata, and source
revision metadata.

The candidate workflow selects additional source and race, distribution
lifecycle, resource and live-probe, Compose, browser, failure-recovery, and
reproducibility gates from the version and changes since the previous stable
release. It runs full validation when the change cannot be classified reliably.
A gate classified as inapplicable may be skipped; every selected gate must
succeed before publication.

<!-- release-evidence:start -->
Candidate validation has not completed.
<!-- release-evidence:end -->

## Release Artifacts

The candidate directory contains:

- no-CGO Agent binaries for Linux AMD64 and ARM64;
- OCI Center images for Linux AMD64 and ARM64;
- CycloneDX SBOMs for the Agent and Center;
- `compose.yaml`, `compose.cloudflare-tunnel.yaml`, and
  `default.env.example`;
- `install-agent.sh`;
- operator documentation, license, third-party notices, and build metadata;
- `release-manifest.json` and `checksums.txt` covering every controlled file.

The release verifier rejects missing, extra, oversized, digest-mismatched, or
incorrectly executable files. The publication workflow accepts only a
successful candidate from the same source revision.

## Deployment And Data Boundaries

- The Center supports Linux with Docker Compose. An operator-managed reverse
  proxy terminates TLS.
- Agents run as root on the documented AMD64/ARM64 Linux distributions with
  systemd or OpenRC.
- `v0.1.1` is the initial persisted-data compatibility baseline. New ordered
  configuration migrations preserve accounts, nodes, credential references,
  schedules, and history. The history format and Agent state schema 9 remain
  unchanged. Stable-version fixture tests cover configuration, history,
  identity, and offline queues through upgrades.
- Development builds, release candidates, and `v0.1.0` are not supported data
  upgrade sources.
- Compose stores configuration, history, and logs in `./data/config`,
  `./data/history`, and `./data/logs` under the installation directory.
- Upgrade the Center before Agents. Direct Center downgrade after migration is
  unsupported; restore matching pre-upgrade backups to roll back. Missing ipapi
  results and NAT/target inconsistencies still require affected-node logs.
- The product has no built-in backup or restore feature. Operators preserve
  both data directories and related Agent state consistently when needed.

## Publication Conditions

The publication workflow runs only after ordinary CI and every selected
release gate pass, and an annotated tag points to the same revision. It
revalidates the candidate, publishes and checks both GHCR architectures and the
GitHub Release, then marks the stable release as Latest and updates GHCR
`latest`.
