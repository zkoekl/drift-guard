# Agents working in this repo

Copy-in anti-drift rules for other projects: source [`docs/agent-behavior/RULES.md`](docs/agent-behavior/RULES.md), files to copy [`docs/install/`](docs/install/), how the checks relate [`docs/agent-behavior/README.md`](docs/agent-behavior/README.md).

This repository is the Phase 0 prototype. The commands that exist are the checklist in `docs/PLAN.md` (section "Phase 0 prototype") and the notes in `docs/phase0/`. Do not add a model, a daemon, telemetry, or `serve`. Do not commit an active project hook: cloud agents run `.cursor/hooks.json`. The per-project install snippet is `docs/phase0/install/`. The global opt-in kit is `docs/install/global/`. The root hook file is gitignored. The runtime (no daemon, two channels, four layers) is `docs/adr/0005-runtime-model.md`. The latency numbers are `docs/phase0/bench.md`.

## Checks and proposals

- Phases 0–2 do not call a model. Do not add one. Ambiguous signals are log lines in the HTML report. A local judge is a later phase, off by default, and only if metrics show the heuristics miss too much.
- The coding agent does not verify its own work. A sentence that says "drift-guard passed" is not evidence. The hook or CI log is.
- Never edit the live rules file from a caught miss. Write a proposal under `proposals/` only. Do not commit, merge, or push it. The owner applies and pushes.
- A proposal that lowers fixture recall or adds false positives is not offered.
- Do not add telemetry, a hosted dashboard, or a long-running process. Metrics stay in local `events.jsonl`. The viewer is `drift-guard report`, which writes one HTML file and exits. Do not start `serve` from a hook.

## Token cost

- Do not inject long rules into an agent prompt as a way to "enforce" them.
- Injected rule text, when it exists, stays under the cap in `docs/PLAN.md`.
- A follow-up hook message stays short. Truncate rather than paste a diff.

## Repo

- Public docs are English.
- `plan.md` front matter is the contract. Do not require the checker to understand the prose body.
- Shell out to `git` for diffs. Do not vendor a git implementation.
- A drift finding is CLI exit `1`. The Cursor `stop` wrapper exits `0` and returns JSON. Exit `2` is for a permission deny (and, on Claude Code, for continuing a `Stop`). `git push` is fail-closed in the per-project hook. The global opt-in kit does not set `failClosed`. Conversation heuristics start log-only, except a missing answer-marker line and a missing or stale canary code, which may send a short follow-up and still do not block. Disputed semantic findings, including a canary whose read log does not show the file, stay in the log and the report. A matching canary means the file was in context this session, not that its rules were applied.

drift-guard-canary: pending
