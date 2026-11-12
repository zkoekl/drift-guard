# Agents working in this repo

If you are an AI agent asked to connect this repo, follow [CONNECT.md](CONNECT.md).

Copy-in anti-drift rules for other projects: source [`docs/agent-behavior/RULES.md`](docs/agent-behavior/RULES.md), files to copy [`docs/install/`](docs/install/), how the checks relate [`docs/agent-behavior/README.md`](docs/agent-behavior/README.md).

The commands that exist are the ones in `drift-guard --help`; setup notes are in `docs/hooks/`. Do not add a model, a daemon, telemetry, or `serve`. This repo commits `.cursor/hooks.json` so a cloud agent that loads project hooks is checked. Do not remove that file to let a push through. The per-project snippet for other repos is `docs/hooks/install/`. The global opt-in kit is `docs/install/global/`. The runtime (no daemon, two channels, four layers) is `docs/adr/0005-runtime-model.md`. The latency numbers are `docs/bench.md`.

## Checks and proposals

- The default path does not call a model. Do not add one. Ambiguous signals are log lines in the HTML report.
- The coding agent does not verify its own work. A sentence that says "drift-guard passed" is not evidence. The hook or CI log is.
- Never edit the live rules file from a caught miss. Write a proposal under `proposals/` only. Do not commit, merge, or push it. The owner applies and pushes.
- A proposal that lowers fixture recall or adds false positives is not offered.
- Do not add telemetry, a hosted dashboard, or a long-running process. Metrics stay in local `events.jsonl`. The viewer is `drift-guard report`, which writes one HTML file and exits. Do not start `serve` from a hook.

## Token cost

- Do not inject long rules into an agent prompt as a way to "enforce" them.
- Injected rule text, when it exists, stays within the cap in `CONTRIBUTING.md`.
- A follow-up hook message stays short. Truncate rather than paste a diff.

## Repo

- Public docs are English.
- `plan.md` front matter is the contract. Do not require the checker to understand the prose body.
- Shell out to `git` for diffs. Do not vendor a git implementation.
- A drift finding is CLI exit `1`. The Cursor `stop` wrapper exits `0` and returns JSON. `hook` exits `0` and returns its decision as JSON: a permission deny, and on Claude Code `"decision": "block"` to continue a `Stop`. Exit `2` is a usage error. A push to main, master, the default branch, a force push, or a tag push is fail-closed in the per-project hook. A normal push of the current feature branch is allowed. The global opt-in kit does not set `failClosed`. Conversation heuristics start log-only, except, only when the rules file sets `require_markers: true`, a missing answer-marker line and a missing or stale canary code, which may send a short follow-up and still do not block. Disputed semantic findings, including a canary whose read log does not show the file, stay in the log and the report. A matching canary means the file was in context this session, not that its rules were applied.
- The dogfood canary file is `.drift-guard/canary.md` and is gitignored. Do not commit a rewritten `drift-guard-canary` line. The committed value in `AGENTS.md` stays `pending`.

drift-guard-canary: pending
