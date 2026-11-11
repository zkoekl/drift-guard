# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- `CONNECT.md` is the single procedure for an agent asked to connect a repo. It installs the binary, the strongest hook that environment runs, and the advisory rules, then asks for a table of enforced and advisory checks. This repo commits that project hook, a root `CLAUDE.md`, `.cursor/rules/drift-guard.mdc`, and `drift-guard.rules.json`. `docs/install/skills/advisory-scan/SKILL.md` is the Grok Bot skill. It is advisory. Grok Bot has no hooks.

- Unrequested-delete check on `beforeShellExecution` and `preToolUse` in an active session. Deleting or replacing a file the latest prompt did not ask for returns permission `ask` (not deny) and appends an `unrequested-delete` event. `unrequested_delete` in `drift-guard.rules.json` is `ask` (default), `log`, or `off`. A destructive verb covers only the files that prompt names. With no file named, the tool is allowed and the event is disputed. When the prompt text is unknown, the finding is disputed and the hook still asks. A vague prompt is recorded and the finding says to ask. Temp directories and `bin/`, `dist/`, `node_modules/`, and `.git/` are ignored. No model.

## [0.0.1-alpha] - 2026-11-10

### Added

- `drift-guard` commands: `hook`, `check`, `scan`, `report`, `scope`, and `canary refresh`. One binary, no daemon, no model, and no telemetry. `drift-guard --version` and `drift-guard -v` print `main.version` (default `dev`, set with `-ldflags "-X main.version=..."`).
- Module path `github.com/zkoekl/drift-guard`. Install with `go install github.com/zkoekl/drift-guard/cmd/drift-guard@latest`.
- Opt-in global Cursor hooks under `docs/install/global/`. Until a session is turned on, the hook allows the command and does not deny `git push`.
- Installable anti-drift rules for coding agents (`docs/agent-behavior/RULES.md`, copies under `docs/install/`, three on-demand skills). `ДОПУЩЕНИЕ` / `ASSUMPTION` is an optional marker. The canary line is checked: the reply quotes it as `ПРОЧИТАНО:` or `READ:`.
- Implementation plan, validation protocol, local metrics, and ADRs. The private case set is not published in this tree.
- Deterministic semantic checks on scan and stop: answer markers, request anchors, flip after pushback, and a path or version stated without a read. Disputed hits stay in the log. A missing marker may follow up. No model.
- Deterministic canary check. `canary.files` lists rule files. `drift-guard canary refresh`, also run from `sessionStart` and `beforeSubmitPrompt`, rewrites `drift-guard-canary:` with a per-session HMAC and does not print the code. The reply quotes it as `ПРОЧИТАНО:` or `READ:`. A missing or stale code may follow up, using the same cap of two. A matching code with no read in the session log is disputed. A match means the file was in context this session, not that its rules were applied. See `docs/canary/README.md`.
- The runtime model is a single fast binary, two channels (code and conversation), and four layers. No daemon. Phases 0–2 use no model. The latency measurement is `docs/phase0/bench.md`. The prototype stays on Go.
- Repository health files: license, contributing guide, code of conduct, security policy, issue templates.

[Unreleased]: https://github.com/zkoekl/drift-guard/compare/v0.0.1-alpha...HEAD
[0.0.1-alpha]: https://github.com/zkoekl/drift-guard/releases/tag/v0.0.1-alpha
