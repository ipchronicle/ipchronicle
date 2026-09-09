#!/usr/bin/env bash
set -euo pipefail

root_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
# shellcheck disable=SC1091
source "$root_dir/scripts/release-images.env"
baseline_revision=9ecd98779546514b043032f4b98f55d77054a8c5
scratch_directory=$(mktemp -d)
cleanup() {
  rm -rf "$scratch_directory"
}
trap cleanup EXIT HUP INT TERM

for revision in "$baseline_revision" HEAD; do
  target=baseline
  if [ "$revision" = HEAD ]; then target=candidate; fi
  mkdir "$scratch_directory/$target"
  git -C "$root_dir" archive "$revision" | tar -x -C "$scratch_directory/$target"
  cp "$root_dir/scripts/upgrade/center_test.go.in" "$scratch_directory/$target/internal/center/nodes/stable_upgrade_test.go"
  cp "$root_dir/scripts/upgrade/agent_test.go.in" "$scratch_directory/$target/internal/agent/state/stable_upgrade_test.go"
done
mkdir "$scratch_directory/fixtures"

docker run --rm --user "$(id -u):$(id -g)" \
  -e HOME=/tmp -e GOCACHE=/tmp/go-build -e GOMODCACHE=/tmp/go-mod \
  -e UPGRADE_FIXTURE_DIRECTORY=/upgrade/fixtures \
  -v "$scratch_directory:/upgrade" "$GO_IMAGE" sh -ceu '
    cd /upgrade/baseline
    UPGRADE_FIXTURE_MODE=seed go test -count=1 -run "^TestStable(Center|Agent)Upgrade$" ./internal/center/nodes ./internal/agent/state
    cd /upgrade/candidate
    UPGRADE_FIXTURE_MODE=verify go test -count=1 -run "^TestStable(Center|Agent)Upgrade$" ./internal/center/nodes ./internal/agent/state
  '
printf '%s\n' 'v0.1.1 data upgrade passed.'
