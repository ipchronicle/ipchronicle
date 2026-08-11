#!/usr/bin/env bash
set -euo pipefail

left=${1:-}
right=${2:-}
if [[ -z $left || ! -d $left || -z $right || ! -d $right ]]; then
  echo "usage: $0 LEFT_RELEASE_DIRECTORY RIGHT_RELEASE_DIRECTORY" >&2
  exit 2
fi
for command_name in cmp diff find grep jq sha256sum sort stat wc; do
  command -v "$command_name" >/dev/null 2>&1 || {
    echo "$command_name is required to compare release candidates" >&2
    exit 1
  }
done

left=$(cd "$left" && pwd)
right=$(cd "$right" && pwd)
scratch_directory=$(mktemp -d)
cleanup() {
  rm -rf "$scratch_directory"
}
trap cleanup EXIT HUP INT TERM

inventory() {
  local directory=$1
  local path name mode size digest
  if find "$directory" -mindepth 1 -maxdepth 1 ! -type f -print -quit | grep -q .; then
    echo "release directory contains a non-regular entry: $directory" >&2
    return 1
  fi
  while IFS= read -r -d '' path; do
    name=${path##*/}
    mode=$(stat -c '%a' "$path")
    size=$(stat -c '%s' "$path")
    digest=$(sha256sum "$path")
    printf '%s\t%s\t%s\t%s\n' "$name" "$mode" "$size" "${digest%% *}"
  done < <(find "$directory" -mindepth 1 -maxdepth 1 -type f -print0)
}

inventory "$left" | sort >"$scratch_directory/left"
inventory "$right" | sort >"$scratch_directory/right"
if ! cmp -s "$scratch_directory/left" "$scratch_directory/right"; then
  echo "release candidate inventories differ" >&2
  diff -u "$scratch_directory/left" "$scratch_directory/right" >&2 || true
  exit 1
fi

left_revision=$(jq -er '.revision' "$left/release-manifest.json")
right_revision=$(jq -er '.revision' "$right/release-manifest.json")
left_artifacts=$(jq -er '.artifacts | length' "$left/release-manifest.json")
right_artifacts=$(jq -er '.artifacts | length' "$right/release-manifest.json")
if [[ $left_revision != "$right_revision" || $left_artifacts != "$right_artifacts" ]]; then
  echo "release candidate manifest identities differ" >&2
  exit 1
fi
file_count=$(wc -l <"$scratch_directory/left")
printf 'Release candidates are reproducible: revision=%s files=%s artifacts=%s\n' \
  "$left_revision" "$file_count" "$left_artifacts"
