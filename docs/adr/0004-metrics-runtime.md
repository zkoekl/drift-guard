# ADR 0004: On-demand static report, not a server

- Status: accepted
- Date: 2026-11-09

## Context

Metrics have to be visible, and they have to stay local. No cloud dashboard and no telemetry. The log is append-only `events.jsonl` (SQLite is an optional read-only input; the tool does not write it). Two ways to look at that log:

1. `drift-guard report` reads the log, writes one self-contained HTML file, opens it, and exits.
2. `drift-guard serve` starts a local HTTP server when asked, and the page reads the log from that process.

Both render with ordinary code. Neither needs a model. Token cost is zero either way. The page includes the ambiguous log rows (possible sycophancy and other signals the heuristics would not block on) so the owner can review them. That section is not a model summary.

## Comparison

| | `report` (chosen) | `serve` (not the default) |
| --- | --- | --- |
| Idle cost | Zero. No process between runs. | Zero until started. While it runs: one process and a bound port. A forgotten terminal is idle cost. |
| Complexity | One subcommand, one file, the OS open. No port, no shutdown, no auth. | Bind address, lifetime, refresh, and a rule that it never listens beyond loopback. |
| Token cost | Zero. Charts are computed in the process. | Zero, for the same reason. |
| Freshness | Rerun the command. | Could refresh while open. Not required for a 14-day review. |
| Failure mode | A bad log makes one bad file. | A bad bind or a process left up after the review. |

## Decision

`drift-guard report` is the only viewer. It writes one HTML file with the CSS and the chart script inlined. No CDN, no remote font, no request after the file is open. Log values are escaped. `--no-open` skips the browser. The process does not listen.

`drift-guard serve` is not built. Add it only if the dogfood log shows the static file is not enough. If it is added later: start only when the user runs it, bind `127.0.0.1` only, no launch agent, no systemd unit, no hook that starts it, exit when the user stops it. It still does not phone home.

Charts, empty states, and the label command are specified in [METRICS.md](../METRICS.md).

## Consequences

- The dogfood review is a file the owner can open offline.
- CI can run `drift-guard report --no-open` and diff nothing by default; the HTML is gitignored.
- There is no metrics URL to secure because there is no metrics port.

## Alternatives

- **A phone-home dashboard.** Rejected. Contradicts local-only.
- **SQLite plus a GUI app.** Rejected. The HTML file is the GUI. SQLite remains an optional input, not a second product.
- **Always-on `serve`.** Rejected. Idle cost and a listener are what the on-demand file avoids.
