#!/bin/sh
# Install drift-guard as a user-level Cursor hook.
# POSIX sh. macOS and Linux. Idempotent.
# Writes only under $HOME/.drift-guard, $HOME/.cursor, and $HOME/.cursor/skills.
set -eu

if [ -z "${HOME:-}" ]; then
  echo "HOME is not set" >&2
  exit 1
fi

unset CDPATH
script_dir=$(cd -- "$(dirname "$0")" && pwd)
repo_root=$(cd -- "$script_dir/../../.." && pwd)
prefix=$HOME/.drift-guard
bin=$prefix/bin
wrapper=$bin/drift-guard-hook
state=$prefix/state
cursor_dir=$HOME/.cursor
hooks=$cursor_dir/hooks.json
backup=$cursor_dir/hooks.json.drift-guard-backup
manifest=$prefix/install-manifest
skills_src=$repo_root/docs/install/skills

case $prefix in
  *[\'\"\`\$\\]*)
    echo "HOME path has a character this installer will not embed: $prefix" >&2
    exit 1
    ;;
esac

# Print one single-quoted shell word. A quote in the value becomes '\''.
shell_quote() {
  awk -v s="$1" 'BEGIN { gsub(/\047/, "\047\\\047\047", s); printf "\047%s\047", s }'
}

json_escape() {
  printf '%s' "$1" | sed 's/\\/\\\\/g; s/"/\\"/g'
}

manifest_has() {
  key=$1
  if [ ! -f "$manifest" ]; then
    return 1
  fi
  grep -q "^$key " "$manifest"
}

manifest_add() {
  key=$1
  val=$2
  if manifest_has "$key"; then
    return 0
  fi
  printf '%s %s\n' "$key" "$val" >> "$manifest"
}

mkdir -p "$bin" "$state" "$cursor_dir"
if [ ! -f "$manifest" ]; then
  : > "$manifest"
fi

(
  cd "$repo_root" || exit 1
  go build -o "$bin/drift-guard" ./cmd/drift-guard
)

log_q=$(shell_quote "$prefix/events.jsonl")
state_q=$(shell_quote "$state")
dump_q=$(shell_quote "$prefix/hook-dump.jsonl")
bin_q=$(shell_quote "$bin/drift-guard")
cat > "$wrapper" <<EOF
#!/bin/sh
export DRIFT_GUARD_ACTIVATION=opt-in
export DRIFT_GUARD_LOG=$log_q
export DRIFT_GUARD_STATE=$state_q
export DRIFT_GUARD_DUMP=$dump_q
exec $bin_q hook
EOF
chmod +x "$wrapper"

if [ -d "$skills_src" ]; then
  mkdir -p "$HOME/.cursor/skills"
  for name in check-ambiguity handoff-constraints verify-before-done; do
    src=$skills_src/$name
    dest=$HOME/.cursor/skills/$name
    if [ ! -d "$src" ]; then
      continue
    fi
    if [ -e "$dest" ]; then
      echo "skill $name already present, left in place"
      continue
    fi
    cp -R "$src" "$dest"
    manifest_add "skill_$name" "$name"
  done
fi

cmd_json=$(json_escape "$wrapper")
tmp=$(mktemp "$prefix/hooks.json.XXXXXX")
cat > "$tmp" <<EOF
{
  "version": 1,
  "hooks": {
    "beforeShellExecution": [
      { "command": "$cmd_json", "timeout": 10 }
    ],
    "preToolUse": [
      { "command": "$cmd_json", "timeout": 10, "matcher": "Shell" }
    ],
    "postToolUse": [
      { "command": "$cmd_json", "timeout": 10 }
    ],
    "sessionStart": [
      { "command": "$cmd_json", "timeout": 10 }
    ],
    "beforeSubmitPrompt": [
      { "command": "$cmd_json", "timeout": 10 }
    ],
    "afterAgentResponse": [
      { "command": "$cmd_json", "timeout": 10 }
    ],
    "stop": [
      { "command": "$cmd_json", "timeout": 10, "loop_limit": 3 }
    ]
  }
}
EOF

if [ ! -f "$hooks" ]; then
  mv "$tmp" "$hooks"
  manifest_add "hooks" "created"
  echo "installed $wrapper"
  exit 0
fi

if cmp -s "$tmp" "$hooks"; then
  rm -f "$tmp"
  echo "installed $wrapper"
  exit 0
fi

# No "command" key means an empty hook file. Replacing that is trivial.
if [ ! -s "$hooks" ] || ! grep -q '"command"' "$hooks"; then
  if [ ! -f "$backup" ]; then
    cp "$hooks" "$backup"
  fi
  mv "$tmp" "$hooks"
  manifest_add "hooks" "replaced"
  echo "installed $wrapper"
  echo "previous hooks.json saved at $backup"
  exit 0
fi

rm -f "$tmp"
if [ ! -f "$backup" ]; then
  cp "$hooks" "$backup"
fi
manifest_add "hooks" "blocked"
echo "hooks.json already exists and was not changed: $hooks" >&2
echo "a copy is at $backup" >&2
echo "the wrapper is ready at $wrapper" >&2
echo "merge it by hand, or move hooks.json aside and run install again" >&2
exit 1
