# tuip

`tuip` is a Go CLI-first foundation for a future terminal UI that aggregates SaaS status pages into shareable dashboards.

The current MVP fetches normalized status for a small reliable provider set:

- Slack
- GitHub
- GitHub Enterprise Cloud regional status pages
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

## TUI

Open the terminal dashboard:

```bash
tuip
```

The TUI loads the configured default dashboard. If no default dashboard exists yet, it shows an `all` dashboard with every built-in provider.

TUI management pane:

- Select visible actions like `(c)reate dashboard`, `(r)ename dashboard`, `(d)elete dashboard`, `(s)et dashboard default`, and `Providers: A-Z/category` with `enter`.
- Select a dashboard with `enter`.
- Select a provider with `enter` to add/remove it from the current dashboard; configured providers are marked with `*`.
- Select `Search providers` under the providers section, or press `/`, to fuzzy-search providers.

TUI navigation shortcuts:

- `tab` switches focus between management and status panes
- `j`/`k` or arrow keys move in the focused pane
- `enter` opens selected status details from the status pane
- `d` also opens selected status details from the status pane
- `esc` closes provider details when open
- `r` refreshes the current dashboard
- `q` quits

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

Fuzzy-search built-in providers:

```bash
tuip providers search github eu
tuip providers search gheceu
```

Current provider sources:

- Slack uses Slack's status API: <https://docs.slack.dev/reference/slack-status-api/>
- GitHub uses Statuspage JSON from <https://www.githubstatus.com/#>
- GitHub Enterprise Cloud EU uses Statuspage JSON from <https://eu.githubstatus.com/>
- GitHub Enterprise Cloud Australia/Japan/US use PagerDuty status-page JSON from `/api/data`
- GitHub Enterprise Cloud regional providers have short aliases: `github-au`, `github-eu`, `github-jp`, `github-us` plus `ghec-au`, `ghec-eu`, `ghec-jp`, `ghec-us`
- Cloudflare uses Statuspage JSON documented from <https://www.cloudflarestatus.com/api>

## Dashboard config

Dashboard config is YAML and is intended for the future TUI and for sharing setups with others.

Default location on macOS/Linux:

```text
~/.config/tuip/config.yaml
```

If `XDG_CONFIG_HOME` is set, tuip uses:

```text
$XDG_CONFIG_HOME/tuip/config.yaml
```

Windows uses the native OS user config directory.

You can override it:

```bash
tuip --config ./tuip.yaml dashboard list
```

Create and manage dashboards:

```bash
tuip dashboard create work slack github cloudflare
tuip dashboard add work cloudflare
tuip dashboard use work
tuip dashboard list
tuip dashboard show work
tuip dashboard remove work github
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
