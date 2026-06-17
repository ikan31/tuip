# tuip Architecture

This document explains how `tuip` is organized for maintainers and provider contributors. User-facing usage docs live in the root [`README.md`](../README.md). Additional provider based documentation lives in [`internal/providers/README.md`](../internal/providers/README.md).

`tuip` is published as a CLI/TUI tool. Packages under `internal/` are implementation details, not a public Go library API.

## Design goals

- Keep the CLI useful on its own for status checks, JSON output, provider discovery, and dashboard config management.
- Let the TUI reuse the same status engine instead of duplicating provider/config logic.
- Prefer public structured status APIs over scraping.
- Normalize every provider into one provider-independent status model.
- Keep dashboard config small, shareable, and based on stable provider IDs.

## High-level flow

```text
cmd/tuip/main.go
        |
        v
internal/cli/root.go  -------------------------------+
        |                                             |
        | status/providers/dashboard commands          | no subcommand launches TUI
        v                                             v
internal/app/status.go                         internal/tui/tui.go
        |                                             |
        +-------------------+-------------------------+
                            |
                            v
                 internal/providers.Registry
                            |
                            v
                    Provider.Fetch(ctx)
                            |
                            v
                 internal/status.Snapshot
                            |
             +--------------+---------------+
             v                              v
      internal/output                 TUI rendering
```

The main idea: CLI commands and the interactive TUI both use the same provider registry, status orchestration, config code, and normalized status model.

## Package map

### `cmd/tuip`

Executable entrypoint. It builds and runs the root Cobra command.

### `internal/cli`

Owns the command tree:

- `tuip` launches the TUI.
- `tuip status [provider...]` checks provider status.
- `tuip providers list/search` discovers built-in providers.
- `tuip dashboard ...` manages YAML dashboards.

This package wires together config loading, provider registry creation, status checks, and CLI output.

### `internal/app`

Application orchestration independent of Cobra and Bubble Tea.

`CheckProviders`:

- canonicalizes and validates provider IDs,
- reuses fresh cache entries when a cache is supplied,
- fetches cache misses concurrently with bounded concurrency,
- applies per-attempt timeouts and one retry for transient failures,
- converts provider/runtime failures into `error` snapshots,
- strips incident/component details unless details are requested,
- returns a `status.Response` for CLI output or TUI rendering.

This is the core status-checking engine.

### `internal/status`

Provider-independent data model.

Important types:

- `State`
- `Snapshot`
- `Incident`
- `Component`
- `Response`

A provider returns a `Snapshot`; the rest of the app should not need provider-specific API shapes.

### `internal/providers`

Provider interface, metadata, registry, alias handling, canonical ID lookup, and search.

Provider interface:

```go
type Provider interface {
    Metadata() Metadata
    Fetch(ctx context.Context) (status.Snapshot, error)
}
```

Provider IDs are stable config/CLI identifiers. Names and descriptions are presentation metadata.

### `internal/providers/builtin`

Registers the provider catalog that ships with `tuip`.

Most providers are simple metadata entries that use an existing adapter. New provider additions usually start here.

### Provider adapters

Reusable adapters live under `internal/providers/*`:

- `statuspage`: Atlassian Statuspage-compatible `/api/v2/summary.json` APIs.
- `pagerdutystatus`: PagerDuty-hosted status pages exposing `/api/data`.
- `uptimekuma`: Uptime Kuma public status pages exposing status-page and heartbeat JSON.
- `slack`: Slack-specific public status API handling.

Add a new provider-specific package only when an existing adapter cannot model the upstream API cleanly.

### `internal/fetch`

Small HTTP helper package for providers. It centralizes:

- default timeout,
- user agent,
- JSON decoding,
- text fetching,
- typed/contextual HTTP errors used by retry logic.

Providers should use this package instead of creating ad-hoc HTTP clients.

### `internal/config`

YAML dashboard config loading and saving.

Responsibilities include:

- resolving the default config path,
- loading existing config or creating an empty config,
- saving config with restrictive permissions,
- creating, renaming, deleting, listing, and selecting dashboards,
- adding/removing providers from dashboards.

Runtime files live beside the configured config file:

```text
<config-dir>/config.yaml
<config-dir>/logs/tuip.jsonl
<config-dir>/cache/status-cache.json
```

### `internal/output`

CLI output formatters:

- human-readable terminal output,
- normalized JSON output.

Output code consumes `status.Response`; it should not fetch providers or know about provider API details.

### `internal/statuscache`

Persistent JSON cache for latest provider snapshots. The cache is keyed by canonical provider ID so the `all` dashboard and user dashboards can share fresh results.

The TUI currently uses:

- 60-second TTL for successful snapshots,
- 10-second TTL for error snapshots.

### `internal/diagnostics`

Optional JSONL diagnostics logging. Logs are disabled by default and can be enabled with `TUIP_LOG_LEVEL` or `--log-level`.

Diagnostics include provider fetches, retries, cache events, TUI refreshes, run ID, PID, and version. Logs rotate at 5MB with three backups.

### `internal/tui`

Bubble Tea terminal UI.

Responsibilities:

- load provider registry and dashboard config,
- render the management/sidebar pane and status cards,
- call `app.StreamProviders` for progressive refreshes,
- mutate dashboard config through `internal/config`,
- reuse provider-level cache entries while switching dashboards,
- handle keyboard navigation and input modes.

The TUI does not fetch provider APIs directly.

## Status model

Every provider result is normalized into a `status.Snapshot`:

```text
ProviderID  stable provider ID, e.g. github
Name        display name
State       operational | degraded | partial_outage | major_outage | maintenance | unknown | error
Summary     short human-readable status line
SourceURL   public status page URL
CheckedAt   when tuip checked the provider
UpdatedAt   upstream update time, if available
Incidents   active incidents and/or scheduled maintenance
Components  sub-service statuses, if exposed
Error       fetch/parse/runtime error text when State is error
```

## Status mapping

Each provider adapter maps its upstream API into tuip's normalized `status.State` values. The mapping lives with the adapter that understands that API shape:

- Statuspage-compatible providers: `internal/providers/statuspage/statuspage.go`
- PagerDuty-hosted status pages: `internal/providers/pagerdutystatus/pagerdutystatus.go`
- Uptime Kuma public status pages: `internal/providers/uptimekuma/uptimekuma.go`
- Slack's custom status API: `internal/providers/slack/slack.go`

Provider/API-specific mapping exceptions should stay in the relevant adapter, alongside the parsing code that understands the upstream fields.

## Dashboard config

Example:

```yaml
version: 1
default_dashboard: work

dashboards:
  work:
    services:
      - provider: slack
      - provider: github
      - provider: jira
      - provider: cloudflare
```

Rules:

- Dashboard names are user-defined.
- `all` is reserved for the dashboard containing every built-in provider.
- Provider IDs should be canonicalized through the registry before saving config mutations.
- Config files are written with `0600`; parent directories use `0750`.

## Testing

Current tests cover config behavior, provider mapping/parsing, registry lookup/search, CLI behavior, cache behavior, diagnostics, and TUI helper logic.

Useful commands:

```bash
make fmt
make lint
go test ./...
```
