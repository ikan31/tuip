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
- Custom providers with their own implementation are registered in `NewRegistry`.

For example, Cloudflare and GitHub are built-in providers, but they do not need separate packages because their status APIs are handled by reusable adapters.

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

### `internal/providers/slack`

Slack-specific provider implementation.

Slack has its own status API shape, so it needs custom code instead of a generic adapter. It fetches Slack's current status JSON, maps active incidents/top-level status into tuip's model, and best-effort parses component details from Slack's public status page.

### `internal/providers/testdata`

Shared provider fixtures used by provider tests.

Put JSON/HTML fixtures here when testing adapter behavior or custom provider parsing.

## Why packages are separated this way

The split is by responsibility, not by whether something is "built in".

- `builtin` is the catalog/registry wiring for all providers that ship with tuip.
- `statuspage` and `pagerdutystatus` are reusable source adapters.
- `slack` is separate because Slack needs custom fetch/parsing logic.
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

### 3. Custom structured API

If the service has its own JSON API shape:

1. Create a package under `internal/providers/<provider-id>` or create a reusable adapter package if multiple services share the same API shape.
2. Implement `providers.Provider`.
3. Map the upstream fields into `status.Snapshot`, `status.State`, `status.Incident`, and `status.Component`.
4. Register it in `builtin.NewRegistry`.
5. Add fixtures and tests for status mapping and parsing.

Slack is the current example of this approach.

### 4. HTML-only status page

Use HTML scraping only as a last resort.

If scraping is required:

- keep parsing isolated in the provider package,
- add representative fixtures to `internal/providers/testdata`,
- write tests that cover expected page structures and missing/changed markup,
- prefer returning `unknown` over guessing when the page cannot be parsed safely.

## Checklist

After adding or changing a provider:

```bash
gofmt -w internal/providers
go test ./...
tuip providers list
tuip status --json <provider-id>
```

Also update docs/examples if the new provider changes user-facing behavior, aliases, or categories.
