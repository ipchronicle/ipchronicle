#!/usr/bin/env bash
set -euo pipefail

repo_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
installer="$repo_root/scripts/install-agent.sh"
test_root=$(mktemp -d)
trap 'rm -rf "$test_root"' EXIT

case "$(uname -m)" in
  x86_64|amd64) architecture=amd64 ;;
  aarch64|arm64) architecture=arm64 ;;
  *) printf 'unsupported test architecture\n' >&2; exit 1 ;;
esac

fake_agent="$test_root/fake-agent"
cat > "$fake_agent" <<EOF
#!/bin/sh
case "\${1:-}" in
  version)
    if [ "\${2:-}" = "--json" ]; then
      printf '%s\n' '{"version":"0.1.0-rc.1","revision":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","component":"agent","os":"linux","arch":"$architecture","capabilities":["agent-update-v1"],"stateSchemaVersion":6}'
    else
      printf '%s\n' '0.1.0-rc.1'
    fi
    ;;
  enroll)
    printf '%s\n' "\$*" >> "\$FAKE_AGENT_LOG"
    ;;
  *) exit 0 ;;
esac
EOF
chmod 0755 "$fake_agent"

fake_bin="$test_root/fake-bin"
mkdir -p "$fake_bin"
cat > "$fake_bin/systemctl" <<'EOF'
#!/bin/sh
printf 'systemctl %s\n' "$*" >> "$FAKE_SERVICE_LOG"
if [ "${1:-}" = "is-active" ] && [ "${3:-}" = "ipchronicle-agent-updater.service" ]; then
  exit 3
fi
exit 0
EOF
cat > "$fake_bin/rc-service" <<'EOF'
#!/bin/sh
printf 'rc-service %s\n' "$*" >> "$FAKE_SERVICE_LOG"
if [ "${1:-}" = "ipchronicle-agent-updater" ] && [ "${2:-}" = "status" ]; then
  exit 3
fi
exit 0
EOF
cat > "$fake_bin/rc-update" <<'EOF'
#!/bin/sh
printf 'rc-update %s\n' "$*" >> "$FAKE_SERVICE_LOG"
exit 0
EOF
cat > "$fake_bin/curl" <<'EOF'
#!/bin/sh
output=""
url=""
while [ "$#" -gt 0 ]; do
  case "$1" in
    --output) output=$2; shift 2 ;;
    --proto) shift 2 ;;
    --header) shift 2 ;;
    --tlsv1.2) shift ;;
    --fail|--location|--silent|--show-error) shift ;;
    *) url=$1; shift ;;
  esac
done
printf 'curl %s\n' "$url" >> "$FAKE_CURL_LOG"
case "$url" in
  *api.github.com/repos/ipchronicle/ipchronicle/releases*) cp "$FAKE_GITHUB_RELEASES" "$output" ;;
  */release-manifest.json) cp "$FAKE_RELEASE_MANIFEST" "$output" ;;
  */checksums.txt) cp "$FAKE_CHECKSUMS" "$output" ;;
  */ipchronicle-agent-linux-*) cp "$FAKE_RELEASE_AGENT" "$output" ;;
  *) exit 22 ;;
esac
EOF
cat > "$fake_bin/dnf" <<'EOF'
#!/bin/sh
printf 'dnf %s\n' "$*" >> "$FAKE_DNF_LOG"
EOF
chmod 0755 "$fake_bin/systemctl" "$fake_bin/rc-service" "$fake_bin/rc-update" "$fake_bin/curl" \
  "$fake_bin/dnf"

run_installer() {
  local root=$1
  local os_release=$2
  shift 2
  IPCHRONICLE_INSTALL_ROOT="$root" \
  IPCHRONICLE_OS_RELEASE="$os_release" \
  IPCHRONICLE_SKIP_PACKAGES="${IPCHRONICLE_SKIP_PACKAGES:-1}" \
  FAKE_AGENT_LOG="$root/enroll.log" \
  FAKE_SERVICE_LOG="$root/service.log" \
  FAKE_CURL_LOG="${FAKE_CURL_LOG:-$root/curl.log}" \
  FAKE_RELEASE_MANIFEST="${FAKE_RELEASE_MANIFEST:-/dev/null}" \
  FAKE_CHECKSUMS="${FAKE_CHECKSUMS:-/dev/null}" \
  FAKE_RELEASE_AGENT="${FAKE_RELEASE_AGENT:-/dev/null}" \
  FAKE_GITHUB_RELEASES="${FAKE_GITHUB_RELEASES:-/dev/null}" \
  FAKE_DNF_LOG="${FAKE_DNF_LOG:-$root/dnf.log}" \
  PATH="$fake_bin:$PATH" \
    "$installer" "$@"
}

assert_installed() {
  local root=$1
  local init_system=$2
  cmp "$fake_agent" "$root/usr/local/bin/ipchronicle-agent"
  cmp "$fake_agent" "$root/usr/local/libexec/ipchronicle-agent-updater"
  [ "$(stat -c '%a' "$root/var/lib/ipchronicle-agent")" = "700" ]
  grep -F -- "--update-init $init_system" "$root/enroll.log" >/dev/null
  if [ "$init_system" = "systemd" ]; then
    grep -F -- "--update-init systemd" "$root/etc/systemd/system/ipchronicle-agent.service" >/dev/null
    grep -F -- "update-supervisor" "$root/etc/systemd/system/ipchronicle-agent-updater.service" >/dev/null
  else
    grep -F -- "--update-init openrc" "$root/etc/init.d/ipchronicle-agent" >/dev/null
    grep -F -- "update-supervisor" "$root/etc/init.d/ipchronicle-agent-updater" >/dev/null
  fi
}

systemd_root="$test_root/systemd-root"
mkdir -p "$systemd_root"
systemd_os_release="$test_root/debian-os-release"
cat > "$systemd_os_release" <<'EOF'
ID=debian
VERSION_ID=13
NAME="Debian GNU/Linux"
EOF
run_installer "$systemd_root" "$systemd_os_release" \
  --center-url https://center.example --registration-key test-key --agent-binary "$fake_agent"
assert_installed "$systemd_root" systemd

rhel_root="$test_root/rhel-root"
mkdir -p "$rhel_root"
rhel_os_release="$test_root/rhel-os-release"
cat > "$rhel_os_release" <<'EOF'
ID=rhel
VERSION_ID=9.7
NAME="Red Hat Enterprise Linux"
EOF
IPCHRONICLE_SKIP_PACKAGES=0 run_installer "$rhel_root" "$rhel_os_release" \
  --center-url https://center.example --registration-key test-key --agent-binary "$fake_agent"
assert_installed "$rhel_root" systemd
grep -Fx 'dnf install --assumeyes jq ca-certificates' "$rhel_root/dnf.log" >/dev/null

run_installer "$systemd_root" "$systemd_os_release" \
  --center-url https://center.example --registration-key test-key --agent-binary "$fake_agent"
[ "$(wc -l < "$systemd_root/enroll.log")" -eq 2 ]
run_installer "$systemd_root" "$systemd_os_release" --uninstall
[ ! -e "$systemd_root/usr/local/bin/ipchronicle-agent" ]
[ ! -e "$systemd_root/usr/local/libexec/ipchronicle-agent-updater" ]
[ ! -e "$systemd_root/etc/systemd/system/ipchronicle-agent.service" ]
[ -d "$systemd_root/var/lib/ipchronicle-agent" ]
run_installer "$systemd_root" "$systemd_os_release" \
  --center-url https://center.example --registration-key test-key --agent-binary "$fake_agent"
printf 'pending-result\n' > "$systemd_root/var/lib/ipchronicle-agent/pending-result"
run_installer "$systemd_root" "$systemd_os_release" --uninstall --purge
[ ! -e "$systemd_root/usr/local/bin/ipchronicle-agent" ]
[ ! -e "$systemd_root/etc/systemd/system/ipchronicle-agent.service" ]
[ ! -e "$systemd_root/var/lib/ipchronicle-agent" ]

openrc_root="$test_root/openrc-root"
mkdir -p "$openrc_root"
openrc_os_release="$test_root/alpine-os-release"
cat > "$openrc_os_release" <<'EOF'
ID=alpine
VERSION_ID=3.24.1
NAME="Alpine Linux"
EOF
run_installer "$openrc_root" "$openrc_os_release" \
  --center-url http://center.example --registration-key test-key --agent-binary "$fake_agent"
assert_installed "$openrc_root" openrc
run_installer "$openrc_root" "$openrc_os_release" \
  --center-url http://center.example --registration-key test-key --agent-binary "$fake_agent"
[ "$(wc -l < "$openrc_root/enroll.log")" -eq 2 ]
run_installer "$openrc_root" "$openrc_os_release" --uninstall
[ ! -e "$openrc_root/etc/init.d/ipchronicle-agent" ]
[ ! -e "$openrc_root/etc/init.d/ipchronicle-agent-updater" ]
[ -d "$openrc_root/var/lib/ipchronicle-agent" ]
run_installer "$openrc_root" "$openrc_os_release" --uninstall --purge
[ ! -e "$openrc_root/var/lib/ipchronicle-agent" ]

remote_root="$test_root/remote-root"
mkdir -p "$remote_root"
remote_manifest="$test_root/release-manifest.json"
remote_checksums="$test_root/checksums.txt"
remote_checksum=$(sha256sum "$fake_agent" | awk '{print $1}')
remote_size=$(wc -c < "$fake_agent" | tr -d ' ')
other_architecture=arm64
if [ "$architecture" = "arm64" ]; then
  other_architecture=amd64
fi
cat > "$remote_manifest" <<EOF
{"schemaVersion":1,"version":"0.1.0-rc.1","tag":"v0.1.0-rc.1","channel":"rc","revision":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","sourceUrl":"https://github.com/ipchronicle/ipchronicle/tree/v0.1.0-rc.1","agentCapabilities":["agent-update-v1"],"artifacts":[{"name":"ipchronicle-agent-linux-$architecture","component":"agent","os":"linux","arch":"$architecture","size":$remote_size,"sha256":"$remote_checksum"},{"name":"ipchronicle-agent-linux-$other_architecture","component":"agent","os":"linux","arch":"$other_architecture","size":1,"sha256":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"}]}
EOF
printf '%s  %s\n' "$remote_checksum" "ipchronicle-agent-linux-$architecture" > "$remote_checksums"
FAKE_CURL_LOG="$remote_root/curl.log" \
FAKE_RELEASE_MANIFEST="$remote_manifest" \
FAKE_CHECKSUMS="$remote_checksums" \
FAKE_RELEASE_AGENT="$fake_agent" \
  run_installer "$remote_root" "$systemd_os_release" \
    --center-url https://center.example --registration-key test-key --version 0.1.0-rc.1
assert_installed "$remote_root" systemd
grep -F '/v0.1.0-rc.1/release-manifest.json' "$remote_root/curl.log" >/dev/null

rc_root="$test_root/rc-root"
mkdir -p "$rc_root"
rc_releases="$test_root/releases.json"
cat > "$rc_releases" <<'EOF'
[
  {"tag_name":"v0.1.0-rc.1","draft":false,"prerelease":true},
  {"tag_name":"v0.0.9","draft":false,"prerelease":false},
  {"tag_name":"v0.2.0-rc.1","draft":true,"prerelease":true}
]
EOF
FAKE_CURL_LOG="$rc_root/curl.log" \
FAKE_GITHUB_RELEASES="$rc_releases" \
FAKE_RELEASE_MANIFEST="$remote_manifest" \
FAKE_CHECKSUMS="$remote_checksums" \
FAKE_RELEASE_AGENT="$fake_agent" \
  run_installer "$rc_root" "$systemd_os_release" \
    --center-url https://center.example --registration-key test-key --channel rc
assert_installed "$rc_root" systemd
grep -F 'api.github.com/repos/ipchronicle/ipchronicle/releases?per_page=100' "$rc_root/curl.log" >/dev/null
grep -F '/v0.1.0-rc.1/release-manifest.json' "$rc_root/curl.log" >/dev/null

unsupported_root="$test_root/unsupported-root"
mkdir -p "$unsupported_root"
unsupported_os_release="$test_root/unsupported-os-release"
cat > "$unsupported_os_release" <<'EOF'
ID=debian
VERSION_ID=11
NAME="Debian GNU/Linux"
EOF
if run_installer "$unsupported_root" "$unsupported_os_release" \
  --center-url https://center.example --registration-key test-key --agent-binary "$fake_agent" \
  >"$test_root/unsupported.out" 2>"$test_root/unsupported.err"; then
  printf 'unsupported distribution unexpectedly installed\n' >&2
  exit 1
fi
grep -F 'unsupported Debian release' "$test_root/unsupported.err" >/dev/null
[ ! -e "$unsupported_root/usr/local/bin/ipchronicle-agent" ]

invalid_purge_root="$test_root/invalid-purge-root"
mkdir -p "$invalid_purge_root"
if run_installer "$invalid_purge_root" "$systemd_os_release" --purge \
  >"$test_root/invalid-purge.out" 2>"$test_root/invalid-purge.err"; then
  printf 'purge without uninstall unexpectedly succeeded\n' >&2
  exit 1
fi
grep -F -- '--uninstall [--purge]' "$test_root/invalid-purge.err" >/dev/null
[ ! -e "$invalid_purge_root/var/lib/ipchronicle-agent" ]

printf 'Agent installer lifecycle tests passed.\n'
