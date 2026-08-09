#!/usr/bin/env bash
set -euo pipefail

root_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
compose_file="$root_dir/deploy/compose.yaml"
project_name="ipchronicle-browser"
browser_port="${IPCHRONICLE_BROWSER_PORT:-18081}"
receiver_port="${IPCHRONICLE_BROWSER_RECEIVER_PORT:-19090}"
playwright_image="${PLAYWRIGHT_IMAGE:-mcr.microsoft.com/playwright:v1.62.1-noble}"
node_image="${NODE_IMAGE:-node:24.19.0-bookworm-slim}"
receiver_name="${project_name}-notification-receiver"
receiver_started=false

cleanup() {
  if [[ "$receiver_started" == "true" ]]; then
    docker rm --force "$receiver_name" >/dev/null 2>&1 || true
  fi
  IPCHRONICLE_HTTP_PORT="$browser_port" docker compose --project-name "$project_name" -f "$compose_file" down --volumes --remove-orphans >/dev/null
}
trap cleanup EXIT

IPCHRONICLE_HTTP_PORT="$browser_port" docker compose --project-name "$project_name" -f "$compose_file" up -d --build --wait --wait-timeout 180

if docker container inspect "$receiver_name" >/dev/null 2>&1; then
  echo "browser-test receiver container already exists: $receiver_name" >&2
  exit 1
fi
docker run --detach --rm \
  --name "$receiver_name" \
  --network "${project_name}_default" \
  --publish "127.0.0.1:${receiver_port}:19090" \
  --volume "$root_dir/web/tests/fixtures/notification-receiver.mjs:/receiver.mjs:ro" \
  "$node_image" node /receiver.mjs >/dev/null
receiver_started=true
for _ in $(seq 1 50); do
  if curl --fail --silent --output /dev/null "http://127.0.0.1:${receiver_port}/healthz"; then
    receiver_ready=true
    break
  fi
  sleep 0.1
done
if [[ "${receiver_ready:-false}" != "true" ]]; then
  echo "browser-test notification receiver did not become ready" >&2
  exit 1
fi

docker run --rm --network host --ipc host \
  --user "$(id -u):$(id -g)" \
  -e HOME=/tmp \
  -e "IPCHRONICLE_E2E_BASE_URL=http://127.0.0.1:$browser_port" \
  -e "IPCHRONICLE_E2E_RECEIVER_URL=http://127.0.0.1:$receiver_port" \
  -e "IPCHRONICLE_E2E_RECEIVER_INTERNAL_URL=http://$receiver_name:19090" \
  -v "$root_dir:/workspace" \
  -w /workspace/web \
  "$playwright_image" \
  sh -ceu 'npm ci; npm run test:e2e -- "$@"' sh "$@"
