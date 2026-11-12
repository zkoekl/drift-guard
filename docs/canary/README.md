# Canary check

A canary proves that a required rule file was in context in **this** session. It does not prove that the agent applied the rules in that file.

The canary is off unless the rules file sets `"require_markers": true` and lists `canary.files`. The check is deterministic. It does not call a model, and it does not run a daemon.

## Approach

The agent is expected to see the code only by reading the file after a refresh.

`drift-guard canary refresh` rewrites one line in each configured file:

```text
drift-guard-canary: <code>
```

The same rewrite runs inside the hook on `sessionStart`, `SessionStart`, `beforeSubmitPrompt`, and `UserPromptSubmit`, and lazily on the first tool hook (`preToolUse`, `beforeShellExecution`, and the matching post events) when none of those session events has run yet. Cursor cloud agents do not fire `sessionStart`; the lazy path covers that. A `stop` hook does not refresh, because that would change the code after the reply. Those events return the usual allow or empty JSON. The code is not a field in that JSON, and it is not written to `events.jsonl` or the hook dump.

At `stop`, drift-guard recomputes the code. It does not read the canary line back out of the file. The file is only the delivery channel. The reply must contain, for each required file:

```text
ПРОЧИТАНО: <file> <code>
```

`READ:` is accepted as well, in any letter case. The code match is case-insensitive. A missing quote or a code from another session is `canary-code`. The stop hook returns `followup_message` (Claude Code: a block decision with the same text) asking for a reread. The message names the files and does not include the code. This uses the existing cap of two follow-ups for the whole session, shared with the answer-marker follow-up and the test-command follow-up. It does not use `failClosed`, and the hook process still exits 0.

If the code matches and the session read log does not show a read of that file, the finding is `canary-read`. It is disputed. It is stored in `events.jsonl` and shown in the report. It does not follow up and it does not block.

The read log is the one the prototype already keeps:

- `postToolUse` / `PostToolUse` for Read, `read_file`, `open`, and `view`, plus shell readers such as `cat` and `head`
- `reads` on a saved transcript, for `drift-guard scan`
- a `transcript_path` on `stop`, when that file exists and contains those read records

`afterFileEdit` records an edit time for time-to-detect. It is not counted as a read of the canary line. An edit of the file with a correct quote is still `canary-read`.

Scan does not refresh files and does not read the live rules file for this check. A transcript opts in with `canary_files` and `canary_secret`. The fixture is `fixtures/transcripts/canary-synthetic.json`. Its secret is a test vector, not a local secret.

## Code

The code is the first 8 hex characters of HMAC-SHA256. The key is the local secret. The message is the sanitized session id, a NUL byte, and the normalized project-relative path.

The session id is `conversation_id`, then `session_id`, the same order as the rest of the hook. Sanitizing keeps letters, digits, `-`, `_`, and `.`. A payload with neither id uses `unknown`, so every such payload shares a code.

The secret is created on first refresh and stored in `.drift-guard/canary-secret` (mode `0600`). That directory is gitignored. Override the path with `--secret-file` or by pointing `DRIFT_GUARD_STATE` somewhere else. The same secret is reused across sessions. The session id is what changes the code. The same session, path, and secret always produce the same code, so a later prompt in that session does not rewrite the line again.

```sh
drift-guard canary refresh --session "<conversation id>" --rules drift-guard.rules.json
```

The command prints `refreshed <path>` or `missing <path>`. It does not print the code. Exit 0 means every configured file was updated or already had this session's line. Exit 1 means a file was missing, outside the project root, or not writable. A missing file is not created. Exit 2 is a usage error. `canary off` means `canary.files` is empty.

## Config

In `drift-guard.rules.json`:

```json
{
  "canary": {
    "files": ["AGENTS.md", "skills/example/SKILL.md"]
  }
}
```

Paths are project-relative. A path that escapes the root is skipped. An empty list, or no `canary` object, turns the check off. In this repo the placeholder line is on `AGENTS.md`. `skills/example/SKILL.md` in the example is not a file here; put the same line in a real skill file before listing that path. The committed placeholder in a tracked file should stay:

```text
drift-guard-canary: pending
```

Refresh replaces that token for the session. Do not commit the session code. This tool does not restore `pending` and does not block `git commit` of the rewritten line. A protected `git push` stays denied by the existing hook.

The example field list is `docs/semantic/rules.example.json`. The install snippets are `docs/hooks/install/.cursor/hooks.json` and `docs/hooks/install/.claude/settings.json`. This repo commits `.cursor/hooks.json` for dogfood. Its configured canary file is `.drift-guard/canary.md`, which is gitignored, so a refresh does not rewrite `AGENTS.md`. Do not commit a session canary code over `pending`.

## Limits

These are limits of this design, not edge cases to ignore.

- A valid quote means the code for this session was in the reply. It does not mean the rules were followed, understood, or even read past that one line.
- The hook stdout does not contain the code. Anything else that can read the file can. Cursor may inject `AGENTS.md` into the prompt after refresh without a Read tool. The code check can then pass with no tool read, and the result is the disputed `canary-read` finding, not a follow-up.
- An agent that reads `.drift-guard/canary-secret` and recomputes the HMAC can quote a correct code without reading the rule file. The read log then disputes the finding unless the rule file was also read. The 8 hex characters are 32 bits. They are short so a reply can quote them. They are not a guess-resistance boundary.
- If no refresh has run in this session, the file may still say `pending`. The stop hook does not follow up and does not ask the agent to quote `pending`. The first tool hook refreshes the line so a later stop can check a real code.
- `stop` does not refresh. Refreshing there would change the code after the reply.
- The read log is only as complete as the hooks that fired. A read that never hit `postToolUse`, a shell reader, or the transcript is invisible. The code can still match. That case is disputed, not blocking.
- Scan of a saved transcript checks the quote and the transcript's own `reads`. It does not open the rule files on disk.
