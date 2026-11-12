# cmd/drift-guard

The `drift-guard` command. Module path: `github.com/zkoekl/drift-guard`. Build:

```sh
go build -o bin/drift-guard ./cmd/drift-guard
```

`drift-guard --version` and `drift-guard -v` print `main.version`. The default is `dev`. A release build sets it with `-ldflags "-X main.version=..."`.

Commands: `hook`, `check`, `scan`, `report`, `scope`, `canary refresh`. No model, no server, no telemetry. The hook reads stdin JSON and writes stdout JSON. `scan` exits 0. `report --no-open` writes one HTML file. `report --summary` prints a short count-only text summary and writes no HTML. `scope` is the pre-commit and pull-request backstop. `canary refresh` rewrites `drift-guard-canary:` in the configured files and does not print the code. See [`docs/canary/README.md`](../../docs/canary/README.md).

The latency program next to this one is `cmd/hookbench`. Numbers: [`docs/bench.md`](../../docs/bench.md). Local install: [`docs/hooks/README.md`](../../docs/hooks/README.md).
