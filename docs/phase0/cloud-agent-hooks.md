# Do Cursor cloud agents run project hooks?

**NOT VERIFIED.** In this cloud-agent run, a `.cursor/hooks.json` added after the session had started did not run.

The product docs, checked 2026-11-09, say the opposite for a hook that is already in the repository when the run starts: cloud agents load command hooks from `.cursor/hooks.json` and run them, including `beforeShellExecution` and `afterShellExecution`. User-level `~/.cursor/hooks.json` is not loaded. Hooks also do not run during an early read-only turn. Source: [Cursor hooks](https://cursor.com/docs/agent/hooks), section "Cloud agent support".

This run did not test that startup path. The checkout at the start of the run had no `.cursor/hooks.json` (the prototype gitignores `/.cursor/hooks.json` and does not commit one). The probe below was written into the working tree mid-run.

## Probe

A hook script appends one line to `docs/phase0/cloud-hook-probe.log` and, for `beforeShellExecution`, prints `{"permission":"allow"}` so a real hook would not block the shell. `.cursor/hooks.json` registered it on `beforeShellExecution` and `afterShellExecution`, fail-open, timeout 10 seconds.

Commands after the file existed, and the result:

```text
$ echo "cloud-hook-probe-trigger-2026-11-09T18:38:38Z"
cloud-hook-probe-trigger-2026-11-09T18:38:38Z
LOG_ABSENT
```

A later command, which would have seen an `afterShellExecution` line from the command above if that hook had run:

```text
LOG_STILL_ABSENT
```

The same script, invoked by hand, did write the log and did print allow:

```text
$ printf '%s' '{"hook_event_name":"beforeShellExecution","command":"direct"}' | .cursor/hooks/cloud-probe.sh beforeShellExecution
{"permission":"allow"}

$ cat docs/phase0/cloud-hook-probe.log
2026-11-09T18:38:46Z event=beforeShellExecution bytes=61
```

No other hook trace showed up under `/tmp` or `~/.cursor` for this probe. The log line above is from the direct invocation only.

## What would count as verified

A cloud agent whose repository already contains `.cursor/hooks.json` at the start of the writable turn, and whose shell commands then append to the probe log without anyone running the script by hand. That was not this run.

The probe files were removed after the test. They are not part of the tree.

## Retest on 2026-11-11

The product page [Cursor hooks](https://cursor.com/docs/hooks), section "Cloud agent support", was fetched again on 2026-11-11. It says cloud agents run command hooks from a committed `.cursor/hooks.json`, and do not load `~/.cursor/hooks.json`. Hooks do not run during an early read-only turn. They start once the agent has a writable environment. Prompt hooks are not supported.

The same page lists these as supported: `beforeShellExecution`, `afterShellExecution`, `beforeReadFile`, `afterFileEdit`, `preToolUse`, `postToolUse`, `postToolUseFailure`, `subagentStart`, `subagentStop`, `beforeSubmitPrompt`, `preCompact`, `afterAgentResponse`, `afterAgentThought`, and `stop`. It lists `sessionStart` as not available on Cursor-managed cloud agents, because a session can still begin read-only and a late `sessionStart` would not be the start of the session. `sessionEnd` is also not available there.

This run repeated the mid-session probe. `.cursor/hooks.json` was written after the session had started, pointing at an allow-only script for `beforeShellExecution`, `afterShellExecution`, `preToolUse`, `postToolUse`, `sessionStart`, `beforeSubmitPrompt`, `afterAgentResponse`, and `stop`. Two later shell commands did not append to the probe log. The only line was from running the script by hand. The startup path is still not verified: this clone did not contain `.cursor/hooks.json` when the run began. The allow-only probe was removed and is not part of the tree.
