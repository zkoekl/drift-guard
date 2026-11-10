# Implementation plan

The prototype in this repository is the `drift-guard` command (`go.mod`, `cmd/drift-guard`) and these docs. The runtime is [ADR 0005](adr/0005-runtime-model.md).

No daemon. One binary, `drift-guard`. Go or GraalVM native-image, picked by a latency benchmark, p95 under 50ms. A plain JVM is about 0.7s to start and does not ship.

## Channels

**Code.** `plan.md` front matter against `git diff`, plus the command log.

- `scope` globs. A changed file that matches none fails. Omitted scope skips the check and warns. `--require-scope` makes that exit 1. CI passes the flag.
- `deny` globs.
- `checklist[].must_touch`.
- `max_diff_lines`, `branch_pattern`.
- Required test command after a matching path change. "Done" with no successful run is a violation.

The prose under the front matter is ignored.

**Conversation.** Needs the assistant text and the tool log for this turn.

- Reply language when a rule names it.
- done / verified / works / passed / fixed / restarted, with no citation that resolves to the command log or the diff.
- A statement about a file that was not read in this turn.
- `constraints.yaml` entries (path absent, command exit, branch pattern). This file is for the checker. It is not pasted into the prompt.
- Goal substitution only when the plan has a structured `goal` field the diff or the reply contradicts by a listed pattern. Free-text intent is not guessed.
- Flip after pushback, as a heuristic only: the conclusion changed after a user objection, and no tool call since that objection returned new evidence (a read, a shell command, or a test whose output was not already in the log). The private notes have no such incident. Do not add one. A synthetic fixture checks that this pattern is logged. It is not a model judgment.
- Answer markers on the final reply (`ЦЕЛЬ` / `GOAL`, `ИСТОЧНИК` / `SOURCE`, `ДОКАЗАНО` / `PROOF`, names configurable). A missing or empty line may send a short follow-up at `stop` (capped at two, not fail-closed). A source file that was not opened, or a proof command that was not run with exit 0, is disputed and stays in the log. `ДОПУЩЕНИЕ` / `ASSUMPTION` is accepted and is not required; a missing assumption line is not a follow-up.
- Request anchors stored at `beforeSubmitPrompt` (file names, code identifiers, paths, numbers, quoted strings). Too little overlap with the goal line, the changed files, and the answer is disputed.
- A path or version string in the reply that was not read in the session is disputed.
- Canary lines in configured rule files. `sessionStart` and `beforeSubmitPrompt` rewrite `drift-guard-canary:` with a per-session code and do not put that code in hook stdout. The reply quotes it as `ПРОЧИТАНО:` or `READ:`. A missing or stale code may follow up, under the same cap of two. A matching code with no read in the session log is disputed and stays in the log. A match means the file was in context this session, not that its rules were applied. See `docs/canary/README.md`.

Clear conversation heuristics start **log-only**. They may send `followup_message` only after the promotion rule in [VALIDATION.md](VALIDATION.md). They never use `failClosed`. Ambiguous signals (possible sycophancy, novelty, tone, an unclear goal) are logged and listed in the HTML report. The owner reviews them. Phases 0–2 do not call a model to classify them.

## Layers

**Layer 0.** One short rule file the agent can see (at most 80 lines and 1500 estimated tokens, ceil(chars/4)). Advisory. On one published coding-agent setup, written policies were followed 64.2–78.0% of the time depending on wording ([arXiv:2611.04367](https://arxiv.org/abs/2611.04367)). Do not add rules to close that gap. The checker does.

**Layer 1, primary.** Hooks. Same engine, two adapters.

Cursor (checked 2026-11-09, [docs](https://cursor.com/docs/agent/hooks)):

- `beforeShellExecution` and `preToolUse`: deny. `git push` is denied here, with `failClosed: true`, so a crash does not let the push through. The global opt-in kit (`docs/install/global/`) does not set `failClosed`. A dead hook there does not block the push. The per-project snippet does.
- `postToolUse`: append the command, exit code, and a short output excerpt to `events.jsonl`. May return a short `additional_context`. Do not store `user_email` or file bodies.
- `afterAgentResponse`: conversation checks. Log-only until promoted.
- `stop`: on a clear code violation, return `followup_message` and exit 0. The wrapper's own exit is 0 even when the check exits 1. Cursor uses exit 2 as a block on permission hooks; do not use it for a drift finding. Set `loop_limit` to 3 (the product default is 5). The wrapper sends at most two follow-ups, then only logs.
- `afterFileEdit` records a path and a time for time-to-detect. It has no blocking output in the docs.
- `transcript_path` is on the common payload. Treat it as possibly behind the latest turn.

Claude Code (checked 2026-11-09, [docs](https://code.claude.com/docs/en/hooks)):

- `PreToolUse` denies `git push` (exit 2 or `permissionDecision: deny`). Set `onFailure: "block"` so a crash denies the call. There is no `failClosed` field; this is the equivalent.
- `PostToolUse` feeds the command log. It cannot undo the tool.
- `Stop`: exit 2, or a block decision, continues the conversation. Cap that the same way (two continuations). Grade `last_assistant_message`, not `transcript_path`, because the transcript file can lag.
- Project file is `.claude/settings.json`. User-level `~/.claude/settings.json` is not loaded in cloud sessions. Cursor cloud agents likewise ignore `~/.cursor/hooks.json`.

A weak or ambiguous signal is a log line with `mode: log` and a short reason. The report renders those rows in their own section. Nothing is sent to a model ([ADR 0003](adr/0003-independent-verification.md)).

**Layer 2.** The code channel only, in pre-commit and a GitHub Action. Same exit codes: 0 clean, 1 violation, 2 usage error. No transcript, so no conversation checks. No model.

**Layer 3.** `drift-guard scan <transcript>` when a person asks. Log-only. This is how a chat bot with no hooks is checked. It does not watch a folder.

`drift-guard propose` writes `proposals/*.md` and replays fixtures. It writes the offer only if recall on the previous fixture set does not fall and false positives do not rise. `drift-guard apply` needs a TTY and the owner to type the id. The tool never commits or pushes ([ADR 0002](adr/0002-permission-gated-proposals.md)). The replay is the deterministic suite. It does not ask a model.

`drift-guard report` reads `events.jsonl` (or a read-only SQLite file with table `events(json text not null)`) and writes one HTML file. `drift-guard serve` is not in the prototype and is not the default ([ADR 0004](adr/0004-metrics-runtime.md)).

## Phase 0

The private case set is not published in this tree. This file is the plan. The prototype below is what the repository contains.

**Token cost.** Zero for the plan. The prototype does not link a model runtime.

## Phase 0 prototype

The Go command and the hook snippets in `docs/phase0/install/` and `docs/install/global/`. Acceptance is the checklist. Do not commit an active project hook: cloud agents run `.cursor/hooks.json`. The root hook file stays gitignored.

Checklist:

1. **Dump-hook.** Log raw stdin for `beforeShellExecution`, `preToolUse`, `postToolUse`, `afterAgentResponse`, and `stop`, with `user_email` removed. Record whether `transcript_path` at stop contains the turn that just finished. One real Cursor session is enough.
2. **Latency.** Same check, three builds: Go, plain JVM, GraalVM native-image. Report p50 and p95, cold and warm. Ship only a build whose p95 is under 50ms. Plain JVM is expected to fail. Write the numbers into ADR 0005 and ADR 0001.
3. **Stop loop.** A planted violation produces one follow-up, a second if it is still there, and no third. `loop_limit` is 3.
4. **Block `git push`.** The command does not run. Kill the hook binary and confirm the push still does not run (`failClosed` / `onFailure: block`).
5. **Two checks.** Language and done-without-tests, as described in [VALIDATION.md](VALIDATION.md). Output is a pass/fail line per check. Conversation hits may be log-only. Code hits must exit 1.
6. **Scan.** `drift-guard scan` on 2–3 saved transcripts (redacted or synthetic). Exit 0. Findings only in `events.jsonl`.
7. **Report.** `drift-guard report --no-open` on that log writes HTML with the five charts. No network URL in the file.
8. **Backstop.** A pre-commit hook and a GitHub Action fail a pull request that touches a path outside `scope`.
9. **Ambiguous row.** A synthetic flip (conclusion changed, no new tool evidence) produces a log line and appears in the report. The process exits 0. No model is called.

**Token cost.** Zero. The prototype does not link a model runtime.

**Not in the prototype.** A daemon. Telemetry. `serve`. A model, Ollama, or `probe`. Applying a proposal. Blocking on a conversation heuristic. Committing captured transcripts.

## Phase 1 — layers 1 and 2

The prototype's binary, cleaned up, with both adapters and the code channel blocking. Conversation heuristics stay log-only. `events.jsonl` and `report` ship. `serve` does not.

**Acceptance.** Phase 1 fixture bar in VALIDATION (recall ≥ 0.80 on in-scope code fixtures, zero false positives on shipped negatives). Push deny still fails closed. The report lists ambiguous log rows. A session records no model call. Follow-up text stays ≤ 500 estimated tokens. Report acceptance from the prototype still holds.

**Token cost.** Zero. Layer 0 file stays under its cap. No model binary is required to build, install, or run.

**Not in this phase.** Promoting a conversation rule. Any judge. Reading the plan prose.

## Phase 2 — tighten, constraints, proposals

Promote a conversation rule from log-only to follow-up only under the validation rule. `constraints.yaml` violations that are path or exit-code checks block. `propose` / `apply` exist. The success-claim matcher blocks only if its snippet-set precision is ≥ 0.90; otherwise it stays a warning.

**Acceptance.** Phase 1 recall does not drop. A candidate that adds a false positive is not written to `proposals/`. Ambiguous rows are still log-only and still visible in the report. The test suite has no model dependency.

**Token cost.** Zero. Do not inject `constraints.yaml` or incident notes.

**Not in this phase.** Auto-apply. Any model. Blocking on flip or novelty. `scan` is already in this phase; it stays heuristic.

## Later, only if metrics say so — optional local judge

Not part of the MVP. Do not build it with phases 0–2. Start it only after the report shows the heuristics miss a class of drift the owner still has to catch by hand, often enough to justify the cost.

If that happens: an Ollama process, off unless the owner sets a daily budget, a different model from the coding agent, comments only on rows already logged as ambiguous, never changes an exit code, never starts from a hook. `drift-guard probe` (a manual "Are you sure?" on 10–20 cases) is part of this same optional phase, requires an explicit cost flag, and is not installed as a dependency of the default command. Installing `drift-guard` still does not install a model.

**Acceptance, if the phase is opened.** Budget unset or 0 means the judge code path is not compiled in, or it exits 0 immediately without spawning a process. A documented miss rate from the dogfood log is the reason the phase exists. Cloud inference is out. The coding agent's own model is out.
