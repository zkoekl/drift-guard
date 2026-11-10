# ADR 0001: CLI language candidates

- Status: superseded by [ADR 0005](0005-runtime-model.md)
- Date: 2026-11-09

## Context

`drift-guard` runs inside a hook, so process start is part of the product. A plain JVM may be too slow to run on every shell call and every stop. That is measured in phase 0, not assumed. The binary has to be one file, with no extra language runtime required on Windows (Cursor runs hooks through PowerShell) or on a cloud-agent VM. The JVM itself counts as a runtime if the plain JVM build is the one that ships.

## Decision

This ADR no longer picks a language. The candidates are Go, Java compiled with GraalVM native-image, and a plain JVM. Phase 0 prototype measures them. The bar is p95 hook latency under 50ms for a no-op check and for the trivial plan-versus-diff check, on a cold and a warm start. A plain JVM is measured in phase 0 and rejected if hook p95 >= 50 ms. The numbers are in [ADR 0005](0005-runtime-model.md) and [bench.md](../phase0/bench.md). The prototype is the Go module in this tree. No Gradle build is added.

Shell out to `git`. Do not vendor libgit2.

## Measurement (2026-11-09)

`scripts/bench.py` timed a minimal hook (read stdin JSON, evaluate git-push and done-without-tests, append one JSONL line, write stdout JSON). 200 cold runs (page cache dropped before each) and 200 warm runs. Nearest-rank p50 and p95. The summary is in [bench.md](../phase0/bench.md). Raw samples are not published.

| Build | Warm p50 ms | Warm p95 ms | Cold p50 ms | Cold p95 ms |
| --- | --- | --- | --- | --- |
| Go | 1.100 | 1.833 | 3.662 | 7.331 |
| Plain JVM, Temurin 25.0.5 | 26.129 | 29.381 | 56.133 | 67.147 |
| GraalVM CE 25.0.5 native-image | 1.945 | 2.718 | 7.204 | 13.124 |

The bar is p95 under 50 ms cold and warm. Go and the native image clear it. The plain JVM misses it on the cold series (67.147 ms). The warm-p95 gap between Go and the native image is under 5 ms, so this run does not pick a winner. The prototype stays on Go. The same numbers are in [ADR 0005](0005-runtime-model.md).

## Alternatives already rejected

- **Python or Node.** A runtime is not guaranteed in the hook environment. A missing runtime that fails open is a silent hole.
- **Rust.** Same binary shape as Go, more toolchain than this parser needs, unless the benchmark says otherwise. Not in the first pair.
- **Shell.** Not enough for globs, a JSONL log, and an HTML report.
