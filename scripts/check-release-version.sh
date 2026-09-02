#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
root_dir="$(cd "$script_dir/.." && pwd)"
version=${1:-}

if [ -z "$version" ]; then
  # Backticks are literal Markdown delimiters in the readiness document.
  # shellcheck disable=SC2016
  version=$(sed -n 's/^- Version: `\([^`]*\)`$/\1/p' "$root_dir/RELEASE_READINESS.md")
fi

semver_number='(0|[1-9][0-9]*)'
if [[ ! $version =~ ^${semver_number}\.${semver_number}\.${semver_number}(-rc\.${semver_number})?$ ]]; then
  echo "release documentation does not identify one canonical release version" >&2
  exit 1
fi

require_exact_line() {
  local path=$1
  local expected=$2
  if [ "$(grep -Fxc -- "$expected" "$path")" != "1" ]; then
    echo "$path does not contain exactly one expected release-version line: $expected" >&2
    exit 1
  fi
}

require_exact_line "$root_dir/RELEASE_READINESS.md" "# IPChronicle v$version Release Readiness"
require_exact_line "$root_dir/RELEASE_READINESS.md" "- Version: \`$version\`"
require_exact_line "$root_dir/RELEASE_READINESS.md" "- Proposed tag: \`v$version\`"
require_exact_line "$root_dir/OPERATOR_GUIDE.md" "IPCHRONICLE_VERSION=$version"
require_exact_line "$root_dir/RELEASE_NOTES.md" "# IPChronicle v$version"
require_exact_line "$root_dir/.github/workflows/release-candidate.yml" "        default: $version"
require_exact_line "$root_dir/.github/workflows/publish-release.yml" "        default: $version"

if grep -EHn '[0-9]+\.[0-9]+\.[0-9]+-rc\.[0-9]+' \
  "$root_dir/RELEASE_READINESS.md" \
  "$root_dir/OPERATOR_GUIDE.md" \
  "$root_dir/RELEASE_NOTES.md" \
  "$root_dir/.github/workflows/release-candidate.yml" \
  "$root_dir/.github/workflows/publish-release.yml" |
  grep -Fv -- "$version" >/dev/null; then
  echo "release-facing files contain a different release candidate version" >&2
  exit 1
fi

printf 'Release-facing version references agree on %s.\n' "$version"
