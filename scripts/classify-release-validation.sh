#!/usr/bin/env bash
set -euo pipefail

version=${1:-}
force_full=${2:-false}
base_revision=${3:-}
head_revision=${4:-HEAD}

semver_number='(0|[1-9][0-9]*)'
if [[ ! $version =~ ^${semver_number}\.(${semver_number})\.(${semver_number})(-rc\.${semver_number})?$ ]]; then
  echo "version is not a supported canonical release version" >&2
  exit 2
fi
case "$force_full" in
  true|false) ;;
  *) echo "force-full must be true or false" >&2; exit 2 ;;
esac
git rev-parse --verify "$head_revision^{commit}" >/dev/null

core_version=${version%%-*}
IFS=. read -r target_major target_minor target_patch <<<"$core_version"

version_is_older() {
  local candidate=$1
  local candidate_major candidate_minor candidate_patch
  IFS=. read -r candidate_major candidate_minor candidate_patch <<<"$candidate"

  (( 10#$candidate_major < 10#$target_major )) ||
    (( 10#$candidate_major == 10#$target_major && 10#$candidate_minor < 10#$target_minor )) ||
    (( 10#$candidate_major == 10#$target_major && 10#$candidate_minor == 10#$target_minor && 10#$candidate_patch < 10#$target_patch ))
}

find_previous_stable_tag() {
  local tag candidate
  while IFS= read -r tag; do
    if [[ $tag =~ ^v(${semver_number})\.(${semver_number})\.(${semver_number})$ ]]; then
      candidate=${tag#v}
      if version_is_older "$candidate"; then
        printf '%s\n' "$tag"
        return 0
      fi
    fi
  done < <(git tag --merged "$head_revision" --sort=-version:refname)
  return 1
}

full=false
distribution=false
resources=false
compose=false
browser=false
failure=false
reason=patch

enable_full() {
  full=true
  distribution=true
  resources=true
  compose=true
  browser=true
  failure=true
}

if [[ $force_full == true ]]; then
  enable_full
  reason=explicit
elif (( 10#$target_patch == 0 )); then
  enable_full
  reason=major-minor
else
  if [[ -z $base_revision ]]; then
    base_revision=$(find_previous_stable_tag || true)
  fi
  if [[ -z $base_revision ]] || ! git rev-parse --verify "$base_revision^{commit}" >/dev/null 2>&1; then
    enable_full
    reason=no-stable-base
  else
    unclassified_path=
    while IFS= read -r path; do
      [[ -n $path ]] || continue
      case "$path" in
        .github/workflows/*|Makefile|go.mod|go.sum|sqlc.yaml|scripts/classify-release-validation.sh|scripts/test-classify-release-validation.sh|scripts/build-release-candidate.sh|scripts/check-release-version.sh|scripts/compare-release-candidates.sh|scripts/finalize-release-candidate.sh|scripts/release-images.env|scripts/verify-release-candidate.sh|cmd/ipchronicle-release-tool/*|internal/releaseinfo/*|internal/releasetool/*)
          enable_full
          reason=release-infrastructure
          break
          ;;
        *.md|LICENSE|.editorconfig|.gitattributes|.gitignore|.github/dependabot.yml|.github/ISSUE_TEMPLATE/*|.github/PULL_REQUEST_TEMPLATE*)
          ;;
        Dockerfile|.dockerignore|.env.example|deploy/*|testdata/compose.yaml|scripts/compose-smoke.sh)
          compose=true
          ;;
        scripts/install-agent.sh|scripts/test-install-agent.sh|scripts/test-release-distribution.sh|scripts/test-release-distribution-inner.sh|cmd/ipchronicle-agent/*|internal/agent/update/*|internal/agent/state/update*|internal/agent/requirements*)
          distribution=true
          failure=true
          ;;
        scripts/test-release-resources.sh|internal/agent/network/*|internal/agent/observation/*|internal/agent/probe/*|internal/agent/probe_control*|internal/center/nodes/address*|internal/center/nodes/network*|internal/center/nodes/probe*|internal/center/nodes/proxy*|internal/probefields/*)
          resources=true
          browser=true
          ;;
        scripts/browser-test.sh|web/*|internal/webui/*)
          browser=true
          ;;
        internal/center/database/migrations/*)
          compose=true
          failure=true
          ;;
        scripts/test-release-failures.sh|internal/agent/state/*|internal/agent/control*|internal/agent/sync*|internal/center/database/*|internal/center/notifications/*)
          failure=true
          ;;
        openapi/*|internal/generated/*)
          browser=true
          failure=true
          ;;
        cmd/ipchronicle-center/*|internal/center/admin/*|internal/center/history/*|internal/center/nodes/*|internal/center/syncws/*|internal/center/systemsettings/*|internal/center/api_*|internal/center/config*|internal/center/http*)
          compose=true
          browser=true
          failure=true
          ;;
        internal/schedule/*)
          browser=true
          failure=true
          ;;
        internal/version/*)
          ;;
        *)
          unclassified_path=$path
          break
          ;;
      esac
    done < <(git diff --name-only "$base_revision" "$head_revision")

    if [[ -n $unclassified_path ]]; then
      enable_full
      reason=unclassified
    fi
  fi
fi

printf 'base_revision=%s\n' "$base_revision"
printf 'full=%s\n' "$full"
printf 'distribution=%s\n' "$distribution"
printf 'resources=%s\n' "$resources"
printf 'compose=%s\n' "$compose"
printf 'browser=%s\n' "$browser"
printf 'failure=%s\n' "$failure"
printf 'reason=%s\n' "$reason"
