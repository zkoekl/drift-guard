#!/usr/bin/env python3
"""Run the scope-backstop fixtures. Exit 1 when a fixture does not match."""

import os
import subprocess
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]


def run(args, env=None):
    proc = subprocess.run(
        args,
        cwd=ROOT,
        capture_output=True,
        text=True,
        check=False,
        env=env,
    )
    return proc.returncode, proc.stdout, proc.stderr


def main():
    binary = Path(os.environ.get("DRIFT_GUARD_BIN", ROOT / "bin" / "drift-guard"))
    if not binary.is_file():
        sys.exit(f"missing binary: {binary}")
    base = ROOT / "fixtures" / "phase0" / "scope"
    failed = False
    for name in ("violation", "clean"):
        directory = base / name
        expected = int((directory / "expected_exit").read_text(encoding="utf-8").strip())
        code, out, err = run(
            [
                str(binary),
                "scope",
                "--require-scope",
                "--files",
                str(directory / "files.txt"),
                "--scope-file",
                str(directory / "scope.txt"),
            ]
        )
        print(f"{name}: exit {code} (expected {expected})")
        if out.strip():
            print(out.strip())
        if err.strip():
            print(err.strip())
        if code != expected:
            failed = True
    env = os.environ.copy()
    env.pop("DRIFT_GUARD_PR_BODY", None)
    code, _, err = run(
        [str(binary), "scope", "--require-scope", "--files", str(base / "violation" / "files.txt")],
        env=env,
    )
    print(f"missing scope with --require-scope: exit {code} (expected 1)")
    if err.strip():
        print(err.strip())
    if code != 1:
        failed = True
    code, _, err = run(
        [str(binary), "scope", "--files", str(base / "clean" / "files.txt")],
        env=env,
    )
    print(f"missing scope without --require-scope: exit {code} (expected 0)")
    if err.strip():
        print(err.strip())
    if code != 0:
        failed = True
    if failed:
        sys.exit(1)
    print("ok: scope fixtures")


if __name__ == "__main__":
    main()
