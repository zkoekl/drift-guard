---
name: handoff-constraints
description: Use when handing work to another agent or starting the next task. Restate constraints still in force, closed decisions, and rejected options with their reasons so the next agent does not drop them.
---

# Handoff constraints

Use this with drift-guard rule 12 before you spawn another agent, write a prompt for one, or start a task that continues earlier work. The next agent repeats the block below in its first reply, before new edits.

## 1. What the handoff carries

Write these as its own lines, in this order:

- `ЦЕЛЬ:` / `GOAL:` — the user goal still in force, in the user's words.
- Constraints still in force. Include paths the user asked to remove, a modularity rule (new work does not land as another flag on one type), a reply language, and a bound such as explain-only or no extra systems.
- Closed decisions. One line each: the decision, the reason, and the date or task where it was closed when you have that.
- Rejected options. One line each, with the reason they were rejected. A later agent that sees only the current files will otherwise reopen them.
- `ДОПУЩЕНИЕ:` / `ASSUMPTION:` for any choice that is still an assumption.

Facts about the machine or the repo are not a substitute for the decision. If the handoff has the facts and drops the decision, the constraint is already lost.

## 2. What the next agent does first

Repeat the block. Then compare the plan to it.

- A path the user asked to remove stays out of the diff, including when an older proposal of yours added it.
- A closed decision stays closed. Propose a change to it only after the user reopens it.
- New experiments follow the modularity constraint already stated. They do not accumulate as flags on the type the user already rejected.

## 3. Fresh reads

Read a constraint file again in this session before you quote it. Cite `path:line` on `ИСТОЧНИК:` / `SOURCE:`.

When that file contains a line `drift-guard-canary: <code>`, the handoff and the next reply include:

```
ПРОЧИТАНО: <file> <code>
```

`READ: <file> <code>` is the English form. The checker reads both. `<code>` is copied from the line you just read. Do not reuse a code from an earlier session.

## 4. Proof the handoff was applied

On the task that follows, `ДОКАЗАНО:` / `PROOF:` is a command that exited 0 and shows the constraint held. Examples: a diff that does not add the removed path, or the test command the project names. Saying the handoff was written is not that command.
