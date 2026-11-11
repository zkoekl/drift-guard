#!/bin/sh
# Project hook for this module. Uses drift-guard on PATH, or builds it, then execs the hook.
# Stdout is only the hook JSON. The dogfood canary file is gitignored.
set -eu
unset CDPATH
script_dir=$(cd -- "$(dirname "$0")" && pwd)
root=$(cd -- "$script_dir/../.." && pwd)
canary=$root/.drift-guard/canary.md
if [ ! -f "$canary" ]; then
  mkdir -p "$root/.drift-guard"
  printf '%s\n' 'drift-guard-canary: pending' > "$canary"
fi
if command -v drift-guard >/dev/null 2>&1; then
  exec drift-guard hook
fi
bin=$root/.drift-guard/bin/drift-guard
if [ ! -x "$bin" ]; then
  mkdir -p "$root/.drift-guard/bin"
  (cd "$root" && go build -o "$bin" ./cmd/drift-guard)
fi
exec "$bin" hook
