#!/usr/bin/env python3
"""Remeasure the Go hook and print nearest-rank percentiles.

The published table is docs/bench.md (measured 2026-11-09). This
script does not rewrite that file and does not write raw samples into the
tree. The JVM and GraalVM programs from that measurement are not in this
tree. There is no default JDK path.

Cold runs need passwordless sudo for `drop_caches`. 200 cold and 200 warm
process starts. Go only: cmd/hookbench and `drift-guard hook`.
"""

import json
import math
import os
import subprocess
import time
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
N = 200

PAYLOAD = {
    "hook_event_name": "beforeShellExecution",
    "command": "git status",
    "test_command": "pytest",
    "globs": ["**/*.py"],
    "changed_paths": ["src/learn.py"],
    "commands": [{"command": "git diff --stat", "exit_code": 0}],
    "conversation_id": "bench",
}
HOOK_PAYLOAD = {
    "hook_event_name": "beforeShellExecution",
    "command": "git status",
    "conversation_id": "bench",
    "user_email": "person@example.com",
}


def percentile(values, p):
    xs = sorted(values)
    idx = math.ceil(p / 100 * len(xs)) - 1
    if idx < 0:
        idx = 0
    if idx >= len(xs):
        idx = len(xs) - 1
    return xs[idx]


def fmt(ms):
    return f"{ms:.3f}"


def drop_caches():
    subprocess.run(
        ["sudo", "-n", "sh", "-c", "sync; echo 3 > /proc/sys/vm/drop_caches"],
        check=True,
    )


def time_series(cmd, payload, env, n, drop):
    times = []
    body = json.dumps(payload).encode("utf-8")
    for i in range(n):
        if drop:
            drop_caches()
        t0 = time.perf_counter()
        proc = subprocess.run(cmd, input=body, capture_output=True, env=env)
        dt = (time.perf_counter() - t0) * 1000.0
        if proc.returncode != 0 or b'"permission"' not in proc.stdout:
            raise SystemExit(
                "command failed\n"
                + " ".join(cmd)
                + "\n"
                + proc.stderr.decode("utf-8", "replace")
                + "\n"
                + proc.stdout.decode("utf-8", "replace")
            )
        times.append(dt)
        if (i + 1) % 50 == 0:
            print(f"  {i + 1}/{n}", flush=True)
    return times


def report(name, times_cold, times_warm):
    print(
        f"{name}: warm p50 {fmt(percentile(times_warm, 50))} ms, "
        f"warm p95 {fmt(percentile(times_warm, 95))} ms, "
        f"cold p50 {fmt(percentile(times_cold, 50))} ms, "
        f"cold p95 {fmt(percentile(times_cold, 95))} ms "
        f"(n={len(times_warm)}/{len(times_cold)})"
    )


def build_go():
    subprocess.run(["go", "build", "-o", "bin/hookbench-go", "./cmd/hookbench"], cwd=ROOT, check=True)
    subprocess.run(["go", "build", "-o", "bin/drift-guard", "./cmd/drift-guard"], cwd=ROOT, check=True)
    subprocess.run(
        [str(ROOT / "bin/hookbench-go"), "--self-test", "bench/vectors.json"],
        cwd=ROOT,
        check=True,
    )


def measure(name, cmd, payload):
    env = os.environ.copy()
    for key in ("DRIFT_GUARD_RULES", "DRIFT_GUARD_DENYLIST", "DRIFT_GUARD_PR_BODY"):
        env.pop(key, None)
    work = Path(os.environ.get("TMPDIR", "/tmp"))
    env["DRIFT_GUARD_LOG"] = str(work / f"drift-bench-{name}.jsonl")
    env["DRIFT_GUARD_DUMP"] = str(work / f"drift-bench-{name}-dump.jsonl")
    env["DRIFT_GUARD_STATE"] = str(work / f"drift-bench-{name}-state")
    print(f"== {name}", flush=True)
    time_series(cmd, payload, env, 1, drop=False)
    print(" cold", flush=True)
    cold = time_series(cmd, payload, env, N, drop=True)
    print(" warm", flush=True)
    warm = time_series(cmd, payload, env, N, drop=False)
    report(name, cold, warm)


def main():
    os.chdir(ROOT)
    build_go()
    measure("go", [str(ROOT / "bin" / "hookbench-go")], PAYLOAD)
    measure("go-hook", [str(ROOT / "bin" / "drift-guard"), "hook"], HOOK_PAYLOAD)
    print("published table: docs/bench.md (this run does not rewrite it)")


if __name__ == "__main__":
    main()
