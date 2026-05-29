# tuip Architecture

This document explains how `tuip` is structured and how the CLI, TUI, providers, config, and output layers fit together. It is intended for future contributors and for project maintenance. User-facing docs live in the root [`README.md`](../README.md).

## Design goals

`tuip` is built around a CLI-first core:

1. The CLI must be useful on its own for ad-hoc status checks, automation, provider discovery, and dashboard config management.
2. The TUI should not reimplement business logic. It should call the same provider registry, status orchestration, config, and normalized status model as the CLI.
3. Provider integrations should prefer public structured APIs over scraping.
4. Every provider should normalize into the same status model so output and UI code stay provider-agnostic.
5. Dashboard config should be shareable YAML that references stable provider IDs.

## High-level flow

```text
cmd/tuip/main.go
        |
        v
internal/cli/root.go  -------------------------------+
        |                                             |
        | status/dashboard/providers commands          | root command launches TUI
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

The important point: the CLI and TUI share the same engine packages rather than having separate status-checking implementations.

## Package map

### `cmd/tuip`

Executable entrypoint. It constructs the root Cobra command and executes it.

### `internal/cli`

Owns the command-line interface:

- `tuip` launches the TUI.
- `tuip status [provider...]` runs status checks.
- `tuip providers list/search` discovers built-in providers.
- `tuip dashboard ...` manages YAML dashboards.

The CLI is the main integration point for the core engine. It wires together config loading, provider registry creation, status orchestration, and output formatting.

### `internal/app`

Application orchestration that is intentionally independent from Cobra and Bubble Tea.

`CheckProviders`:

- validates provider IDs with the registry,
- reuses fresh provider-level cache entries when a cache is supplied,
- fetches cache misses concurrently with a bounded concurrency limit,
- applies per-attempt provider timeouts and one retry for transient failures,
- normalizes runtime failures into `status.StateError` snapshots,
- optionally removes detail-heavy incidents/components unless `Details` is requested,
- writes refreshed snapshots back to cache when a cache is supplied,
- returns a `status.Response` consumed by both CLI and TUI.

This is the core status-checking engine.

### `internal/status`

Provider-independent normalized data model.

Main types:

- `State`: normalized health state enum.
- `Snapshot`: one provider's normalized status at a point in time.
- `Incident`: active incident or scheduled maintenance information.
- `Component`: sub-service/component health when exposed by the upstream provider.
- `Response`: a batch of snapshots from one check run.

The output layer, TUI, tests, and JSON output all consume these types.

### `internal/providers`

Provider contract and registry.

Provider interface:

```go
type Provider interface {
    Metadata() Metadata
    Fetch(ctx context.Context) (status.Snapshot, error)
}
```

Registry responsibilities:

- register provider factories,
- resolve aliases to canonical IDs,
- validate provider IDs,
- return provider metadata for listing/search/TUI provider browsing,
- fuzzy-search metadata for provider discovery.

Provider IDs are stable API/config identifiers. Names and descriptions are presentation metadata.

### `internal/providers/builtin`

Registers all built-in providers into a registry. This is where new built-in providers are usually added if they can use an existing reusable provider adapter.

Current source families:

- Slack custom JSON API.
- Atlassian Statuspage JSON (`/api/v2/summary.json`).
- PagerDuty status-page JSON (`/api/data`) for some GitHub Enterprise Cloud regional pages.

### `internal/providers/statuspage`

Reusable adapter for Atlassian Statuspage-compatible status pages.

It maps:

- top-level `status.indicator` to `status.State`,
- components to normalized components,
- active incidents and scheduled maintenance to normalized incidents,
- provider page timestamps to `UpdatedAt`.

Most built-in providers use this adapter and only need metadata plus a `SummaryURL`.

### `internal/providers/pagerdutystatus`

Reusable adapter for PagerDuty-hosted public status pages that expose `/api/data`. This currently supports GitHub Enterprise Cloud regional providers that do not use Statuspage JSON.

### `internal/providers/slack`

Slack-specific provider. Slack has its own public status API for current status and active incidents. The provider also performs a best-effort text fetch of the status page for component details; failures there do not fail the provider check.

### `internal/fetch`

Small HTTP client wrapper with shared defaults:

- timeout,
- user-agent,
- JSON decoding,
- text fetching,
- contextual HTTP errors including typed non-2xx HTTP status errors for retry decisions.

All providers should use this package rather than constructing ad-hoc HTTP clients.

### `internal/statuscache`

Persistent JSON cache for latest provider snapshots. It is keyed by canonical provider ID, which lets `all` and user dashboards share fresh results. The TUI currently uses a 60-second TTL for successful snapshots and a 10-second TTL for error snapshots.

### `internal/diagnostics`

Optional structured diagnostics logging. Logs are JSONL and are disabled by default. `TUIP_LOG_LEVEL=debug` or `--log-level debug` writes provider fetch, retry, cache, and TUI refresh events under the configured runtime directory. The active log is `logs/tuip.jsonl`; it rotates at 5MB and keeps three backups (`tuip.1.jsonl` through `tuip.3.jsonl`). Each record includes `run_id`, `pid`, and `version` fields.

### `internal/config`

YAML dashboard config:

- resolves default config path,
- loads existing config,
- creates empty config when missing,
- saves with restrictive file permissions,
- normalizes dashboard/service data,
- manages dashboard operations like create, rename, delete, add/remove providers, and set default.

The config schema is intentionally small and shareable. Runtime files generated by tuip live beside the configured config file:

```text
<config-dir>/config.yaml
<config-dir>/logs/tuip.jsonl
<config-dir>/cache/status-cache.json
```

### `internal/output`

CLI output formatters:

- `WriteHuman`: colored terminal cards with optional details.
- `WriteJSON`: normalized JSON output for scripts/tests.

CLI output consumes `status.Response`; it should not fetch providers or know about provider-specific APIs.

### `internal/tui`

Bubble Tea terminal UI.

Responsibilities:

- load registry and dashboard config,
- display management sidebar and status grid,
- call shared app/config/provider code for refreshes and mutations,
- render normalized snapshots,
- handle keyboard navigation and input modes.

The TUI does not own provider-specific fetch logic. It calls `app.CheckProviders`, just like the CLI.

## Status model

Every provider returns a `status.Snapshot`:

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

This model is the contract between providers and the rest of the app.

## Status mapping rules

### Statuspage

Statuspage `indicator` values are mapped as:

- `none` -> `operational`
- `minor` -> `degraded`
- `major` -> `major_outage`
- `critical` -> `major_outage`
- `maintenance` -> `maintenance`
- empty/unknown -> `unknown`

Component statuses are mapped as:

- `operational` -> `operational`
- `degraded_performance` -> `degraded`
- `partial_outage` -> `partial_outage`
- `major_outage` -> `major_outage`
- `under_maintenance` -> `maintenance`

If top-level status is operational but scheduled maintenance is in progress, the adapter reports `maintenance`.

### Slack

Slack maps `ok`, `active`, and `resolved` to `operational`. Active incidents override the top-level status to `degraded`. Unknown or empty values become `unknown`.

### Provider fetch errors

Provider fetch errors are converted by `app.CheckProviders` into `error` snapshots so the user can see partial results. The whole command still exits non-zero because `tuip` failed to check everything requested.

## CLI as the engine

The CLI is not just a wrapper around the TUI. It is the canonical way to exercise the core engine:

```bash
tuip status slack github cloudflare
tuip status --json github jira asana
tuip providers list
tuip dashboard create work slack github jira
```

The TUI uses the same lower-level packages:

- provider registry from `internal/providers/builtin`,
- dashboard config from `internal/config`,
- status orchestration from `internal/app`,
- normalized model from `internal/status`.

When adding features, prefer implementing reusable behavior in `internal/app`, `internal/config`, `internal/providers`, or `internal/status`, then expose it through both CLI and TUI as needed.

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
- Provider IDs should be canonicalized through the registry before saving when commands mutate config.
- `all` is a virtual dashboard used by the TUI/status resolution to mean every built-in provider.
- Config file writes use `0600`; parent directories use `0750`.

## TUI state model

The TUI model tracks:

- app context and registry,
- active dashboard and provider IDs,
- loaded dashboard names/default dashboard,
- latest `status.Response`,
- loading/error state,
- focus area (`sidebar` or `status`),
- input mode for provider search and dashboard create/rename/delete,
- persistent provider status cache used during dashboard refreshes,
- sidebar/status/detail scroll state,
- selected status card.

Navigation currently uses:

- arrow keys or `h/j/k/l` for movement,
- `enter` for selecting/opening,
- `esc` for backing out of detail/status focus,
- `c`, `r`, `d`, `s` for create, rename, delete/details, set default,
- `R` for force-refreshing the active dashboard and bypassing cache,
- `ctrl+c` to quit.

The status grid scrolls by full card rows so highlighted cards are not partially hidden.

## Adding a new provider

Preferred order:

1. Use an existing structured public API.
2. Use `internal/providers/statuspage` if the service exposes `/api/v2/summary.json`.
3. Use `internal/providers/pagerdutystatus` if the service exposes PagerDuty `/api/data`.
4. Add a new reusable adapter if several providers share another status-page product/API.
5. Use HTML scraping only as a last resort, with fixtures and tests.

For a Statuspage provider, add a `statuspage.Options` entry in `internal/providers/builtin/statuspageRegistrations` with:

- stable `ID`,
- optional `Aliases`,
- display `Name`,
- `Description`,
- `Category`,
- `SourceURL`,
- `APIURL`,
- `SummaryURL`.

Then run:

```bash
make lint
go test ./...
tuip providers list
tuip status --json <provider-id>
```

## Testing strategy

Current tests cover:

- config round trips and dashboard mutations,
- provider status mapping and fixture parsing,
- registry aliases/search/canonicalization,
- CLI dashboard/status behaviors,
- TUI rendering helpers and scroll invariants.

Useful commands:

```bash
go test ./...
make lint
make check
```

`make lint` currently runs `golangci-lint run --fix`, so it can apply formatter/simple fixes.

## Future architecture opportunities

Potential future work:

- Split large TUI file into focused files: model, update, view, sidebar, status grid, details, layout.
- Add reusable adapters for Status.io, Instatus, Google Workspace JSON, Zendesk Status API, and Microsoft Graph service health.
- Add cache freshness indicators per status card/detail view.
- Add provider fixture tests for every built-in Statuspage provider if long-term API stability becomes a concern.
- Add screenshots or terminal recordings to user docs.
