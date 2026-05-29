package pagerdutystatus

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/tuipcli/tuip/internal/fetch"
	"github.com/tuipcli/tuip/internal/status"
)

func TestMapHeadline(t *testing.T) {
	t.Parallel()

	tests := map[string]status.State{
		"All Systems Operational": status.StateOperational,
		"Scheduled Maintenance":   status.StateMaintenance,
		"Partial System Outage":   status.StatePartialOutage,
		"Degraded Performance":    status.StateDegraded,
		"Major Service Outage":    status.StateMajorOutage,
		"":                        status.StateUnknown,
		"Something surprising":    status.StateUnknown,
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
		_, _ = w.Write([]byte(`{
			"layout": {
				"layout_settings": {
					"name": "GitHub Enterprise Cloud - Australia",
					"statusPage": {
						"globalStatusHeadline": "All Systems Operational"
					}
				}
			}
		}`))
	}))
	t.Cleanup(server.Close)

	provider := NewProvider(fetch.NewClient(5*time.Second), Options{
		ID:          "github-enterprise-cloud-au",
		Name:        "GitHub Enterprise Cloud - Australia",
		Description: "GitHub Enterprise Cloud Australia regional status",
		SourceURL:   server.URL,
		APIURL:      server.URL + "/api/data",
		DataURL:     server.URL,
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
}
