#!/bin/sh
set -eu

program_name="install-agent.sh"
center_url=""
registration_key=""
agent_version=""
agent_binary=""
state_directory="/var/lib/ipchronicle-agent"
install_path="/usr/local/bin/ipchronicle-agent"

fail() {
  printf '%s: %s\n' "$program_name" "$*" >&2
  exit 1
}

usage() {
  printf 'usage: %s --center-url URL --registration-key KEY [--version VERSION]\n' "$program_name" >&2
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
    --agent-binary)
      [ "$#" -ge 2 ] || usage
      agent_binary=$2
      shift 2
      ;;
    *) usage ;;
  esac
done

[ "$(id -u)" -eq 0 ] || fail "must run as root"
[ -n "$center_url" ] || usage
[ -n "$registration_key" ] || usage
[ "$(uname -s)" = "Linux" ] || fail "only Linux is supported"
command -v curl >/dev/null 2>&1 || fail "curl is required to run the installer"
[ -r /etc/os-release ] || fail "/etc/os-release is required"

# shellcheck disable=SC1091
. /etc/os-release
distribution_id=${ID:-}
case "$distribution_id" in
  debian|ubuntu)
    package_family="apt"
    init_system="systemd"
    ;;
  rhel|rocky|almalinux|centos)
    package_family="dnf"
    init_system="systemd"
    ;;
  alpine)
    package_family="apk"
    init_system="openrc"
    ;;
  *) fail "unsupported Linux distribution: ${distribution_id:-unknown}" ;;
esac

machine=$(uname -m)
case "$machine" in
  x86_64|amd64) architecture="amd64" ;;
  aarch64|arm64) architecture="arm64" ;;
  *) fail "unsupported CPU architecture: $machine" ;;
esac

if [ "$init_system" = "systemd" ]; then
  command -v systemctl >/dev/null 2>&1 || fail "systemd is required on $distribution_id"
else
  command -v rc-service >/dev/null 2>&1 || fail "OpenRC is required on Alpine Linux"
fi

if [ -n "$agent_binary" ]; then
  [ -f "$agent_binary" ] || fail "Agent binary does not exist: $agent_binary"
fi

temporary_directory=$(mktemp -d)
cleanup() {
  rm -rf "$temporary_directory"
}
trap cleanup EXIT HUP INT TERM

case "$package_family" in
  apt)
    export DEBIAN_FRONTEND=noninteractive
    apt-get update
    apt-get install --yes --no-install-recommends \
      bash curl jq bc netcat-openbsd dnsutils iproute2 ca-certificates
    ;;
  dnf)
    dnf install --assumeyes \
      bash curl jq bc nmap-ncat bind-utils iproute ca-certificates
    ;;
  apk)
    apk add --no-cache \
      bash curl jq bc netcat-openbsd bind-tools iproute2 ca-certificates
    ;;
esac

downloaded_binary="$temporary_directory/ipchronicle-agent"
if [ -n "$agent_binary" ]; then
  cp "$agent_binary" "$downloaded_binary"
else
  artifact="ipchronicle-agent-linux-$architecture"
  if [ -n "$agent_version" ]; then
    release_base="https://github.com/ipchronicle/ipchronicle/releases/download/v$agent_version"
  else
    release_base="https://github.com/ipchronicle/ipchronicle/releases/latest/download"
  fi
  curl --proto '=https' --tlsv1.2 --fail --location --silent --show-error \
    --output "$downloaded_binary" "$release_base/$artifact"
  curl --proto '=https' --tlsv1.2 --fail --location --silent --show-error \
    --output "$temporary_directory/checksums.txt" "$release_base/checksums.txt"
  expected_checksum=$(awk -v artifact="$artifact" '$2 == artifact || $2 == "*" artifact { print $1; exit }' "$temporary_directory/checksums.txt")
  [ -n "$expected_checksum" ] || fail "release checksum is missing for $artifact"
  actual_checksum=$(sha256sum "$downloaded_binary" | awk '{ print $1 }')
  [ "$actual_checksum" = "$expected_checksum" ] || fail "Agent checksum verification failed"
fi

chmod 0755 "$downloaded_binary"
"$downloaded_binary" version >/dev/null
install -d -m 0700 -o root -g root "$state_directory"
install -m 0755 -o root -g root "$downloaded_binary" "$install_path"
"$install_path" enroll \
  --center-url "$center_url" \
  --registration-key "$registration_key" \
  --state-dir "$state_directory"

if [ "$init_system" = "systemd" ]; then
  cat > /etc/systemd/system/ipchronicle-agent.service <<'EOF'
[Unit]
Description=IPChronicle Agent
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=root
Group=root
ExecStart=/usr/local/bin/ipchronicle-agent run --state-dir /var/lib/ipchronicle-agent
Restart=on-failure
RestartSec=10s
UMask=0077
NoNewPrivileges=no
PrivateTmp=yes

[Install]
WantedBy=multi-user.target
EOF
  systemctl daemon-reload
  systemctl enable --now ipchronicle-agent.service
else
  cat > /etc/init.d/ipchronicle-agent <<'EOF'
#!/sbin/openrc-run

name="IPChronicle Agent"
description="IPChronicle managed-node Agent"
command="/usr/local/bin/ipchronicle-agent"
command_args="run --state-dir /var/lib/ipchronicle-agent"
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
  chmod 0755 /etc/init.d/ipchronicle-agent
  rc-update add ipchronicle-agent default
  rc-service ipchronicle-agent restart
fi

printf 'IPChronicle Agent installed and started.\n'
