# ADR 0002: The tool may propose rule changes and may not apply them

- Status: accepted
- Date: 2026-11-09

## Context

New misses should become checks or fixtures. If the agent that just failed is also allowed to edit the rules, it can make the miss legal. The owner is the only person who pushes to this repository. A copy installed in another repo is even less trusted: that repo's agent must not merge a rules change because a local suite went green.

## Decision

1. `drift-guard propose` may write a proposal: a markdown file and a candidate rule snippet under `proposals/`, plus a fixture. It replays the fixture suite before writing.
2. Before the file is written, the fixture suite runs against the **previous** fixture set. Recall must not fall. False positives must not rise. The threshold is zero regression ([VALIDATION.md](../VALIDATION.md)).
3. If that fails, do not write a proposal and do not open a pull request. Append `proposal_rejected_by_suite` to the local log.
4. If it passes, the file is an offer, not a change to the live rules. The tool may `git add` only paths under `proposals/`, and only when the user passed `--stage`. It must not `git commit`, `git merge`, or `git push`. It has no remote helper.
5. `drift-guard apply <id>` edits the live rules only after the operator types that id on a TTY. It refuses a pipe, a non-TTY, and `CI`. The owner pushes the result themselves, from their own machine, as a normal commit.

Passing the suite is required to offer. It is not permission to apply.

## Consequences

- A cloud agent or a hook cannot land a rule change by calling the CLI.
- Rejected ideas are still visible in the local log and the HTML report, without a review queue of bad patches.
- The owner can ignore `proposals/` entirely. Nothing in the checker reads that directory as active rules.

## Alternatives

- **Auto-apply when the suite passes.** Rejected. The suite is small and will not encode every false alarm.
- **Open a pull request from the tool.** Rejected. That is a push. Only the owner pushes.
- **No proposals, the owner writes every rule.** Rejected as the only path. The point of the log is that a repeated miss should turn into a drafted fixture. The draft still waits.
