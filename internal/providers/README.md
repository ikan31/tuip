# Providers

This directory contains the provider layer: the code that knows how to fetch an upstream service's public status data and normalize it into tuip's shared `internal/status` model.

## Package layout

### `internal/providers`

Core provider contracts and registry.

- `Provider` is the interface every provider implementation satisfies:

  ```go
  type Provider interface {
      Metadata() Metadata
      Fetch(ctx context.Context) (status.Snapshot, error)
  }
  ```

- `Metadata` describes a provider for lookup, listing, search, aliases, config validation, and UI presentation.
- `Registry` stores provider factories, resolves aliases, validates provider IDs, and supports provider search.

This package should stay provider-agnostic. It should not know about Slack, GitHub, Cloudflare, Statuspage, PagerDuty, or any specific vendor.

### `internal/providers/builtin`

The built-in provider catalog.

This package answers: "Which providers ship with tuip?" It wires provider metadata and factories into a registry.

Most concrete SaaS providers do **not** need their own package. If they use an existing reusable adapter, add them directly here:

- Statuspage-backed services go in `statuspageRegistrations`.
- PagerDuty-hosted status pages go in `pagerDutyStatusRegistrations`.
- Uptime Kuma-backed status pages go in `uptimeKumaRegistrations`.
- Custom providers with their own implementation are registered in `NewRegistry`.

### `internal/providers/statuspage`

Reusable adapter for Atlassian Statuspage-compatible APIs, usually:

```text
/api/v2/summary.json
```

Use this when the upstream JSON includes fields like:

- `status.indicator`
- `status.description`
- `components`
- `incidents`
- `scheduled_maintenances`

The adapter maps Statuspage indicators and component statuses into tuip's normalized states, collects incidents/maintenance, parses timestamps, and returns a `status.Snapshot`.

### `internal/providers/pagerdutystatus`

Reusable adapter for PagerDuty-hosted public status pages exposing:

```text
/api/data
```

Use this when a service does not expose Statuspage JSON but has PagerDuty status-page data. This adapter currently maps the page's global status headline into a normalized state.

### `internal/providers/uptimekuma`

Reusable adapter for Uptime Kuma public status pages exposing endpoints like:

```text
/api/status-page/<slug>
/api/status-page/heartbeat/<slug>
```

Use this when a service publishes Uptime Kuma JSON for monitor groups, incidents, and heartbeat status. The adapter maps heartbeat status values into tuip's normalized states and exposes monitors as components.

### `internal/providers/aws`, `internal/providers/azure`, and `internal/providers/docker`

Cloud/package registry providers backed by public RSS feeds.

RSS defines item structure but not incident semantics, so each package keeps provider-specific parsing for active versus historical items, severity mapping, timestamps, and incident summaries.

### `internal/providers/gcp`

Google Cloud-specific provider implementation.

Google Cloud exposes a structured public `incidents.json` endpoint. The provider maps active incidents, severity, affected products, and affected locations into tuip snapshots, incidents, and components.

### `internal/providers/slack`

Slack-specific provider implementation.

Slack has its own status API shape, so it needs custom code instead of a generic adapter. It fetches Slack's current status JSON, maps active incidents/top-level status into tuip's model, and best-effort parses component details from Slack's public status page.

### `internal/providers/testdata`

Shared provider fixtures used by provider tests.

Put JSON/HTML fixtures here when testing adapter behavior or custom provider parsing.

## Why packages are separated this way

The split is by responsibility, not by whether something is "built in".

- `builtin` is the catalog/registry wiring for all providers that ship with tuip.
- `statuspage`, `pagerdutystatus`, and `uptimekuma` are reusable source adapters.
- `aws`, `azure`, `docker`, `gcp`, and `slack` are separate because they need custom fetch/parsing logic.
- The root `providers` package is only the provider interface, metadata, and registry.

Avoid creating a new vendor package unless the provider needs custom implementation logic or several providers can share a new reusable adapter.

## How to add a provider

First identify what kind of status source the service exposes.

### 1. Statuspage `/api/v2/summary.json`

If the service exposes a Statuspage summary endpoint, add an entry to `statuspageRegistrations` in `internal/providers/builtin/builtin.go`:

```go
{
    ID:          "example",
    Aliases:     []string{"optional-alias"},
    Name:        "Example",
    Description: "Example service status",
    Category:    "Developer Tools",
    SourceURL:   "https://status.example.com/",
    APIURL:      "https://status.example.com/api",
    SummaryURL:  "https://status.example.com/api/v2/summary.json",
},
```

No new package is needed.

### 2. PagerDuty-hosted `/api/data`

If the service exposes PagerDuty status-page data, add an entry to `pagerDutyStatusRegistrations` in `internal/providers/builtin/builtin.go`:

```go
{
    ID:          "example-region",
    Aliases:     []string{"example-rgn"},
    Name:        "Example - Region",
    Description: "Example regional status",
    Category:    "Developer Tools",
    SourceURL:   "https://region.examplestatus.com/",
    APIURL:      "https://region.examplestatus.com/api/data",
    DataURL:     "https://region.examplestatus.com/api/data",
},
```

No new package is needed.

### 3. Uptime Kuma public status page

If the service exposes Uptime Kuma public status JSON, add an entry to `uptimeKumaRegistrations` in `internal/providers/builtin/builtin.go`:

```go
{
    ID:           "example",
    Aliases:      []string{"optional-alias"},
    Name:         "Example",
    Description:  "Example service status",
    Category:     "Developer Tools",
    SourceURL:    "https://status.example.com/",
    APIURL:       "https://status.example.com/api/status-page/example",
    StatusURL:    "https://status.example.com/api/status-page/example",
    HeartbeatURL: "https://status.example.com/api/status-page/heartbeat/example",
},
```

No new package is needed.

### 4. Custom structured API

If the service has its own JSON API shape:

1. Create a package under `internal/providers/<provider-id>` or create a reusable adapter package if multiple services share the same API shape.
2. Implement `providers.Provider`.
3. Map the upstream fields into `status.Snapshot`, `status.State`, `status.Incident`, and `status.Component`.
4. Register it in `builtin.NewRegistry`.
5. Add fixtures and tests for status mapping and parsing.

Slack and Google Cloud are examples of this approach.

### 5. RSS or Atom feed

Use feed-based providers when the service exposes public incident feeds but no structured current-status JSON. Feed providers should document whether the feed is active-only or historical, and should avoid treating every feed item as an active outage unless the upstream semantics support that.

AWS, Azure, and Docker are examples of this approach.

### 6. HTML-only status page

Use HTML scraping only as a last resort.

If scraping is required:

- keep parsing isolated in the provider package,
- add representative fixtures to `internal/providers/testdata`,
- write tests that cover expected page structures and missing/changed markup,
- prefer returning `unknown` over guessing when the page cannot be parsed safely.

## Checklist

After adding or changing a provider:

```bash
make fmt
go test ./...
go run ./cmd/tuip providers list
go run ./cmd/tuip status --json <provider-id>
```
