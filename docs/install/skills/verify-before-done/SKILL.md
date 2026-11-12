---
name: verify-before-done
description: Use before a reply says work is done, works, passed, was fixed, was restarted, or was already added, and before accepting names, authors, or files taken from a chat. Cite a command that exited 0 or a source opened this session.
---

# Verify before done

Use this with the drift-guard rules for a completion claim (rule 10), an extraction (rule 8), or a status line (rule 7). When `require_markers` is on in the rules file, the final reply has non-empty `ЦЕЛЬ:` / `GOAL:`, `ИСТОЧНИК:` / `SOURCE:`, and `ДОКАЗАНО:` / `PROOF:`.

## 1. Write the claim as one sentence

Name the object and the place: which process, which album, which test, which file. A sentence that fits two steps is not done yet. Split it.

## 2. Choose a check that fails when the claim is false

Restart, or "it is running". List processes and match the filename that is current now. Expect one process. Read a log line from that process. A pattern left over from an older filename fails the check.

A bulk add, or "the rest were already there". Take both numbers from tool output in this session: how many were added, and how many were already present. A gap you did not open item by item stays unverified.

A code or config edit, including a cleanup. Run the project's test command after the edit. Exit 0 is the proof.

An extracted transcript or file set. Open the source messages in this session. Check each saved name against the source, which file is which, and who sent each message. Two files with the same bytes are duplicates until you say so. A photo stays a photo until you have read text from it. A delivery tick is not an author.

A status the user will read. Name the step and the place with words that do not swap (not downloaded, not placed in the album, not indexed). A count names its unit.

A file fact or a version inside the claim. Read that file again this session before the proof command. Cite it as `path:line` on `ИСТОЧНИК:`.

## 3. Run the check

`ДОКАЗАНО:` is the command you ran, and only when the exit code is 0. The command string on that line is the one in the tool log, not a paraphrase.

`ИСТОЧНИК:` is `path:line` with no other words, or a command that exited 0. Extra words make a file cite look like a command.

## 4. When the check cannot be run

The status is blocked. Name the command that would decide the claim. Leave done, works, passed, fixed, restarted, and already-added out of the reply. Run some command that did exit 0 before you send a final reply, because an empty `ДОКАЗАНО:` line is a miss. That command is not a substitute for the check you still owe.

## 5. Lines to emit (only with `require_markers`)

```
ЦЕЛЬ: <user's goal, in their words>
ИСТОЧНИК: <path:line or a command that exited 0>
ДОКАЗАНО: <command that exited 0 and fails when the claim is false>
```

`GOAL:`, `SOURCE:`, and `PROOF:` are the same three lines. Add `ДОПУЩЕНИЕ:` / `ASSUMPTION:` only for a choice the user did not state. Add `ПРОЧИТАНО: <file> <code>` or `READ: <file> <code>` when the file you just read contains `drift-guard-canary: <code>`. Copy `<code>` from that line. The checker reads both forms.
