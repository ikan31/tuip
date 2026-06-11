# Contributing providers to tuip

The most useful contribution right now is adding or fixing built-in status providers.

`tuip` is currently published as a CLI/TUI tool, not a Go library. Packages under `internal/` are implementation details and are not intended to be imported by other projects.

## Before adding a provider

Check whether the service exposes a public status endpoint. Prefer structured APIs over scraping.

Common source types:

- Atlassian Statuspage summary API: `/api/v2/summary.json`
- PagerDuty-hosted status page API: `/api/data`
- Custom public JSON API

Avoid providers that require credentials or private customer data.

## How to add a provider

Detailed provider architecture and examples live in [`internal/providers/README.md`](./internal/providers/README.md).

Typical flow:

1. Find the provider's public status page/API.
2. Add the provider to the right built-in registration list in `internal/providers/builtin/builtin.go` when it uses an existing adapter.
3. Add a custom provider package only when the provider has a unique API shape.
4. Add or update tests and fixtures for status mapping/parsing behavior.
5. Update README provider docs if the user-facing provider list, aliases, or categories change.

## Provider ID guidelines

Provider IDs are used in CLI commands and dashboard config, so keep them stable and predictable.

Good IDs:

- lowercase
- hyphen-separated
- vendor/product oriented
- stable even if display names change

Examples:

```text
github
quickbooks-online
github-enterprise-cloud-eu
```

Aliases are fine for common shorthand, but dashboards should store canonical IDs.

## Local checks

Before opening a PR, run:

```bash
make fmt
go test ./...
```

If you have `golangci-lint` installed, also run:

```bash
make lint
```

For provider changes, also sanity-check the provider manually when practical:

```bash
go run ./cmd/tuip providers search <name>
go run ./cmd/tuip status --json <provider-id>
```

## Pull request checklist

A provider PR should include:

- the provider status source URL
- the canonical provider ID and any aliases
- tests or fixtures when parsing/mapping behavior changes
- README updates when user-facing provider lists change

Small focused PRs are preferred.
