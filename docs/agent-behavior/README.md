# Agent behavior rules

Short rules a coding agent can see, plus three procedures that are too long for every turn. This is Layer 0 from [`docs/PLAN.md`](../PLAN.md): advisory text. The checker is the enforcement. Incident notes are not part of the prompt.

The source file is [`RULES.md`](RULES.md). <!-- rules-budget: lines=27 tokens=821 --> The cap is 80 lines and 1500 estimated tokens (`ceil(chars/4)`), counted on that file and again on the Cursor copy with its front matter. `python3 scripts/agent_rules.py` prints the counts and fails when a generated copy diverges or the cap breaks.

## What to copy

Install steps: [`docs/install/README.md`](../install/README.md).

| File | Role |
| --- | --- |
| `docs/agent-behavior/RULES.md` | Source. Plain markdown. |
| `docs/install/AGENTS.md` | Same bytes, for a project's `AGENTS.md`. |
| `docs/install/CLAUDE.md` | Same bytes, for a project's `CLAUDE.md`. |
| `docs/install/.cursor/rules/drift-guard.mdc` | Same body, with `alwaysApply: true`. |
| `docs/install/skills/<name>/SKILL.md` | On demand. Copy to `.agents/skills/` and `.claude/skills/`. |

Do not replace this repository's root `AGENTS.md` with the snippet. That file is how agents work on cursor-drift-guard. The snippet is for other projects.

## Markers

The parser is `internal/drift/semantic.go`. Names are case-insensitive. A value is the text after the colon, trimmed. An empty value does not count.

| Marker | English alias | Required on the final reply | What the code does with it |
| --- | --- | --- | --- |
| `ЦЕЛЬ:` | `GOAL:` | Yes | `answer-markers` asks for a follow-up at `stop` when the line is missing or empty (at most two, not fail-closed). The goal line is part of the `anchor-miss` haystack. |
| `ИСТОЧНИК:` | `SOURCE:` | Yes | Same follow-up. `marker-citation` disputes a `path:line` that was not opened, or a command that is absent from the log or did not exit 0. The file shape is `path.ext` or `path.ext:line` with no other words. |
| `ДОКАЗАНО:` | `PROOF:` | Yes | Same follow-up. `marker-citation` disputes a command that is absent or did not exit 0. |
| `ДОПУЩЕНИЕ:` | `ASSUMPTION:` | No | Accepted, including from `markers.assumption` in the rules file. Not part of `answer-markers`. The text is not checked. |
| `ПРОЧИТАНО:` | `READ:` | No | The checker reads both forms. `canary-code` may follow up when the quote is missing or the code is from another session. `canary-read` disputes a matching code whose read log does not show the file. See [`docs/canary/README.md`](../canary/README.md). |

## Rules and checks

"Check" means a rule id or command that exists in this tree. "None" means the prose is the only control. A disputed finding is a log line. It does not change the process exit. The private case set is not in this tree. Counts from that set are one sentence in [`docs/semantic/RESULTS.md`](../semantic/RESULTS.md).

| Rule | What it is aimed at | Deterministic check |
| --- | --- | --- |
| 1. Silent assumption | A voice, encoding, default, signal, threshold, or world parameter the user did not state. | None. `ДОПУЩЕНИЕ` is stored as a marker name and is not judged. A denylist of a newly added file could catch that class of miss; this binary does not implement deny globs. |
| 2. Goal substitution | A linter cap, a smaller question, a session close, or a change of tone replacing the user's goal. | None for those substitutions. `anchor-miss` disputes a reply whose goal line, changed paths, and text share too few anchors with the prompt. A prompt with no anchors does not fire. A structured `goal` field on the plan is described in `docs/PLAN.md` and is not implemented. |
| 3. Goal drift | The next edit moves to infrastructure, process, or checklists. | `drift-guard scope` exits 1 when a changed path matches no scope glob. That catches a diff that lands outside the declared paths. It does not see process docs inside an allowed path, and it does not measure whether the goal got closer. `anchor-miss` is the same weak overlap check as for rule 2. |
| 4. Sycophancy | Overstated novelty, and unearned success. The private notes have no incident of dropping a correct claim under pushback. The rule still says to hold a claim until a new read or tool result, because that is the checkable form. | `sycophancy` logs a flip: a pushback phrase, then the next assistant answer agrees or reverses the previous claim, with no tool call, file read, or command between them. It does not score novelty. It did not fire on the private case transcripts. Unearned "done" is rule 10's checks, not this one. |
| 5. Known context | Advice or edits that ignore facts already in this session. | None. There is no store of user facts, and the checker does not read advice against that store. |
| 6. Repeated failure | The same approach retried after a failure, or a second allowed method never tried. | None. Nothing counts identical non-zero command strings. A UI failure would not be in a shell log. |
| 7. Status | A status that does not name the step, the place, or the unit of a count. | None. There is no status schema to compare the words to. |
| 8. Extraction | Names, authors, or which file is which, taken from something other than a source opened this session. | None for swapped names, a photo described as text, or an author inferred from a tick. This binary does not checksum duplicates. `marker-citation` only checks that a cited file was opened or a cited command exited 0, not that the extraction matches the source. |
| 9. Memory | A claim about a file, page, config, or version that was not read this session. | `answered-from-memory` disputes a path or file name in the reply that is not in the read set, and a dotted version (`1.2.3`) when the session read nothing. It does not compare a vendor UI or a worked example to the live source. The semantic measurement on the private case set did not fire it. |
| 10. Done | A claim that the work is done, works, passed, was fixed, was restarted, or was already added, without a command that exited 0. | `done-without-tests` blocks when the rules file names `test_command`, a changed path matches `test_path_globs` (or any change, when the glob list is empty), and the command log has no exit-0 command containing that string. That is the named-test case once the project names its test command. It is not a restart claim and not a bulk count. `marker-citation` disputes `ДОКАЗАНО` when that command did not exit 0. There is no matcher for the words done, works, restarted, or already-added. |
| 11. Scope | Paths, a stack, extra systems, or a second tree outside the plan, and a bound such as explain-only. | `drift-guard scope` exits 1 for a path outside the declared globs (a different tree, a file the plan does not list). It does not count extra systems, a different stack, or a debug control inside an allowed file. |
| 12. Handoff | A removal, a modularity rule, or a closed decision that does not survive the next task. | None. Re-adding a removed path would be a deny glob; deny globs are not implemented. A closed decision that exists only in the handoff text is not compared to the next reply. |
| 13. Language | The reply is not in the language the user requested or the language named by `reply_language`. | `reply-language`, log only, when `reply_language` is `ru` or `en`. Replies with fewer than 40 letters are skipped. For `ru`, a Cyrillic letter ratio under 0.4 fires. For `en`, a ratio over 0.4 fires. With no language set, the check stays off. |

## Skills

| Skill | Load | What the procedure is for | Check that grades the skill |
| --- | --- | --- | --- |
| `verify-before-done` | On demand | Done, status, and extraction claims, and the file or version half of rule 9 | The same partial checks as rules 7, 8, and 10. The skill does not add a checker. |
| `handoff-constraints` | On demand | Constraints, closed decisions, and rejected options passed to the next task | None for the handoff text. The canary line is checked: the reply quotes `ПРОЧИТАНО:` or `READ:`, and the checker reads both. |
| `check-ambiguity` | On demand | An unstated choice, a second method, or a named action that was not the one asked | `drift-guard scope` only when the other action is a path outside scope. No check sees an unstated voice, encoding, or remote name. |

Acting before a confirmation is not one of the thirteen always-on lines. The ambiguity skill carries it because the moment to catch it is before the first irreversible step.

`unrequested-delete` is a separate hook check, not one of those thirteen lines. In an active session, `beforeShellExecution` and `preToolUse` return permission `ask` when a delete or replace was not in the latest prompt. The rules field is `unrequested_delete` (`ask`, `log`, or `off`; default `ask`). The event is in `events.jsonl` and in the report. Prompt text is not copied into the log. See [`docs/METRICS.md`](../METRICS.md).
