# ADR 0005: Runtime model

- Status: accepted
- Date: 2026-11-09

## Context

Rules in the prompt are not a checker. On Claude Code with claude-opus-5, compliance with a written policy was 78.0% for prescriptive wording and 64.2% for conditional wording ([arXiv:2611.04367](https://arxiv.org/abs/2611.04367)). The missing fraction is the product. Extra prompt text is also paid on every turn.

Sycophancy is not theoretical. Assistants change a correct answer when the user pushes back, including admitting a mistake they did not make ([arXiv:2310.13548](https://arxiv.org/html/2310.13548)). A later eval on math and medical questions found sycophancy in 58.19% of rebuttal trials, of which 14.66% were regressive (the reply got worse), and the behavior persisted in 78.5% of cases ([arXiv:2502.08177](https://arxiv.org/abs/2502.08177)). The detector is specified for that reason.

Hooks were checked on 2026-11-09:

- Cursor: [cursor.com/docs/agent/hooks](https://cursor.com/docs/agent/hooks). `beforeShellExecution` and `preToolUse` can deny. `postToolUse` can add context. `afterAgentResponse` carries the assistant text. `stop` can return `followup_message`. `loop_limit` defaults to 5. `failClosed` defaults to false. `transcript_path` is on the common input. Project hooks are `.cursor/hooks.json`. Cloud agents do not load `~/.cursor/hooks.json`.
- Claude Code: [code.claude.com/docs/en/hooks](https://code.claude.com/docs/en/hooks). `PreToolUse` can deny (exit 2 or `permissionDecision`). `PostToolUse` can add context. `Stop` exit 2 continues the conversation instead of ending it. `transcript_path` can lag the current turn; the Stop payload's `last_assistant_message` is the text to grade. Project hooks are `.claude/settings.json`. A crash denies the tool call only when `onFailure` is `"block"`. Cloud sessions do not read `~/.claude/settings.json`.

## Decision

No daemon. One deterministic CLI, `drift-guard`, as a single fast-start binary. Language: Go or GraalVM native-image, chosen by a prototype benchmark ([bench.md](../bench.md)). p95 under 50ms on a cold start and a warm start. A plain JVM does not ship.

Measured on 2026-11-09 with `scripts/bench.py` (200 cold runs after dropping the page cache, 200 warm runs, nearest-rank percentiles). The minimal hook reads stdin JSON, evaluates git-push and done-without-tests, appends one JSONL line, and writes stdout JSON.

| Build | Warm p50 ms | Warm p95 ms | Cold p50 ms | Cold p95 ms |
| --- | --- | --- | --- | --- |
| Go | 1.100 | 1.833 | 3.662 | 7.331 |
| Plain JVM, Temurin 25.0.5 | 26.129 | 29.381 | 56.133 | 67.147 |
| GraalVM CE 25.0.5 native-image | 1.945 | 2.718 | 7.204 | 13.124 |

Go and the native image clear the bar. The plain JVM does not: cold p95 is 67.147 ms. The warm-p95 gap between Go and the native image is under 5 ms, so the run does not pick a winner. The prototype binary is Go. The shipping `drift-guard hook` on an allow was also under the bar (warm p95 1.795 ms, cold p95 6.718 ms). Summary: [bench.md](../bench.md). Raw samples are not published. The same table is in [ADR 0001](0001-cli-language.md).

Two channels, one engine:

- **Code.** Diff against `plan.md` scope, denied paths, and "done" without the required test command in the command log.
- **Conversation.** Reply language. A done/verified/works claim with no citation in the tool log. A claim about code with no read of that file in this turn. A `constraints.yaml` violation. Goal substitution when the plan names the goal in a structured field. Sycophancy flip: the conclusion changed after a user objection and no tool call since that objection produced new evidence.

Clear code violations and the push deny send a correction. Deterministic conversation heuristics (language, an uncited done/verified claim, a claim about a file this turn did not read, a `constraints.yaml` entry) start log-only and may send a follow-up once their false-alarm rate is low enough. Ambiguous signals, including possible sycophancy, are only logged. `drift-guard report` shows them to the owner. No model is involved.

The default binary does not call a model. Installing and using `drift-guard` does not require one. A local Ollama judge is a possible later addition, off by default, built only if the metrics show the heuristics miss too much. It is not part of the default binary's dependency set. If it is ever turned on, the judge uses a different model from the coding agent, has a daily token budget, and never blocks.

### Layers

| Layer | What | Blocks? |
| --- | --- | --- |
| 0 | One short rule file the agent can see. Advisory. | No. |
| 1 | Hooks. Primary. Cursor and, behind the same engine, Claude Code. | Clear violations: `followup_message` (Cursor) or Stop exit 2 (Claude), cap 2 follow-ups, `loop_limit` 3. Critical denies (`git push`): deny, and fail closed if the hook crashes. Weak signals: log only. |
| 2 | The code channel again, in a pre-commit hook and a GitHub Action. | Yes, for code violations. No conversation channel, because CI has no transcript. |
| 3 | `drift-guard scan` on a saved transcript, run by a person. | No. This is the only path for a chat bot that has no hooks. |

`drift-guard probe` is not built. It calls a model ("Are you sure?" on 10–20 cases). It belongs with the judge: off by default, and only if the owner enables it. It is not a hook and not CI.

`drift-guard propose` writes `proposals/*.md` and replays fixtures. `drift-guard apply` runs only after the owner types the id on a TTY. The owner pushes. See [ADR 0002](0002-permission-gated-proposals.md).

The agent does not verify itself. A claim is checked against the command log the hook recorded, not against the agent's summary. See [ADR 0003](0003-independent-verification.md).

Metrics: append-only `events.jsonl`, then `drift-guard report` (static HTML). `drift-guard serve` is optional, on demand, not a daemon. See [ADR 0004](0004-metrics-runtime.md).

## Consequences

- Layer 0 stays under the token cap in the plan. Growing it does not close the 64–78% gap.
- The first conversation heuristics will look quiet. That is the log-only start, not a missing feature.
- Possible sycophancy is a row in the HTML report, not a model call. The structural heuristic (conclusion changed, no new tool evidence) still logs one. It does not block.
- Two adapters (Cursor JSON, Claude JSON), no second engine.
- A checkout of this tool runs with git and the binary. No Ollama, no API key, no model download.

## Not chosen

- A resident process, a menu-bar app, or a hook that starts `serve`.
- Any model on the default path, including a "small" local one.
- Blocking on flip, novelty, or other ambiguous signals. Those stay in the report.
- Letting the coding agent's model be the judge.
- Turning the judge on because it is interesting. The only reason to build it is a measured miss rate the heuristics do not cover.
- Treating `probe` as CI.
