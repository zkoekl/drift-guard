#!/bin/sh
# Local half is verified here. Whether Cursor continues the agent is NOT VERIFIED
# until this is run inside Cursor. See docs/phase0/STOP.md.
set -eu
ROOT=$(CDPATH= cd -- "$(dirname "$0")/.." && pwd)
BIN=${DRIFT_GUARD_BIN:-$ROOT/bin/drift-guard}
if [ ! -x "$BIN" ]; then
  echo "build bin/drift-guard first: go build -o bin/drift-guard ./cmd/drift-guard" >&2
  exit 1
fi
TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT
export DRIFT_GUARD_LOG="$TMP/events.jsonl"
export DRIFT_GUARD_DUMP="$TMP/dump.jsonl"
export DRIFT_GUARD_STATE="$TMP/state"
export DRIFT_GUARD_RULES="$TMP/rules.json"
printf '%s\n' '{"test_command":"pytest","test_path_globs":["**/*.py"]}' > "$DRIFT_GUARD_RULES"
payload='{"hook_event_name":"stop","conversation_id":"stop-exp","status":"completed","loop_count":0,"changed_paths":["src/learn.py"],"test_command":"pytest","transcript_path":""}'
printf '%s' "$payload" | "$BIN" hook > "$TMP/1.json"
printf '%s' "$payload" | "$BIN" hook > "$TMP/2.json"
printf '%s' "$payload" | "$BIN" hook > "$TMP/3.json"
grep -q followup_message "$TMP/1.json"
grep -q followup_message "$TMP/2.json"
if grep -q followup_message "$TMP/3.json"; then
  echo "LOCAL: fail (a third follow-up was returned)" >&2
  exit 1
fi
echo "LOCAL: pass (follow-up on stop 1 and 2, none on stop 3, loop_limit 3)"
cat <<'EOF'

CURSOR: NOT VERIFIED
This process did not call Cursor. The local JSON above only shows what the hook prints.
Expected observation, after the hooks in docs/phase0/install/.cursor/hooks.json are installed
and a rule file names a test command:
  1. Finish a turn that edits a matching path and does not run that test.
     The agent should receive one follow-up and keep going.
  2. If the test still has not exited 0, a second follow-up should arrive.
  3. There must be no third follow-up. The hook only logs after two.
Record what Cursor actually did in docs/phase0/STOP.md under Result. Until that
note exists from a real session, the answer is NOT VERIFIED.
EOF
