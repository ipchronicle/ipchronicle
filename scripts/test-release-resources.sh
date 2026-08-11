#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC1091
source "$script_dir/release-images.env"

release_directory=${1:-}
architecture=${2:-}
if [[ -z $release_directory || ! -d $release_directory ]]; then
  echo "usage: $0 RELEASE_DIRECTORY amd64|arm64" >&2
  exit 2
fi
case "$architecture" in
  amd64|arm64) ;;
  *) echo "usage: $0 RELEASE_DIRECTORY amd64|arm64" >&2; exit 2 ;;
esac
for command_name in curl docker jq; do
  command -v "$command_name" >/dev/null 2>&1 || {
    echo "$command_name is required for release resource validation" >&2
    exit 1
  }
done

release_directory=$(cd "$release_directory" && pwd)
version=$(jq -er '.version' "$release_directory/release-manifest.json")
revision=$(jq -er '.revision' "$release_directory/release-manifest.json")
image_ref=$(jq -er '.centerImage' "$release_directory/build-metadata.json")
test -x "$release_directory/ipchronicle-agent-linux-$architecture"
test -f "$release_directory/ipchronicle-center-linux-$architecture.oci.tar"

suffix="${architecture}-$$"
network_name="ipchronicle-resource-$suffix"
center_name="ipchronicle-resource-center-$suffix"
agent_name="ipchronicle-resource-agent-$suffix"
scratch_directory=$(mktemp -d "/var/tmp/ipchronicle-resource-$suffix.XXXXXX")
cookie_file="$scratch_directory/cookies.txt"
response_file="$scratch_directory/response.json"
center_started=false
agent_started=false
network_created=false
image_loaded=false

cleanup() {
  status=$?
  if [[ $status -ne 0 ]]; then
    if [[ $agent_started == true ]]; then
      printf '%s\n' '--- Agent logs ---' >&2
      docker logs --tail 200 "$agent_name" >&2 || true
    fi
    if [[ $center_started == true ]]; then
      printf '%s\n' '--- Center logs ---' >&2
      docker logs --tail 200 "$center_name" >&2 || true
    fi
  fi
  if [[ $agent_started == true ]]; then
    docker rm --force "$agent_name" >/dev/null 2>&1 || true
  fi
  if [[ $center_started == true ]]; then
    docker rm --force "$center_name" >/dev/null 2>&1 || true
  fi
  if [[ $network_created == true ]]; then
    docker network rm "$network_name" >/dev/null 2>&1 || true
  fi
  if [[ $image_loaded == true ]]; then
    docker image rm "$image_ref" >/dev/null 2>&1 || true
  fi
  rm -rf "$scratch_directory"
}
trap cleanup EXIT
trap 'exit 129' HUP
trap 'exit 130' INT
trap 'exit 143' TERM

if docker image inspect "$image_ref" >/dev/null 2>&1; then
  echo "refusing to replace existing local image $image_ref during resource validation" >&2
  exit 1
fi
container_user="$(id -u):$(id -g)"
docker_archive="$scratch_directory/center.docker.tar"
docker run --rm --user "$container_user" -e HOME=/tmp \
  -v "$release_directory:/release:ro" -v "$scratch_directory:/scratch" \
  "$SKOPEO_IMAGE" copy --insecure-policy \
  "oci-archive:/release/ipchronicle-center-linux-$architecture.oci.tar" \
  "docker-archive:/scratch/center.docker.tar:$image_ref" >/dev/null
docker load --input "$docker_archive" >/dev/null
rm -f "$docker_archive"
image_loaded=true

mkdir -p "$scratch_directory/config" "$scratch_directory/history" "$scratch_directory/agent-state"
chmod 0700 "$scratch_directory/agent-state"
docker run --rm --platform "linux/$architecture" \
  -v "$scratch_directory/config:/config" -v "$scratch_directory/history:/history" \
  "$ALPINE_IMAGE" chown 10001:10001 /config /history
docker network create "$network_name" >/dev/null
network_created=true

docker run --detach --name "$center_name" --platform "linux/$architecture" \
  --network "$network_name" --network-alias center \
  --memory 512m --memory-swap 512m --cpus 1 --pids-limit 256 \
  --read-only --user 10001:10001 --cap-drop ALL --security-opt no-new-privileges \
  --env IPCHRONICLE_LISTEN_ADDRESS=:8080 \
  --env IPCHRONICLE_CONFIG_DATABASE_PATH=/var/lib/ipchronicle/config/config.db \
  --env IPCHRONICLE_HISTORY_DATABASE_PATH=/var/lib/ipchronicle/history/history.db \
  --env IPCHRONICLE_MASTER_KEY_PATH=/var/lib/ipchronicle/config/master.key \
  --env IPCHRONICLE_ADMIN_USERNAME=admin --env IPCHRONICLE_ADMIN_PASSWORD=admin \
  --publish 127.0.0.1::8080 \
  --volume "$scratch_directory/config:/var/lib/ipchronicle/config" \
  --volume "$scratch_directory/history:/var/lib/ipchronicle/history" \
  --tmpfs /tmp:size=64m,mode=1777 \
  "$image_ref" >/dev/null
center_started=true

published_address=$(docker port "$center_name" 8080/tcp | head -n 1)
base_url="http://$published_address"
center_ready=false
for _ in $(seq 1 180); do
  if curl --fail --silent --output /dev/null "$base_url/healthz"; then
    center_ready=true
    break
  fi
  sleep 1
done
if [[ $center_ready != true ]]; then
  echo "Center did not become healthy within 180 seconds" >&2
  exit 1
fi

curl --fail --silent --show-error --cookie-jar "$cookie_file" \
  --header "Origin: $base_url" --header 'Content-Type: application/json' \
  --data '{"username":"admin","password":"admin"}' \
  "$base_url/api/v1/auth/login" >"$response_file"
csrf_token=$(jq -er '.csrfToken' "$response_file")
curl --fail --silent --show-error --cookie "$cookie_file" \
  --header "Origin: $base_url" --header "X-CSRF-Token: $csrf_token" \
  --request POST "$base_url/api/v1/agent-enrollment/key" >"$response_file"
registration_key=$(jq -r '.installationCommand' "$response_file" | sed -n "s/.*--registration-key '\([^']*\)'.*/\1/p")
if [[ -z $registration_key ]]; then
  echo "enrollment response did not contain a registration key" >&2
  exit 1
fi

docker run --detach --name "$agent_name" --hostname "release-$architecture" \
  --platform "linux/$architecture" --network "$network_name" \
  --memory 256m --memory-swap 256m --cpus 1 --pids-limit 512 \
  --env CENTER_URL=http://center:8080 --env REGISTRATION_KEY="$registration_key" \
  --env "AGENT_PATH=/release/ipchronicle-agent-linux-$architecture" \
  --volume "$release_directory:/release:ro" \
  --volume "$scratch_directory/agent-state:/var/lib/ipchronicle-agent" \
  --entrypoint /bin/sh "$ALPINE_IMAGE" -ceu \
  'apk add --no-cache bash curl jq bc netcat-openbsd bind-tools iproute2 ca-certificates
   "$AGENT_PATH" enroll \
     --center-url "$CENTER_URL" --registration-key "$REGISTRATION_KEY"
   exec "$AGENT_PATH" run' >/dev/null
agent_started=true

node_ready=false
node_id=""
for _ in $(seq 1 150); do
  if ! docker container inspect "$agent_name" --format '{{.State.Running}}' | grep -Fx true >/dev/null; then
    echo "Agent stopped before configuration convergence" >&2
    exit 1
  fi
  if curl --fail --silent --show-error --cookie "$cookie_file" \
    "$base_url/api/v1/nodes" >"$response_file"; then
    node_id=$(jq -r --arg arch "$architecture" --arg revision "$revision" '
      .items[] | select(.architecture == $arch and .sourceRevision == $revision and
      .status == "online" and .configurationStatus == "current") | .id
    ' "$response_file" | head -n 1)
    if [[ -n $node_id ]] && curl --fail --silent --show-error --cookie "$cookie_file" \
      "$base_url/api/v1/nodes/$node_id/network" >"$response_file" &&
      jq -e '.egresses | any(.kind == "default" and .family == "ipv4" and .enabled == true)' \
        "$response_file" >/dev/null; then
      node_ready=true
      break
    fi
  fi
  sleep 2
done
if [[ $node_ready != true ]]; then
  echo "Agent did not converge with an enabled default IPv4 egress" >&2
  exit 1
fi

agent_rss_kib=$(docker exec "$agent_name" awk '$1 == "VmRSS:" {print $2}' /proc/1/status)
if [[ ! $agent_rss_kib =~ ^[0-9]+$ || $agent_rss_kib -gt 32768 ]]; then
  echo "Agent idle RSS is ${agent_rss_kib:-unknown} KiB, want at most 32768 KiB" >&2
  exit 1
fi

curl --fail --silent --show-error --cookie "$cookie_file" \
  --header "Origin: $base_url" --header "X-CSRF-Token: $csrf_token" \
  --request POST "$base_url/api/v1/nodes/$node_id/sync-session" >/dev/null
curl --fail --silent --show-error --cookie "$cookie_file" \
  --header "Origin: $base_url" --header "X-CSRF-Token: $csrf_token" \
  --request POST "$base_url/api/v1/nodes/$node_id/probe/tasks" >"$response_file"
task_id=$(jq -er '.id' "$response_file")

probe_succeeded=false
run_id=""
for _ in $(seq 1 240); do
  if ! docker container inspect "$agent_name" --format '{{.State.Running}}' | grep -Fx true >/dev/null; then
    echo "Agent stopped while the live IPQuality probe was running" >&2
    exit 1
  fi
  curl --fail --silent --show-error --cookie "$cookie_file" \
    "$base_url/api/v1/nodes/$node_id/probe" >"$response_file"
  task_status=$(jq -r --arg task "$task_id" 'if .task.id == $task then .task.status else "missing" end' "$response_file")
  case "$task_status" in
    succeeded)
      run_id=$(jq -er --arg task "$task_id" '.task | select(.id == $task) | .runId' "$response_file")
      probe_succeeded=true
      break
      ;;
    partial|failed|rejected|expired)
      echo "live IPQuality task reached terminal status $task_status" >&2
      jq '{task, recentRuns}' "$response_file" >&2
      failed_run_id=$(jq -r --arg task "$task_id" '.task | select(.id == $task) | .runId // empty' "$response_file")
      if [[ -n $failed_run_id ]] && curl --fail --silent --show-error --cookie "$cookie_file" \
        "$base_url/api/v1/probe-runs/$failed_run_id" >"$response_file"; then
        jq '{id, status, executions: [.executions[] | {egressId, status, failureStage, diagnostic}]}' \
          "$response_file" >&2
      fi
      exit 1
      ;;
  esac
  sleep 5
done
if [[ $probe_succeeded != true ]]; then
  echo "live IPQuality task did not complete within 20 minutes" >&2
  exit 1
fi

curl --fail --silent --show-error --cookie "$cookie_file" \
  "$base_url/api/v1/probe-runs/$run_id" >"$response_file"
jq -e '
  .status == "succeeded" and .expectedExecutions >= 1 and
  .expectedExecutions == (.executions | length) and
  (.executions | all(.status == "succeeded" and (.snapshotId | type == "string")))
' "$response_file" >/dev/null

agent_oom_kills=$(docker exec "$agent_name" awk '$1 == "oom_kill" {print $2}' /sys/fs/cgroup/memory.events)
center_oom_kills=$(docker exec "$center_name" awk '$1 == "oom_kill" {print $2}' /sys/fs/cgroup/memory.events)
agent_memory_peak=$(docker exec "$agent_name" cat /sys/fs/cgroup/memory.peak)
center_memory_peak=$(docker exec "$center_name" cat /sys/fs/cgroup/memory.peak)
if [[ $agent_oom_kills != 0 || $center_oom_kills != 0 ]]; then
  echo "resource gate observed OOM kills: Agent=$agent_oom_kills Center=$center_oom_kills" >&2
  exit 1
fi
if [[ ! $agent_memory_peak =~ ^[0-9]+$ || $agent_memory_peak -gt 268435456 ]]; then
  echo "Agent cgroup peak is outside the 256 MiB limit: $agent_memory_peak" >&2
  exit 1
fi
if [[ ! $center_memory_peak =~ ^[0-9]+$ || $center_memory_peak -gt 536870912 ]]; then
  echo "Center cgroup peak is outside the 512 MiB limit: $center_memory_peak" >&2
  exit 1
fi

printf 'Release resource gate passed: version=%s revision=%s arch=%s Agent-RSS-KiB=%s Agent-peak-bytes=%s Center-peak-bytes=%s run=%s\n' \
  "$version" "$revision" "$architecture" "$agent_rss_kib" "$agent_memory_peak" "$center_memory_peak" "$run_id"
