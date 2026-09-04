# IPChronicle v0.1.1

[简体中文](RELEASE_NOTES.md) | English

IPChronicle is a personal self-hosted service for discovering the public IPs
reachable from managed Linux nodes, running complete IP-quality probes, and
tracking meaningful result changes over time.

This is the first IPChronicle release intended for production use. There is no
production data from `v0.1.0`, so this release uses a clean deployment and does
not migrate development, release-candidate, or `v0.1.0` data.

## Highlights

- Runs the complete IP-quality probe directly in the root Agent as a bounded,
  no-CGO Go implementation on Linux AMD64 and ARM64. A failed provider leaves
  only its own fields unavailable.
- Treats public IPs as the probe subjects. Current and historical addresses are
  separated visually, while reports remain associated with the actual public
  IP that produced them.
- Discovers direct and node-scoped proxy exits, supports IPv4 and IPv6, marks
  NAT observations, enables new public IPs by default, and can automatically
  probe new addresses on established nodes.
- Keeps one-time manual target selection separate from recurring probe
  enablement. Scheduled probes use a searchable IANA timezone and show the next
  execution time.
- Adds a monitoring overview, moves system status under Settings, and keeps
  node details focused on current public IPs, probes, and history.
- Adds broad notification routing, Telegram groups and optional topics,
  unsaved destination tests, and selectable text or image delivery for every
  event. Known probe values are rendered as human-readable Simplified Chinese
  or English while raw JSON and machine event values remain unchanged.
- Adds an optional centrally managed ipapi API key, removes the unused IPWHOIS
  field, and bounds retries for transient IPQS failures.
- Keeps Center-delivered probes and Agent updates recoverable when the two
  hosts' clocks differ. An RC4 Agent stuck retrying an out-of-window task
  report recovers after a non-purge reinstall of this release.
- Simplifies Docker Compose deployment by storing configuration and history in
  `./data/config` and `./data/history`, and includes a Cloudflare Tunnel example
  that requires only a Tunnel token.
- Tiers daily CI and release validation by change risk. GHCR `latest` moves
  only after a stable GitHub Release is published successfully.

## Deployment

- Center: Linux with Docker Compose, `linux/amd64` and `linux/arm64` images.
- Agent: root service on the documented Linux distributions using systemd or
  OpenRC, on AMD64 or ARM64.
- TLS: terminate HTTPS in an operator-managed reverse proxy when desired.

See the [operator guide](OPERATOR_GUIDE.en.md) for installation and operation.

## Known Boundaries

- One local administrator; no multi-user, tenant, role, or public-result mode.
- No built-in backup or restore workflow. Back up both Center data directories
  and Agent state consistently when preservation is required.
- Third-party database and media services can change or rate-limit responses.
  Unknown values remain visible instead of being guessed or silently mapped.
- An HTTP Center URL is allowed but is not protected in transit.
