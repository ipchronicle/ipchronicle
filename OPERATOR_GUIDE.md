# IPChronicle Operator Guide

This guide covers the supported operation of one IPChronicle installation for
one self-hosting administrator. Commands use release assets from the official
`ipchronicle/ipchronicle` GitHub repository.

## Supported Environment

The Center is supported on Linux with Docker Engine and Docker Compose. It does
not terminate TLS or configure a reverse proxy. The release gate validates the
Center with 512 MiB of memory.

The root Agent supports Linux AMD64 and ARM64 on:

- Debian 12 and 13;
- Ubuntu 24.04 and 26.04;
- RHEL, Rocky Linux, and AlmaLinux 8, 9, and 10;
- CentOS Stream 9 and 10; and
- Alpine Linux 3.23 and 3.24 with OpenRC.

The installer rejects other operating systems, releases, architectures, and
init systems. Complete probes are validated with 256 MiB of memory. An Agent
below 256 MiB continues address observation but pauses complete probes until
the administrator enables the low-memory override.

The Center needs outbound HTTPS access to GitHub for release discovery. Each
Agent needs outbound HTTP or HTTPS access to its Center, configured public-IP
discovery services, official GitHub release assets for installation and
updates, the official IPQuality download, and services contacted by that
script. A configured network proxy applies only to the explicit discovery path
that references it;
it is not a global Center or Agent proxy.

## Install The Center

Choose the release version, create an empty installation directory, and
download the deployment assets. The version must omit the leading `v`.

```sh
IPCHRONICLE_VERSION=0.1.0-rc.3
mkdir ipchronicle
cd ipchronicle
curl --proto '=https' --tlsv1.2 -fLO \
  "https://github.com/ipchronicle/ipchronicle/releases/download/v${IPCHRONICLE_VERSION}/compose.yaml"
curl --proto '=https' --tlsv1.2 -fL \
  "https://github.com/ipchronicle/ipchronicle/releases/download/v${IPCHRONICLE_VERSION}/default.env.example" \
  -o .env
curl --proto '=https' --tlsv1.2 -fLO \
  "https://github.com/ipchronicle/ipchronicle/releases/download/v${IPCHRONICLE_VERSION}/checksums.txt"
curl --proto '=https' --tlsv1.2 -fLO \
  "https://github.com/ipchronicle/ipchronicle/releases/download/v${IPCHRONICLE_VERSION}/release-manifest.json"
grep -E '  (default\.env\.example|compose\.yaml|release-manifest\.json)$' checksums.txt |
  sed 's/  default\.env\.example$/  .env/' | sha256sum --check
chmod 0600 .env
```

Review `.env` before starting. Bootstrap credentials are used only when
`config.db` has no administrator account. If they are left unchanged, the
initial login is `admin` / `admin`; the interface warns but does not force a
change.

```sh
docker compose --env-file .env -f compose.yaml pull
docker compose --env-file .env -f compose.yaml up -d
docker compose --env-file .env -f compose.yaml ps
docker compose --env-file .env -f compose.yaml exec -T center \
  /usr/local/bin/ipchronicle-center healthcheck
```

Open the published address in a browser. Use **Account** to change the
administrator username or password, select Simplified Chinese or English, and
optionally enable TOTP two-factor authentication. Changing bootstrap values in
`.env` does not change an existing account.

The release Compose file creates two independent volumes:

- `ipchronicle_center-config` contains `config.db` and `master.key`; and
- `ipchronicle_center-history` contains `history.db`.

Never treat the config volume as disposable history. The master key must stay
with its matching `config.db`; encrypted credentials cannot be recovered
without it.

## Environment Variables

The release `default.env.example` exposes these operator settings:

| Variable | Default | Purpose |
| --- | --- | --- |
| `IPCHRONICLE_HTTP_PORT` | `8080` | Host port published by Compose. |
| `IPCHRONICLE_ADMIN_USERNAME` | `admin` | First-start administrator username. |
| `IPCHRONICLE_ADMIN_PASSWORD` | `admin` | First-start administrator password. |
| `IPCHRONICLE_TRUSTED_PROXIES` | empty | Comma-separated proxy source CIDRs allowed to supply forwarded headers. |

The fixed database paths and listen address in `compose.yaml` match the two
volumes. Editing those internal values is outside the supported release
deployment.

## Reverse Proxy And TLS

HTTPS at an operator-managed reverse proxy is recommended but not enforced.
Set `IPCHRONICLE_TRUSTED_PROXIES` only to the source CIDRs from which the Center
actually receives proxy connections. This may be a Docker bridge CIDR rather
than `127.0.0.1`. The external address is managed on the system settings page;
automatic mode follows the current browser request, while a custom value is
used in Agent installation commands and notification links.

Forward the original host, client address, and scheme. WebSocket Upgrade must
work under `/api/v1/agent/sync/`; ordinary 30-second Agent HTTP polling remains
the source of truth if temporary WebSocket sync is unavailable. A minimal
Nginx location is:

```nginx
location / {
    proxy_pass http://127.0.0.1:8080;
    proxy_http_version 1.1;
    proxy_set_header Host $host;
    proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    proxy_set_header X-Forwarded-Proto $scheme;
    proxy_set_header Upgrade $http_upgrade;
    proxy_set_header Connection "upgrade";
}
```

IPChronicle does not request certificates, redirect HTTP to HTTPS, or modify
the proxy. Intentional plain HTTP remains usable and is shown with a warning.
Restart the Center after changing `.env`:

```sh
docker compose --env-file .env -f compose.yaml up -d
```

## Enroll Agents

Open **Nodes**, rotate the automatic-registration key if none exists, and run
the displayed command as root on each supported node. The one-line command
downloads the fixed official installer, which selects the latest Agent from
the deployment's stable or RC release channel. It verifies the official
manifest and checksums, installs IPQuality dependencies, enrolls the node, and
starts the systemd or OpenRC service. The Center does not pin the Agent to its
own version. An operator may pass `--version VERSION` directly to the installer
only when deliberately installing a specific release.

The shared registration key is used only for enrollment. Each Agent receives a
node-specific credential and stores it encrypted in the root-only directory
`/var/lib/ipchronicle-agent`, alongside its local queues. Re-running the same
installation command preserves a valid local identity. Disabling automatic
enrollment blocks new nodes but does not disconnect registered Agents.

Registration does not run a complete probe. The default local schedule is
enabled for midnight in the Agent's local timezone, and the administrator may
run the first probe manually.

Use these commands to inspect an Agent:

```sh
# systemd
systemctl status ipchronicle-agent
journalctl -u ipchronicle-agent -f

# OpenRC
rc-service ipchronicle-agent status
tail -f /var/log/ipchronicle-agent.log
```

An online node has reported within the last two minutes. Configuration changes
normally converge on the next 30-second poll. Starting a temporary sync session
from the node page keeps WebSocket wake-ups available for up to ten minutes;
it does not replace polling or create an inbound Agent port.

Disabling a node stops its polling work and local schedules after the Agent
receives the change. Revoking credentials permanently disconnects that Agent.
Deleting a node removes its Center-owned configuration, hidden discovery paths,
and node-level state but does not uninstall software from the host. Public-IP
reports and address events already assigned to a global public-IP identity are
retained.

## Public-IP Discovery And Address Observation

Open **Settings > Network probes** to configure two to eight distinct public-IP
discovery service hosts for each address family. HTTP and HTTPS URLs are
accepted; HTTP produces a warning. An address is confirmed only when the
configured services agree, so service failures remain visible instead of being
presented as an address change.

Reusable HTTP, HTTPS, and SOCKS5 proxies are configured on the same page. Proxy
passwords are encrypted in `config.db`, sent only to Agents with a referencing
proxy discovery path, and are never displayed again. Leaving a password blank during an edit
preserves it unless **Clear password** is selected.

Open a node's **Public IPs** page to manage the canonical public addresses that
the node can reach:

- usable default routes and stable routable sources are discovered
  automatically as hidden paths;
- one public IP found through several interfaces, sources, NAT mappings,
  proxies, or nodes appears once across the Center;
- a newly discovered public IP is visible but complete probing is disabled
  until the administrator enables it; and
- explicit proxy discovery paths bind one reusable proxy and address family to
  one node because they cannot be inferred from network inventory.

Interfaces, routes, local source addresses, selectors, and automatic path IDs
are internal execution details and are not displayed as user objects. Temporary
IPv6 privacy sources do not create their own durable path. When discovery
indicates NAT, the Center marks the public IP accordingly. Some upstream DNS or
raw-mail checks may still use the default route or fail to bind.

Each public IP has a complete-probe switch and a **probe after rediscovery**
switch. The latter defaults on but only applies after complete probing has been
enabled for that IP. The default discovery interval is ten minutes. First
observation does not trigger a complete probe. Address transitions, failure
boundaries, recoveries, and node-level queue gaps are retained; unchanged checks
are not historical records.

## Complete Probes

Open a node's **Complete probes** page to run a probe or edit its schedule. A
schedule uses a six-field Cron expression including seconds and either
`agent-local` or an IANA timezone such as `Asia/Shanghai`. Missed occurrences
are not caught up after downtime, and an occurrence is skipped while another
node probe is active. IPChronicle does not impose an additional frequency
limit.

An immediate task exists only for an online node, expires after two minutes,
and occupies the node's single task slot. There is no task backlog. The page
shows whether the Agent received the task and the progress or terminal state of
each public-IP execution.

For every attempted public IP the Agent downloads a fresh official IPQuality
script, supervises its process tree as root, and validates bounded JSON output.
IPChronicle does not pin or cache the upstream script. A missing known field is
shown as missing; if a known field changes to an incompatible data type, that
field is shown as unavailable while the raw report and format status remain
inspectable.

An Agent retains at most 30 pending address events and 30 pending complete
results per public IP while the Center is unavailable. Address events are
bounded independently per hidden discovery path. Oldest-item eviction is
reported as an explicit history gap. Upload retry retransmits stored data and
never reruns the probe.

## History, Comparison, And Retention

The **History** page filters complete reports and address transitions by node,
public IP, time, status, trigger, format state, and change state. A report can be
compared with its chronological baseline or with another snapshot from the
same public IP. Starred snapshots are protected from retention cleanup.

Open **Settings > History and storage** to choose one policy:

- indefinite retention;
- age retention from 1 to 36,500 days; or
- logical-size retention from 1 MiB to 1 TiB.

Cleanup runs when the policy changes, on demand, and every six hours. Current
state, starred snapshots, active notification deliveries, and required
comparison baselines are protected. Protected data may keep physical usage
above a configured logical-size budget; the page reports logical, protected,
database, WAL, and shared-memory usage separately.

**Clear observed history** removes address events, probe runs, executions,
snapshots, and gaps while preserving the administrator, nodes, public-IP
settings and hidden paths,
proxies, schedules, notification configuration, and pending task state. It also
advances the history generation so Agents discard queued data from the old
generation. No complete probe starts automatically after a reset.

Before the first stable release, development and RC versions do not promise
`history.db` compatibility. This exception does not apply to `config.db`,
silent corruption recovery, or any future published stable history schema.

## Notifications

Open **Notifications** to configure Telegram, generic Webhook, or isolated
JavaScript senders, then create rules for address, probe, gap, or format events.
Use **Test delivery** before enabling rules. Test delivery follows the same
durable queue, timeout, retry, and terminal-state path as real events.

No notification is sent until the administrator configures a destination and
an enabled matching rule. Sender credentials are encrypted in `config.db`.
See [NOTIFICATIONS.md](NOTIFICATIONS.md) for payloads, JavaScript APIs, queue
limits, retries, and redaction behavior.

## Agent And Center Updates

The Center is upgraded by the server operator with Docker Compose. Download the
new release's `compose.yaml` and `default.env.example`, compare new environment
settings with the existing `.env`, then replace only `compose.yaml`:

```sh
docker compose --env-file .env -f compose.yaml pull
docker compose --env-file .env -f compose.yaml up -d
docker compose --env-file .env -f compose.yaml ps
```

Database migrations run before the Center begins serving. Do not downgrade a
Center against databases migrated by a newer release. A Center rollback
requires restoring a compatible pre-upgrade volume backup.

Open **Settings > System** to select stable or RC release discovery. The
selection controls Center and Agent discovery together; it does not update the
Center automatically. On **Nodes**, select online nodes with an available
version and request an Agent update. The Agent validates platform, version,
capabilities, manifest, size, and checksum before atomic replacement. An
independent root supervisor restores the previous binary and state checkpoint
if the new Agent fails to start or report healthy. Update and probe work share
the single immediate-task slot.

## Uninstall An Agent

Use the fixed official installer as root:

```sh
curl --proto '=https' --tlsv1.2 -fsSL \
  "https://raw.githubusercontent.com/ipchronicle/ipchronicle/main/scripts/install-agent.sh" |
  sh -s -- --uninstall
```

This removes the service definitions and installed binaries but deliberately
preserves `/var/lib/ipchronicle-agent`. Remove the node from the Center
separately. Delete the preserved state directory only after deciding that its
identity and pending data are no longer needed.

## Local Administrator Recovery

These commands require server-level Compose access and revoke every existing
administrator session. They do not expose a remote recovery API.

```sh
read -r -s IPCHRONICLE_RECOVERY_PASSWORD
printf '%s\n' "$IPCHRONICLE_RECOVERY_PASSWORD" |
  docker compose --env-file .env -f compose.yaml exec -T center \
    /usr/local/bin/ipchronicle-center admin reset-password --password-stdin
unset IPCHRONICLE_RECOVERY_PASSWORD

docker compose --env-file .env -f compose.yaml exec -T center \
  /usr/local/bin/ipchronicle-center admin disable-totp
```

## Backup And Disaster-Recovery Boundaries

The first release has no built-in backup or restore command. Use volume or
filesystem tooling controlled by the server operator. Stop the Center or use a
SQLite-aware snapshot mechanism before copying database files; copying live
database files without their WAL state is not a valid backup.

Back up the config and history volumes as a matched set. The config volume is
required to recover the account, Agent identities, proxy and notification
secrets, schedules, and history generation. The history volume may be reset
independently through the interface, but deleting it is not a substitute for a
coordinated **Clear observed history** operation while Agents have queued data.

Also preserve `/var/lib/ipchronicle-agent` when host-level recovery must retain
an Agent's identity and offline queues. Losing that directory requires a new
enrollment and creates a new node identity.

For a failed Center start, inspect logs before changing data:

```sh
docker compose --env-file .env -f compose.yaml ps
docker compose --env-file .env -f compose.yaml logs --tail=200 center
```

IPChronicle does not silently recreate a missing master key, replace a corrupt
database, or claim success after a migration failure. Restore a compatible
backup or make an explicit history-only reset decision; never delete
`ipchronicle_center-config` as a history cleanup step.
