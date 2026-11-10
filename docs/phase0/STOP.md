# Stop follow-up experiment

Status: **NOT VERIFIED** in Cursor. The hook's own JSON was checked by `scripts/stop_experiment.sh` and `go test ./internal/drift`. That does not show whether Cursor submits `followup_message` as the next user turn.

## What is planted

A rules file names a test command. The stop payload says a matching path changed and the command log has no successful run of that command. The hook should return `followup_message` on the first two stops and omit it on the third. `loop_limit` in the install snippet is 3, so Cursor itself should also stop after three automatic follow-ups. The wrapper's own cap is two.

Claude Code is the same cap, with `{"decision":"block","reason":"..."}` instead of `followup_message`. That JSON shape is unit-tested. A real Claude Code session was not run.

## How to run it in Cursor

1. Build `drift-guard` and put it on the PATH Cursor uses, or edit the `command` in the snippet to an absolute path.
2. Copy `docs/phase0/install/.cursor/hooks.json` to `.cursor/hooks.json` in a scratch project, not this repo. Do not commit it. Cloud agents run a committed project hook file.
3. In that project, write `drift-guard.rules.json`:

```json
{"test_command":"pytest","test_path_globs":["**/*.py"]}
```

4. Ask the agent to edit a `.py` file and stop without running pytest.
5. Watch the next turns.

## Expected observation

- Turn 1 ends. Cursor submits one follow-up that mentions the test command. The agent continues.
- If pytest still has not exited 0, a second follow-up arrives.
- There is no third follow-up. Later stops are only lines in `events.jsonl`.

## Result

NOT VERIFIED. No Cursor session was available in the run that added this file. Paste the observation here after a real session: how many follow-ups arrived, and whether the agent continued.
