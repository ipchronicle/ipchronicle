# IPChronicle v0.1.0-rc.5 Release Readiness

Status: Pre-publication validation in progress

This report maps this public release candidate to its product scope,
automated validation, artifacts, operational documentation, and publication
gate. The source revision and artifact digests in `release-manifest.json` and
`checksums.txt` identify a particular build. A candidate is not ready to
publish merely because this file exists: every required gate below must pass
for the manifest revision, and the final run links must be recorded here.

## Candidate Identity

- Version: `0.1.0-rc.5`
- Proposed tag: `v0.1.0-rc.5`
- Channel: `rc`
- License: `AGPL-3.0-only`
- Source: <https://github.com/ipchronicle/ipchronicle>
- Publication state: no tag, GitHub Release, public Center image, or stable
  channel entry is authorized by this report

## Product Scope Evidence

| Capability | Implementation evidence | Deterministic validation |
| --- | --- | --- |
| Single-administrator authentication, sessions, TOTP, and local recovery | `internal/center/admin`, `cmd/ipchronicle-center` | package tests, Compose smoke, browser tests, migration and recovery failure gate |
| Agent enrollment, persistent identity, 30-second polling, configuration convergence, and temporary sync | `internal/agent`, `internal/center/nodes`, `internal/center/syncws` | package and race tests, Compose smoke, browser tests, distribution lifecycle tests |
| Linux interface, address, route, egress, proxy, NAT, and temporary-IPv6 handling | `internal/agent/network`, `internal/agent/observation`, `internal/center/nodes` | inventory, selector, proxy, observation, outage, restart, and queue tests |
| Manual, scheduled, and newly-current-address complete probes with one immediate slot | `internal/agent/probe`, `internal/schedule`, `internal/center/nodes` | scheduler, native execution, result publication, retry, resource, and live complete-probe tests |
| Known-field interpretation, raw results, localized semantic presentation, format drift, comparison, starring, and retention | `internal/probefields`, `internal/center/history`, `internal/center/nodes`, `web/src/pages/probe-snapshot-page.tsx` | interpretation, value presentation, comparison, retention, reset, capacity, API, and browser tests |
| Telegram text/image, Webhook, and isolated JavaScript notifications | `internal/center/notifications`, `cmd/ipchronicle-center` | sender, renderer, queue, retry, isolation, redaction, overflow, API, and browser tests |
| Agent update discovery, validation, atomic replacement, health commitment, and rollback | `internal/agent/update`, `internal/center/updates` | update manager, supervisor, rollback, distribution lifecycle, and browser tests |
| Bilingual administrator interface | `web/src`, `web/src/locales` | locale parity, component, production build, and desktop/mobile Chromium tests |
| Separate configuration and history ownership | `internal/center/database/migrations`, independent sqlc packages | migration, corruption, reset, retention, Compose, and failure-gate tests |

## Required Validation Gates

All rows are fail-closed. A missing, cancelled, skipped, or inconclusive run is
not a pass.

| Gate | Command or workflow evidence | Required result |
| --- | --- | --- |
| Local release-version, source, generated-file, formatting, lint, type, ordinary-test, and static-analysis preflight | `make preflight` | Pass before push |
| Generated bindings, ordinary and race tests, native Center, no-CGO Agents, and production web build | `make check`; GitHub Actions `CI / check` | Pass on the candidate revision |
| Committed-source secret scan | `make secret-scan`; GitHub Actions `CI / check` | Pass, no leaks |
| Production Center image and Compose boundary | `make compose-smoke`; GitHub Actions `CI / compose` | Pass |
| Simplified Chinese and English desktop/mobile workflows | `make browser-test`; GitHub Actions `CI / browser` | Pass |
| AMD64 and ARM64 Center image metadata | GitHub Actions `CI / image (linux/amd64)` and `CI / image (linux/arm64)` | Pass |
| Candidate creation, manifest, checksums, SBOMs, and artifact contract | `make release-candidate VERSION=0.1.0-rc.5`; `make verify-release-candidate VERSION=0.1.0-rc.5`; `Release candidate artifact / candidate` | Pass |
| Install, reinstall, uninstall, migration, history reset, outage, restart, unavailable selector, update rollback, and queue overflow | `make release-failure-gate`; `Release candidate artifact / candidate` | Pass |
| Supported distribution and init lifecycle | 17 distributions x AMD64/ARM64 in `Release candidate artifact / distribution` | All 34 pass |
| Native resource limits and live complete-probe execution | AMD64 and ARM64 `Release candidate artifact / resources` | Both pass at 64 MiB Agent and 512 MiB Center limits |
| 70-node and 420-egress capacity | `internal/center/nodes/release_capacity_test.go`; `make check` | Pass |
| Reproducibility | two clean builds from the manifest revision; compare names, modes, sizes, and SHA-256 for every file | Exact match |
| Source hygiene | `shellcheck scripts/*.sh`; `actionlint .github/workflows/*.yml`; `git diff --check`; clean product worktree | Pass |

## Validation Results

<!-- release-evidence:start -->
The final candidate revision, successful ordinary CI URL, successful release
candidate workflow URL, reproducibility comparison, and validation date will be
recorded here after every required gate has completed. Until then, the status
of this report remains **Pre-publication validation in progress**.
<!-- release-evidence:end -->

## Release Artifact Contract

The release directory contains these operator-facing and machine-verifiable
assets:

- no-CGO Agent binaries for Linux AMD64 and ARM64;
- OCI Center images for Linux AMD64 and ARM64;
- CycloneDX SBOMs for both Agent binaries and both Center images;
- `compose.yaml` and `default.env.example`;
- `install-agent.sh`;
- `README.md`, `OPERATOR_GUIDE.md`, `NOTIFICATIONS.md`, and this report;
- `LICENSE`, `THIRD_PARTY_NOTICES.md`, and `build-metadata.json`;
- `release-manifest.json` covering the controlled artifact set; and
- `checksums.txt` covering every controlled artifact and the manifest.

The release verifier rejects missing, extra, non-regular, oversized, tampered,
or unexpectedly non-executable files. The manifest records the source revision,
channel, capabilities, size, SHA-256, operating system, and architecture where
applicable.

`RELEASE_NOTES.md` is the source for the GitHub prerelease description. The
candidate builder rejects release-facing documentation and workflow defaults
that do not identify the same version.

## Operator Workflow Coverage

`OPERATOR_GUIDE.md` is the standalone operating entry point. It covers Center
installation and upgrade, environment variables, reverse proxying, Agent
enrollment and service inspection, egress and proxy configuration, address
observation, complete probes, history and comparison, retention and reset,
notifications, Agent updates and rollback behavior, uninstall, local account
recovery, and backup/disaster-recovery boundaries. `NOTIFICATIONS.md` defines
sender configuration, the JavaScript API, queue limits, retries, failure
isolation, and redaction behavior.

## RC Limitations And Boundaries

- The product serves one personal self-hosting administrator. It has no
  multi-user, tenant, role, or public-result feature.
- The Center is supported only on Linux with Docker Compose. It does not
  terminate TLS or configure an operator's reverse proxy.
- Agents require root and support only the documented Linux AMD64/ARM64
  distribution matrix with systemd or OpenRC.
- The first release has no built-in backup or restore command. The operator is
  responsible for consistent volume and Agent-state backups.
- Development and RC `config.db`, `history.db`, and Agent-state data are not
  promised to remain compatible before the first stable release is published
  and put into use. Incompatible development data fails explicitly and is
  rebuilt or purged by the operator.
- The built-in Go probe is derived from the AGPL-licensed IPQuality behavior at
  the revision recorded in `THIRD_PARTY_NOTICES.md`. Its database, media, AI,
  DNSBL, and mail checks still depend on third-party services whose availability
  and response formats IPChronicle does not control.
- HTTP, HTTPS, and SMTP checks use the selected egress path. DNS and DNSBL
  lookups use the node's resolver and are not tunneled through an egress proxy.
  The interface reports unavailable provider data and format changes without
  presenting unsupported routing guarantees.

## Publication Gate

After every gate passes, this report must identify the exact candidate revision
and successful run URLs. Publication then still requires an explicit human
decision and an immutable tag for that revision. An explicit dispatch of the
`Publish release` workflow revalidates the final candidate artifact, publishes
and anonymously verifies both Center image platforms, verifies every GitHub
Release asset, and only then publishes the prerelease. Publication must not
happen implicitly, and changing a stable channel remains outside this workflow.
