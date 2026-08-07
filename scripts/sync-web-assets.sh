#!/usr/bin/env bash
set -euo pipefail

root_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
source_dir="$root_dir/web/dist"
target_dir="$root_dir/internal/webui/dist"

if [[ ! -f "$source_dir/index.html" ]]; then
  echo "web build is missing $source_dir/index.html" >&2
  exit 1
fi

find "$target_dir" -depth -mindepth 1 ! -name .gitkeep -delete
cp -R "$source_dir"/. "$target_dir"/
