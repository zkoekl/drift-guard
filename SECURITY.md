# Security

There is no hosted service. The CLI reads a plan, a git diff, and a local log on the machine where it runs. It does not have an account system.

## Reporting

Use a private [GitHub security advisory](https://github.com/zkoekl/drift-guard/security/advisories/new) on this repository. Do not open a public issue for a vulnerability, and do not include session logs, tokens, or personal data in the report.

## Supported versions

This project is pre-alpha. There is no stable release. Reports against `main` are accepted.

| Version | Supported |
| ------- | --------- |
| main    | yes       |

## What the CLI does

- Default path: no network, no telemetry, no background process.
- `drift-guard report` writes one HTML file and exits. It must not load remote scripts, fonts, or images.
- The log must not store prompts, file contents, or the email Cursor passes to hooks.
- A local `serve` command is not the default. If it is ever added, it binds to loopback only and runs only while the user keeps that process. See [`docs/adr/0004-metrics-runtime.md`](docs/adr/0004-metrics-runtime.md).

## Secrets

Do not send credentials, cookies, or private keys in an issue or an advisory. Maintainers will delete them.
