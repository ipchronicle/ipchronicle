#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
root_dir="$(cd "$script_dir/.." && pwd)"
# shellcheck disable=SC1091
source "$script_dir/release-images.env"

directory=${1:-}
ci_run_url=${2:-}
rc_run_url=${3:-}
validation_date=${4:-}
if [[ -z $directory || ! -d $directory || -z $ci_run_url || -z $rc_run_url || -z $validation_date ]]; then
  echo "usage: $0 RELEASE_DIRECTORY CI_RUN_URL RC_RUN_URL YYYY-MM-DD" >&2
  exit 2
fi
for command_name in docker jq; do
  command -v "$command_name" >/dev/null 2>&1 || {
    echo "$command_name is required to finalize a release candidate" >&2
    exit 1
  }
done

directory=$(cd "$directory" && pwd)
version=$(jq -er '.version' "$directory/release-manifest.json")
revision=$(jq -er '.revision' "$directory/release-manifest.json")
container_user="$(id -u):$(id -g)"
scratch_directory=$(mktemp -d)
cleanup() {
  rm -rf "$scratch_directory"
}
trap cleanup EXIT HUP INT TERM

docker run --rm --user "$container_user" \
  -e HOME=/tmp -e GOCACHE=/tmp/go-build -e GOMODCACHE=/tmp/go-mod \
  -v "$root_dir:/src:ro" -v "$scratch_directory:/scratch" -w /src "$GO_IMAGE" \
  sh -ceu 'CGO_ENABLED=0 go build -trimpath -buildvcs=false -o /scratch/ipchronicle-release-tool ./cmd/ipchronicle-release-tool'

"$scratch_directory/ipchronicle-release-tool" finalize \
  --directory "$directory" \
  --version "$version" \
  --revision "$revision" \
  --ci-run-url "$ci_run_url" \
  --rc-run-url "$rc_run_url" \
  --validation-date "$validation_date"
