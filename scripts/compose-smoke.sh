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
enrollment_file="$(mktemp)"
registration_file="$(mktemp)"

cleanup() {
  IPCHRONICLE_HTTP_PORT="$smoke_port" docker compose --project-name "$project_name" -f "$compose_file" down --volumes --remove-orphans >/dev/null
  rm -f "$status_file" "$login_file" "$cookie_file" "$enrollment_file" "$registration_file"
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
if ! jq -e '.service == "ipchronicle-center" and .status == "ok" and .configSchemaVersion == 18 and .historySchemaVersion == 5 and (.version | length > 0)' "$status_file" >/dev/null; then
  echo "system status did not report the expected service and schema versions" >&2
  jq . "$status_file" >&2
  exit 1
fi
curl --fail --silent --show-error "$base_url/system/status" | grep -Fq '<div id="root"></div>'

curl --fail --silent --show-error \
  --cookie "$cookie_file" \
  --header "Origin: $base_url" \
  --header "X-CSRF-Token: $csrf_token" \
  --request POST \
  "$base_url/api/v1/agent-enrollment/key" >"$enrollment_file"
registration_key="$(jq -r '.installationCommand' "$enrollment_file" | sed -n "s/.*--registration-key '\([^']*\)'.*/\1/p")"
if [[ -z "$registration_key" ]]; then
  echo "enrollment response did not contain a registration key" >&2
  exit 1
fi
jq -n --arg key "$registration_key" '{registrationKey:$key,metadata:{hostname:"smoke-node",agentVersion:"dev",operatingSystem:"linux",architecture:"amd64",physicalMemoryBytes:536870912,capabilities:["control-v1","configuration-v6","complete-probe-v1"]}}' | \
  curl --fail --silent --show-error \
    --header 'Content-Type: application/json' \
    --data-binary @- \
    "$base_url/api/v1/agent/enroll" >"$registration_file"
agent_credential="$(jq -er '.credential' "$registration_file")"
jq -n '{appliedConfigurationRevision:0,metadata:{hostname:"smoke-node",agentVersion:"dev",operatingSystem:"linux",architecture:"amd64",physicalMemoryBytes:536870912,capabilities:["control-v1","configuration-v6","complete-probe-v1"]}}' | \
  curl --fail --silent --show-error \
    --header 'Content-Type: application/json' \
    --header "Authorization: Bearer $agent_credential" \
    --data-binary @- \
    "$base_url/api/v1/agent/control" | \
  jq -e '.desiredConfigurationRevision == 1 and .pollIntervalSeconds == 30' >/dev/null
configuration_file="$(mktemp)"
curl --fail --silent --show-error \
  --header "Authorization: Bearer $agent_credential" \
  "$base_url/api/v1/agent/configuration" >"$configuration_file"
if ! jq -e '.schemaVersion == 6 and .revision == 1 and .enabled == true and (.historyGeneration | length == 64) and .discoveryPaths == [] and .probeTargets == [] and .proxies == [] and .probeSchedule.enabled == true and .probeSchedule.cron == "0 0 0 * * *" and .probeSchedule.timezone == "agent-local" and .probeLowMemoryOverride == false' "$configuration_file" >/dev/null; then
  echo "Agent configuration did not match the expected initial configuration-v6 contract" >&2
  jq . "$configuration_file" >&2
  rm -f "$configuration_file"
  exit 1
fi
rm -f "$configuration_file"
jq -n '{appliedConfigurationRevision:1,metadata:{hostname:"smoke-node",agentVersion:"dev",operatingSystem:"linux",architecture:"amd64",physicalMemoryBytes:536870912,capabilities:["control-v1","configuration-v6","complete-probe-v1"]}}' | \
  curl --fail --silent --show-error \
    --header 'Content-Type: application/json' \
    --header "Authorization: Bearer $agent_credential" \
    --data-binary @- \
    "$base_url/api/v1/agent/control" >/dev/null
curl --fail --silent --show-error \
  --cookie "$cookie_file" \
  "$base_url/api/v1/nodes" | \
  jq -e '.items | length == 1 and .[0].name == "smoke-node" and .[0].status == "online" and .[0].configurationStatus == "current"' >/dev/null

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
