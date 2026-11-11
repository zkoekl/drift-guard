# cursor-drift-guard

[![CI](https://github.com/zkoekl/drift-guard/actions/workflows/ci.yml/badge.svg)](https://github.com/zkoekl/drift-guard/actions/workflows/ci.yml)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](https://github.com/zkoekl/drift-guard/blob/main/LICENSE)

Tested with scripted agent sessions and live Cursor CLI and cloud-agent sessions.

If you are an AI agent asked to connect a repo, follow [CONNECT.md](CONNECT.md).

`drift-guard` is a local checker for an AI coding agent. It compares the diff and the session log with the plan and the rules that were written down. One Go binary, no daemon, no model, and no telemetry. Cursor hooks are the first adapter. Claude Code uses the same engine. A chat with no hooks is checked only when someone runs `drift-guard scan` on a saved transcript.

## Install

Requires Go 1.22.

```sh
go install github.com/zkoekl/drift-guard/cmd/drift-guard@latest
```

Global opt-in, for every local project, until a session turns the checks on. From a checkout of this repository:

```sh
sh docs/install/global/install.sh
```

That builds `~/.drift-guard/bin/drift-guard` and a wrapper, and writes `~/.cursor/hooks.json` when that file is empty or missing. If the file already has commands, the script stops and leaves a backup. Remove it with `sh docs/install/global/uninstall.sh`. Cloud agents do not load `~/.cursor/hooks.json`. Details: [`docs/install/global/README.md`](docs/install/global/README.md).

Per project, the checks run for that project. Put `drift-guard` on `PATH`, then copy the snippet. Commit it when a Cursor cloud agent should run it. This repo already commits its own `.cursor/hooks.json`. See [CONNECT.md](CONNECT.md).

```sh
go build -o "$HOME/.local/bin/drift-guard" ./cmd/drift-guard
mkdir -p .cursor .claude
cp docs/phase0/install/.cursor/hooks.json .cursor/hooks.json
cp docs/phase0/install/.claude/settings.json .claude/settings.json
```

The Cursor snippet sets `failClosed: true` on `git push`. The Claude snippet sets `onFailure` to `block`. Walkthrough: [`docs/phase0/HOWTO.md`](docs/phase0/HOWTO.md).

Rule text for other projects, separate from the hooks, is [`docs/install/README.md`](docs/install/README.md).

## Example

```sh
echo '{"hook_event_name":"beforeShellExecution","command":"git push origin HEAD:main"}' | drift-guard hook
```

The hook prints `"permission":"deny"`. A normal `git push -u origin HEAD` from a feature branch is allowed. A push to main, master, or the default branch, a force push, and a tag push are denied.

## Turn the checks on

The global install answers "allow" and does nothing else until the session is on. Turn it on in either of these ways:

- Send `#dg` as its own word in the chat. Letter case does not matter. The wrapper's `DRIFT_GUARD_TAG` changes the tag.
- Put `drift-guard.rules.json` in the project root.

A per-project hook file is already on for that project. The same rules file names the reply language, the test command, and the canary files. Without it, `git push` is still denied and the language and test checks stay quiet.

```json
{
  "reply_language": "en",
  "unrequested_delete": "ask",
  "test_command": "go test ./...",
  "test_path_globs": ["**/*.go"],
  "canary": { "files": ["AGENTS.md"] }
}
```

`unrequested_delete` is `ask`, `log`, or `off`. The default in an active session is `ask`. `ask` returns permission `ask` before a delete or replace the latest prompt did not request, and it does not deny the call. `log` records the event and allows the call. `off` skips the check. When the session never received the prompt text, the event is disputed. The field list is in [`docs/METRICS.md`](docs/METRICS.md).

`DRIFT_GUARD_RULES` points at another path. An example with the optional lists is [`docs/semantic/rules.example.json`](docs/semantic/rules.example.json).

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

`hook` reads stdin JSON and writes stdout JSON. Exit `0` is clean, exit `1` is a violation, exit `2` is a usage error. The Cursor `stop` wrapper exits `0` and returns JSON. `scan` exits `0` and writes findings to the log. `report --no-open` writes one HTML file and exits. `report --summary` prints a short count-only text summary with no paths, commands, prompts, file names, or session ids, and writes no HTML. `scope` is the pre-commit and pull-request backstop: a changed path that matches no glob fails. `canary refresh` rewrites `drift-guard-canary:` in the configured files and does not print the code.

Scope for a pull request is a block in the body. CI passes `--require-scope` and fails when the block is missing or a changed file matches none of the globs. See [`.github/PULL_REQUEST_TEMPLATE.md`](.github/PULL_REQUEST_TEMPLATE.md).

## Limitations

Hooks are not verified in a live Cursor session. The local tests check the JSON the hook prints.

Semantic checks are weak. On a private case set, checks 2–4 together caught 3 of 24 drift cases with 0 of 12 false alarms.

The canary can be forged by reading `.drift-guard/canary-secret` and recomputing the code. Each session rewrites `drift-guard-canary:` in the configured files, and those files are often tracked. A matching code means the file was in context, not that its rules were applied. See [`docs/canary/README.md`](docs/canary/README.md).

Push deny is not fail-closed in the global install. The global `hooks.json` does not set `failClosed`, so a dead or slow hook does not block `git push`. The per-project snippet does set `failClosed`. Whether a killed binary still blocks the push was not verified in Cursor.

The unrequested-delete check reads `rm`, `unlink`, `git rm`, `mv`, and `>` truncation. It skips globs, variables, `sudo`, `sh -c`, and deletes inside another program. A destructive verb allows only the files the latest prompt names. With no file named, the tool is allowed and the event is disputed. The skipped forms are listed in [`docs/METRICS.md`](docs/METRICS.md).

## More

Design: [runtime](docs/adr/0005-runtime-model.md), [plan](docs/PLAN.md), [validation](docs/VALIDATION.md), [local metrics](docs/METRICS.md). Contributing: [CONTRIBUTING.md](CONTRIBUTING.md).

## License

[MIT](https://github.com/zkoekl/drift-guard/blob/main/LICENSE)
