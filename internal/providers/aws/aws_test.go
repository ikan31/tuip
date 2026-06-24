package aws

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
		{name: "service disruption", value: "Service disruption: Increased Error Rates", want: status.StateMajorOutage},
		{name: "service impact", value: "Service impact: Increased Connectivity Issues", want: status.StateDegraded},
		{name: "maintenance", value: "Scheduled maintenance for EC2", want: status.StateMaintenance},
		{name: "unknown", value: "", want: status.StateUnknown},
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

func TestProviderFetchDeduplicatesAWSRSSUpdates(t *testing.T) {
	t.Parallel()

	provider := providerWithRSS(t, `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0">
  <channel>
    <title><![CDATA[Amazon Web Services Service Status]]></title>
    <link>https://status.aws.amazon.com/</link>
    <lastBuildDate>Tue, 23 Jun 2026 20:21:35 PDT</lastBuildDate>
    <description><![CDATA[AWS service status.]]></description>
    <item>
      <title><![CDATA[Service disruption: Increased Error Rates]]></title>
      <link>https://status.aws.amazon.com/</link>
      <pubDate>Thu, 30 Apr 2026 00:25:54 PDT</pubDate>
      <guid isPermaLink="false">https://status.aws.amazon.com/#multipleservices-me-central-1_1777533954</guid>
      <description><![CDATA[We are providing an update on the ongoing service disruption. The Middle East (UAE) Region (ME-CENTRAL-1) is currently unable to reliably support customer applications.]]></description>
    </item>
    <item>
      <title><![CDATA[Service disruption: Increased Error Rates]]></title>
      <link>https://status.aws.amazon.com/</link>
      <pubDate>Tue, 03 Mar 2026 08:14:45 PST</pubDate>
      <guid isPermaLink="false">https://status.aws.amazon.com/#multipleservices-me-central-1_1772554485</guid>
      <description><![CDATA[Older update for the same event.]]></description>
    </item>
    <item>
      <title><![CDATA[Service impact: Increased Connectivity Issues and API Error Rates]]></title>
      <link>https://status.aws.amazon.com/</link>
      <pubDate>Thu, 30 Apr 2026 00:07:11 PDT</pubDate>
      <guid isPermaLink="false">https://status.aws.amazon.com/#multipleservices-me-south-1_1777532831</guid>
      <description><![CDATA[We are providing an update on the ongoing service impact in ME-SOUTH-1.]]></description>
    </item>
  </channel>
</rss>`)

	snapshot, err := provider.Fetch(context.Background())
	if err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}

	if snapshot.ProviderID != "aws" {
		t.Fatalf("ProviderID = %q", snapshot.ProviderID)
	}

	if snapshot.State != status.StateMajorOutage {
		t.Fatalf("State = %q, want %q", snapshot.State, status.StateMajorOutage)
	}

	if snapshot.Summary != "2 AWS service health events" {
		t.Fatalf("Summary = %q", snapshot.Summary)
	}

	if snapshot.UpdatedAt == nil {
		t.Fatalf("UpdatedAt is nil")
	}

	if len(snapshot.Incidents) != 2 {
		t.Fatalf("incidents len = %d, want 2", len(snapshot.Incidents))
	}

	if len(snapshot.Components) != 2 {
		t.Fatalf("components len = %d, want 2", len(snapshot.Components))
	}

	if snapshot.Components[0].Name != "Multiple services" || snapshot.Components[0].Group != "me-central-1" {
		t.Fatalf("component[0] = %#v", snapshot.Components[0])
	}

	if snapshot.Components[1].Name != "Multiple services" || snapshot.Components[1].Group != "me-south-1" {
		t.Fatalf("component[1] = %#v", snapshot.Components[1])
	}
}

func TestProviderFetchNoEventsFixture(t *testing.T) {
	t.Parallel()

	provider := providerWithRSS(t, `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0">
  <channel>
    <title><![CDATA[Amazon Web Services Service Status]]></title>
    <link>https://status.aws.amazon.com/</link>
    <lastBuildDate>Tue, 23 Jun 2026 20:21:35 PDT</lastBuildDate>
    <description><![CDATA[AWS service status.]]></description>
  </channel>
</rss>`)

	snapshot, err := provider.Fetch(context.Background())
	if err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}

	if snapshot.State != status.StateOperational {
		t.Fatalf("State = %q, want %q", snapshot.State, status.StateOperational)
	}

	if snapshot.Summary != "No active AWS service health events" {
		t.Fatalf("Summary = %q", snapshot.Summary)
	}

	if len(snapshot.Incidents) != 0 {
		t.Fatalf("incidents len = %d, want 0", len(snapshot.Incidents))
	}
}

func TestComponentFromEventKey(t *testing.T) {
	t.Parallel()

	name, group := componentFromEventKey("ec2-us-east-1")
	if name != "EC2" || group != "us-east-1" {
		t.Fatalf("componentFromEventKey(ec2-us-east-1) = %q, %q", name, group)
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
