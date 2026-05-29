# tuip MVP Status

> [!ATTENTION]
> This file is a read-only historical record of the initial MVP planning and execution. Do not update it for post-MVP feature work, provider additions, roadmap changes, or current project status. Current source-of-truth docs are [`README.md`](../README.md) for usage and [`docs/architecture.md`](./architecture.md) for implementation details.

The original MVP has been completed and surpassed. This file tracks what the MVP required, what existed when the MVP was closed, and likely next steps identified at that time.

## Current status

`tuip` is now a working terminal-first SaaS status dashboard with both CLI and TUI entry points.

Implemented:

- Go/Cobra CLI launched from `cmd/tuip`.
- Bubble Tea/Lip Gloss TUI launched with `tuip` or `make run`.
- Shared status engine in `internal/app` used by both CLI and TUI.
- Normalized status model in `internal/status`.
- Built-in provider registry with provider list/search and alias resolution.
- Built-in providers across communication, developer tools, infrastructure, project management, collaboration, CRM/sales, HR/workforce, finance/accounting, and file storage.
- Reusable Statuspage and PagerDuty status-page adapters.
- Slack-specific status provider.
- YAML dashboard config with create/list/show/use/add/remove flows.
- Virtual `all` dashboard containing every built-in provider.
- TUI dashboard switching, provider add/remove/search, status grid, provider details, and dashboard management actions.
- Persistent TUI status cache at `<config-dir>/cache/status-cache.json`.
- Optional JSONL diagnostics logs at `<config-dir>/logs/tuip.jsonl`.
- Log rotation at 5MB with three retained backups.
- Bounded concurrent provider fetches and one retry for transient failures.
- Tests and lint coverage for core model, providers, config, CLI, TUI helpers, cache, and diagnostics.

## Original MVP requirements

The original MVP called for:

- CLI-first Go implementation.
- `tuip status slack github cloudflare`.
- Concurrent provider checks with timeouts.
- Normalized states.
- Human and JSON output.
- `--details` output.
- `providers list`.
- YAML dashboard config and dashboard CLI commands.
- A future TUI using the same core engine.

All of those requirements are complete. The project has also surpassed them with expanded providers, the interactive TUI, persistent cache, diagnostics, retry, and refresh behavior.

## Current runtime behavior

### Status checks

- `tuip status <provider...>` checks explicit providers.
- `tuip status` checks the configured default dashboard.
- `tuip status --dashboard <name>` checks a named dashboard.
- Runtime failures produce `error` snapshots and non-zero exit codes.
- Successfully fetched upstream degraded/outage states exit `0` by default.
- `--fail-on-degraded` makes unhealthy upstream states exit non-zero for automation.

### TUI refresh/cache

- Dashboard switches use the persistent provider cache when entries are fresh.
- Successful cached snapshots live for 60 seconds.
- Error snapshots live for 10 seconds.
- Uppercase `R` or the `(R)efresh dashboard` sidebar action force-refreshes the active dashboard and bypasses cache.
- Provider-level cache keys allow the `all` dashboard and user dashboards to share results.

### Diagnostics

Diagnostics are off by default.

Enable them with:

```bash
TUIP_LOG_LEVEL=debug tuip
# or
tuip --log-level debug
```

Runtime files live beside the config file:

```text
~/.config/tuip/
  config.yaml
  logs/tuip.jsonl
  cache/status-cache.json
```

Each log line includes `run_id`, `pid`, and `version`. Logs rotate at 5MB and retain `tuip.1.jsonl`, `tuip.2.jsonl`, and `tuip.3.jsonl`.

## Recommended next work

Near-term polish:

- Show per-card cache age/freshness in the TUI.
- Add a small diagnostics/help screen or README troubleshooting examples for reading logs.
- Consider a `tuip cache clear` command if cache debugging becomes common.
- Split `internal/tui/tui.go` into smaller files by responsibility.
- Add provider fixture tests for more expanded providers.
- Add screenshots or terminal recordings to the README.

Possible future features:

- Configurable cache TTL and retry/concurrency settings.
- More reusable provider adapters for other status-page platforms.
- Optional historical status tracking if the project grows beyond “latest status” snapshots.
- Optional auto-refresh in the TUI, with clear cache/refresh indicators.
