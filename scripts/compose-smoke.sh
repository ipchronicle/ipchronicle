#!/usr/bin/env bash
set -euo pipefail

root_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
compose_file="$root_dir/deploy/compose.yaml"
smoke_port="${IPCHRONICLE_SMOKE_PORT:-18080}"
base_url="http://127.0.0.1:$smoke_port"
status_file="$(mktemp)"

cleanup() {
  IPCHRONICLE_HTTP_PORT="$smoke_port" docker compose -f "$compose_file" down --remove-orphans >/dev/null
  rm -f "$status_file"
}
trap cleanup EXIT

IPCHRONICLE_HTTP_PORT="$smoke_port" docker compose -f "$compose_file" up -d --build --wait --wait-timeout 180

curl --fail --silent --show-error "$base_url/healthz" | rg -x 'ok'
curl --fail --silent --show-error "$base_url/api/v1/system/status" >"$status_file"
jq -e '.service == "ipchronicle-center" and .status == "ok" and (.version | length > 0)' "$status_file" >/dev/null
curl --fail --silent --show-error "$base_url/system/status" | rg -q '<div id="root"></div>'

missing_status="$(curl --silent --output /dev/null --write-out '%{http_code}' "$base_url/api/v1/missing")"
if [[ "$missing_status" != "404" ]]; then
  echo "unknown API returned HTTP $missing_status instead of 404" >&2
  exit 1
fi
