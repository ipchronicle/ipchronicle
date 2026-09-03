#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
classifier="$script_dir/classify-release-validation.sh"
scratch_directory=$(mktemp -d)
trap 'rm -rf "$scratch_directory"' EXIT

git -C "$scratch_directory" init --quiet
git -C "$scratch_directory" config user.name test
git -C "$scratch_directory" config user.email test@example.invalid
touch "$scratch_directory/README.md"
git -C "$scratch_directory" add README.md
git -C "$scratch_directory" commit --quiet -m base
git -C "$scratch_directory" tag v0.1.0
base_revision=$(git -C "$scratch_directory" rev-parse HEAD)

assert_output() {
  local output=$1 key=$2 expected=$3 actual
  actual=$(sed -n "s/^$key=//p" <<<"$output")
  if [[ $actual != "$expected" ]]; then
    echo "expected $key=$expected, got $actual" >&2
    exit 1
  fi
}

run_classifier() {
  git -C "$scratch_directory" diff --quiet || {
    echo "test repository has uncommitted changes" >&2
    exit 1
  }
  (cd "$scratch_directory" && "$classifier" "$@")
}

output=$(run_classifier 0.2.0 false "$base_revision")
assert_output "$output" full true
assert_output "$output" reason major-minor

output=$(run_classifier 0.1.1 true "$base_revision")
assert_output "$output" full true
assert_output "$output" reason explicit

printf '%s\n' documentation >"$scratch_directory/README.md"
git -C "$scratch_directory" add README.md
git -C "$scratch_directory" commit --quiet -m documentation
output=$(run_classifier 0.1.1 false "$base_revision")
assert_output "$output" full false
assert_output "$output" browser false

mkdir -p "$scratch_directory/web/src"
touch "$scratch_directory/web/src/App.tsx"
git -C "$scratch_directory" add web/src/App.tsx
git -C "$scratch_directory" commit --quiet -m web
output=$(run_classifier 0.1.1 false "$base_revision")
assert_output "$output" full false
assert_output "$output" browser true
assert_output "$output" compose false

mkdir -p "$scratch_directory/internal/agent/probe"
touch "$scratch_directory/internal/agent/probe/probe.go"
git -C "$scratch_directory" add internal/agent/probe/probe.go
git -C "$scratch_directory" commit --quiet -m probe
output=$(run_classifier 0.1.1 false "$base_revision")
assert_output "$output" resources true

output=$(run_classifier 0.1.1 false)
assert_output "$output" base_revision v0.1.0

mkdir -p "$scratch_directory/internal/center/database/migrations/config"
touch "$scratch_directory/internal/center/database/migrations/config/00002_test.sql"
git -C "$scratch_directory" add internal/center/database/migrations/config/00002_test.sql
git -C "$scratch_directory" commit --quiet -m migration
output=$(run_classifier 0.1.1 false "$base_revision")
assert_output "$output" compose true
assert_output "$output" failure true

touch "$scratch_directory/unknown.file"
git -C "$scratch_directory" add unknown.file
git -C "$scratch_directory" commit --quiet -m unknown
output=$(run_classifier 0.1.1 false "$base_revision")
assert_output "$output" full true
assert_output "$output" reason unclassified

printf '%s\n' 'Release validation classifier tests passed.'
