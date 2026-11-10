# Contributing

The prototype is the Go command in this repository, the hook snippets under [`docs/phase0/install/`](docs/phase0/install/) and [`docs/install/global/`](docs/install/global/), and the notes in [`docs/phase0/`](docs/phase0/). The checklist is in [`docs/PLAN.md`](docs/PLAN.md). Design changes belong in `docs/PLAN.md` or a new ADR under `docs/adr/`. Do not add a server, telemetry, or a model runtime.

## Code

- The command name is `drift-guard`.
- `drift-guard scope` is the backstop. `.githooks/pre-commit` runs it on staged paths and exits 0 with a warning when no scope is declared. CI passes `--require-scope`. Enable it locally with `git config core.hooksPath .githooks`.
- Tests are `go test ./...` and the fixtures described in [`docs/VALIDATION.md`](docs/VALIDATION.md). A change to rules or checks must not lower recall or add false positives on the existing fixture set.
- The tool never commits, merges, or pushes. Proposals stay files until the owner applies them.
- No network calls on the default path. No telemetry.
- Regenerate the installable rule copies with `python3 scripts/agent_rules.py --write` after editing `docs/agent-behavior/RULES.md`. CI runs `python3 scripts/agent_rules.py` and fails when a copy diverges.
- Agent-facing files stay within 80 lines and 1500 estimated tokens (`ceil(chars/4)`): root `AGENTS.md`, `docs/agent-behavior/RULES.md` and its `docs/install/` copies, and each skill under `docs/install/skills`. `python3 scripts/agent_rules.py` enforces it. The cap is the small end of public files measured on 2026-11-10 (Claude Code `CLAUDE.md` 40 lines / 555 tokens, Supabase `AGENTS.md` 77 lines / 1783 tokens).

## Pull requests

Use the pull-request template. The scope block is required. CI reads only the lines between `<!-- drift-guard-scope:start -->` and `<!-- drift-guard-scope:end -->`, and fails the job when a changed file matches none of those globs or the block is missing. Say what the change explicitly does not do.

## Conduct

[`CODE_OF_CONDUCT.md`](CODE_OF_CONDUCT.md). Security reports go through [`SECURITY.md`](SECURITY.md), not a public issue.
