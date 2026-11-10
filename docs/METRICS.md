# Metrics

All of these stay on the machine that ran the check. There is no account, no upload, and no telemetry. The viewer is a file. The runtime choice is [ADR 0004](adr/0004-metrics-runtime.md).

## Log

Append-only JSONL at `events.jsonl` in the working tree, or the path in `DRIFT_GUARD_LOG`. The tool writes one JSON object per line and flushes. It does not rewrite earlier lines. It does not write a database.

`drift-guard report` reads that JSONL. It also reads SQLite, read-only, if the path is a database with a single table `events(json text not null)`. The tool does not create that database. JSONL is the writer so the log stays an append-only file a person can delete.

Gitignore the log and the HTML report. They can contain session ids and paths. They must not contain prompts, file bodies, hook `user_email`, or secrets. Command stdout stored in Phase 2 is capped (first and last 2 KiB).

Deleting the file is the retention policy.

### Event kinds

Common fields: `ts` (RFC3339 UTC), `kind`, `session_id`.

**verdict** — one per `drift-guard check` or stop-hook run.

| Field | Meaning |
| --- | --- |
| `source` | `cli`, `hook-stop`, `hook-edit`, `ci`, `scan`, or `probe` |
| `channel` | `code` or `conversation` |
| `mode` | `block` or `log` |
| `violations` | Count. |
| `rule_ids` | Ids that fired. |
| `elapsed_ms` | Checker runtime. |
| `time_to_detect_ms` | From the first `hook-edit` note for a path that later failed, to this verdict. `null` if that note does not exist. Do not substitute "CI job duration". |
| `tokens_judge` | Always `0` in phases 0–2. No model runs. |
| `tokens_rules_injected` | `0` unless a rule file was injected. Estimated tokens = ceil(chars / 4). |
| `tokens_followup_estimate` | Same estimate for `followup_message`. `0` when there is none. |
| `disputed` | True when a semantic finding is flagged for review and does not block. Omitted when false. |
| `disputed_ids` | Rule ids of those findings. |

**label** — appended by `drift-guard label`. `false_alarm` (bool), `found_by` (`tool` or `user`), `session_id` of the verdict. The report joins labels onto verdicts. Unlabeled verdicts are "unlabeled", not "true catches".

**fixture_run** — `precision`, `recall`, `tp`, `fp`, `fn`, `in_scope` (phase name). Precision is omitted, not zero, when `tp + fp = 0`.

**proposal_offered** / **proposal_rejected_by_suite** — `proposal_id`, and for a rejection the recall delta and the false-positive delta. No rule body in the log.

**rules_changed** — `lines_added`, `lines_removed`. Written by `drift-guard apply` and by `rules measure` when the user asks to record a change. This feeds rule churn.

**judge** — not written by phases 0–2. A later optional phase may append `tokens_in`, `tokens_out`, and `calls`. Until that phase is enabled, the report's token chart is the follow-up estimate only, and it is zero when no follow-up was sent.

## Definitions

**Drift caught per session.** Number of `verdict` events with `violations > 0` in that `session_id`. Also shown as a daily sum (catches over time). A session id is the Cursor `conversation_id` when the hook has one, otherwise a CI run id, otherwise a random id the CLI created for that invocation.

**False-alarm rate.** Labeled false alarms divided by labeled tool alerts. Unlabeled alerts are excluded, not treated as true. If there are no labeled alerts, the rate is absent. The fixture false-positive count is a separate number and is not mixed into this rate.

**Precision and recall on fixtures.** The latest `fixture_run` for each phase, plus the history of those runs. Formulas are in [VALIDATION.md](VALIDATION.md). Out-of-scope fixtures are not in the denominator.

**Time to detect.** `time_to_detect_ms` as defined above. The chart splits `hook-stop` from `ci` so a CI duration is not drawn as agent latency. Missing values are gaps, not zeros.

**User-caught vs tool-caught.** Among labeled confirmed drifts (`false_alarm: false`), the share whose `found_by` is `tool` versus `user`. Goal: move share toward `tool`. The case set is the baseline to beat (0 tool-caught out of 48, 31 user review), and it is drawn as a caption, not as live data. Live data is only the log.

**Rule churn.** Sum of `lines_added + lines_removed` on `rules_changed` events, by week, plus counts of proposals offered, rejected by the suite, and applied. High churn is a smell. The only automatic fail is the regression gate in VALIDATION, not a churn quota.

**Tokens per session.** Sum of `tokens_judge` plus `tokens_rules_injected` plus `tokens_followup_estimate` for that session. Deterministic checks contribute 0. Estimates are labeled as estimates on the chart. Report generation adds 0.

## Viewing them

```
drift-guard report
drift-guard report --no-open --output /tmp/drift-guard-report.html
```

The command reads the log, writes one HTML file, opens it in the browser unless `--no-open` is set or stdout is not a TTY, and exits. Idle cost is zero because nothing is running between invocations. Rendering does not call a model.

The page contains:

- catches over time
- false alarms over time
- precision and recall on fixtures
- user-caught vs tool-caught
- tokens per session
- a rule-churn table

Each chart has an empty state. False alarms stay empty until someone runs `drift-guard label`. That is intentional: the tool does not guess which of its alerts were wrong.

`drift-guard serve` is not part of this phase and is not the default. Comparison is in [ADR 0004](adr/0004-metrics-runtime.md).
