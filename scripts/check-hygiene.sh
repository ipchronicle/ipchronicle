#!/usr/bin/env bash
set -euo pipefail

root_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$root_dir"

git diff --check
git diff --cached --check

forbidden_files='(^|/)(node_modules|dist|bin|coverage|playwright-report|test-results)(/|$)|(^|/)\.env($|\.)|\.db($|-shm$|-wal$)'
if git ls-files | grep -E "$forbidden_files" | grep -Ev '^(\.env\.example|internal/webui/dist/\.gitkeep)$'; then
  echo "tracked generated artifact or local state found" >&2
  exit 1
fi

if git grep -I -n -E 'AKIA[0-9A-Z]{16}|-----BEGIN (RSA |EC |OPENSSH )?PRIVATE KEY-----|gh[pousr]_[A-Za-z0-9_]{36,}' -- .; then
  echo "possible secret found in tracked source" >&2
  exit 1
fi
