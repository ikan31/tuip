package docker

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ikan31/tuip/internal/fetch"
	"github.com/ikan31/tuip/internal/status"
)

func TestProviderFetchIgnoresResolvedHistoryItems(t *testing.T) {
	t.Parallel()

	provider := providerWithRSS(t, `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0">
  <channel>
    <title><![CDATA[Docker Systems Status Page]]></title>
    <lastBuildDate>Wed, 24 Jun 2026 03:30:58 GMT</lastBuildDate>
    <item>
      <title><![CDATA[Customers are experiencing Image push webhooks not firing as expected]]></title>
      <description><![CDATA[<small>June 4, 2026 05:15 PDT</small><br /><b>Investigating</b> - We are aware of the issue and investigating<br /><br /><small>June 4, 2026 06:43 PDT</small><br /><b>Resolved</b> - The issue is now resolved<br /><br />]]></description>
      <link>https://status.io/pages/incident/533c6539221ae15e3f000031/6a216c468614df05b0372d0d</link>
      <guid isPermaLink="false">6a216c468614df05b0372d0d</guid>
      <pubDate>Thu, 04 Jun 2026 13:43:08 GMT</pubDate>
    </item>
    <item>
      <title><![CDATA[Scheduled Database Maintenance]]></title>
      <description><![CDATA[<small>May 28, 2026 07:00 PDT</small><br /><b>Active</b> - Scheduled maintenance is now underway.<br /><br /><small>May 28, 2026 09:00 PDT</small><br /><b>Completed</b> - Scheduled maintenance has been completed successfully.<br /><br />]]></description>
      <link>https://status.io/pages/maintenance/533c6539221ae15e3f000031/6a0ec04beac9dc05ffa1f136</link>
      <guid isPermaLink="false">6a0ec04beac9dc05ffa1f136</guid>
      <pubDate>Thu, 28 May 2026 16:00:05 GMT</pubDate>
    </item>
  </channel>
</rss>`)

	snapshot, err := provider.Fetch(context.Background())
	if err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}

	if snapshot.ProviderID != "docker" {
		t.Fatalf("ProviderID = %q", snapshot.ProviderID)
	}

	if snapshot.State != status.StateOperational {
		t.Fatalf("State = %q, want %q", snapshot.State, status.StateOperational)
	}

	if snapshot.Summary != "No active Docker status events" {
		t.Fatalf("Summary = %q", snapshot.Summary)
	}

	if len(snapshot.Incidents) != 0 {
		t.Fatalf("incidents len = %d, want 0", len(snapshot.Incidents))
	}
}

func TestProviderFetchActiveIncidentFixture(t *testing.T) {
	t.Parallel()

	provider := providerWithRSS(t, `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0">
  <channel>
    <title><![CDATA[Docker Systems Status Page]]></title>
    <lastBuildDate>Wed, 24 Jun 2026 03:30:58 GMT</lastBuildDate>
    <item>
      <title><![CDATA[Degraded Hub performance]]></title>
      <description><![CDATA[<small>April 20, 2026 01:48 PDT</small><br /><b>Investigating</b> - We are investigating latency issues for Hub<br /><br /><small>April 20, 2026 02:50 PDT</small><br /><b>Monitoring</b> - Docker Hub performance is improving.<br /><br />]]></description>
      <link>https://status.io/pages/incident/533c6539221ae15e3f000031/69e5e86ffcdb9205f3a36fa8</link>
      <guid isPermaLink="false">69e5e86ffcdb9205f3a36fa8</guid>
      <pubDate>Tue, 21 Apr 2026 14:32:19 GMT</pubDate>
    </item>
  </channel>
</rss>`)

	snapshot, err := provider.Fetch(context.Background())
	if err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}

	if snapshot.State != status.StateDegraded {
		t.Fatalf("State = %q, want %q", snapshot.State, status.StateDegraded)
	}

	if snapshot.Summary != "1 active Docker status event" {
		t.Fatalf("Summary = %q", snapshot.Summary)
	}

	if len(snapshot.Incidents) != 1 {
		t.Fatalf("incidents len = %d, want 1", len(snapshot.Incidents))
	}

	incident := snapshot.Incidents[0]
	if incident.Name != "Degraded Hub performance" {
		t.Fatalf("incident name = %q", incident.Name)
	}

	if incident.Status != "Monitoring" || incident.Impact != "degraded" {
		t.Fatalf("incident status/impact = %q/%q", incident.Status, incident.Impact)
	}

	if incident.Summary != "Docker Hub performance is improving." {
		t.Fatalf("incident summary = %q", incident.Summary)
	}
}

func TestLatestUpdate(t *testing.T) {
	t.Parallel()

	update := latestUpdate(`<small>June 4, 2026 05:15 PDT</small><br /><b>Investigating</b> - We are aware of the issue<br /><br /><small>June 4, 2026 06:43 PDT</small><br /><b>Resolved</b> - The issue is now resolved<br /><br />`)
	if update.Status != "Resolved" || update.Text != "The issue is now resolved" {
		t.Fatalf("latestUpdate() = %#v", update)
	}
}

func providerWithRSS(t *testing.T, data string) *Provider {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/xml; charset=utf-8")
		_, _ = w.Write([]byte(data))
	}))
	t.Cleanup(server.Close)

	return NewWithEndpoint(fetch.NewClient(5*time.Second), server.URL, server.URL)
}
