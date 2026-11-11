# Global drift-guard hooks

The hook is installed in `~/.cursor/hooks.json`, so it runs in every project. Until a session is turned on, it immediately answers "allow" or with empty JSON and does nothing else: it does not deny `git push`, does not write a dump, does not refresh the canary, and does not send a follow-up.

## How to turn it on

- Send `#dg` as its own word in the chat. Letter case does not matter. A different tag is set with `DRIFT_GUARD_TAG` in the wrapper.
- Or put `drift-guard.rules.json` in the project root.

The session stays on after that. The canary is rewritten only when that file contains `canary.files`. The canary secret lives in `~/.drift-guard/state`, not in the repository.

A short follow-up (markers, test, canary) is written in Russian when the rules or `DRIFT_GUARD_REPLY_LANGUAGE` is `ru`. Otherwise the text is English. You can add that variable to the wrapper yourself.

## Install and remove

From the root of this repository, with Go installed:

```sh
sh docs/install/global/install.sh
sh docs/install/global/uninstall.sh
```

The script builds `~/.drift-guard/bin/drift-guard` and the wrapper `~/.drift-guard/bin/drift-guard-hook`. Running it again overwrites nothing. If `~/.cursor/hooks.json` already exists and contains its own commands, the script copies it to `hooks.json.drift-guard-backup` and stops, without replacing the file. An empty hooks file is replaced, and the previous copy is kept. `uninstall.sh` removes only what the install put in place, and restores that copy. The log and the state directory are not deleted.

## Push deny

The global `hooks.json` does not set `failClosed`. If the binary is broken or does not answer in time, the command is not blocked. The protected-push deny runs only for a session that is on, and only while the hook process is alive. It blocks a push to main, master, or the default branch, a force push, and a tag push. A normal push of the current feature branch is allowed. A broken hook then does not stop agents in every project. For a project where the deny must be fail-closed, install the hook from `docs/phase0/install/`, not this kit.

## Statistics

Everything stays in the home directory, not in repositories:

- log: `~/.drift-guard/events.jsonl`
- state: `~/.drift-guard/state`
- stdin dump, only for sessions that are on: `~/.drift-guard/hook-dump.jsonl`

Report:

```sh
~/.drift-guard/bin/drift-guard report --log ~/.drift-guard/events.jsonl --no-open
```

The report includes the number of active and inactive sessions, and how each active session was turned on: `tag`, `project`, or `project+tag`.

## Skills and rules

Checked on 2026-11-10 against the Cursor documentation.

User skills are described as living in `~/.cursor/skills/` and `~/.agents/skills/` ([Agent Skills](https://cursor.com/docs/skills)). `install.sh` copies `docs/install/skills/` into `~/.cursor/skills/`. An agent can see them in any project. That copy does not turn the hook on. An existing skill directory is not overwritten.

User rule files are described in `~/.cursor/rules` ([Rules](https://cursor.com/help/customization/rules)). The script puts nothing there: the rule from `docs/install` is marked `alwaysApply` and would be injected into every chat.

Cloud agents do not read `~/.cursor/hooks.json`. This kit is for local Cursor.
