## Summary

<!-- What changed. -->

## Scope

<!-- Required. CI runs `drift-guard scope --require-scope` and reads only this block from the pull request body. One glob per line. The globs must cover every file this pull request adds or modifies. Deletions are not part of that check. A missing block fails the job. -->

<!-- drift-guard-scope:start -->
-
<!-- drift-guard-scope:end -->

## Out of scope

<!-- What this change deliberately does not do. -->

## Checks

- [ ] `go test ./...`
- [ ] `python3 scripts/agent_rules.py` (if `docs/agent-behavior/RULES.md` or a generated copy changed)
- [ ] No new personal data, secrets, or telemetry
- [ ] No model call on the default path

## Token cost

<!-- Zero, or the budget this change adds. "n/a" only for docs that do not add a runtime. -->
