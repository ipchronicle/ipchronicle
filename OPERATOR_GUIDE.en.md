# IPChronicle Operator Guide

[简体中文](OPERATOR_GUIDE.md) | English

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
init systems. Complete probes are validated with 64 MiB of memory. An Agent
below 64 MiB continues address observation but pauses complete probes until
the administrator enables the low-memory override.

The Center needs outbound HTTPS access to GitHub for release discovery. Each
Agent needs outbound HTTP or HTTPS access to its Center, configured public-IP
discovery services, official GitHub release assets for installation and
updates, and the third-party database, media, AI, DNSBL, and mail services used
by complete probes. A configured network proxy applies only to the explicit
discovery path that references it; it is not a global Center or Agent proxy.

## Install The Center

Create an installation directory and download the latest stable Compose and
environment examples:

```sh
mkdir ipchronicle
cd ipchronicle
curl --proto '=https' --tlsv1.2 -fLO \
  "https://github.com/ipchronicle/ipchronicle/releases/latest/download/compose.yaml"
curl --proto '=https' --tlsv1.2 -fL \
  "https://github.com/ipchronicle/ipchronicle/releases/latest/download/default.env.example" \
  -o .env
docker compose up -d
```

Open `http://server-address:8080` in a browser. The default login is `admin` /
`admin`, and the interface prompts for a password change. Use **Account** to
change the username or password, switch language, and enable TOTP. Credentials
in `.env` apply only when the administrator is first created.

Configuration and history are stored under the installation directory:

- `./data/config` stores `config.db` and `master.key`;
- `./data/history` stores `history.db`.

Preserve `master.key` together with its `config.db`; encrypted credentials
cannot be recovered without it.

### Use Cloudflare Tunnel

Create a Tunnel in Cloudflare Zero Trust and set its Public Hostname service to
`http://center:8080`. Download the Tunnel Compose file and add the token to
`.env`:

```sh
curl --proto '=https' --tlsv1.2 -fL \
  "https://github.com/ipchronicle/ipchronicle/releases/latest/download/compose.cloudflare-tunnel.yaml" \
  -o compose.yaml
printf '\nCLOUDFLARE_TUNNEL_TOKEN=your_Tunnel_token\n' >> .env
docker compose up -d
```

The Center and `cftunnel` communicate over the explicit
`ipchronicle_network`; the Center does not publish a host port in this setup.

## Environment Variables

The release `.env` supports these Compose variables:

| Variable                     | Default | Purpose                                              |
| ---------------------------- | ------- | ---------------------------------------------------- |
| `IPCHRONICLE_HTTP_PORT`      | `8080`  | Host port published by the standard Compose          |
| `IPCHRONICLE_ADMIN_USERNAME` | `admin` | Initial administrator username                       |
| `IPCHRONICLE_ADMIN_PASSWORD` | `admin` | Initial administrator password                       |
| `CLOUDFLARE_TUNNEL_TOKEN`    | none    | Tunnel token required by the Cloudflare Compose file |

When running the Center image directly or writing a custom Compose file, these
image variables are also available:

| Variable                           | Default                                   | Purpose                               |
| ---------------------------------- | ----------------------------------------- | ------------------------------------- |
| `IPCHRONICLE_LISTEN_ADDRESS`       | `:8080`                                   | Center HTTP listen address            |
| `IPCHRONICLE_DATA_DIR`             | `/var/lib/ipchronicle`                    | Default persistent-data root          |
| `IPCHRONICLE_CONFIG_DATABASE_PATH` | `/var/lib/ipchronicle/config/config.db`   | Configuration database path           |
| `IPCHRONICLE_HISTORY_DATABASE_PATH` | `/var/lib/ipchronicle/history/history.db` | History database path                 |
| `IPCHRONICLE_MASTER_KEY_PATH`      | `/var/lib/ipchronicle/config/master.key`  | Credential-encryption master key path |
| `IPCHRONICLE_ADMIN_USERNAME`       | `admin`                                   | Initial administrator username        |
| `IPCHRONICLE_ADMIN_PASSWORD`       | `admin`                                   | Initial administrator password        |
| `IPCHRONICLE_HEALTHCHECK_URL`      | `http://127.0.0.1:8080/healthz`           | URL used by `healthcheck`              |

The three persistent-file paths must be absolute. An explicit database or
master-key path overrides the corresponding path derived from
`IPCHRONICLE_DATA_DIR`.

## Reverse Proxy And TLS

HTTPS at an operator-managed reverse proxy is recommended but not enforced.
The external address is managed on the system settings page; automatic mode
follows the current browser request, while a custom value is used in Agent
installation commands and notification links.

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

IPChronicle does not request certificates or modify the proxy. Plain HTTP
remains usable and is shown with a warning. Restart the Center after changing
`.env`:

```sh
docker compose up -d
```

## Enroll Agents

Open **Nodes**, rotate the automatic-registration key if none exists, and run
the displayed command as root on each supported node. The one-line command
downloads the fixed official installer, which selects the latest Agent from
the deployment's stable or RC release channel. It verifies the official
manifest and checksums, installs the release download prerequisites, enrolls
the node, and starts the systemd or OpenRC service. The Center does not pin the
Agent to its own version. An operator may pass `--version VERSION` directly to
the installer only when deliberately installing a specific release.

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

Open a node's **Public IPs** page to manage the canonical public addresses that
the node can reach. Select **Manage proxies** in the upper right to add, edit,
enable, disable, or delete HTTP, HTTPS, and SOCKS5 proxies that belong only to
that node. After a new proxy is saved or an existing proxy changes, the Agent
automatically checks its IPv4 and IPv6 public egresses.

Proxy passwords are encrypted in `config.db`, sent only to the owning node's
Agent, and never displayed again. Leaving a password blank during an edit
preserves it unless **Clear password** is selected. Disabling a proxy stops
discovery and probing through it while retaining its configuration.

The Public IPs page follows these rules:

- usable default routes and stable routable sources are discovered
  automatically as hidden paths;
- one public IP found through several interfaces, sources, NAT mappings,
  proxies, or nodes appears once across the Center;
- a newly discovered public IP is enabled for complete probing by default and
  can be disabled by the administrator; and
- node proxies automatically check single-stack or dual-stack public egresses,
  because those egresses cannot be inferred from direct network inventory.

Interfaces, routes, local source addresses, selectors, and automatic path IDs
are internal execution details and are not displayed as user objects. Temporary
IPv6 privacy sources do not create their own durable path. When discovery
indicates NAT, the Center marks the public IP accordingly. DNS-based checks use
the node's resolver and may follow its default route.

Each public IP has a complete-probe switch that defaults on. The node has one
setting, also enabled by default, that runs a complete probe for a public IP
when an established discovery path adds it to the node's current confirmed set.
The first observation on a new path only establishes a baseline. The default
discovery interval is ten minutes. Set-entry, set-exit, failure, recovery, and
node-level queue-gap events are retained; unchanged checks are not historical
records.

## Complete Probes

Open a node's **Complete probes** page to run a probe or edit its schedule. A
schedule uses a six-field Cron expression including seconds and an explicit
IANA timezone such as `Asia/Shanghai`. The registration key captures the
administrator browser's timezone as the default for nodes registered with that
key. Missed occurrences are not caught up after downtime, and an occurrence is
skipped while another node probe is active. IPChronicle does not impose an
additional frequency limit.

An immediate task exists only for an online node, expires after two minutes,
and occupies the node's single task slot. There is no task backlog. The page
shows whether the Agent received the task and the progress or terminal state of
each public-IP execution.

The public-IP enablement switch controls recurring and automatic new-address
probes. A node-level immediate command selects targets only for that run and
does not change those switches. The immediate action on a public-IP row starts a
single-IP task directly, including when recurring probing is disabled for that
IP.

For every attempted public IP the Agent runs its built-in Go probe and validates
bounded JSON output. HTTP, HTTPS, and SMTP checks use the selected source,
interface, or configured proxy path. DNS resolution and DNSBL lookups use the
node's resolver. A failed third-party provider leaves only that provider's
fields unavailable; it does not fabricate data or fail unrelated checks. A
missing known field is shown as missing; if a known field changes to an
incompatible data type, that field is shown as unavailable while the raw report
and format status remain inspectable.

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

`v0.1.1` entered production use on 2026-09-04 and is the initial persisted-data
compatibility baseline. Supported upgrades within the same major version must
preserve its configuration database, master key, history database, Compose data
directories, and Agent state. `v0.1.1` does not migrate data from development
builds, release candidates, or `v0.1.0`; unreadable data fails explicitly.

## Notifications

Open **Notifications** to configure Telegram, generic Webhook, or isolated
JavaScript senders, then create rules for address, probe, gap, or format events.
Use **Test delivery** before enabling rules. Test delivery follows the same
durable queue, timeout, retry, and terminal-state path as real events.

A Telegram destination accepts a private chat or group ID and an optional
topic ID. Choose image or text delivery per sender; every supported event uses
the selected format. The create form can send one synchronous test before the
sender is saved. Known probe fields and values are localized for human
notifications, while Webhook payloads and the JavaScript event object retain
their machine values.

No notification is sent until the administrator configures a destination and
an enabled matching rule. Sender credentials are encrypted in `config.db`.
See [NOTIFICATIONS.en.md](NOTIFICATIONS.en.md) for payloads, JavaScript APIs, queue
limits, retries, and redaction behavior.

## Agent And Center Updates

The server operator updates the Center with Docker Compose. Review the release
notes, update the Compose file, and pull the new image:

```sh
curl --proto '=https' --tlsv1.2 -fLO \
  "https://github.com/ipchronicle/ipchronicle/releases/latest/download/compose.yaml"
docker compose pull
docker compose up -d
docker compose ps
```

Database migrations run before the Center begins serving. Releases that change
a persisted format list their upgrade and rollback requirements and verify
upgrades from `v0.1.1` and every other supported stable version.

Open **Settings > System** to select stable or RC release discovery. The
selection controls Center and Agent discovery together; it does not update the
Center automatically. On **Nodes**, select online nodes with an available
version and request an Agent update. The Agent validates platform, version,
capabilities, manifest, size, and checksum before atomic replacement. An
independent root supervisor restores the previous binary and state checkpoint
if the new Agent fails to start or report healthy. Update and probe work share
the single immediate-task slot. An in-place Agent update is rejected before
replacement when the target uses a different local-state schema. For
unreleased builds, purge and reinstall that Agent so it enrolls as a new node.

## Uninstall An Agent

Use the fixed official installer as root:

```sh
curl --proto '=https' --tlsv1.2 -fsSL \
  "https://raw.githubusercontent.com/ipchronicle/ipchronicle/main/scripts/install-agent.sh" |
  sh -s -- --uninstall
```

This removes the service definitions and installed binaries but deliberately
preserves `/var/lib/ipchronicle-agent`, including the node identity and pending
data. A later installation reuses that identity.

To remove the Agent and all of its local state in one operation, run:

```sh
curl --proto '=https' --tlsv1.2 -fsSL \
  "https://raw.githubusercontent.com/ipchronicle/ipchronicle/main/scripts/install-agent.sh" |
  sh -s -- --uninstall --purge
```

Purge irreversibly discards the node credential, retained configuration,
encrypted referenced proxy credentials, update recovery state, task identity,
and results waiting for upload. A later installation creates a new node.
Remove or revoke the old node from the Center separately; neither uninstall
mode deletes center configuration or history.

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

The first release has no built-in backup or restore command. Use filesystem
tooling controlled by the server operator. Stop the Center or use a
SQLite-aware snapshot mechanism before copying database files; copying live
database files without their WAL state is not a valid backup.

Back up `./data/config` and `./data/history` as a matched set. The configuration
directory is required to recover the account, Agent identities, proxy and
notification secrets, schedules, and history generation. The history
directory may be reset independently through the interface, but deleting it is
not a substitute for a coordinated **Clear observed history** operation while
Agents have queued data.

Also preserve `/var/lib/ipchronicle-agent` when host-level recovery must retain
an Agent's identity and offline queues. Losing that directory requires a new
enrollment and creates a new node identity.

For a failed Center start, inspect logs before changing data:

```sh
docker compose ps
docker compose logs --tail=200 center
```

IPChronicle does not silently recreate a missing master key, replace a corrupt
database, or claim success after a migration failure. Restore a usable backup
or make an explicit history-only reset decision. Deleting `./data/config` also
loses the account, node identities, and encrypted credentials.
