# Phase 0 latency

These figures were measured by `scripts/bench.py` on 2026-11-09. They were not typed by hand. The raw samples are not in this tree. The Java hook used for the JVM and GraalVM rows is not in this tree. `scripts/bench.py` can remeasure the Go programs. It does not rewrite this file, and it has no default JDK path.

## Method

The timed program reads one JSON object from stdin, evaluates two rules (`git push`, and done-without-tests), appends one JSON line to `DRIFT_GUARD_LOG`, and writes one JSON object to stdout. The Go program is `cmd/hookbench` and calls `internal/rules`. It must print `ok 8` from `--self-test bench/vectors.json` before timing. The JVM and GraalVM rows used a Java program with the same vectors.

Each sample is a new process. `scripts/bench.py` times `subprocess.run` with `time.perf_counter`, so spawn is included. The percentile is nearest rank: index `ceil(p/100*n)-1` on the ascending list.

Cold: 200 runs, and before each run `sudo -n sh -c 'sync; echo 3 > /proc/sys/vm/drop_caches'`.
Warm: 200 runs after the cold series, with no cache drop.

Payload for the minimal hook:

```json
{
  "hook_event_name": "beforeShellExecution",
  "command": "git status",
  "test_command": "pytest",
  "globs": [
    "**/*.py"
  ],
  "changed_paths": [
    "src/learn.py"
  ],
  "commands": [
    {
      "command": "git diff --stat",
      "exit_code": 0
    }
  ],
  "conversation_id": "bench"
}
```

## Results

| Build | Warm n | Warm p50 ms | Warm p95 ms | Cold n | Cold p50 ms | Cold p95 ms |
| --- | --- | --- | --- | --- | --- | --- |
| go | 200 | 1.100 | 1.833 | 200 | 3.662 | 7.331 |
| jvm | 200 | 26.129 | 29.381 | 200 | 56.133 | 67.147 |
| graalvm | 200 | 1.945 | 2.718 | 200 | 7.204 | 13.124 |
| go-hook | 200 | 1.233 | 1.795 | 200 | 4.030 | 6.718 |

## Versions

| Build | What ran | Version |
| --- | --- | --- |
| go | `cmd/hookbench` | go1.22.2 linux/amd64 |
| jvm | Java `HookBench` (not in this tree) | openjdk 25.0.5 2026-10-20 LTS |
| graalvm | GraalVM native-image of that Java program | native-image 25.0.5.0.3 2026-10-24 |
| go-hook | `drift-guard hook` | go1.22.2 linux/amd64 |

## Decision

The ship bar is p95 under 50 ms on both the cold series and the warm series. A plain JVM does not ship.
Go warm p95 is 1.833 ms and cold p95 is 7.331 ms (clears the bar).
Plain JVM warm p95 is 29.381 ms and cold p95 is 67.147 ms (misses the bar). It does not ship.
GraalVM native-image warm p95 is 2.718 ms and cold p95 is 13.124 ms (clears the bar).
Both clear the bar and the warm-p95 gap is under 5 ms, so the bench does not pick a winner. The prototype stays on Go.

`go-hook` is the shipping `drift-guard hook` on a `beforeShellExecution` allow (`git status`), not the two-rule minimal program. It is in the table so the real binary's startup is visible. The three-way comparison is `go`, `jvm`, and `graalvm`.

The GraalVM build warned that `--no-fallback` is deprecated and has no effect. The image still built.
