---
name: advisory-scan
description: Use when this agent has no drift-guard hook, including a Grok Bot skill library. Advisory only. Follow the connected repo's drift-guard rules and run drift-guard scan when a transcript exists.
---

# Advisory scan

This skill is advisory. It enforces nothing. Grok Bot has no user-configurable hooks. An agent that only reads this file is not checked.

## Rules

Read `docs/agent-behavior/RULES.md` in the connected repository and follow it. When the task matches, also read `verify-before-done`, `handoff-constraints`, and `check-ambiguity` under `docs/install/skills/`. If `RULES.md` is not in the workspace, say the rules are not loaded. Do not invent a substitute.

## What you can run

These commands do not block a later action. Cite one only after it exits 0.

- `drift-guard scan <transcript> --log <events.jsonl>` when a saved transcript exists.
- `drift-guard scope` when the repo declares a scope and you can see the diff.
- `drift-guard hook` only as a synthetic stdin check. Allow or deny on your own stdin does not mean a hook is installed.

A sentence that says the checks passed is not evidence. The command and the exit code are.

## Report

Say that every drift-guard row is advisory. Do not call a rule enforced.
