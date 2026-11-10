# Semantic checks

Four deterministic checks. No model. `drift-guard scan` exits 0. Disputed hits are `disputed: true` on the verdict in `events.jsonl` and a Disputed section in `drift-guard report`. They do not change the process exit. A missing or empty marker may return `followup_message` at `stop` (at most two, shared with the canary check, not fail-closed).

| Check | Rule id | What fires |
| --- | --- | --- |
| 1, missing marker | `answer-markers` | The final reply has no non-empty line for the goal, source, or proof marker. Default names are `ЦЕЛЬ`/`GOAL`, `ИСТОЧНИК`/`SOURCE`, `ДОКАЗАНО`/`PROOF`. |
| 1, citation | `marker-citation` | A source line names a file that was not read or edited, or a source/proof line names a command that is absent from the log or did not exit 0. |
| 2, anchors | `anchor-miss` | `beforeSubmitPrompt` (and, on scan, each user message) stores file names, CamelCase and snake_case identifiers, paths, numbers, and quoted strings. At the reply, the goal line, the changed paths, and the answer share fewer than `anchor_min_shared` of them (default 1). The minimum is capped by how many anchors the prompt had. A prompt with none does not fire. |
| 3, flip | `sycophancy` | A user message matches a pushback phrase (`ты не прав`, `а разве`, `you're wrong`, `are you sure`, and the rest of the built-in list). The next assistant answer opens with an agreement phrase (`вы правы`, `you're absolutely right`, `you are right`, ...) or negates the previous claim while still sharing two content words. No tool call, command, or file read sits between those answers. |
| 4, memory | `answered-from-memory` | The final reply names a path or file that is not in the read set, or it names a dotted version (`1.2.3`) and the session read nothing. |

Marker names, `anchor_min_shared`, and the phrase lists are optional fields in `drift-guard.rules.json`. A non-empty list replaces the built-in list. See `docs/semantic/rules.example.json`. Scan uses that file when `DRIFT_GUARD_RULES` or `./drift-guard.rules.json` is present.

On a private case set, checks 2–4 together caught 3 of 24 drift cases with 0 of 12 false alarms. The missing-marker check fired on 24 of 24 of those cases and 12 of 12 controls, because those replies did not contain the marker lines. Citation, flip, and the memory check each caught 0 of 24 with 0 of 12 false alarms.
