#!/usr/bin/env bash
set -euo pipefail

root_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
compose_file="$root_dir/deploy/compose.yaml"
browser_port="${IPCHRONICLE_BROWSER_PORT:-18081}"
playwright_image="${PLAYWRIGHT_IMAGE:-mcr.microsoft.com/playwright:v1.62.1-noble}"

cleanup() {
  IPCHRONICLE_HTTP_PORT="$browser_port" docker compose -f "$compose_file" down --remove-orphans >/dev/null
}
trap cleanup EXIT

IPCHRONICLE_HTTP_PORT="$browser_port" docker compose -f "$compose_file" up -d --build --wait --wait-timeout 180

docker run --rm --network host --ipc host \
  --user "$(id -u):$(id -g)" \
  -e HOME=/tmp \
  -e "IPCHRONICLE_E2E_BASE_URL=http://127.0.0.1:$browser_port" \
  -v "$root_dir:/workspace" \
  -w /workspace/web \
  "$playwright_image" \
  sh -ceu 'npm ci; npm run test:e2e'
