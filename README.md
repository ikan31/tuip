# tuip

`tuip` is a terminal-first SaaS status dashboard. It checks public vendor status APIs, normalizes them into one status model, and lets you use the same engine from either a scriptable CLI or an interactive TUI.

Use it to answer questions like:

- "Is GitHub, Slack, Cloudflare, or Jira degraded right now?"
- "What providers are in my work dashboard?"
- "Can I get machine-readable status JSON for automation?"

## Features

- **CLI-first engine** for status checks, JSON output, provider discovery, and dashboard config management.
- **Interactive TUI** launched with `tuip` for browsing dashboards, providers, status cards, and details.
- **Shareable YAML dashboards** with provider IDs that can be committed or copied between machines.
- **Normalized states** across different providers: `operational`, `degraded`, `partial_outage`, `major_outage`, `maintenance`, `unknown`, and `error`.
- **Provider details** when available: active incidents, scheduled maintenance, components, source URL, checked time, and provider update time.
- **Persistent TUI status cache** so dashboard switches can reuse fresh provider results instead of refetching every subset.
- **Optional JSONL diagnostics logs** for provider fetch timing, cache hits/misses, retries, and errors.
- **No credentials required** for built-in providers; sources are public status APIs/pages.

## Install / run locally

Run from source:

```bash
go run ./cmd/tuip --help
```

Build a local binary:

```bash
go build -o tuip ./cmd/tuip
./tuip --help
```

Install with Go:

```bash
go install ./cmd/tuip
```

## Quick start

Check explicit providers:

```bash
tuip status slack github cloudflare
```

Open the interactive dashboard:

```bash
tuip
```

List built-in providers:

```bash
tuip providers list
```

Create a reusable dashboard:

```bash
tuip dashboard create work slack github jira asana cloudflare
tuip dashboard use work
tuip status
```

## CLI reference

The CLI is the engine for `tuip`. The TUI uses the same provider registry, status orchestration, normalized model, and dashboard config packages that these commands use.

Global flags:

- `--config <path>` overrides the config file path.
- `--log-level <off|debug|info|warn|error>` enables diagnostics logging. It defaults to `TUIP_LOG_LEVEL`, then `off`.

### `tuip status`

Fetch provider statuses.

```bash
tuip status [provider...]
```

Examples:

```bash
tuip status slack github cloudflare
tuip status --details cloudflare
tuip status --json github jira asana
tuip status --dashboard work
tuip status --fail-on-degraded github cloudflare
```

Flags:

- `--json` writes normalized JSON for scripts.
- `--details` includes incidents, scheduled maintenance, and components when the provider exposes them.
- `--dashboard <name>` checks a named configured dashboard.
- `--fail-on-degraded` exits non-zero when a successfully checked provider is not healthy.

Exit behavior:

- Runtime/check failures, unknown providers, invalid API responses, or timeouts exit non-zero.
- Successfully fetched degraded/outage statuses exit `0` by default so humans can inspect them.
- Use `--fail-on-degraded` for CI/monitoring workflows that should fail on unhealthy upstream services.

### `tuip providers`

Discover built-in provider IDs.

```bash
tuip providers list
tuip providers search github eu
tuip providers search qbo
```

Provider aliases are accepted in CLI commands and dashboard config. For example, `qbo` resolves to `quickbooks-online`, and `ghec-eu` resolves to `github-enterprise-cloud-eu`.

### `tuip dashboard`

Manage YAML dashboards.

```bash
tuip dashboard create work slack github cloudflare
tuip dashboard add work jira asana
tuip dashboard remove work github
tuip dashboard use work
tuip dashboard list
tuip dashboard show work
```

`dashboard` also has the alias `dashboards`.

## Interactive TUI

Run the TUI with no subcommand:

```bash
tuip
```

The TUI loads the configured default dashboard. If no default dashboard exists, it shows the virtual `all` dashboard with every built-in provider.

Management pane:

- Select visible actions like `Filter dashboard`, `(c)reate dashboard`, `(r)ename dashboard`, `(d)elete dashboard`, `(s)et dashboard default`, and provider grouping with `enter`.
- Select a dashboard with `enter`.
- Select a provider with `enter` to add/remove it from the active dashboard; configured providers are marked with `*`.
- Select `Search providers` under the providers section to fuzzy-search providers.

Navigation:

- Arrow keys or `h`/`j`/`k`/`l` move through panes and status cards.
- `enter` selects management items or opens selected status details.
- `/` focuses the dashboard filter shown at the top of the status pane; type to filter visible status cards and press `enter` or `esc` when done.
- `esc` backs out of status/details focus without quitting the TUI.
- `c`, `r`, `d`, and `s` trigger create, rename, delete/details, and set-default actions.
- `R` force-refreshes the active dashboard and bypasses the cache.
- `ctrl+c` quits.

The TUI keeps a 60-second provider-level status cache. Switching from `all` to a dashboard that is a subset of `all` reuses fresh cached provider snapshots. Error snapshots are cached for only 10 seconds.

## Built-in providers

Current built-in providers include:

- **AI:** Anthropic, OpenAI
- **Analytics & BI:** Hex, Metabase Cloud, Omni Analytics, Preset, Sigma Computing
- **Cloud & hosting:** DigitalOcean, Fly.io, Netlify, Render, Vercel
- **Communication:** Slack, Twilio
- **Developer tools:** GitHub, GitHub Enterprise Cloud regional providers, Bitbucket
- **Infrastructure:** Cloudflare
- **Data integration:** Fivetran, Hevo, Matillion
- **Data platforms:** Astronomer Astro, Atlan, ClickHouse Cloud, Confluent Cloud, dbt Cloud, Dremio Cloud, Snowflake
- **Databases:** Aiven, Elastic Cloud, MongoDB Cloud, Pinecone, PlanetScale, Supabase, Zilliz Cloud
- **Observability:** Bigeye, Datadog regional providers, New Relic, Sentry
- **Project management:** Asana, Hive, Jira, monday.com, Trello
- **Collaboration:** Confluence
- **CRM & sales:** Accelo, Affinity, Capsule, HubSpot, Nutshell
- **HR & workforce:** Ashby, Greenhouse, Gusto, Officient, Workable
- **Finance & accounting:** FreshBooks, QuickBooks Online, Xero
- **File storage:** Box, Dropbox

Provider source notes:

- Most providers use Atlassian Statuspage-compatible JSON (`/api/v2/summary.json`).
- GitHub Enterprise Cloud Australia/Japan/US use PagerDuty status-page JSON (`/api/data`).
- Slack uses Slack's public status API for top-level status and active incidents.
- Providers that need non-Statuspage adapters, such as Databricks, Airbyte, Tableau, Collibra, and Informatica, are intentionally deferred.
- Provider IDs are stable and intended for dashboards; use `tuip providers list` for the current full list and aliases.

## Dashboard config

Dashboard config is YAML. Dashboard name `all` is reserved for tuip's virtual dashboard containing every built-in provider.

Default location on macOS/Linux:

```text
~/.config/tuip/config.yaml
```

If `XDG_CONFIG_HOME` is set, tuip uses:

```text
$XDG_CONFIG_HOME/tuip/config.yaml
```

Windows uses the native OS user config directory.

Override the config path:

```bash
tuip --config ./tuip.yaml dashboard list
```

Runtime files live beside the configured config file:

```text
~/.config/tuip/
  config.yaml
  logs/tuip.jsonl
  cache/status-cache.json
```

Diagnostics are off by default. Enable them with either:

```bash
TUIP_LOG_LEVEL=debug tuip
# or
tuip --log-level debug
```

`tuip.jsonl` is rotated when it reaches 5MB. tuip keeps up to three older files as `tuip.1.jsonl`, `tuip.2.jsonl`, and `tuip.3.jsonl`. Each log line includes a `run_id`, `pid`, and `version` so one run can be filtered from the shared log file.

Example config:

```yaml
version: 1
default_dashboard: work

dashboards:
  work:
    services:
      - provider: slack
      - provider: github
      - provider: jira
      - provider: asana
      - provider: cloudflare
```

## Development

Common commands:

```bash
go test ./...
make lint
make check
```

Build locally:

```bash
make build
./bin/tuip --help
```

Architecture and contributor-oriented implementation notes live in [`docs/architecture.md`](./docs/architecture.md). MVP status and near-term roadmap notes live in [`docs/mvp.md`](./docs/mvp.md).


