package azure

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ikan31/tuip/internal/fetch"
	"github.com/ikan31/tuip/internal/status"
)

func TestMapEventState(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		value string
		want  status.State
	}{
		{name: "major outage", value: "Storage outage impacting multiple regions", want: status.StateMajorOutage},
		{name: "maintenance", value: "Scheduled maintenance for Azure Portal", want: status.StateMaintenance},
		{name: "degraded", value: "Customers may experience intermittent latency", want: status.StateDegraded},
		{name: "empty", value: "", want: status.StateDegraded},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := MapEventState(tt.value); got != tt.want {
				t.Fatalf("MapEventState(%q) = %q, want %q", tt.value, got, tt.want)
			}
		})
	}
}

func TestProviderFetchNoActiveEventsFixture(t *testing.T) {
	t.Parallel()

	provider := providerWithFeed(t, `<?xml version="1.0" encoding="utf-8"?>
<rss version="2.0">
  <channel>
    <title>Azure Status</title>
    <link>https://azure.status.microsoft/en-us/status/</link>
    <description>Azure Status</description>
    <language>en-us</language>
    <lastBuildDate>Wed, 24 Jun 2026 03:01:00 Z</lastBuildDate>
  </channel>
</rss>`)

	snapshot, err := provider.Fetch(context.Background())
	if err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}

	if snapshot.ProviderID != "azure" {
		t.Fatalf("ProviderID = %q", snapshot.ProviderID)
	}

	if snapshot.State != status.StateOperational {
		t.Fatalf("State = %q, want %q", snapshot.State, status.StateOperational)
	}

	if snapshot.Summary != "No active Azure status events" {
		t.Fatalf("Summary = %q", snapshot.Summary)
	}

	if snapshot.UpdatedAt == nil {
		t.Fatalf("UpdatedAt is nil")
	}

	if len(snapshot.Incidents) != 0 {
		t.Fatalf("incidents len = %d, want 0", len(snapshot.Incidents))
	}
}

func TestProviderFetchActiveEventFixture(t *testing.T) {
	t.Parallel()

	provider := providerWithFeed(t, `<?xml version="1.0" encoding="utf-8"?>
<rss version="2.0">
  <channel>
    <title>Azure Status</title>
    <link>https://azure.status.microsoft/en-us/status/</link>
    <description>Azure Status</description>
    <lastBuildDate>Wed, 24 Jun 2026 03:01:00 Z</lastBuildDate>
    <item>
      <title>Azure Storage - Service issue</title>
      <link>https://azure.status.microsoft/en-us/status/</link>
      <description><![CDATA[Customers may experience intermittent errors in East US.]]></description>
      <pubDate>Wed, 24 Jun 2026 02:45:00 Z</pubDate>
      <guid>https://azure.status.microsoft/incidents/example</guid>
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

	if snapshot.Summary != "1 active Azure status event" {
		t.Fatalf("Summary = %q", snapshot.Summary)
	}

	if len(snapshot.Incidents) != 1 {
		t.Fatalf("incidents len = %d, want 1", len(snapshot.Incidents))
	}

	incident := snapshot.Incidents[0]
	if incident.Name != "Azure Storage - Service issue" {
		t.Fatalf("incident name = %q", incident.Name)
	}

	if incident.Summary != "Customers may experience intermittent errors in East US." {
		t.Fatalf("incident summary = %q", incident.Summary)
	}

	if incident.UpdatedAt == nil {
		t.Fatalf("incident UpdatedAt is nil")
	}
}

func providerWithFeed(t *testing.T, data string) *Provider {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/xml; charset=utf-8")
		_, _ = w.Write([]byte(data))
	}))
	t.Cleanup(server.Close)

	return NewWithEndpoint(fetch.NewClient(5*time.Second), server.URL, server.URL)
}
