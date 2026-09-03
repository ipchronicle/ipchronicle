#!/bin/sh
set -eu

program_name="install-agent.sh"
mode="install"
center_url=""
registration_key=""
agent_version=""
agent_channel="stable"
agent_channel_set=0
agent_binary=""
purge_state=0
state_directory="/var/lib/ipchronicle-agent"
install_path="/usr/local/bin/ipchronicle-agent"
updater_path="/usr/local/libexec/ipchronicle-agent-updater"
root_prefix=${IPCHRONICLE_INSTALL_ROOT:-}
os_release_path=${IPCHRONICLE_OS_RELEASE:-/etc/os-release}
skip_packages=${IPCHRONICLE_SKIP_PACKAGES:-0}

fail() {
  printf '%s: %s\n' "$program_name" "$*" >&2
  exit 1
}

usage() {
  printf 'usage: %s --center-url URL --registration-key KEY [--channel stable|rc] [--version VERSION]\n' "$program_name" >&2
  printf '       %s --uninstall [--purge]\n' "$program_name" >&2
  exit 2
}

while [ "$#" -gt 0 ]; do
  case "$1" in
    --center-url)
      [ "$#" -ge 2 ] || usage
      center_url=$2
      shift 2
      ;;
    --registration-key)
      [ "$#" -ge 2 ] || usage
      registration_key=$2
      shift 2
      ;;
    --version)
      [ "$#" -ge 2 ] || usage
      agent_version=$2
      shift 2
      ;;
    --channel)
      [ "$#" -ge 2 ] || usage
      agent_channel=$2
      agent_channel_set=1
      shift 2
      ;;
    --agent-binary)
      [ "$#" -ge 2 ] || usage
      agent_binary=$2
      shift 2
      ;;
    --uninstall)
      mode="uninstall"
      shift
      ;;
    --purge)
      purge_state=1
      shift
      ;;
    *) usage ;;
  esac
done

[ "$(id -u)" -eq 0 ] || fail "must run as root"
case "$root_prefix" in
  "") ;;
  /*) root_prefix=${root_prefix%/} ;;
  *) fail "IPCHRONICLE_INSTALL_ROOT must be an absolute path" ;;
esac
[ -r "$os_release_path" ] || fail "$os_release_path is required"

# shellcheck disable=SC1090
. "$os_release_path"
distribution_id=${ID:-}
distribution_version=${VERSION_ID:-}
[ -n "$distribution_version" ] || fail "VERSION_ID is missing from $os_release_path"

case "$distribution_id" in
  debian)
    case "$distribution_version" in 12|13) ;; *) fail "unsupported Debian release: $distribution_version" ;; esac
    package_family="apt"
    init_system="systemd"
    ;;
  ubuntu)
    case "$distribution_version" in 24.04|26.04) ;; *) fail "unsupported Ubuntu release: $distribution_version" ;; esac
    package_family="apt"
    init_system="systemd"
    ;;
  rhel|rocky|almalinux)
    distribution_major=${distribution_version%%.*}
    case "$distribution_major" in 8|9|10) ;; *) fail "unsupported $distribution_id release: $distribution_version" ;; esac
    package_family="dnf"
    init_system="systemd"
    ;;
  centos)
    case ${NAME:-} in *Stream*) ;; *) fail "only CentOS Stream is supported" ;; esac
    distribution_major=${distribution_version%%.*}
    case "$distribution_major" in 9|10) ;; *) fail "unsupported CentOS Stream release: $distribution_version" ;; esac
    package_family="dnf"
    init_system="systemd"
    ;;
  alpine)
    alpine_branch=${distribution_version%.*}
    case "$alpine_branch" in 3.23|3.24) ;; *) fail "unsupported Alpine release: $distribution_version" ;; esac
    package_family="apk"
    init_system="openrc"
    ;;
  *) fail "unsupported Linux distribution: ${distribution_id:-unknown}" ;;
esac

if [ "$init_system" = "systemd" ]; then
  command -v systemctl >/dev/null 2>&1 || fail "systemd is required on $distribution_id"
else
  command -v rc-service >/dev/null 2>&1 || fail "OpenRC is required on Alpine Linux"
  command -v rc-update >/dev/null 2>&1 || fail "rc-update is required on Alpine Linux"
fi

uninstall_agent() {
  if [ "$init_system" = "systemd" ]; then
    if [ -f "$root_prefix/etc/systemd/system/ipchronicle-agent-updater.service" ] && systemctl is-active --quiet ipchronicle-agent-updater.service; then
      systemctl stop ipchronicle-agent-updater.service
    fi
    if [ -f "$root_prefix/etc/systemd/system/ipchronicle-agent.service" ]; then
      systemctl disable --now ipchronicle-agent.service
    fi
    rm -f \
      "$root_prefix/etc/systemd/system/ipchronicle-agent.service" \
      "$root_prefix/etc/systemd/system/ipchronicle-agent-updater.service"
    systemctl daemon-reload
  else
    if [ -f "$root_prefix/etc/init.d/ipchronicle-agent-updater" ] && rc-service ipchronicle-agent-updater status >/dev/null 2>&1; then
      rc-service ipchronicle-agent-updater stop
    fi
    if [ -f "$root_prefix/etc/init.d/ipchronicle-agent" ]; then
      if rc-service ipchronicle-agent status >/dev/null 2>&1; then
        rc-service ipchronicle-agent stop
      fi
      rc-update del ipchronicle-agent default
    fi
    rm -f \
      "$root_prefix/etc/init.d/ipchronicle-agent" \
      "$root_prefix/etc/init.d/ipchronicle-agent-updater"
  fi
  rm -f "$root_prefix$install_path" "$root_prefix$updater_path"
  if [ "$purge_state" = "1" ]; then
    rm -rf "$root_prefix$state_directory"
    printf 'IPChronicle Agent and local state removed.\n'
  else
    printf 'IPChronicle Agent uninstalled. State was preserved at %s.\n' "$state_directory"
  fi
}

if [ "$mode" = "uninstall" ]; then
  if [ -n "$center_url" ] || [ -n "$registration_key" ] || [ -n "$agent_version" ] || [ -n "$agent_binary" ] || [ "$agent_channel_set" = "1" ]; then
    usage
  fi
  uninstall_agent
  exit 0
fi

[ "$purge_state" = "0" ] || usage
[ -n "$center_url" ] || usage
[ -n "$registration_key" ] || usage
case "$agent_channel" in stable|rc) ;; *) fail "--channel must be stable or rc" ;; esac
if [ -n "$agent_version" ] && [ "$agent_channel_set" = "1" ]; then
  fail "--channel and --version cannot be used together"
fi
[ "$(uname -s)" = "Linux" ] || fail "only Linux is supported"

machine=$(uname -m)
case "$machine" in
  x86_64|amd64) architecture="amd64" ;;
  aarch64|arm64) architecture="arm64" ;;
  *) fail "unsupported CPU architecture: $machine" ;;
esac

if [ -n "$agent_binary" ]; then
  [ -f "$agent_binary" ] || fail "Agent binary does not exist: $agent_binary"
else
  command -v curl >/dev/null 2>&1 || fail "curl is required to download the Agent"
  command -v sha256sum >/dev/null 2>&1 || fail "sha256sum is required to validate the Agent"
fi

if [ "$skip_packages" != "1" ]; then
  case "$package_family" in
    apt)
      export DEBIAN_FRONTEND=noninteractive
      apt-get update
      apt-get install --yes --no-install-recommends \
        curl jq ca-certificates
      ;;
    dnf)
      if command -v curl >/dev/null 2>&1; then
        dnf install --assumeyes \
          jq ca-certificates
      else
        dnf install --assumeyes \
          curl jq ca-certificates
      fi
      ;;
    apk)
      apk add --no-cache \
        curl jq ca-certificates
      ;;
  esac
fi
command -v jq >/dev/null 2>&1 || fail "jq is required to validate Agent metadata"

temporary_directory=$(mktemp -d)
cleanup() {
  rm -rf "$temporary_directory"
}
trap cleanup EXIT HUP INT TERM

downloaded_binary="$temporary_directory/ipchronicle-agent"
artifact="ipchronicle-agent-linux-$architecture"
manifest_revision=""
manifest_version=""
if [ -n "$agent_binary" ]; then
  cp "$agent_binary" "$downloaded_binary"
else
  if [ -n "$agent_version" ]; then
    case "$agent_version" in v*|*' '*|"") fail "--version must omit the v prefix and whitespace" ;; esac
    release_base="https://github.com/ipchronicle/ipchronicle/releases/download/v$agent_version"
  elif [ "$agent_channel" = "rc" ]; then
    curl --proto '=https' --tlsv1.2 --fail --location --silent --show-error \
      --header 'Accept: application/vnd.github+json' \
      --header 'X-GitHub-Api-Version: 2022-11-28' \
      --output "$temporary_directory/releases.json" \
      'https://api.github.com/repos/ipchronicle/ipchronicle/releases?per_page=100'
    release_tag=$(jq -er '
      [
        .[] |
        select(.draft == false) |
        . as $release |
        (.tag_name | capture("^v(?<major>[0-9]+)\\.(?<minor>[0-9]+)\\.(?<patch>[0-9]+)(?:-rc\\.(?<rc>[0-9]+))?$")) as $version |
        select(($version.rc != null) == $release.prerelease) |
        {
          tag: .tag_name,
          order: [
            ($version.major | tonumber),
            ($version.minor | tonumber),
            ($version.patch | tonumber),
            (if $version.rc == null then 1 else 0 end),
            (($version.rc // "0") | tonumber)
          ]
        }
      ] |
      sort_by(.order) |
      last.tag
    ' "$temporary_directory/releases.json") || fail "no official stable or RC Agent release is available"
    release_base="https://github.com/ipchronicle/ipchronicle/releases/download/$release_tag"
  else
    release_base="https://github.com/ipchronicle/ipchronicle/releases/latest/download"
  fi
  curl --proto '=https' --tlsv1.2 --fail --location --silent --show-error \
    --output "$temporary_directory/release-manifest.json" "$release_base/release-manifest.json"
  manifest_version=$(jq -er --arg artifact "$artifact" --arg arch "$architecture" '
    select(.schemaVersion == 1) |
    select(.version | type == "string" and test("^[0-9]+\\.[0-9]+\\.[0-9]+(-rc\\.[0-9]+)?$")) |
    select(.tag == ("v" + .version)) |
    select(.revision | type == "string" and test("^[0-9a-f]{40}$")) |
    select(.sourceUrl == ("https://github.com/ipchronicle/ipchronicle/tree/" + .tag)) |
    select([.artifacts[] | select(.name == $artifact and .component == "agent" and .os == "linux" and .arch == $arch)] | length == 1) |
    .version
  ' "$temporary_directory/release-manifest.json") || fail "release manifest is invalid"
  manifest_revision=$(jq -er '.revision' "$temporary_directory/release-manifest.json")
  if [ -n "$agent_version" ]; then
    [ "$manifest_version" = "$agent_version" ] || fail "release manifest version does not match --version"
  elif [ "$agent_channel" = "stable" ]; then
    [ "$(jq -er '.channel' "$temporary_directory/release-manifest.json")" = "stable" ] || fail "latest release is not on the stable channel"
  else
    [ "v$manifest_version" = "$release_tag" ] || fail "release manifest version does not match the discovered release"
  fi
  expected_size=$(jq -er --arg artifact "$artifact" '.artifacts[] | select(.name == $artifact) | .size' "$temporary_directory/release-manifest.json")
  expected_checksum=$(jq -er --arg artifact "$artifact" '.artifacts[] | select(.name == $artifact) | .sha256' "$temporary_directory/release-manifest.json")
  case "$expected_size" in ''|*[!0-9]*) fail "release manifest Agent size is invalid" ;; esac
  printf '%s\n' "$expected_checksum" | grep -Eq '^[0-9a-f]{64}$' || fail "release manifest Agent checksum is invalid"
  curl --proto '=https' --tlsv1.2 --fail --location --silent --show-error \
    --output "$downloaded_binary" "$release_base/$artifact"
  curl --proto '=https' --tlsv1.2 --fail --location --silent --show-error \
    --output "$temporary_directory/checksums.txt" "$release_base/checksums.txt"
  checksum_file_value=$(awk -v artifact="$artifact" '$2 == artifact || $2 == "*" artifact { print $1; exit }' "$temporary_directory/checksums.txt")
  [ "$checksum_file_value" = "$expected_checksum" ] || fail "checksums.txt does not match the release manifest"
  actual_size=$(wc -c < "$downloaded_binary" | tr -d ' ')
  [ "$actual_size" = "$expected_size" ] || fail "Agent artifact length verification failed"
  actual_checksum=$(sha256sum "$downloaded_binary" | awk '{ print $1 }')
  [ "$actual_checksum" = "$expected_checksum" ] || fail "Agent checksum verification failed"
fi

chmod 0755 "$downloaded_binary"
binary_metadata=$("$downloaded_binary" version --json) || fail "Agent binary metadata could not be read"
printf '%s' "$binary_metadata" | jq -e --arg arch "$architecture" --arg version "$manifest_version" --arg revision "$manifest_revision" '
  .component == "agent" and .os == "linux" and .arch == $arch and
  (.version | type == "string" and length > 0) and
  ($version == "" or .version == $version) and
  ($revision == "" or .revision == $revision) and
  (.stateSchemaVersion | type == "number" and . >= 1)
' >/dev/null || fail "Agent binary metadata does not match this host or release"

if [ "$init_system" = "systemd" ]; then
  if [ -f "$root_prefix/etc/systemd/system/ipchronicle-agent-updater.service" ] && systemctl is-active --quiet ipchronicle-agent-updater.service; then
    fail "an Agent update is already in progress"
  fi
  if [ -f "$root_prefix/etc/systemd/system/ipchronicle-agent.service" ]; then
    systemctl stop ipchronicle-agent.service
  fi
elif [ -f "$root_prefix/etc/init.d/ipchronicle-agent-updater" ] && rc-service ipchronicle-agent-updater status >/dev/null 2>&1; then
  fail "an Agent update is already in progress"
elif [ -f "$root_prefix/etc/init.d/ipchronicle-agent" ] && rc-service ipchronicle-agent status >/dev/null 2>&1; then
  rc-service ipchronicle-agent stop
fi

install -d -m 0700 -o root -g root "$root_prefix$state_directory"
install -d -m 0755 -o root -g root "$(dirname "$root_prefix$install_path")" "$(dirname "$root_prefix$updater_path")"
install -m 0755 -o root -g root "$downloaded_binary" "$root_prefix$install_path"
install -m 0755 -o root -g root "$downloaded_binary" "$root_prefix$updater_path"
"$root_prefix$install_path" enroll \
  --center-url "$center_url" \
  --registration-key "$registration_key" \
  --state-dir "$state_directory" \
  --update-init "$init_system"

if [ "$init_system" = "systemd" ]; then
  install -d -m 0755 "$root_prefix/etc/systemd/system"
  cat > "$root_prefix/etc/systemd/system/ipchronicle-agent.service" <<'EOF'
[Unit]
Description=IPChronicle Agent
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=root
Group=root
ExecStart=/usr/local/bin/ipchronicle-agent run --state-dir /var/lib/ipchronicle-agent --update-init systemd --agent-path /usr/local/bin/ipchronicle-agent --updater-path /usr/local/libexec/ipchronicle-agent-updater
Restart=on-failure
RestartSec=10s
UMask=0077
NoNewPrivileges=no
PrivateTmp=yes

[Install]
WantedBy=multi-user.target
EOF
  cat > "$root_prefix/etc/systemd/system/ipchronicle-agent-updater.service" <<'EOF'
[Unit]
Description=IPChronicle Agent Update Supervisor
After=network-online.target

[Service]
Type=oneshot
User=root
Group=root
ExecStart=/usr/local/libexec/ipchronicle-agent-updater update-supervisor --state-dir /var/lib/ipchronicle-agent --agent-path /usr/local/bin/ipchronicle-agent --updater-path /usr/local/libexec/ipchronicle-agent-updater --update-init systemd
TimeoutStartSec=5min
UMask=0077
NoNewPrivileges=no
PrivateTmp=yes
EOF
  systemctl daemon-reload
  systemctl enable --now ipchronicle-agent.service
  systemctl is-active --quiet ipchronicle-agent.service || fail "Agent service did not become active"
else
  install -d -m 0755 "$root_prefix/etc/init.d"
  cat > "$root_prefix/etc/init.d/ipchronicle-agent" <<'EOF'
#!/sbin/openrc-run

name="IPChronicle Agent"
description="IPChronicle managed-node Agent"
command="/usr/local/bin/ipchronicle-agent"
command_args="run --state-dir /var/lib/ipchronicle-agent --update-init openrc --agent-path /usr/local/bin/ipchronicle-agent --updater-path /usr/local/libexec/ipchronicle-agent-updater"
command_background="yes"
pidfile="/run/ipchronicle-agent.pid"
output_log="/var/log/ipchronicle-agent.log"
error_log="/var/log/ipchronicle-agent.log"

depend() {
  need net
  after firewall
}

start_pre() {
  checkpath --directory --mode 0700 --owner root:root /var/lib/ipchronicle-agent
}
EOF
  cat > "$root_prefix/etc/init.d/ipchronicle-agent-updater" <<'EOF'
#!/sbin/openrc-run

name="IPChronicle Agent Update Supervisor"
description="IPChronicle independent Agent update and rollback supervisor"
command="/usr/local/libexec/ipchronicle-agent-updater"
command_args="update-supervisor --state-dir /var/lib/ipchronicle-agent --agent-path /usr/local/bin/ipchronicle-agent --updater-path /usr/local/libexec/ipchronicle-agent-updater --update-init openrc"
command_background="yes"
pidfile="/run/ipchronicle-agent-updater.pid"
output_log="/var/log/ipchronicle-agent.log"
error_log="/var/log/ipchronicle-agent.log"

depend() {
  need net
}
EOF
  chmod 0755 "$root_prefix/etc/init.d/ipchronicle-agent" "$root_prefix/etc/init.d/ipchronicle-agent-updater"
  rc-update add ipchronicle-agent default
  rc-service ipchronicle-agent restart
  rc-service ipchronicle-agent status >/dev/null 2>&1 || fail "Agent service did not become active"
fi

printf 'IPChronicle Agent installed and started.\n'
