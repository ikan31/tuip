package fivetran

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ikan31/tuip/internal/fetch"
	"github.com/ikan31/tuip/internal/status"
)

func TestProviderFetchOperational(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch r.URL.Path {
		case "/status":
			_, _ = w.Write([]byte(`{
				"groups": [
					{
						"group_name": "Database connectors",
						"status": "operational",
						"updated_at": "2026-07-18T14:59:34.141152Z",
						"services": [
							{"name": "Amazon Aurora MySQL", "status": "operational"}
						]
					}
				]
			}`))
		case "/active":
			_, _ = w.Write([]byte(`{"incidents": [], "maintenances": []}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	provider := NewWithURLs(fetch.NewClient(time.Second), server.URL+"/status", server.URL+"/active")

	snapshot, err := provider.Fetch(context.Background())
	if err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}

	if snapshot.State != status.StateOperational {
		t.Fatalf("State = %q, want %q", snapshot.State, status.StateOperational)
	}

	if snapshot.Summary != "All Systems Operational" {
		t.Fatalf("Summary = %q, want All Systems Operational", snapshot.Summary)
	}

	if len(snapshot.Components) != 2 {
		t.Fatalf("Components len = %d, want 2", len(snapshot.Components))
	}

	if snapshot.Components[1].Group != "Database connectors" {
		t.Fatalf("Components[1].Group = %q, want Database connectors", snapshot.Components[1].Group)
	}

	if snapshot.UpdatedAt == nil || snapshot.UpdatedAt.Format(time.RFC3339Nano) != "2026-07-18T14:59:34.141152Z" {
		t.Fatalf("UpdatedAt = %v, want 2026-07-18T14:59:34.141152Z", snapshot.UpdatedAt)
	}
}

func TestProviderFetchActiveIncident(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch r.URL.Path {
		case "/status":
			_, _ = w.Write([]byte(`{
				"groups": [
					{
						"group_name": "System",
						"status": "operational",
						"updated_at": "2026-07-18T14:59:34Z",
						"services": []
					}
				]
			}`))
		case "/active":
			_, _ = w.Write([]byte(`{
				"incidents": [
					{
						"title": "Connection syncs delayed",
						"message": "Investigating delayed syncs.",
						"status": "investigating",
						"impact": "major",
						"created_at": "2026-08-07T12:00:00Z",
						"updated_at": "2026-08-07T12:30:00Z"
					}
				],
				"maintenances": []
			}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	provider := NewWithURLs(fetch.NewClient(time.Second), server.URL+"/status", server.URL+"/active")

	snapshot, err := provider.Fetch(context.Background())
	if err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}

	if snapshot.State != status.StatePartialOutage {
		t.Fatalf("State = %q, want %q", snapshot.State, status.StatePartialOutage)
	}

	if snapshot.Summary != "Connection syncs delayed" {
		t.Fatalf("Summary = %q, want Connection syncs delayed", snapshot.Summary)
	}

	if len(snapshot.Incidents) != 1 {
		t.Fatalf("Incidents len = %d, want 1", len(snapshot.Incidents))
	}

	incident := snapshot.Incidents[0]
	if incident.Kind != "incident" || incident.Impact != "major" || incident.Summary != "Investigating delayed syncs." {
		t.Fatalf("Incident = %#v, want mapped active incident", incident)
	}

	if snapshot.UpdatedAt == nil || snapshot.UpdatedAt.Format(time.RFC3339) != "2026-08-07T12:30:00Z" {
		t.Fatalf("UpdatedAt = %v, want 2026-08-07T12:30:00Z", snapshot.UpdatedAt)
	}
}
