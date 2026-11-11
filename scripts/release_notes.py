#!/usr/bin/env python3
"""Print the CHANGELOG section for a version. Exit 1 if it is missing."""

from __future__ import annotations

import argparse
import re
import sys
from pathlib import Path


def section(text: str, version: str) -> str | None:
    ver = version.strip()
    if ver.startswith("v") and re.match(r"v\d", ver):
        ver = ver[1:]
    pat = re.compile(
        r"^## \[v?" + re.escape(ver) + r"\][^\n]*\n(.*?)(?=^## |\Z)",
        re.S | re.M,
    )
    m = pat.search(text)
    if not m:
        return None
    body = m.group(1)
    # Keep a Changelog reference links after the last section are not part of it.
    body = re.sub(r"\n(?:\[[^\]]+\]:\s+\S+\s*)+\Z", "", body).strip()
    return body or None


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("version")
    parser.add_argument(
        "--changelog",
        default=str(Path(__file__).resolve().parent.parent / "CHANGELOG.md"),
    )
    args = parser.parse_args()
    text = Path(args.changelog).read_text(encoding="utf-8")
    body = section(text, args.version)
    if body is None:
        return 1
    sys.stdout.write(body + "\n")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
