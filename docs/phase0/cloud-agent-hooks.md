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
