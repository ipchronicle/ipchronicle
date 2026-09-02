#!/bin/sh
set -eu

for required in EXPECTED_ARCH EXPECTED_ID EXPECTED_INIT EXPECTED_VERSION_PREFIX RELEASE_REVISION RELEASE_VERSION; do
  eval "value=\${$required:-}"
  [ -n "$value" ] || {
    printf '%s is required\n' "$required" >&2
    exit 2
  }
done

# shellcheck disable=SC1091
. /etc/os-release
[ "${ID:-}" = "$EXPECTED_ID" ] || {
  printf 'distribution ID is %s, want %s\n' "${ID:-missing}" "$EXPECTED_ID" >&2
  exit 1
}
case "${VERSION_ID:-}" in
  "$EXPECTED_VERSION_PREFIX"*) ;;
  *)
    printf 'distribution version is %s, want branch %s\n' "${VERSION_ID:-missing}" "$EXPECTED_VERSION_PREFIX" >&2
    exit 1
    ;;
esac
if [ "$EXPECTED_ID" = "centos" ]; then
  case "${NAME:-}" in
    *Stream*) ;;
    *) printf 'CentOS image is not a Stream release\n' >&2; exit 1 ;;
  esac
fi

test_root=/release-test
fake_bin="$test_root/bin"
mkdir -p "$fake_bin"

cat > "$fake_bin/systemctl" <<'EOF'
#!/bin/sh
printf 'systemctl %s\n' "$*" >> "$SERVICE_LOG"
if [ "${1:-}" = "is-active" ] && [ "${3:-}" = "ipchronicle-agent-updater.service" ]; then
  exit 3
fi
exit 0
EOF
cat > "$fake_bin/rc-service" <<'EOF'
#!/bin/sh
printf 'rc-service %s\n' "$*" >> "$SERVICE_LOG"
if [ "${1:-}" = "ipchronicle-agent-updater" ] && [ "${2:-}" = "status" ]; then
  exit 3
fi
exit 0
EOF
cat > "$fake_bin/rc-update" <<'EOF'
#!/bin/sh
printf 'rc-update %s\n' "$*" >> "$SERVICE_LOG"
exit 0
EOF
chmod 0755 "$fake_bin/systemctl" "$fake_bin/rc-service" "$fake_bin/rc-update"

fake_agent="$test_root/fake-agent"
cat > "$fake_agent" <<'EOF'
#!/bin/sh
case "${1:-}" in
  version)
    case "$(uname -m)" in
      x86_64|amd64) architecture=amd64 ;;
      aarch64|arm64) architecture=arm64 ;;
      *) exit 1 ;;
    esac
    printf '{"version":"%s","revision":"%s","component":"agent","os":"linux","arch":"%s","capabilities":["agent-update-v1"],"stateSchemaVersion":6}\n' \
      "$RELEASE_VERSION" "$RELEASE_REVISION" "$architecture"
    ;;
  enroll)
    printf '%s\n' "$*" >> "$ENROLL_LOG"
    ;;
  *) exit 0 ;;
esac
EOF
chmod 0755 "$fake_agent"

export ENROLL_LOG="$test_root/enroll.log"
export SERVICE_LOG="$test_root/service.log"
export PATH="$fake_bin:$PATH"

/release/install-agent.sh \
  --center-url https://center.example \
  --registration-key release-matrix-registration-key \
  --agent-binary "$fake_agent"

fake_agent_checksum=$(sha256sum "$fake_agent" | awk '{print $1}')
[ "$(sha256sum /usr/local/bin/ipchronicle-agent | awk '{print $1}')" = "$fake_agent_checksum" ]
[ "$(sha256sum /usr/local/libexec/ipchronicle-agent-updater | awk '{print $1}')" = "$fake_agent_checksum" ]
[ "$(stat -c '%a' /var/lib/ipchronicle-agent)" = "700" ]
[ "$(wc -l < "$ENROLL_LOG")" -eq 1 ]
for dependency in curl jq; do
  command -v "$dependency" >/dev/null 2>&1 || {
    printf 'installer dependency %s is unavailable\n' "$dependency" >&2
    exit 1
  }
done

candidate_metadata=$("/release/ipchronicle-agent-linux-$EXPECTED_ARCH" version --json)
printf '%s\n' "$candidate_metadata" | jq -e \
  --arg version "$RELEASE_VERSION" --arg revision "$RELEASE_REVISION" --arg arch "$EXPECTED_ARCH" '
    .version == $version and .revision == $revision and .component == "agent" and
    .os == "linux" and .arch == $arch and .stateSchemaVersion >= 1 and
    (.capabilities | index("agent-update-v1") != null)
  ' >/dev/null

if [ "$EXPECTED_INIT" = "systemd" ]; then
  grep -F -- '--update-init systemd' /etc/systemd/system/ipchronicle-agent.service >/dev/null
  grep -F -- 'update-supervisor' /etc/systemd/system/ipchronicle-agent-updater.service >/dev/null
  grep -F -- 'systemctl enable --now ipchronicle-agent.service' "$SERVICE_LOG" >/dev/null
else
  grep -F -- '--update-init openrc' /etc/init.d/ipchronicle-agent >/dev/null
  grep -F -- 'update-supervisor' /etc/init.d/ipchronicle-agent-updater >/dev/null
  grep -F -- 'rc-update add ipchronicle-agent default' "$SERVICE_LOG" >/dev/null
fi

/release/install-agent.sh \
  --center-url https://center.example \
  --registration-key release-matrix-registration-key \
  --agent-binary "$fake_agent"
[ "$(wc -l < "$ENROLL_LOG")" -eq 2 ]

/release/install-agent.sh --uninstall
[ ! -e /usr/local/bin/ipchronicle-agent ]
[ ! -e /usr/local/libexec/ipchronicle-agent-updater ]
[ -d /var/lib/ipchronicle-agent ]
if [ "$EXPECTED_INIT" = "systemd" ]; then
  [ ! -e /etc/systemd/system/ipchronicle-agent.service ]
  [ ! -e /etc/systemd/system/ipchronicle-agent-updater.service ]
else
  [ ! -e /etc/init.d/ipchronicle-agent ]
  [ ! -e /etc/init.d/ipchronicle-agent-updater ]
fi

printf 'Distribution lifecycle gate passed: id=%s version=%s arch=%s init=%s\n' \
  "$ID" "$VERSION_ID" "$EXPECTED_ARCH" "$EXPECTED_INIT"
