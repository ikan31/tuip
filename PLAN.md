# tuip Architecture Plan

## Context

- Goal: design `tuip`, a terminal UI for aggregating SaaS status pages into shareable dashboards.
- Current repository state: Go CLI implementation exists for the approved MVP.
- The user originally wanted planning first; the approved plan has now been implemented through the CLI/config milestones.
- The final product should support a TUI launched with `tuip`, a left-side manager/search panel, and a right-side dashboard grid showing service health with colors and details.

## Approach

Recommended architecture, based on user decisions:

1. Build in **Go**.
2. Start with a **CLI-first core**. The first useful command should be similar to:

   ```bash
   tuip status slack github cloudflare
   ```

3. Treat the CLI core as the future engine for the TUI. The TUI should call the same registry, provider, fetch, normalize, and config packages rather than reimplementing status logic.
4. Implement each SaaS source as a **code provider**. YAML should configure user dashboards and shareable setups, not replace provider code for the MVP.
5. Favor a small, reliable first provider set: **Slack**, **GitHub**, and **Cloudflare**.
6. Normalize all provider outputs into one internal `StatusSnapshot` model so the CLI, TUI, JSON output, and future config/dashboard features all consume the same shape.
7. Use direct APIs where available before RSS/Atom or scraping. For the first three services:
   - Slack has a JSON status API at `https://status.slack.com/api/v2.0.0/current`.
   - GitHub uses Atlassian Statuspage-style JSON at `https://www.githubstatus.com/api/v2/summary.json` or `status.json`.
   - Cloudflare uses Atlassian Statuspage-style JSON at `https://www.cloudflarestatus.com/api/v2/summary.json` or `status.json`.
8. Defer dashboard YAML/config editing until after the first CLI provider MVP. YAML's purpose is future dashboard configuration and sharing setups with others, not first-pass provider fetching.
9. Use Bubble Tea/Lip Gloss later for the TUI so the CLI card styling and TUI styling can share concepts.

## Files to modify

Implemented files/modules:

- `PLAN.md` — current architecture/gameplan.
- `README.md` — project goals, CLI examples, status badges/screenshots later, install instructions, roadmap.
- `docs/architecture.md` — durable architecture decisions and terminology.
- `docs/provider-model.md` — provider contract, normalized status schema, source types, mapping rules.
- `docs/config.md` — future dashboard YAML format and config locations.
- `go.mod` — Go module definition once coding begins.
- `cmd/tuip/main.go` — CLI entrypoint.
- `internal/app/status.go` — orchestration for `tuip status ...`.
- `internal/status/model.go` — normalized status model shared by CLI and future TUI.
- `internal/providers/registry.go` — built-in provider registry.
- `internal/providers/provider.go` — provider interface.
- `internal/providers/slack/` — Slack provider.
- `internal/providers/statuspage/` — reusable Atlassian Statuspage client/provider helper for GitHub, Cloudflare, and many future services.
- `internal/providers/github/` — GitHub provider using the shared Statuspage helper.
- `internal/providers/cloudflare/` — Cloudflare provider using the shared Statuspage helper.
- `internal/httpclient/` or `internal/fetch/` — shared HTTP client settings: timeout, user-agent, retries later.
- `internal/output/` — colored card and JSON output formatting.
- `internal/providers/testdata/` — saved API fixtures for mapping/provider tests.
- Later:
  - `internal/config/` — YAML dashboard loading/saving.
  - `internal/tui/` — terminal UI implementation.

## Reuse

- No reusable project code exists yet.
- Design reuse from the first commit:
  - `StatusSnapshot` model reused by CLI, JSON output, future dashboard YAML, and TUI.
  - `Provider` interface reused by direct CLI lookup, dashboard config loading, search/add flows, and TUI refresh.
  - `Registry` reused by `tuip status slack github`, future `tuip providers list`, and future TUI manager/search panel.
  - Shared HTTP/fetch client reused by all providers for timeouts, user-agent, future retries, and future cache headers.
  - Shared Statuspage helper reused by GitHub, Cloudflare, and many future SaaS services hosted on Atlassian Statuspage.

## Recommended Go libraries once coding begins

- CLI command structure: `spf13/cobra` is a good fit for subcommands like `status`, `providers list`, and later `dashboards add/remove`.
- Human styling/cards: `charmbracelet/lipgloss` can render colored boxes/cards in the CLI and can also be reused with Bubble Tea later.
- TUI later: `charmbracelet/bubbletea` plus `lipgloss`.
- YAML later: `gopkg.in/yaml.v3`.
- Tests: standard library `testing`, `net/http/httptest`, and JSON fixtures should be enough initially.

## Initial architecture notes

### Main concepts

- **Provider**: Go code that knows how to fetch, parse, and normalize one SaaS status source.
- **Provider ID**: stable CLI/config identifier, e.g. `slack`, `github`, `cloudflare`.
- **Registry**: map of provider IDs to provider constructors/metadata.
- **Status snapshot**: normalized result returned by every provider, regardless of whether the source was JSON API, RSS, Atom, or HTML.
- **Incident/update item**: optional summary of active incidents or scheduled maintenance.
- **Service instance**: future dashboard config entry referencing a provider ID and optional display/settings.
- **Dashboard**: future named collection of service instances plus layout/display preferences.

### Recommended normalized status model

Each provider should return roughly:

```text
StatusSnapshot
- provider_id: slack | github | cloudflare
- name: Slack | GitHub | Cloudflare
- state: operational | degraded | partial_outage | major_outage | maintenance | unknown | error
- summary: short human-readable line from provider or mapped by tuip
- source_url: status page URL
- checked_at: local time or UTC timestamp when tuip fetched it
- updated_at: provider's last updated timestamp, if available
- incidents: zero or more active incidents/maintenance entries
- components: optional component statuses for providers that expose them
- error: provider/fetch/parse error, only when state is `error`
```

### Status mapping

Internal states:

- `operational`
- `degraded`
- `partial_outage`
- `major_outage`
- `maintenance`
- `unknown`
- `error`

For Atlassian Statuspage-style services like GitHub and Cloudflare, initial mapping can be:

- `indicator: none` -> `operational`
- `indicator: minor` -> `degraded`
- `indicator: major` -> `major_outage`
- `indicator: critical` -> `major_outage`
- active scheduled maintenance with no incident -> `maintenance` only if the overall indicator is otherwise `none`, or expose it as an incident item while keeping the top-level state operational/degraded based on the page indicator.

For Slack's custom API:

- `status: ok` -> `operational`
- non-empty `active_incidents` or non-`ok` status -> map to `degraded`, `partial_outage`, or `major_outage` depending on available API fields; start conservatively with `degraded` unless Slack exposes clearer severity.

### Source-type strategy

MVP should only use direct JSON APIs, but the architecture should leave space for more source types:

1. **Direct JSON APIs**
   - Preferred source type.
   - Most reliable when available.
   - Used by Slack, GitHub, and Cloudflare in MVP.
2. **Atlassian Statuspage JSON**
   - Treat as a reusable provider helper, not three separate one-off implementations.
   - Many SaaS vendors expose the same `/api/v2/status.json` and `/api/v2/summary.json` shape.
3. **RSS/Atom feeds**
   - Add later via a shared feed parser helper.
   - Feeds are usually incident/update timelines, not always authoritative top-level health.
   - Feed-backed providers may need provider-specific logic to decide whether latest entries represent active incidents, resolved incidents, or only history.
4. **Webhooks**
   - Not a fit for the first CLI-only MVP because a normal CLI command is not a long-running receiver.
   - Consider later only if `tuip` grows a daemon/local cache, e.g. `tuip daemon`, that receives webhook events and stores local status snapshots for the TUI.
5. **HTML scraping**
   - Last resort only.
   - Use provider-specific code with fixtures/tests because status page HTML is brittle.
   - Prefer APIs, Statuspage JSON, RSS, or Atom before scraping.

### CLI command/output contract

First MVP commands:

```bash
tuip status slack github cloudflare
tuip status --details slack github cloudflare
tuip status --json slack github cloudflare
tuip providers list
```

Human output should be colored card/box style. Default card content should stay concise:

```text
Slack
State: Operational
Summary: All systems operational
Checked: 2026-05-13T21:35:00Z
```

`--details` can include active incidents, scheduled maintenance, and component details when the provider exposes them.

`--json` should output the same normalized schema regardless of provider source. Rough shape:

```json
{
  "checked_at": "2026-05-13T21:35:00Z",
  "results": [
    {
      "provider_id": "github",
      "name": "GitHub",
      "state": "operational",
      "summary": "All Systems Operational",
      "source_url": "https://www.githubstatus.com/",
      "checked_at": "2026-05-13T21:35:00Z",
      "updated_at": "2026-05-13T17:03:51Z",
      "incidents": [],
      "components": []
    }
  ]
}
```

### CLI execution/error model

- Fetch requested providers concurrently.
- Use a 5s per-provider timeout.
- Unknown provider IDs should fail fast before network calls.
- If one provider times out or returns invalid data, show that provider as `error` in human/JSON output and exit non-zero because tuip failed to check everything requested.
- If a provider reports degraded/outage status successfully, show the degraded state but exit `0` by default.
- Add `--fail-on-degraded` later for automation that wants non-operational statuses to produce non-zero exit codes.

### MVP phases

1. **CLI provider MVP**
   - `tuip status slack github cloudflare`
   - Fetch providers concurrently.
   - Print one concise row/card per provider.
   - No YAML required yet.
2. **CLI polish MVP**
   - Add `--json` output for scriptability and tests.
   - Add `--details` output for active incidents/scheduled maintenance/components.
   - Add basic timeout handling.
   - Add clear handling for unknown provider IDs.
   - Add `tuip providers list`.
3. **Dashboard config MVP**
   - Add YAML dashboards after status fetching works.
   - YAML is for user dashboard configuration and shareable setups.
   - First config can simply list provider IDs.
   - Include CLI commands to create/list/show/use dashboards and add/remove services.
   - Recommended future UX:
     - `tuip status slack github` checks explicitly provided providers and does not need config.
     - `tuip status` checks the configured default dashboard once config exists.
     - `tuip status --dashboard work` checks a named dashboard.
     - `tuip dashboards list` lists configured dashboards later.
     - `tuip dashboards use work` sets the default dashboard later.
     - `tuip dashboards add work slack github` adds providers to a dashboard later.
     - `tuip dashboards remove work github` removes providers from a dashboard later.
4. **TUI MVP**
   - Reuse the core status engine.
   - Render dashboard grid.
   - Add manual refresh first; optional auto-refresh later.
5. **Manager UX**
   - Search providers, add/remove services, switch dashboards.
   - Write YAML config from TUI/CLI.
6. **Provider expansion**
   - Add RSS/Atom-backed providers.
   - Add HTML scraping only when no API/feed exists.
   - Prefer reusable helpers for common status page platforms.

## Steps

### Planning steps

- [x] Confirm language/runtime choice: Go.
- [x] Confirm interface order: CLI first, future TUI wraps the same core.
- [x] Confirm initial providers: Slack, GitHub, Cloudflare.
- [x] Confirm MVP priority: small set implemented reliably.
- [x] Confirm provider/config split: Go provider code does the work; YAML config is for shareable user dashboards later.
- [x] Confirm first command idea: `tuip status slack github`.
- [x] Decide default CLI output shape: colored box/card style for humans, standardized `--json` for machines.
- [x] Decide exit code behavior: degraded services do not mean tuip failed; default non-zero exit is reserved for runtime/tool errors, with future `--fail-on-degraded` for automation.
- [x] Decide detail level: default shows overall service status only; an option will show active incidents/scheduled maintenance/components.
- [x] Decide initial timeout/concurrency behavior: fetch providers concurrently with a 5s per-provider timeout.
- [x] Decide first release command set: include `tuip status <provider...>` and `tuip providers list`.
- [x] Decide YAML timing: defer config/dashboard YAML until after the provider/CLI MVP.
- [x] Decide dashboard/config UX direction: `tuip status` uses default dashboard later; `tuip status --dashboard <name>` uses a named dashboard; explicit providers always work without config.
- [x] Decide post-MVP config behavior: include commands to create/list/show/use dashboards and add/remove services.
- [x] Recommend future TUI framework: Bubble Tea with Lip Gloss, after CLI core stabilizes.

### Future implementation checklist

- [x] Initialize Go module.
- [x] Create normalized status model.
- [x] Create provider interface.
- [x] Create provider registry.
- [x] Create shared HTTP fetch client with 5s timeout and user-agent.
- [x] Implement Slack provider using Slack's current status API.
- [x] Implement reusable Statuspage helper.
- [x] Implement GitHub provider using Statuspage helper.
- [x] Implement Cloudflare provider using Statuspage helper.
- [x] Implement `tuip status <providers...>` orchestration with concurrent provider fetches.
- [x] Implement colored box/card human output.
- [x] Implement `--json` standardized output for tests/scriptability.
- [x] Implement optional `--details` flag to show active incidents/scheduled maintenance/components.
- [x] Implement `tuip providers list`.
- [x] Add unit tests for status mappings.
- [x] Add provider fixture tests using saved API responses.
- [x] Add README usage examples.

### Post-MVP config/dashboard checklist

- [x] Create YAML config model.
- [x] Load config from default user config path.
- [x] Add `--config` override.
- [x] Implement `tuip status` against default dashboard.
- [x] Implement `tuip status --dashboard <name>`.
- [x] Implement `tuip dashboards create <name>`.
- [x] Implement `tuip dashboards list`.
- [x] Implement `tuip dashboards show <name>`.
- [x] Implement `tuip dashboards use <name>`.
- [x] Implement `tuip dashboards add <name> <provider...>`.
- [x] Implement `tuip dashboards remove <name> <provider...>`.
- [x] Validate provider IDs when writing dashboard config.

## Verification

Planning verification:

- The MVP can be described as: "a Go CLI that fetches Slack, GitHub, and Cloudflare status through provider code and prints normalized statuses."
- The first command and expected output are agreed before coding:
  - `tuip status slack github cloudflare` -> colored card output.
  - `tuip status --json slack github cloudflare` -> standardized JSON output.
  - `tuip status --details slack` -> includes incident/maintenance/component detail.
  - `tuip providers list` -> lists built-in providers.
- The provider contract is concrete enough that Slack, GitHub, and Cloudflare can all implement it.
- Failure behavior is agreed: unknown provider, network timeout, and parse errors are tuip failures; degraded upstream SaaS status is not a tuip failure by default.
- There is a clear path from CLI core to future YAML dashboards and TUI.

Future implementation verification:

- Unit tests for Statuspage indicator -> internal state mapping.
- Unit tests for Slack status -> internal state mapping.
- Provider fixture tests using saved Slack/GitHub/Cloudflare API responses.
- CLI smoke test: `tuip status slack github cloudflare` prints three statuses.
- CLI JSON test: `tuip status --json slack github cloudflare` emits stable structured output.
- Manual network test with timeout/failure handling.
- Later config tests for creating dashboards, adding/removing providers, default dashboard selection, and YAML round-tripping.
- Later TUI manual test with simulated slow/failing providers.

## Source discovery notes

Checked live endpoints during planning:

- Slack `https://status.slack.com/api/v2.0.0/current`
  - Returns JSON with fields like `status`, `date_created`, `date_updated`, `active_incidents`.
  - `history` endpoint exists for recent incident history.
- GitHub `https://www.githubstatus.com/api/v2/status.json`
  - Returns Statuspage-style `status.indicator` and `status.description`.
- GitHub `https://www.githubstatus.com/api/v2/summary.json`
  - Returns page, components, incidents, scheduled maintenance, and status.
- GitHub Enterprise Cloud regional pages:
  - EU exposes Statuspage JSON at `https://eu.githubstatus.com/api/v2/summary.json`.
  - Australia, Japan, and US expose PagerDuty status-page JSON at `/api/data`.
  - Registered provider IDs are `github-enterprise-cloud-au`, `github-enterprise-cloud-eu`, `github-enterprise-cloud-jp`, and `github-enterprise-cloud-us`.
  - Short aliases are available as `github-au`, `github-eu`, `github-jp`, `github-us`, and `ghec-*` equivalents.
- Slack regional note:
  - Slack exposes one official current status API. Incidents may mention affected regions, but there are no separate official regional provider pages to register yet.
- Cloudflare regional note:
  - Cloudflare exposes regional/datacenter state as components on the main Cloudflare status page rather than separate regional status pages.
- Cloudflare `https://www.cloudflarestatus.com/api/v2/status.json`
  - Returns Statuspage-style `status.indicator` and `status.description`.
- Cloudflare `https://www.cloudflarestatus.com/api/v2/summary.json`
  - Returns page, components, incidents, scheduled maintenance, and status.

## Recommended future dashboard/config UX

Config should be added after the initial provider/CLI MVP. Recommended command behavior:

- `tuip status slack github cloudflare`
  - Explicit provider check.
  - Does not require config.
  - This is the first MVP behavior.
- `tuip status`
  - Later, once config exists, checks the configured default dashboard.
  - If no config/default dashboard exists, show a helpful message explaining how to pass providers directly or create a dashboard.
- `tuip status --dashboard work`
  - Checks a named dashboard from YAML config.
- `tuip status --details --dashboard work`
  - Checks a named dashboard and includes incidents/maintenance/components.
- `tuip dashboards list`
  - Later command to list dashboards.
- `tuip dashboards use work`
  - Later command to set the default dashboard.

This keeps the MVP simple while giving the future TUI a clean model: the TUI starts from the default dashboard, and its manager panel edits the same YAML config.

## Locked dashboard/config decisions

- The future dashboard/config UX above is the intended direction.
- YAML config is deferred until after the first CLI/provider MVP.
- When config is added, it should include both YAML loading and CLI commands to add/remove services, not read-only YAML loading only.
- Recommended future commands:
  - `tuip dashboards create work`
  - `tuip dashboards list`
  - `tuip dashboards show work`
  - `tuip dashboards use work`
  - `tuip dashboards add work slack github cloudflare`
  - `tuip dashboards remove work github`

### Future YAML config sketch

```yaml
version: 1
default_dashboard: work

dashboards:
  work:
    services:
      - provider: slack
      - provider: github
      - provider: cloudflare
  personal:
    services:
      - provider: github
```

Recommended config location once implemented:

- Use `~/.config/tuip/config.yaml` by default on macOS/Linux because tuip is a terminal-first developer tool.
- Honor `$XDG_CONFIG_HOME/tuip/config.yaml` when `XDG_CONFIG_HOME` is set.
- Use Go's `os.UserConfigDir()` on Windows.
- Support `--config /path/to/config.yaml` for sharing/testing/custom setups.


## Exit code decision notes

There are two different concepts that can both be called "failure":

1. **tuip failed to do its job**
   - Unknown provider ID.
   - Network request timed out.
   - Provider returned invalid JSON.
   - Config file could not be read.
   - These should almost always exit non-zero because the command itself failed.

2. **tuip succeeded, but a service is unhealthy**
   - GitHub reports degraded performance.
   - Cloudflare reports a partial outage.
   - Slack has an active incident.
   - The command successfully fetched the truth, so this can reasonably exit `0` by default.

Recommended behavior:

- Default: exit `0` if tuip successfully checked the requested providers, even if some providers are degraded.
- Runtime/tool errors: exit non-zero.
- Add `--fail-on-degraded` for scripts/CI/automation that want unhealthy services to produce a non-zero exit.
- Possible later flags:
  - `--fail-on unknown,degraded,major_outage`
  - `--fail-on-severity major`

This keeps the CLI pleasant for humans while still supporting automation.
