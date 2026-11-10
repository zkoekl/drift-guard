# Validation

The checker is real only if labeled fixtures fail and pass for the reasons they were built, and if a period of real use does not drown in false alarms. This file is the protocol. Phase 1's full fixture set is not in the tree yet. The prototype fixtures that exist are under `fixtures/phase0/`.

Two channels, matching [PLAN.md](PLAN.md). A code fixture is a tiny git repo and a `plan.md`. A conversation fixture adds a short transcript and a tool log. `expected.json` includes `channel` (`code` or `conversation`) and `mode` (`block` or `log`). A log-mode fixture fails the run if the checker exits non-zero, and fails if the expected finding is absent from `events.jsonl`.

## Prototype gate

Before Layer 1 is the default on a real repo, the Phase 0 prototype checklist in the plan is done and the notes are in ADR 0005 (latency numbers included). The two checks named for that gate:

| Check | Channel | Mode in the prototype |
| --- | --- | --- |
| Reply language, when a rule names it | conversation | log |
| A named test command never exited 0 after a matching path changed | code | block |

`drift-guard scan` is run on two or three transcripts: one where the reply language misses the rule, one where the reply claims a restart, and one synthetic flip (user objects, the conclusion changes, no tool call in between). The flip file is `origin: synthetic`. The files in this tree are under `fixtures/phase0/transcripts/`.

`drift-guard probe` is not in phases 0–2. It calls a model, so it is not part of the default tool. The prototype gate does not run it.

## Promoting a conversation check

A rule id leaves log-only mode only when all of these hold:

- At least 10 labeled alerts for that id.
- False-alarm rate on those labels ≤ 0.20. Unlabeled alerts do not count as true.
- Recall on the fixtures for that id does not drop.

Promotion means `followup_message` (or Claude Stop continue). It does not mean `failClosed`. It applies only to deterministic heuristics (language id, uncited success claim, missing file read, path and exit-code constraints). Flip, novelty, and free-text goal checks do not get this promotion. They stay `mode: log` and are a section of the HTML report. Phases 0–2 do not attach a model note to them.

A failing exit code is always a deterministic rule. A later local judge, if the metrics justify building one, still does not fail a job.

## Fixtures

A fixture is a tiny git repo, a `plan.md`, an optional rules file, an optional command log, and an expected verdict. It lives under `fixtures/<phase>/<id>/`. It is synthetic. It must not contain text from private notes.

Each fixture has `expected.json`:

```json
{
  "phase": "1",
  "origin": "synthetic",
  "in_scope": true,
  "verdict": "fail",
  "rule_ids": ["deny-path"]
}
```

`origin` is `synthetic` for a fixture written for the suite. `in_scope: false` means the category is one the phase does not claim to catch. Those fixtures are not in the recall denominator. A phase must not "pass" by ignoring them silently: the run prints them as out of scope.

### Phase 1 set

In-scope failures (the diff or branch check should fire):

| Fixture pattern |
| --- |
| Changed paths outside `scope`, including a different language tree than the plan names, and a debug marker the plan denies |
| Diff is build and CI while `scope` is the paths the plan named |
| A denied path reappears |
| A second top-level tree appears |
| Branch name misses `branch_pattern`, or the diff exceeds `max_diff_lines` |

In-scope passes:

| Fixture pattern |
| --- |
| Edit inside `scope`, nothing denied |
| No diff and no shell when the plan says the turn must not change files |
| Deviations file present is not required yet |

Also: empty plan exits `0` with a warning; `--require-scope` exits `1`.

Out of scope for Phase 1, present so nobody counts them as misses of this phase: a UI described from memory, overstated novelty, and tone. Expected verdict for the phase is `out_of_scope`, not `fail`.

### Phase 2 set

| Fixture pattern | Expected |
| --- | --- |
| Assistant text says the process restarted; log shows two processes | fail |
| Cleanup touches the watched paths; no successful test command in the log | fail |
| "Verified" cites a log line with exit 0 | pass |
| The word "fixed" appears only inside a file path | pass |
| Short run misses the written threshold; a later full-run command is present | fail |
| Same scout, no full-run command | pass |
| Report file is English while the rule requires another language | fail |
| Known export command on a read-only task | fail |

The success-claim matcher also has a snippet set of at least six labeled lines (unearned-success lines plus non-claims). It is not a blocking rule until precision on that set is at least 0.90. Until then, and until the promotion rule above is met, reply language and the success-claim rows are `mode: log`: the finding must be in `events.jsonl`, and the process must still exit 0.

### Ambiguous rows

Overstated novelty, and a synthetic flip (conclusion changed after an objection, no new tool evidence), are `mode: log`. Expected result: a report row, exit 0, no model process. The synthetic flip is `origin: synthetic`. The private notes have no such incident. These are not a later-phase model suite. A model suite is not part of phases 0–2.

## Metrics on fixtures

Definitions match [METRICS.md](METRICS.md).

- True positive: in-scope fixture with expected `fail`, and the checker fails on an expected `rule_id`.
- False positive: expected `pass` or a negative fixture, and the checker fails.
- False negative: expected `fail`, and the checker passes or fails only on an unexpected rule.
- Out-of-scope fixtures are neither.

Recall = TP / (TP + FN). Precision = TP / (TP + FP). If there are no predictions, precision is undefined, not 1.

Phase 1 bar: recall ≥ 0.80, and zero false positives on the shipped negatives. That zero is the false-positive threshold.

Phase 2 bar: the same, on the Phase 2 in-scope set, without lowering Phase 1 recall or adding a Phase 1 false positive.

There is no model bar in phases 0–2. The optional later judge is not started unless the dogfood log shows a repeated class of owner-caught misses that the heuristics did not flag. If that phase is opened, its notes stay non-blocking, and precision and recall on a labeled slice have to be measured before those notes are shown as findings. The exit code stays the deterministic result either way. The default install has no model, so this bar is not a release gate for 0.1.0.

On a private case set, checks 2–4 together caught 3 of 24 drift cases with 0 of 12 false alarms. That sentence is the published count. See [docs/semantic/RESULTS.md](semantic/RESULTS.md).

## Proposals must clear the suite before they are offered

A drafted rule is compared to the parent fixture set, not to a set the draft itself enlarged. New fixtures are reported beside the result and do not count toward recall. The offer is written only when:

- recall on the previous set does not decrease
- false positives on the previous set do not increase

The threshold is zero regression. There is no slack while the suite is small. A failed candidate is a log line, not a file under `proposals/`, and not a pull request. See [ADR 0002](adr/0002-permission-gated-proposals.md).

## Dogfood

After Phase 1 meets the fixture bar, the owner uses the hook and CI on real Cursor coding sessions for 14 days.

- Only sessions that have a `plan.md`.
- The log stays on disk (`dogfood/local/` or `events.jsonl`). It is gitignored. Do not commit transcripts.
- For each session the owner appends a label with `drift-guard label`: tool catch, false alarm, or a miss noticed later, and `found_by` `tool` or `user`.
- Review is `drift-guard report`. No spreadsheet export is required. No account.

During those two weeks, conversation rules stay log-only unless a rule id has already met the promotion rule above. The missing answer-marker follow-up is a separate stop hint: it is capped at two, it is not fail-closed, and it is not a promotion of flip or the other disputed rows. The report is how that split is read: code blocks on one chart, conversation log lines on another. Disputed semantic rows are their own section.

## When to tag 0.1.0

All of these:

- Phase 1 fixture bar holds, and the command's README matches what it actually flags.
- At least 10 real coding sessions are labeled in the log.
- At least one confirmed catch was flagged by the tool before the user.
- Labeled false alarms do not outnumber confirmed tool catches.
- `drift-guard report` on that log produces the charts with no network.
- Categories the checker does not cover are listed in the release notes.
- A session records no model call and 0 injected-rule tokens.
- The install and the CI job do not fetch a model. Ambiguous rows are visible in the report.
- No proposal has been applied except by the owner, on a TTY, after the suite passed.

Do not tag because the plan is interesting. Tag because the fixture bar and the two-week log agree.
