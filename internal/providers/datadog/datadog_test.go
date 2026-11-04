package datadog

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ikan31/tuip/internal/fetch"
	"github.com/ikan31/tuip/internal/providers"
	"github.com/ikan31/tuip/internal/status"
)

func TestProviderFetchRegionalStatus(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{
			"components": [
				{"type": "Component", "name": "APM", "status": "operational"},
				{"type": "ComponentGroup", "name": "Logs", "components": [
					{"type": "Component", "name": "Log Search", "status": "partial_outage"}
				]}
			],
			"incidents": [
				{"title": "Earlier outage", "resolved": true},
				{"title": "Search delayed", "currentStatus": "investigating", "publishedDate": "2026-11-03T18:00:00Z"}
			],
			"maintenances": [
				{"title": "Database upgrade", "currentStatus": "scheduled", "startDate": "2026-11-04T00:00:00Z"},
				{"title": "Earlier upgrade", "currentStatus": "completed"}
			]
		}`))
	}))
	defer server.Close()

	provider := NewProvider(fetch.WithHTTPClient(server.Client()), providers.Metadata{ID: "datadog-us1", APIURL: server.URL})

	snapshot, err := provider.Fetch(t.Context())
	if err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}

	if snapshot.State != status.StatePartialOutage {
		t.Fatalf("state = %q, want %q", snapshot.State, status.StatePartialOutage)
	}

	if len(snapshot.Components) != 2 || snapshot.Components[1].Group != "Logs" {
		t.Fatalf("components = %+v, want APM and grouped Log Search", snapshot.Components)
	}

	if len(snapshot.Incidents) != 2 {
		t.Fatalf("incidents = %+v, want the active incident and upcoming maintenance", snapshot.Incidents)
	}

	incident, maintenance := snapshot.Incidents[0], snapshot.Incidents[1]
	if incident.Name != "Search delayed" || incident.StartedAt == nil {
		t.Fatalf("incident = %+v, want the active incident with a start time", incident)
	}

	if maintenance.Kind != "maintenance" || maintenance.ScheduledFor == nil {
		t.Fatalf("maintenance = %+v, want upcoming maintenance with a scheduled start time", maintenance)
	}
}
