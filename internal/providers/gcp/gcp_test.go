package gcp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ikan31/tuip/internal/fetch"
	"github.com/ikan31/tuip/internal/status"
)

func TestMapImpact(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		value string
		want  status.State
	}{
		{name: "available", value: "AVAILABLE", want: status.StateOperational},
		{name: "maintenance", value: "SERVICE_MAINTENANCE", want: status.StateMaintenance},
		{name: "disruption", value: "SERVICE_DISRUPTION", want: status.StateDegraded},
		{name: "outage", value: "SERVICE_OUTAGE", want: status.StateMajorOutage},
		{name: "unknown", value: "", want: status.StateUnknown},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := MapImpact(tt.value); got != tt.want {
				t.Fatalf("MapImpact(%q) = %q, want %q", tt.value, got, tt.want)
			}
		})
	}
}

func TestProviderFetchActiveIncidentFixture(t *testing.T) {
	t.Parallel()

	provider := providerWithJSON(t, `[
  {
    "id": "5fGQt4VbkDnr3Yp8PXPr",
    "begin": "2026-06-05T07:00:00+00:00",
    "created": "2026-06-10T01:13:50+00:00",
    "modified": "2026-06-23T22:52:49+00:00",
    "external_desc": "Network traffic to Google Cloud is experiencing intermittent periods of elevated latency and possible packet loss.",
    "most_recent_update": {
      "modified": "2026-06-23T22:52:50+00:00",
      "when": "2026-06-23T22:52:49+00:00",
      "text": "**Summary**\nNetwork traffic to Google Cloud is experiencing intermittent periods of elevated latency.",
      "status": "SERVICE_DISRUPTION"
    },
    "status_impact": "SERVICE_DISRUPTION",
    "severity": "medium",
    "affected_products": [
      {"title": "Hybrid Connectivity", "id": "5x6CGnZvSHQZ26KtxpK1"},
      {"title": "Media CDN", "id": "FK8WX6iZ3FuQL6qUwski"}
    ],
    "currently_affected_locations": [
      {"title": "Delhi (asia-south2)", "id": "asia-south2"},
      {"title": "Global", "id": "global"}
    ],
    "uri": "incidents/5fGQt4VbkDnr3Yp8PXPr"
  },
  {
    "id": "resolved",
    "begin": "2026-02-27T13:00:00+00:00",
    "end": "2026-02-27T14:35:00+00:00",
    "external_desc": "Resolved incident.",
    "most_recent_update": {"status": "AVAILABLE"},
    "status_impact": "SERVICE_INFORMATION",
    "severity": "low"
  }
]`)

	snapshot, err := provider.Fetch(context.Background())
	if err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}

	if snapshot.ProviderID != "gcp" {
		t.Fatalf("ProviderID = %q", snapshot.ProviderID)
	}

	if snapshot.State != status.StateDegraded {
		t.Fatalf("State = %q, want %q", snapshot.State, status.StateDegraded)
	}

	if snapshot.Summary != "1 active Google Cloud incident" {
		t.Fatalf("Summary = %q", snapshot.Summary)
	}

	if snapshot.UpdatedAt == nil {
		t.Fatalf("UpdatedAt is nil")
	}

	if len(snapshot.Incidents) != 1 {
		t.Fatalf("incidents len = %d, want 1", len(snapshot.Incidents))
	}

	incident := snapshot.Incidents[0]
	if incident.Status != "SERVICE_DISRUPTION" || incident.Impact != "medium" {
		t.Fatalf("incident status/impact = %q/%q", incident.Status, incident.Impact)
	}

	if incident.URL != "https://status.cloud.google.com/incidents/5fGQt4VbkDnr3Yp8PXPr" {
		t.Fatalf("incident URL = %q", incident.URL)
	}

	if len(snapshot.Components) != 2 {
		t.Fatalf("components len = %d, want 2", len(snapshot.Components))
	}

	if snapshot.Components[0].Name != "Hybrid Connectivity" || snapshot.Components[0].State != status.StateDegraded {
		t.Fatalf("component[0] = %#v", snapshot.Components[0])
	}

	if snapshot.Components[0].Group != "Delhi (asia-south2), Global" {
		t.Fatalf("component[0] Group = %q", snapshot.Components[0].Group)
	}
}

func TestProviderFetchNoActiveIncidentsFixture(t *testing.T) {
	t.Parallel()

	provider := providerWithJSON(t, `[
  {
    "id": "resolved",
    "begin": "2026-02-27T13:00:00+00:00",
    "end": "2026-02-27T14:35:00+00:00",
    "external_desc": "Resolved incident.",
    "most_recent_update": {"status": "AVAILABLE"},
    "status_impact": "SERVICE_INFORMATION",
    "severity": "low"
  }
]`)

	snapshot, err := provider.Fetch(context.Background())
	if err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}

	if snapshot.State != status.StateOperational {
		t.Fatalf("State = %q, want %q", snapshot.State, status.StateOperational)
	}

	if snapshot.Summary != "No active Google Cloud incidents" {
		t.Fatalf("Summary = %q", snapshot.Summary)
	}

	if len(snapshot.Incidents) != 0 {
		t.Fatalf("incidents len = %d, want 0", len(snapshot.Incidents))
	}
}

func providerWithJSON(t *testing.T, data string) *Provider {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(data))
	}))
	t.Cleanup(server.Close)

	return NewWithEndpoint(fetch.NewClient(5*time.Second), server.URL, server.URL)
}
