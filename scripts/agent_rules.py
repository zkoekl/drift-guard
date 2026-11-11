#!/usr/bin/env python3
"""Keep the installable anti-drift copies identical to the source file.

Source: docs/agent-behavior/RULES.md
Copies: docs/install/AGENTS.md, docs/install/CLAUDE.md,
        docs/install/.cursor/rules/drift-guard.mdc,
        root CLAUDE.md, .cursor/rules/drift-guard.mdc

The check also enforces the agent-file budget: at most 80 lines and
1500 estimated tokens, ceil(chars/4). That is the small end of public
files measured on 2026-11-10 (Claude Code CLAUDE.md 40 lines / 555
tokens, Supabase AGENTS.md 77 lines / 1783 tokens). It covers
docs/agent-behavior/RULES.md, the install copies, the dogfood copies,
root AGENTS.md, CONNECT.md, and each skill under docs/install/skills.

    python3 scripts/agent_rules.py          # compare, exit 1 on drift
    python3 scripts/agent_rules.py --write  # regenerate the copies
"""

import math
import re
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
SOURCE = ROOT / "docs" / "agent-behavior" / "RULES.md"
README = ROOT / "docs" / "agent-behavior" / "README.md"
INSTALL = ROOT / "docs" / "install"
AGENTS = INSTALL / "AGENTS.md"
CLAUDE = INSTALL / "CLAUDE.md"
MDC = INSTALL / ".cursor" / "rules" / "drift-guard.mdc"
PROJECT_MDC = ROOT / ".cursor" / "rules" / "drift-guard.mdc"
SKILLS = INSTALL / "skills"
MAX_LINES = 80
MAX_TOKENS = 1500
MAX_SKILL_LINES = 80
MAX_SKILL_TOKENS = 1500
ROOT_AGENTS = ROOT / "AGENTS.md"
ROOT_CLAUDE = ROOT / "CLAUDE.md"
CONNECT = ROOT / "CONNECT.md"

FRONT = """---
description: Anti-drift behavior for coding agents. Apply on every turn.
alwaysApply: true
---

"""

STAMP = re.compile(r"<!-- rules-budget: lines=\d+ tokens=\d+ -->")
SKILL_NAME = re.compile(r"^name:\s*(\S+)\s*$", re.M)
SKILL_DESC = re.compile(r"^description:\s*\S", re.M)


def tokens(text):
    return math.ceil(len(text) / 4)


def line_count(text):
    if text == "":
        return 0
    return text.count("\n") + (0 if text.endswith("\n") else 1)


def load_body():
    if not SOURCE.is_file():
        sys.exit(f"missing {SOURCE.relative_to(ROOT)}")
    body = SOURCE.read_text(encoding="utf-8")
    if not body.endswith("\n"):
        sys.exit("RULES.md must end with a newline")
    return body


def expected(body):
    return {
        AGENTS: body,
        CLAUDE: body,
        ROOT_CLAUDE: body,
        MDC: FRONT + body,
        PROJECT_MDC: FRONT + body,
    }


def budget_stamp(body):
    return f"<!-- rules-budget: lines={line_count(body)} tokens={tokens(body)} -->"


def check_budget(label, text):
    lines = line_count(text)
    est = tokens(text)
    if lines > MAX_LINES or est > MAX_TOKENS:
        sys.exit(f"{label}: {lines} lines, {est} tokens; cap is {MAX_LINES} lines and {MAX_TOKENS} tokens")
    return lines, est


def check_skills():
    found = sorted(SKILLS.glob("*/SKILL.md"))
    if not 2 <= len(found) <= 4:
        sys.exit(f"expected 2–4 skills under docs/install/skills, found {len(found)}")
    names = []
    for path in found:
        text = path.read_text(encoding="utf-8")
        lines, est = check_budget(path.relative_to(ROOT).as_posix(), text)
        if lines > MAX_SKILL_LINES or est > MAX_SKILL_TOKENS:
            sys.exit(
                f"{path.relative_to(ROOT)}: {lines} lines, {est} tokens; "
                f"cap is {MAX_SKILL_LINES} lines and {MAX_SKILL_TOKENS} tokens"
            )
        if not text.startswith("---\n"):
            sys.exit(f"{path.relative_to(ROOT)}: missing front matter")
        end = text.find("\n---\n", 4)
        if end < 0:
            sys.exit(f"{path.relative_to(ROOT)}: front matter does not close")
        head = text[4:end]
        name = SKILL_NAME.search(head)
        if not name:
            sys.exit(f"{path.relative_to(ROOT)}: front matter needs name")
        if name.group(1) != path.parent.name:
            sys.exit(f"{path.relative_to(ROOT)}: name {name.group(1)} != directory {path.parent.name}")
        if not SKILL_DESC.search(head):
            sys.exit(f"{path.relative_to(ROOT)}: front matter needs a description")
        if "alwaysApply" in head:
            sys.exit(f"{path.relative_to(ROOT)}: skills stay on demand")
        names.append(f"{name.group(1)} {lines} lines, {tokens(text)} tokens")
    return names


def check_pointer():
    if not ROOT_AGENTS.is_file():
        sys.exit("missing AGENTS.md")
    agents = ROOT_AGENTS.read_text(encoding="utf-8")
    lines, est = check_budget(ROOT_AGENTS.relative_to(ROOT).as_posix(), agents)
    print(f"{ROOT_AGENTS.relative_to(ROOT)}: {lines} lines, {est} tokens")
    if "docs/agent-behavior/RULES.md" not in agents or "docs/install/" not in agents:
        sys.exit("root AGENTS.md must point at docs/agent-behavior/RULES.md and docs/install/")
    if "CONNECT.md" not in agents:
        sys.exit("root AGENTS.md must point at CONNECT.md")


def check_connect():
    if not CONNECT.is_file():
        sys.exit("missing CONNECT.md")
    text = CONNECT.read_text(encoding="utf-8")
    lines, est = check_budget(CONNECT.relative_to(ROOT).as_posix(), text)
    print(f"{CONNECT.relative_to(ROOT)}: {lines} lines, {est} tokens")
    for needle in ("Disconnect", "advisory", "go install", "git push", "drift-guard scan"):
        if needle not in text:
            sys.exit(f"CONNECT.md must mention {needle}")


def check_readme(body):
    if not README.is_file():
        sys.exit("missing docs/agent-behavior/README.md")
    text = README.read_text(encoding="utf-8")
    stamp = budget_stamp(body)
    if stamp not in text:
        sys.exit(f"docs/agent-behavior/README.md must contain {stamp}")
    if not STAMP.search(text):
        sys.exit("README budget stamp missing")


def write_copies(body):
    files = expected(body)
    for path, text in files.items():
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(text, encoding="utf-8")
    readme = README.read_text(encoding="utf-8")
    stamp = budget_stamp(body)
    if STAMP.search(readme):
        readme = STAMP.sub(stamp, readme, count=1)
    elif stamp not in readme:
        sys.exit("README has no rules-budget stamp to update")
    README.write_text(readme, encoding="utf-8")


def main():
    write = "--write" in sys.argv[1:]
    body = load_body()
    if "alwaysApply" not in FRONT or "alwaysApply: true" not in FRONT:
        sys.exit("Cursor rule front matter must set alwaysApply: true")
    src_lines, src_tokens = check_budget(SOURCE.relative_to(ROOT).as_posix(), body)
    mdc_text = FRONT + body
    mdc_lines, mdc_tokens = check_budget(MDC.relative_to(ROOT).as_posix(), mdc_text)
    if write:
        write_copies(body)
    files = expected(body)
    for path, text in files.items():
        if not path.is_file():
            sys.exit(f"missing {path.relative_to(ROOT)}; run python3 scripts/agent_rules.py --write")
        got = path.read_text(encoding="utf-8")
        if got != text:
            sys.exit(f"{path.relative_to(ROOT)} diverges from {SOURCE.relative_to(ROOT)}")
    if not mdc_text.startswith(FRONT) or "alwaysApply: true" not in mdc_text.split("---", 2)[1]:
        sys.exit("drift-guard.mdc lost alwaysApply: true")
    skills = check_skills()
    check_pointer()
    check_connect()
    check_readme(body)
    print(f"{SOURCE.relative_to(ROOT)}: {src_lines} lines, {len(body)} chars, {src_tokens} tokens")
    print(f"{MDC.relative_to(ROOT)}: {mdc_lines} lines, {len(mdc_text)} chars, {mdc_tokens} tokens")
    print(f"{AGENTS.relative_to(ROOT)}: {line_count(files[AGENTS])} lines, {tokens(files[AGENTS])} tokens")
    print(f"{CLAUDE.relative_to(ROOT)}: {line_count(files[CLAUDE])} lines, {tokens(files[CLAUDE])} tokens")
    for row in skills:
        print(f"skill {row}")
    print("ok: install copies match the source")


if __name__ == "__main__":
    main()
