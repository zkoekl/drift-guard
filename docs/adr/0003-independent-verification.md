# ADR 0003: No agent verifies its own work

- Status: accepted
- Date: 2026-11-09

## Context

Several misses are an agent declaring success without a check, or describing a system from memory. A checker that trusts the agent's summary repeats the failure. A judge that sees the agent's rationale tends to agree with it. That is the sycophancy problem inside the tool.

## Decision

- The verdict is the process that CI or the hook starts. It is not a sentence the coding agent writes. "drift-guard passed" in chat is not evidence. A `verdict` line the hook appended is evidence. Claims of done, verified, or "I read the file" are checked against that command log. A read counts only if this turn's log has it.
- Phases 0–2 are deterministic. The inputs are the diff, exit codes, file existence, globs, line counts, branch names, a language id, the command log, and transcript text matched by heuristics. No model is installed or called. Ambiguous rows (possible sycophancy, novelty, an unclear goal) are written to `events.jsonl` and shown in the HTML report for the owner. They do not change the exit code.
- A local Ollama judge is not part of this decision's default. It is a later phase, off, and only worth building if those report rows show the heuristics miss too much. If it is ever enabled: a different model from the coding agent, context limited to the plan front matter, the diff, the command log, and the claim lines, never the coding agent's reasoning, never an exit-code change, never fail-closed. Citations must point at a hunk or a log line that was provided. The default install still has no model.
- A proposed rule change is not shown to the owner as a recommendation until the fixture regression bar holds ([ADR 0002](0002-permission-gated-proposals.md)). The suite is deterministic. It does not ask a model whether the proposal is good.

## Consequences

- Hooks should call `drift-guard hook stop`, not ask the agent to run the checker and paste the result.
- The log stores the checker's result. It does not store the agent's explanation of why the check is wrong.
- The contradiction check in phases 0–2 is a claim against a recorded fact or exit code. There is no model devil's advocate on that path.

## Alternatives

- **Let the coding agent run the checker and decide.** Rejected. That is self-verification.
- **One model call that both edits and grades.** Rejected. Same context, same incentive to confirm the edit.
- **Ship a judge in the MVP, off until someone sets a budget.** Rejected. The default install must not need a model. Ambiguous cases are a report section. A judge is a later phase, and only if the metrics say the heuristics are not enough.
