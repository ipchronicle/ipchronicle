#!/usr/bin/env bash
set -euo pipefail

root_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
compose_file="$root_dir/deploy/compose.yaml"
project_name="ipchronicle-smoke"
smoke_port="${IPCHRONICLE_SMOKE_PORT:-18080}"
base_url="http://127.0.0.1:$smoke_port"
status_file="$(mktemp)"
login_file="$(mktemp)"
cookie_file="$(mktemp)"

cleanup() {
  IPCHRONICLE_HTTP_PORT="$smoke_port" docker compose --project-name "$project_name" -f "$compose_file" down --volumes --remove-orphans >/dev/null
  rm -f "$status_file" "$login_file" "$cookie_file"
}
trap cleanup EXIT

IPCHRONICLE_HTTP_PORT="$smoke_port" docker compose --project-name "$project_name" -f "$compose_file" up -d --build --wait --wait-timeout 180

curl --fail --silent --show-error "$base_url/healthz" | grep -Fx 'ok'
unauthenticated_status="$(curl --silent --output /dev/null --write-out '%{http_code}' "$base_url/api/v1/system/status")"
if [[ "$unauthenticated_status" != "401" ]]; then
  echo "protected status returned HTTP $unauthenticated_status instead of 401" >&2
  exit 1
fi
curl --fail --silent --show-error \
  --cookie-jar "$cookie_file" \
  --header "Origin: $base_url" \
  --header 'Content-Type: application/json' \
  --data '{"username":"admin","password":"admin"}' \
  "$base_url/api/v1/auth/login" >"$login_file"
csrf_token="$(jq -er '.csrfToken' "$login_file")"
curl --fail --silent --show-error --cookie "$cookie_file" \
  "$base_url/api/v1/system/status" >"$status_file"
jq -e '.service == "ipchronicle-center" and .status == "ok" and .configSchemaVersion == 1 and .historySchemaVersion == 1 and (.version | length > 0)' "$status_file" >/dev/null
curl --fail --silent --show-error "$base_url/system/status" | grep -Fq '<div id="root"></div>'

logout_status="$(curl --silent --output /dev/null --write-out '%{http_code}' \
  --cookie "$cookie_file" \
  --header "Origin: $base_url" \
  --header "X-CSRF-Token: $csrf_token" \
  --request POST \
  "$base_url/api/v1/auth/logout")"
if [[ "$logout_status" != "204" ]]; then
  echo "logout returned HTTP $logout_status instead of 204" >&2
  exit 1
fi

recovery_password="compose-recovered-password"
printf '%s\n' "$recovery_password" | \
  IPCHRONICLE_HTTP_PORT="$smoke_port" docker compose --project-name "$project_name" -f "$compose_file" \
    exec -T center /usr/local/bin/ipchronicle-center admin reset-password --password-stdin
rm -f "$cookie_file"
curl --fail --silent --show-error \
  --cookie-jar "$cookie_file" \
  --header "Origin: $base_url" \
  --header 'Content-Type: application/json' \
  --data "{\"username\":\"admin\",\"password\":\"$recovery_password\"}" \
  "$base_url/api/v1/auth/login" >"$login_file"
jq -e '.account.username == "admin" and .account.usesDefaultCredentials == false' "$login_file" >/dev/null

missing_status="$(curl --silent --output /dev/null --write-out '%{http_code}' "$base_url/api/v1/missing")"
if [[ "$missing_status" != "404" ]]; then
  echo "unknown API returned HTTP $missing_status instead of 404" >&2
  exit 1
fi
