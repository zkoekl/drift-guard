---
name: check-ambiguity
description: Use before the first edit, the first remote create, or any other hard-to-undo step when the request leaves a choice unstated. Ask, or mark the choice with ДОПУЩЕНИЕ, and do the action the user named.
---

# Check ambiguity before starting

Use this with drift-guard rules 1, 2, and 6 before the first edit or any step that is hard to undo. The goal is a list of choices the request does not settle, each one either asked or written down.

## 1. Restate the goal

Write `ЦЕЛЬ:` / `GOAL:` in the user's words. A linter cap, a smaller first slice, ending the session, or a harsher tone does not replace that sentence. When the goal and a tool limit cannot both be met, stop and name the conflict. Do not drop the part of the goal the tool cannot express.

## 2. List choices the request leaves open

Walk the request once and write down anything the user did not set that would change the result. The list that has shown up in real misses:

- a voice, a name, or a label the user will hear or see
- an encoding, a transform, or dummy values added to an observation
- a signal at an episode boundary, or a fix that hides the frame the task is about
- a threshold or a world size chosen so a planner or a limit will fit
- controls or positions fixed so a rule does not have to transfer
- a repository or remote name
- a stack, extra systems, or a second tree the spec does not list

## 3. Ask, or mark one line per choice

For each item that changes the result, ask the user before you apply it. When you must proceed, add one line:

```
ДОПУЩЕНИЕ: <the choice, and what it changes>
```

`ASSUMPTION:` is the same line. Docs and code then state that choice the same way. A choice with no line and no question is a silent assumption.

Creating a repository, picking its name, or any other remote side effect waits for an explicit yes. A collision with a name the user already uses is theirs to catch only if you ask first.

## 4. Do the action that was named

When the user named an action, that action is the next step: open the app, use the key, read the window that is already open, edit in place. A different action is a substitution, and it waits.

When the request allows two methods, one failure is the cue to use the second method. The same command string is not a second method. After that same command exits non-zero twice, change approach or stop and report both exits.

## 5. Then start

A bound the user stated (explain only, no edits, no runs, a short content list) stays in the `ЦЕЛЬ:` line for the rest of the task. Scope globs and spec exclusions are the same kind of bound.
