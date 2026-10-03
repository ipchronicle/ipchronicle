# IPChronicle v0.1.4

[简体中文](RELEASE_NOTES.md) | English

This release adds notification event exclusions and snapshot comparison
improvements, and cleans up the Web dependency chain. Upgrades preserve
configuration, history, and node identities from v0.1.1 and later stable
releases.

## Highlights

- Notification rules add an all-events option with per-event exclusions. Field
  change choices use human-readable meanings instead of internal field names.
- Snapshot comparison uses a real time axis, opens with the earliest and latest
  snapshots, supports continuous scrolling, and switches reports immediately
  when a snapshot is selected.
- Add the notification-exclusion configuration migration while preserving
  existing senders, rules, nodes, and history.
- Remove the unused shadcn CLI dependency, reduce the frontend dependency tree,
  and resolve the dependency audit findings.

## Upgrade From v0.1.1

Upgrade the Center before Agents. First stop the Center and consistently back up
`./data/config` and `./data/history`. Retain `/var/lib/ipchronicle-agent` on
each node.

The Center applies the notification-exclusion configuration migration
automatically, preserving accounts, the master key, nodes, proxies, schedules,
notification senders, notification rules, and history. Directly switching to
an older Center image after migration is unsupported. To roll back, stop the
Center, restore the matching pre-upgrade backup, and start the older version.
See the [operator guide](OPERATOR_GUIDE.en.md).

## Issues Awaiting Diagnosis

- Some ipapi results are missing when nodes share an API key. Quota, concurrency,
  and request failures have not been established as the cause.
- In some networks, NAT markers disagree with direct interface addresses, or a
  task cannot resolve a public IP already displayed by the Center.

The existing logs support diagnosis; this release does not claim to fix these
root causes. Temporarily set affected nodes to `debug`, reproduce the issue,
inspect request and task logs, and restore `info` afterwards. Inspect
third-party failure response bodies for sensitive content before sharing logs.
