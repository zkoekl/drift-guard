# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- Three deny checks that run beside the push guard on `beforeShellExecution`, `preToolUse`, `PreToolUse`, and now `beforeReadFile`. Destructive commands (`git reset --hard`, `git clean -f`, whole-tree `git checkout`/`restore`, `git stash drop`/`clear`, `rm -r` on the root, home, repo root, `.`, `..` or `*`, a download piped into a shell). Secrets (`.env`, `.env.*` except `.example`/`.sample`/`.template`, `~/.ssh`, `~/.aws`, `~/.config/gcloud`, `*.pem`, `*.key`, `id_rsa*`, `.npmrc`, `.netrc`) in shell commands and file tools. Guard the guard (`.cursor/hooks.json`, `.claude/settings*.json`, `drift-guard.rules.json`, `~/.cursor/hooks.json`, `~/.drift-guard/`) against edit, delete, and move. Rule ids `destructive-command`, `secret-access`, `guard-tamper`. In an inactive global session they allow, as before. `guard_allow` in the rules file lets listed paths through the last two.
- The hook templates and the global installer also match the file tools and `beforeReadFile`. The hook dump drops the `content` of a `beforeReadFile` payload.

### Changed

- README rewritten around what the tool does, with real example output. Title is `drift-guard`.
- Docs moved out of internal-sounding paths: `docs/phase0/install` to `docs/hooks/install`, `docs/phase0/HOWTO.md` to `docs/hooks/README.md`, `docs/phase0/bench.md` to `docs/bench.md`, `fixtures/phase0/*` to `fixtures/*`, `docs/semantic/RESULTS.md` to `docs/semantic/README.md`.
- `internal/drift/hook.go` split into `hook_*.go` files by topic. No behavior change.
- ADRs, `docs/METRICS.md`, and agent docs no longer refer to project phases.

### Removed

- `docs/PLAN.md`, `docs/VALIDATION.md`, `docs/phase0/STOP.md`, `docs/phase0/cloud-agent-hooks.md`, and `scripts/stop_experiment.sh` (working notes and a one-off experiment), plus the CI step that ran the script.

- `docs/agent-behavior/RULES.md` is marker-free: 17 lines, about 400 tokens, down from 27 lines and about 820 tokens. It no longer asks the agent for `ЦЕЛЬ:`/`ИСТОЧНИК:`/`ДОКАЗАНО:` lines or a canary quote.
- The missing-marker follow-up and the stop-time canary check are off by default. The rules file setting `"require_markers": true` turns both back on, and also canary refresh on hooks. The `check`/`scan` input accepts the same field.

## [0.0.1-alpha] - 2026-11-12

### Added

- A `v*` tag starts a CI release that attaches Linux and macOS (amd64, arm64) and Windows amd64 archives, plus `checksums.txt`. The tag is written into `drift-guard --version`.
- `drift-guard report --summary` prints a short count-only text summary of the event log (period, counts by kind and check, action counts, session count) and writes no HTML. Paths, commands, prompts, file names, and session ids are omitted so the text can be pasted from a machine that must not leak them.
- PostToolUse now reads Claude Code's `tool_response`. For Bash the documented shape is `stdout`, `stderr`, `interrupted`, and `isImage` (https://code.claude.com/docs/en/hooks). A finished, non-interrupted command counts as exit 0, so a passing test run is no longer missed. `tool_output` and `output`, including `exitCode`, still work.
- Canary lines are also refreshed on the first tool hook of a session when `sessionStart` or `beforeSubmitPrompt` did not run, as with Cursor cloud agents. A stop hook does not ask the agent to quote `pending`.
- In a Cursor cloud agent, an unrequested delete that would have returned permission `ask` is denied instead, because that environment has no user to answer the prompt. Detection uses `CURSOR_AGENT_WORKER_ID` or `CURSOR_WORKER_POOL_NAME`. The local CLI's `CURSOR_AGENT` does not match.
- `CONNECT.md` is the single procedure for an agent asked to connect a repo. It installs the binary, the strongest hook that environment runs, and the advisory rules, then asks for a table of enforced and advisory checks. This repo commits that project hook, a root `CLAUDE.md`, `.cursor/rules/drift-guard.mdc`, and `drift-guard.rules.json`. `docs/install/skills/advisory-scan/SKILL.md` is the Grok Bot skill. It is advisory. Grok Bot has no hooks.
- A normal push of the current feature branch is allowed. A push to main, master, or the default branch, a force push, or a tag push is denied. The dogfood canary file is `.drift-guard/canary.md`, which is gitignored, so a refresh does not rewrite `AGENTS.md`.
- Unrequested-delete check on `beforeShellExecution` and `preToolUse` in an active session. Deleting or replacing a file the latest prompt did not ask for returns permission `ask` (not deny), except in a Cursor cloud agent, where `ask` is denied, and appends an `unrequested-delete` event. `unrequested_delete` in `drift-guard.rules.json` is `ask` (default), `log`, or `off`. A destructive verb covers only the files that prompt names. With no file named, the tool is allowed and the event is disputed. When the prompt text is unknown, the finding is disputed and the hook still asks. A vague prompt is recorded and the finding says to ask. Temp directories and `bin/`, `dist/`, `node_modules/`, and `.git/` are ignored. No model.
- `drift-guard` commands: `hook`, `check`, `scan`, `report`, `scope`, and `canary refresh`. One binary, no daemon, no model, and no telemetry. `drift-guard --version` and `drift-guard -v` print `main.version` (default `dev`, set with `-ldflags "-X main.version=..."`).
- Module path `github.com/zkoekl/drift-guard`. Install with `go install github.com/zkoekl/drift-guard/cmd/drift-guard@latest`.
- Opt-in global Cursor hooks under `docs/install/global/`. Until a session is turned on, the hook allows the command and does not deny `git push`.
- Installable anti-drift rules for coding agents (`docs/agent-behavior/RULES.md`, copies under `docs/install/`, three on-demand skills). `ДОПУЩЕНИЕ` / `ASSUMPTION` is an optional marker. The canary line is checked: the reply quotes it as `ПРОЧИТАНО:` or `READ:`.
- Local metrics doc and ADRs.
- Deterministic semantic checks on scan and stop: answer markers, request anchors, flip after pushback, and a path or version stated without a read. Disputed hits stay in the log. A missing marker may follow up. No model.
- Deterministic canary check. `canary.files` lists rule files. `drift-guard canary refresh`, also run from `sessionStart` and `beforeSubmitPrompt`, rewrites `drift-guard-canary:` with a per-session HMAC and does not print the code. The reply quotes it as `ПРОЧИТАНО:` or `READ:`. A missing or stale code may follow up, using the same cap of two. A matching code with no read in the session log is disputed. A match means the file was in context this session, not that its rules were applied. See `docs/canary/README.md`.
- The runtime model is a single fast binary, two channels (code and conversation), and four layers. No daemon. The default checks use no model. The latency measurement is `docs/bench.md`. The prototype stays on Go.
- Repository health files: license, contributing guide, code of conduct, security policy, issue templates.

[Unreleased]: https://github.com/zkoekl/drift-guard/compare/v0.0.1-alpha...HEAD
[0.0.1-alpha]: https://github.com/zkoekl/drift-guard/releases/tag/v0.0.1-alpha
