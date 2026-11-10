# How to run the prototype locally

Build the binary, confirm it answers on stdin, then copy the hooks into the project you want to watch. Hooks are not enabled in this repository. Cloud agents run a committed `.cursor/hooks.json`, so the ready file lives in `docs/phase0/install/`, and the root `.cursor/hooks.json` is gitignored.

## Build

```sh
go build -o "$HOME/.local/bin/drift-guard" ./cmd/drift-guard
export PATH="$HOME/.local/bin:$PATH"
```

Check, before installing hooks:

```sh
printf '%s' '{"hook_event_name":"beforeShellExecution","command":"git status"}' | drift-guard hook
printf '%s' '{"hook_event_name":"beforeShellExecution","command":"git push"}' | drift-guard hook
```

The first command prints `"permission":"allow"`. The second prints `"permission":"deny"`.

If Cursor cannot find `drift-guard`, replace `drift-guard hook` in the snippet with an absolute path.

## Where to copy

Cursor, in the project whose session you want to watch (do not commit it):

```sh
mkdir -p .cursor
cp docs/phase0/install/.cursor/hooks.json .cursor/hooks.json
```

Claude Code, in the project (not in `~/.claude`; a cloud session does not read that directory):

```sh
mkdir -p .claude
cp docs/phase0/install/.claude/settings.json .claude/settings.json
```

The Cursor snippet attaches `drift-guard hook` to `sessionStart`, `beforeSubmitPrompt`, `beforeShellExecution`, `preToolUse` (Shell), `postToolUse`, `afterAgentResponse`, and `stop` (`loop_limit` 3). `git push` has `failClosed: true`. Claude sets `onFailure` to `"block"` on `PreToolUse`/`Bash`. The same files include `SessionStart` and `UserPromptSubmit`: they rewrite the canary line and do not print the code.

Until the binary is on `PATH`, Shell calls and `git push` are blocked. Run the two commands above first. To turn the hooks off, delete the copied JSON.

The hook reads language and test rules from `drift-guard.rules.json` in the project root, or from `DRIFT_GUARD_RULES`:

```json
{"reply_language":"ru","unrequested_delete":"ask","test_command":"pytest","test_path_globs":["**/*.py"]}
```

Without that file, the `git push` deny still works. The language check and the "tests were not run" check stay quiet until a rule names them. `unrequested_delete` defaults to `ask` in an active session: `beforeShellExecution` and `preToolUse` return permission `ask` when a delete or replace was not in the latest prompt. `log` only writes the event. `off` skips it. `cursor-agent --print` does not send the prompt hooks; that case is disputed and still asks. Paths in temp directories and in `bin/`, `dist/`, `node_modules/`, and `.git/` are ignored.

Four semantic checks do not call a model. The default line names are `ЦЕЛЬ`/`GOAL`, `ИСТОЧНИК`/`SOURCE`, and `ДОКАЗАНО`/`PROOF`. On `stop`, an empty or missing line produces a short follow-up, at most two, and does not block the session. A file named on the source line that was not opened, or a command on `ДОКАЗАНО` without a zero exit, is disputed in `events.jsonl` and in the report. `beforeSubmitPrompt` stores the request anchors. If the goal, the changed files, and the reply share too few anchors, that is disputed. A change of conclusion after pushback, with no read and no command, is disputed `sycophancy`. A path or version in the reply that was not read is disputed `answered-from-memory`. Example fields: `docs/semantic/rules.example.json`. The default `anchor_min_shared` is 1. A non-empty phrase list in the rules replaces the built-in list. It does not extend it.

The canary is a separate check, also with no model. The rules field `canary.files` lists files. Each file contains the line `drift-guard-canary: <code>`. `sessionStart` and `beforeSubmitPrompt` (and the command `drift-guard canary refresh --session <id>`) rewrite the code for that session. Hook stdout does not contain the code. The reply needs the line `ПРОЧИТАНО: <file> <code>` or `READ:`. A wrong or empty code on `stop` produces a short follow-up. The limit of two is shared by every follow-up, and the session is not blocked. A correct code with no read in the session log is disputed `canary-read`, with no follow-up. A match means the file was in context this session, not that its rules were applied. Limits and the chosen delivery: `docs/canary/README.md`. The secret is `.drift-guard/canary-secret` and is not in git. Do not commit the session code line: the repository keeps `pending`.

## What is written to disk

All of it is local, gitignored, and not sent anywhere.

- Raw stdin, with `user_email` emptied: `.drift-guard/hook-dump.jsonl` (or the path in `DRIFT_GUARD_DUMP`).
- Short log: `events.jsonl` (or `DRIFT_GUARD_LOG`). File bodies and email are not stored there.
- Session state: `.drift-guard/session-*.json`.

On `stop`, the dump has a `transcript` object:

- `exists` — the file at `transcript_path` is present.
- `complete` — the file contains this turn's reply text. For Cursor the text comes from the previous `afterAgentResponse`. For Claude Code it comes from `last_assistant_message`. If the text is missing, `complete` is false and `reason` says it cannot be confirmed.
- The Cursor and Claude docs say `transcript_path` can lag the current turn. A live session is where that shows up.

## git push

`beforeShellExecution` and `preToolUse`/`PreToolUse` for a shell do not let the command run when it is `git push` (including after `&&` and inside `sh -c`). `echo "git push"` is not blocked. Killing the binary and checking that the push still does not run can only be done in Cursor or Claude: a crash triggers `failClosed` / `onFailure: block`. That is **not verified** here.

## Does the agent continue after stop

See `docs/phase0/STOP.md`. Locally the script checks only the hook JSON:

```sh
sh scripts/stop_experiment.sh
```

It prints `LOCAL: pass` and `CURSOR: NOT VERIFIED`. On a live machine the expectation is one follow-up, a second if the test is still not green, and no third.

## Help verify on a live Cursor

The commands above do not run inside Cursor. On a live session, check the points below yourself. Do not commit the logs. If a dump has personal data, do not keep it and do not share it.

1. A slice of `.drift-guard/hook-dump.jsonl` for `beforeShellExecution`, `preToolUse`, `postToolUse`, `afterAgentResponse`, and `stop`. Before you keep or share it, check by eye that the email is empty and that the raw JSON has no secrets. If you are unsure, keep only the field names and the `transcript` object from `stop`.
2. For one `stop`: `transcript.exists`, `transcript.complete`, and `transcript.reason`. One live session is enough.
3. Whether the `git push` deny fired, and, if you can try it, whether the deny remains after the binary is killed.
4. How many follow-ups arrived in the experiment from `STOP.md`, and whether the agent continued the turn. Until that is observed, the experiment is not verified.

Report from the log, with no network:

```sh
drift-guard report --no-open --output /tmp/drift-guard-report.html
```

Scan a synthetic transcript (already in the repository; it has no personal data). `--log` may come before the path or after it:

```sh
drift-guard scan fixtures/phase0/transcripts/reply-language-synthetic.json --log /tmp/events.jsonl
drift-guard scan --log /tmp/events.jsonl fixtures/phase0/transcripts/reply-language-synthetic.json
```
