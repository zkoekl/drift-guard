# Connect this repository

You are an AI agent asked to connect a repo to yourself. Follow this file, then stop. A check is `enforced` only when a hook in that environment runs it. Otherwise write `advisory`, `not observed`, or `impossible`.

The URL in the request is the project to protect. Clone it if you are not already there. Install from this drift-guard checkout. When this file is already in that checkout, protect the project the user named, or this one if they named none.

## Detect

Use the first row that matches.

| Signal | Id |
| --- | --- |
| This session is a Cursor cloud agent | `cursor-cloud` |
| Cursor desktop | `cursor-ide` |
| `cursor-agent` or `agent` on `PATH`, including `--print` | `cursor-cli` |
| `claude` on `PATH`, or a Claude Code session | `claude-code` |
| A `SKILL.md` library, and no hook file you can set | `grok-bot` |
| You can read `AGENTS.md` and none of the above | `rules-only` |

## Binary

```sh
go install github.com/zkoekl/drift-guard/cmd/drift-guard@latest
drift-guard --version
```

If that module is not reachable, from this checkout: `go build -o "$HOME/.local/bin/drift-guard" ./cmd/drift-guard` and put that directory on `PATH`.

## Install

Rules and skills, for every environment that will read them: [`docs/install/README.md`](docs/install/README.md). That text is advisory.

- `cursor-ide`: every local project, `sh docs/install/global/install.sh` (no `failClosed`, idle until `#dg` or a rules file). One project: copy `docs/phase0/install/.cursor/hooks.json` to `.cursor/hooks.json`. Commit that copy only when the user asked the repo to carry it. The hook denies a push to main, master, or the default branch, a force push, and a tag push. A normal push of the current feature branch is allowed. `failClosed` still applies to that hook.
- `cursor-cli`: the same files. `--print` fires only `sessionStart`, `preToolUse`, `beforeShellExecution`, and `postToolUse`.
- `claude-code`: copy `docs/phase0/install/.claude/settings.json` to the project's `.claude/settings.json`. A cloud Claude session does not read `~/.claude`.
- `cursor-cloud`: commit the project `.cursor/hooks.json`. `~/.cursor/hooks.json` is not loaded, and `sessionStart` does not run. Commit it only when `drift-guard` is already on that environment's `PATH`. A missing binary with `failClosed` on `preToolUse`/`Shell` blocks every shell command, including the install. This repo is the exception: `.cursor/hooks/drift-guard.sh` builds `./cmd/drift-guard`. Do not copy that script into another module.
- `grok-bot`: hooks are impossible. Save `docs/install/skills/advisory-scan/SKILL.md` into the bot skill library.
- `rules-only`: the copy in `docs/install/README.md` is the whole install.

Language, tests, and the canary turn on through `drift-guard.rules.json`, as in the root `README.md`. This repo already has that file and the committed hook. Its canary file is `.drift-guard/canary.md`, which is gitignored, so a refresh does not appear in `git status`.

## Verify

```sh
printf '%s' '{"hook_event_name":"beforeShellExecution","command":"git push -u origin HEAD"}' | drift-guard hook
printf '%s' '{"hook_event_name":"beforeShellExecution","command":"git push origin HEAD:main"}' | drift-guard hook
printf '%s' '{"hook_event_name":"beforeShellExecution","command":"git push --force"}' | drift-guard hook
```

On a feature branch whose upstream is not protected, expect allow, then deny, then deny. On main, or when git cannot name the branch, the first is deny too. That proves the binary, not that the host runs hooks. Then run one shell command and look for that event in `.drift-guard/hook-dump.jsonl` (global: `~/.drift-guard/hook-dump.jsonl`). No new line means `not observed`.

## Report

| Check | Result |
| --- | --- |
| protected git push (main, default, force, tags) | |
| unrequested-delete ask | |
| command log | |
| stop follow-up (markers, canary, tests) | |
| prompt anchors and conversation heuristics | |
| rules, skills, AGENTS.md | advisory |
| `drift-guard scan` | advisory |

On `cursor-cli` `--print`, the protected git push deny, the delete ask, and the command log are enforced when those four events fire. A normal feature-branch push stays allowed. Stop follow-up, anchors, and conversation heuristics stay advisory. On `cursor-cloud`, `sessionStart` is impossible; other hook rows are enforced only if the dump shows the event. On `grok-bot` and `rules-only`, every hook row is impossible. Scan does not block.

## Disconnect

- Global Cursor: `sh docs/install/global/uninstall.sh`
- One project: delete the copied `.cursor/hooks.json` or `.claude/settings.json`
- Rules: remove the copied snippet, `CLAUDE.md`, `.cursor/rules/drift-guard.mdc`, and the copied skills
- Grok Bot: delete the saved skill from the library
- This repo's committed hook is the dogfood. Remove it only when asked to disconnect drift-guard itself
