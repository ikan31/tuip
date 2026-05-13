# tuip

`tuip` is a Go CLI-first foundation for a future terminal UI that aggregates SaaS status pages into shareable dashboards.

The current MVP fetches normalized status for a small reliable provider set:

- Slack
- GitHub
- Cloudflare

## Install / run locally

```bash
go run ./cmd/tuip --help
```

Or build a local binary:

```bash
go build -o tuip ./cmd/tuip
./tuip --help
```

## Status checks

Check explicit providers:

```bash
tuip status slack github cloudflare
```

Human output is rendered as colored terminal cards.

Get standardized JSON:

```bash
tuip status --json slack github cloudflare
```

Show active incidents, scheduled maintenance, and component details when available:

```bash
tuip status --details cloudflare
```

By default, a degraded upstream service does **not** mean `tuip` failed. The command exits non-zero for tuip/runtime errors like unknown providers, timeouts, or invalid API responses.

For automation, you can opt into non-zero exits for unhealthy services:

```bash
tuip status --fail-on-degraded slack github cloudflare
```

## Providers

List built-in providers:

```bash
tuip providers list
```

Current provider sources:

- Slack uses Slack's status API: <https://docs.slack.dev/reference/slack-status-api/>
- GitHub uses Statuspage JSON from <https://www.githubstatus.com/#>
- Cloudflare uses Statuspage JSON documented from <https://www.cloudflarestatus.com/api>

## Dashboard config

Dashboard config is YAML and is intended for the future TUI and for sharing setups with others.

Default location:

```text
<os user config dir>/tuip/config.yaml
```

You can override it:

```bash
tuip --config ./tuip.yaml dashboards list
```

Create and manage dashboards:

```bash
tuip dashboards create work
tuip dashboards add work slack github cloudflare
tuip dashboards use work
tuip dashboards list
tuip dashboards show work
tuip dashboards remove work github
```

Once a default dashboard exists, this checks it:

```bash
tuip status
```

Check a named dashboard:

```bash
tuip status --dashboard work
```

Example YAML:

```yaml
version: 1
default_dashboard: work

dashboards:
  work:
    services:
      - provider: slack
      - provider: github
      - provider: cloudflare
```

## Development

Run tests:

```bash
go test ./...
```

The architecture is documented in [`PLAN.md`](./PLAN.md).
