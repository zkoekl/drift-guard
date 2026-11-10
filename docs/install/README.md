# Install the anti-drift rules

These files are for a project whose coding agents should follow the drift-guard behavior rules. They are not this repository's own agent instructions. Leave the root `AGENTS.md` in cursor-drift-guard as it is.

The text of the always-on rule has one source, [`docs/agent-behavior/RULES.md`](../agent-behavior/RULES.md). `AGENTS.md`, `CLAUDE.md`, and `.cursor/rules/drift-guard.mdc` in this directory are generated copies. Regenerate them with `python3 scripts/agent_rules.py --write`. CI runs `python3 scripts/agent_rules.py` and fails when a copy diverges.

## Copy into the other project

From a checkout of this repository:

```sh
mkdir -p .cursor/rules .agents/skills .claude/skills
cp docs/install/.cursor/rules/drift-guard.mdc .cursor/rules/drift-guard.mdc
cp -a docs/install/skills/. .agents/skills/
cp -a docs/install/skills/. .claude/skills/
```

`AGENTS.md` and `CLAUDE.md` here are the same markdown as the source. Paste that markdown into the project file, or copy the file when the project does not have one yet:

```sh
cp docs/install/AGENTS.md AGENTS.md
cp docs/install/CLAUDE.md CLAUDE.md
```

The Cursor file sets `alwaysApply: true`. The skills load when a task matches their description. Hooks stay a separate install: [`docs/phase0/install/`](../phase0/install/).

How the rules relate to the checks is [`docs/agent-behavior/README.md`](../agent-behavior/README.md).
