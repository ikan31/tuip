package pagerdutystatus

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ikan31/tuip/internal/fetch"
	"github.com/ikan31/tuip/internal/status"
)

func TestMapHeadline(t *testing.T) {
	t.Parallel()

	tests := map[string]status.State{
		"All Systems Operational":         status.StateOperational,
		"Everything is running smoothly":  status.StateOperational,
		"Everything is paddling smoothly": status.StateOperational,
		"Scheduled Maintenance":           status.StateMaintenance,
		"Partial System Outage":           status.StatePartialOutage,
		"Degraded Performance":            status.StateDegraded,
		"Major Service Outage":            status.StateMajorOutage,
		"":                                status.StateUnknown,
		"Something surprising":            status.StateUnknown,
	}
	for input, want := range tests {
		t.Run(input, func(t *testing.T) {
			t.Parallel()

			if got := MapHeadline(input); got != want {
				t.Fatalf("MapHeadline(%q) = %q, want %q", input, got, want)
			}
		})
	}
}

func TestProviderFetch(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch r.URL.Path {
		case "/api/data":
			_, _ = w.Write([]byte(`{
				"layout": {
					"layout_settings": {
						"name": "GitHub Enterprise Cloud - Australia",
						"statusPage": {
							"globalStatusHeadline": "All Systems Operational"
						},
						"business_services": [
							{"id": "GROUP1", "name": "Core", "grouping_element": true},
							{"id": "BS1", "status_page_service_id": "SVC1", "displayName": "API Requests", "name": "prod-au - API Requests", "Head_ID": "GROUP1"}
						]
					}
				}
			}`))
		case "/api/post_enums":
			_, _ = w.Write([]byte(`{"post_enums": []}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	provider := NewProvider(fetch.NewClient(5*time.Second), Options{
		ID:          "github-enterprise-cloud-au",
		Name:        "GitHub Enterprise Cloud - Australia",
		Description: "GitHub Enterprise Cloud Australia regional status",
		SourceURL:   server.URL,
		APIURL:      server.URL + "/api/data",
		DataURL:     server.URL + "/api/data",
	})

	snapshot, err := provider.Fetch(context.Background())
	if err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}

	if snapshot.ProviderID != "github-enterprise-cloud-au" {
		t.Fatalf("ProviderID = %q", snapshot.ProviderID)
	}

	if snapshot.Name != "GitHub Enterprise Cloud - Australia" {
		t.Fatalf("Name = %q", snapshot.Name)
	}

	if snapshot.State != status.StateOperational {
		t.Fatalf("State = %q, want %q", snapshot.State, status.StateOperational)
	}

	if len(snapshot.Components) != 1 {
		t.Fatalf("Components len = %d, want 1", len(snapshot.Components))
	}

	component := snapshot.Components[0]
	if component.Name != "API Requests" || component.Group != "Core" || component.State != status.StateOperational {
		t.Fatalf("component = %#v, want operational API Requests in Core", component)
	}
}

func TestProviderFetchMapsActivePostsAndImpacts(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch r.URL.Path {
		case "/api/data":
			_, _ = w.Write([]byte(`{
				"layout": {
					"layout_settings": {
						"name": "PagerDuty Status Page",
						"statusPage": {
							"globalStatusHeadline": "Major Service Outage"
						},
						"business_services": [
							{"id": "GROUP1", "name": "APIs", "grouping_element": true},
							{"id": "BS1", "status_page_service_id": "SVC1", "summary": "REST API (US)", "name": "REST API (US)", "Head_ID": "GROUP1"}
						]
					}
				}
			}`))
		case "/api/post_enums":
			_, _ = w.Write([]byte(`{
				"post_enums": [
					{"id": "ST_INV", "name": "investigating", "post_type": "incident", "post_enum_type": "status"},
					{"id": "SEV_MAJOR", "name": "major", "post_type": "incident", "post_enum_type": "severity"},
					{"id": "IMP_OUTAGE", "name": "outage", "post_type": "incident", "post_enum_type": "impacts"}
				]
			}`))
		case "/api/posts":
			_, _ = w.Write([]byte(`{
				"posts": [
					{
						"id": "POST1",
						"title": "REST API disruption",
						"post_type": "incident",
						"starts_at": "2026-06-16T02:00:00Z",
						"first_update_at": "2026-06-16T02:01:00Z",
						"latest_update": {
							"reported_at": "2026-06-16T02:05:00Z",
							"message": "<p>Investigating REST API errors</p>",
							"status_id": "ST_INV",
							"severity_id": "SEV_MAJOR",
							"impacts": [
								{"service_id": "SVC1", "severity_id": "IMP_OUTAGE"}
							]
						}
					}
				]
			}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	provider := NewProvider(fetch.NewClient(5*time.Second), Options{
		ID:        "pagerduty",
		Name:      "PagerDuty",
		SourceURL: server.URL,
		APIURL:    server.URL + "/api/data",
		DataURL:   server.URL + "/api/data",
	})

	snapshot, err := provider.Fetch(context.Background())
	if err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}

	if len(snapshot.Incidents) != 1 {
		t.Fatalf("Incidents len = %d, want 1", len(snapshot.Incidents))
	}

	incident := snapshot.Incidents[0]
	if incident.Name != "REST API disruption" || incident.Status != "investigating" || incident.Impact != "major" {
		t.Fatalf("incident = %#v", incident)
	}

	if incident.Summary != "Investigating REST API errors" {
		t.Fatalf("incident summary = %q", incident.Summary)
	}

	if len(snapshot.Components) != 1 {
		t.Fatalf("Components len = %d, want 1", len(snapshot.Components))
	}

	component := snapshot.Components[0]
	if component.Name != "REST API (US)" || component.Group != "APIs" || component.Status != "outage" || component.State != status.StateMajorOutage {
		t.Fatalf("component = %#v, want REST API outage impact", component)
	}
}
