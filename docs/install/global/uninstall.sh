#!/bin/sh
# Remove only the files docs/install/global/install.sh added, and restore a hooks backup.
# POSIX sh. The event log and session state under ~/.drift-guard are left in place.
set -eu

if [ -z "${HOME:-}" ]; then
  echo "HOME is not set" >&2
  exit 1
fi

prefix=$HOME/.drift-guard
manifest=$prefix/install-manifest
hooks=$HOME/.cursor/hooks.json
backup=$HOME/.cursor/hooks.json.drift-guard-backup
mode=""

if [ -f "$manifest" ]; then
  mode=$(awk '$1 == "hooks" { print $2; exit }' "$manifest" || true)
fi

case "$mode" in
  created)
    rm -f "$hooks"
    ;;
  replaced)
    if [ -f "$backup" ]; then
      mv "$backup" "$hooks"
    else
      rm -f "$hooks"
    fi
    ;;
  blocked)
    if [ ! -f "$hooks" ] && [ -f "$backup" ]; then
      mv "$backup" "$hooks"
    else
      rm -f "$backup"
    fi
    ;;
  *)
    ;;
esac

if [ -f "$manifest" ]; then
  while IFS=' ' read -r key val; do
    case "$key" in
      skill_*)
        ;;
      *)
        continue
        ;;
    esac
    case "$val" in
      ""|*[!a-z-]*)
        continue
        ;;
    esac
    rm -rf "$HOME/.cursor/skills/$val"
  done < "$manifest"
  rm -f "$manifest"
fi

rm -f "$prefix/bin/drift-guard" "$prefix/bin/drift-guard-hook"
rmdir "$HOME/.cursor/skills" 2>/dev/null || true
rmdir "$prefix/bin" 2>/dev/null || true
rmdir "$prefix/state" 2>/dev/null || true
rmdir "$prefix" 2>/dev/null || true

echo "removed the drift-guard global hook install"
echo "left $prefix/events.jsonl and $prefix/state in place when they exist"
