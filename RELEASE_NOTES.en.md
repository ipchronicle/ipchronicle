# IPChronicle v0.1.2

[简体中文](RELEASE_NOTES.md) | English

This release adds detailed Agent logs and central queries, improves node management
and reports, and bounds retries for third-party probe requests. Upgrades preserve
v0.1.1 configuration, history, and node identities.

## Highlights

- Agents log discovery, configuration sync, tasks, and third-party requests. The
  Center supports per-node log levels, filters for time, level, component, public
  IP, task, and other fields, plus failure response details.
- Logs use independent storage with seven-day default retention. Agents buffer
  offline logs in a bounded queue and upload them after reconnecting.
- Transient complete-probe request failures receive at most three attempts,
  respecting Retry-After and task deadlines, with retry and final-failure logs.
- Dedicated recovery installation commands let reinstalled hosts take over their
  original nodes while retaining Center configuration and history.
- Node selection supports batch updates, probes, and log-level changes. Public IPs
  have a dialog for their latest successful reports.
- Homepage attention focuses on actionable failures; a NAT path alone is no
  longer an issue.
- Detail navigation preserves its source. Probe confirmation names the node,
  temporary sync has an explanation, and node lists omit source hashes and
  internal configuration counters.
- Reports distinguish Yes, No, and No data. PNG fonts and clipped long labels are
  fixed.
- Development dependencies receive security patches.
- Fixes intermittent memory-limit failures for small JavaScript HTTP deliveries
  by allowing for runtime reservations and collecting Go memory earlier, while
  retaining isolated worker resource and time boundaries.

## Upgrade From v0.1.1

Upgrade the Center before Agents. First stop the Center and consistently back up
`./data/config` and `./data/history`. Retain `/var/lib/ipchronicle-agent` on each node.

The Center applies new log-settings and node-recovery migrations, preserving
accounts, the master key, nodes, proxies, schedules, and history. Agent state
remains at schema 9, preserving identity and offline result queues. Both Compose
examples add a `./data/logs` mount. Add it to custom Compose files to retain
diagnostic logs across container recreation. Cloudflare Tunnel installations
should retain the matching example and token.

After configuration migration, switching directly to an older Center image is
unsupported. To roll back, stop the Center, restore the matching pre-upgrade
backup, and start the older version. See the [operator guide](OPERATOR_GUIDE.en.md).

## Issues Awaiting Diagnosis

- Some ipapi results are missing when nodes share an API key. Quota, concurrency,
  and request failures have not been established as the cause.
- In some networks, NAT markers disagree with direct interface addresses, or a
  task cannot resolve a public IP already displayed by the Center.

The new logs support diagnosis; this release does not claim to fix these root
causes. Temporarily set affected nodes to `debug`, reproduce the issue, inspect
request and task logs, and restore `info` afterwards. Inspect third-party failure
response bodies for sensitive content before sharing logs.
