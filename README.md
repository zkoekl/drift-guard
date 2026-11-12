# drift-guard

[![CI](https://github.com/zkoekl/drift-guard/actions/workflows/ci.yml/badge.svg)](https://github.com/zkoekl/drift-guard/actions/workflows/ci.yml)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](https://github.com/zkoekl/drift-guard/blob/main/LICENSE)

AI coding agents drift. They change things nobody asked for, delete files, push to main, and say "done" without running anything. `drift-guard` is a local checker that runs from agent hooks (Cursor, Claude Code) and from CI, and stops or flags those moves. One Go binary: no daemon, no model, no telemetry.

## What it does

Checks that block or ask:

- **Push guard.** Denies `git push` to main, master, or the default branch, force pushes, and tag pushes. A normal push of a feature branch goes through.
- **Destructive commands.** Denies `git reset --hard`, `git clean -f`, `git checkout .` and `git restore .`, `git stash drop` and `clear`, `rm -r` on `/`, `~`, `.`, `..`, `*` or the repo root, and a download piped into a shell (`curl ... | sh`).
- **Secrets.** Denies reading or writing `.env` and `.env.*` (not `.env.example`, `.sample`, `.template`), `~/.ssh`, `~/.aws`, `~/.config/gcloud`, `*.pem`, `*.key`, `id_rsa*`, `.npmrc`, and `.netrc`, in shell commands and in the file tools of Cursor and Claude Code.
- **Guard the guard.** Denies an agent editing, deleting, or moving `.cursor/hooks.json`, `.claude/settings.json`, `.claude/settings.local.json`, `drift-guard.rules.json`, or `~/.drift-guard/`.
- **Unrequested deletes.** A delete, move, or truncation of a file the latest prompt did not name returns `ask`. In a Cursor cloud agent, where nobody can answer, it returns `deny`.
- **PR scope.** `drift-guard scope` fails when a changed file matches none of the globs declared for the change. It runs in CI and as a pre-commit hook.
- **Tests before done.** If the rules file names a test command and a matching file changed, stopping without that command exiting 0 sends the agent a short follow-up (at most two).

Checks that only log and show up in the report:

- Reply in the wrong language, when a rule names one.
- An agent that drops its answer after pushback with no new read or command in between.
- A file path or version in the reply that the session never read.
- A reply that shares too little with the prompt.
- Optional (`require_markers`): answer marker lines and a per-session canary code.

`drift-guard report` turns the local event log into one offline HTML file.

## Example

```sh
echo '{"hook_event_name":"beforeShellExecution","command":"git push origin HEAD:main"}' | drift-guard hook
```

```json
{"agent_message":"Push the feature branch without --force. Do not push main, the default branch, or a tag.","permission":"deny","user_message":"A push to main, master, the default branch, a force push, or a tag push is denied."}
```

A prompt "Fix the typo in README.md" followed by `rm -rf src`:

```json
{"agent_message":"Deleting or replacing src was not part of the task. Ask the user before deleting it.","permission":"ask","user_message":"Deleting or replacing src was not part of the task. Ask the user before deleting it."}
```

A scope check on a change that touches CI and deploy files when only `gameplay/**` is allowed:

```text
scope violation (scope file):
outside: .github/workflows/build.yml
outside: deploy/Dockerfile
```

## Install

Download a binary from [Releases](https://github.com/zkoekl/drift-guard/releases) (Linux and macOS, amd64 and arm64; Windows amd64), unpack it, and put `drift-guard` on `PATH`. Or, with Go 1.22 or later:

```sh
go install github.com/zkoekl/drift-guard/cmd/drift-guard@latest
```

**Global, for every local Cursor project**, idle until a session turns it on. From a checkout of this repository:

```sh
sh docs/install/global/install.sh
```

This builds `~/.drift-guard/bin/drift-guard` and a wrapper, and writes `~/.cursor/hooks.json` if that file is empty or missing. If it already has commands, the script stops and leaves a backup. `sh docs/install/global/uninstall.sh` removes it. Cloud agents do not load `~/.cursor/hooks.json`. Details: [`docs/install/global/README.md`](docs/install/global/README.md).

**Per project**, always on for that project. Commit the files if a Cursor cloud agent should run them:

```sh
mkdir -p .cursor .claude
cp docs/hooks/install/.cursor/hooks.json .cursor/hooks.json
cp docs/hooks/install/.claude/settings.json .claude/settings.json
```

The Cursor snippet sets `failClosed: true` on `git push`; the Claude snippet sets `onFailure` to `block`. Walkthrough: [`docs/hooks/README.md`](docs/hooks/README.md). Copy-in rule text for an agent, separate from the hooks: [`docs/install/README.md`](docs/install/README.md).

## Turn the checks on

The global install allows everything until the session is on. Either:

- write `#dg` as its own word in the chat (case does not matter; the wrapper's `DRIFT_GUARD_TAG` changes the tag), or
- put `drift-guard.rules.json` in the project root. `DRIFT_GUARD_RULES` points at another path.

Without a rules file, the push, destructive-command, secrets, and guard-the-guard checks still work and the language and test checks stay quiet.

## Config

```json
{
  "reply_language": "en",
  "unrequested_delete": "ask",
  "test_command": "go test ./...",
  "test_path_globs": ["**/*.go"],
  "canary": { "files": ["AGENTS.md"] },
  "guard_allow": [".env.test"]
}
```

`unrequested_delete` is `ask` (the default in an active session), `log`, or `off`. `log` records the event and allows the call. When the session never saw the prompt text, the event is marked disputed. `guard_allow` lists paths or file names (globs) that the secrets and guard-the-guard checks let through. `require_markers` is `false` by default. When `true`, a `stop` reply missing the `GOAL:`, `SOURCE:`, or `PROOF:` lines, or the `READ:` canary quote for `canary.files`, gets a short follow-up. More optional fields: [`docs/semantic/rules.example.json`](docs/semantic/rules.example.json). Log fields: [`docs/METRICS.md`](docs/METRICS.md).

## Commands

```text
drift-guard --version
drift-guard hook
drift-guard check --input file.json [--log events.jsonl]
drift-guard scan [--log events.jsonl] <transcript>
drift-guard report [--log events.jsonl] [--output file.html] [--no-open] [--summary]
drift-guard scope [--require-scope] [--scope-file f] [--files f|-] [--staged]
                  [--base ref --head ref]
drift-guard canary refresh --session ID [--rules f] [--root dir] [--secret-file f]
```

`hook` reads JSON on stdin and writes JSON on stdout. `check` and `scope` exit `0` when clean, `1` on a violation, `2` on a usage error. `scan` checks a saved transcript, exits `0`, and writes findings to the log. `report --summary` prints counts only, with no paths, commands, prompts, or session ids. `canary refresh` rewrites `drift-guard-canary:` in the configured files and does not print the code.

A pull request declares its scope as a block in the description. CI runs `scope --require-scope` and fails when the block is missing or a changed file matches no glob. See [`.github/PULL_REQUEST_TEMPLATE.md`](.github/PULL_REQUEST_TEMPLATE.md).

## Limitations

- The checks see only what the host sends to hooks. A push made inside a script or another program is not seen.
- The global install is fail-open: a dead or slow hook does not block `git push`. The per-project snippet is fail-closed.
- The delete check reads `rm`, `unlink`, `git rm`, `mv`, and `>` truncation. It skips globs, variables, `sudo`, `sh -c`, and deletes inside another program. If the prompt names no file, the call is allowed and the event is disputed. Full list: [`docs/METRICS.md`](docs/METRICS.md).
- The destructive, secrets, and guard-the-guard checks read command words and tool paths. They do not see a script, an interpreter (`python -c`), `xargs`, `find -exec`, a path built from a variable, a symlink, or a `cd` into another directory. They match `git` and `rm` by name, so an alias or a wrapper script slips through. `ls .env` and `grep ... .env` are denied too: any argument that names a secrets file counts.
- File-tool checks need the hook matchers from `docs/hooks/install` (`Read`, `Write`, `StrReplace`, `Delete`, `Grep`, and Cursor `beforeReadFile`; for Claude Code `Read`, `Write`, `Edit`, `MultiEdit`, `NotebookEdit`, `Grep`). A patch-style edit tool that carries paths only inside its patch text is not read.
- The language, pushback, and memory checks are heuristics. They log; they do not block.
- The canary can be forged by reading `.drift-guard/canary-secret` and recomputing the code. A matching code means the file was in context, not that its rules were applied. See [`docs/canary/README.md`](docs/canary/README.md).

## More

Design: [runtime model](docs/adr/0005-runtime-model.md) and the other [ADRs](docs/adr/README.md). Hook latency: [`docs/bench.md`](docs/bench.md). Contributing: [CONTRIBUTING.md](CONTRIBUTING.md). If you are an AI agent asked to connect a repo, follow [CONNECT.md](CONNECT.md).

## License

[MIT](LICENSE)
