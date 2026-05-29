package slack

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/tuipcli/tuip/internal/fetch"
	"github.com/tuipcli/tuip/internal/status"
)

func TestMapStatus(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                string
		apiStatus           string
		activeIncidentCount int
		want                status.State
	}{
		{name: "ok", apiStatus: "ok", want: status.StateOperational},
		{name: "active incident overrides ok", apiStatus: "ok", activeIncidentCount: 1, want: status.StateDegraded},
		{name: "maintenance", apiStatus: "maintenance", want: status.StateMaintenance},
		{name: "unknown empty", apiStatus: "", want: status.StateUnknown},
		{name: "unexpected", apiStatus: "bad", want: status.StateDegraded},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := MapStatus(tt.apiStatus, tt.activeIncidentCount); got != tt.want {
				t.Fatalf("MapStatus(%q, %d) = %q, want %q", tt.apiStatus, tt.activeIncidentCount, got, tt.want)
			}
		})
	}
}

func TestProviderFetchOKFixture(t *testing.T) {
	t.Parallel()

	provider := providerWithFixture(t, "slack_current_ok.json")

	snapshot, err := provider.Fetch(context.Background())
	if err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}

	if snapshot.ProviderID != "slack" {
		t.Fatalf("ProviderID = %q", snapshot.ProviderID)
	}

	if snapshot.State != status.StateOperational {
		t.Fatalf("State = %q, want %q", snapshot.State, status.StateOperational)
	}

	if snapshot.UpdatedAt == nil {
		t.Fatalf("UpdatedAt is nil")
	}

	if len(snapshot.Incidents) != 0 {
		t.Fatalf("incidents len = %d, want 0", len(snapshot.Incidents))
	}
}

func TestParseSlackComponents(t *testing.T) {
	t.Parallel()

	page := `<div id="services">
		<div class="service header align_center"><div><p class="bold">Login/SSO</p><p class="tiny">No issues</p></div></div>
		<div class="service header align_center"><div><p class="bold">Messaging</p><p class="tiny">Incident</p></div></div>
		<div class="service header align_center"><div><p class="bold">Files</p><p class="tiny">Outage</p></div></div>
	</div>`

	components := parseSlackComponents(page)
	if len(components) != 3 {
		t.Fatalf("components len = %d, want 3", len(components))
	}

	if components[0].Name != "Login/SSO" || components[0].State != status.StateOperational {
		t.Fatalf("component[0] = %#v", components[0])
	}

	if components[1].Name != "Messaging" || components[1].State != status.StateDegraded {
		t.Fatalf("component[1] = %#v", components[1])
	}

	if components[2].Name != "Files" || components[2].State != status.StateMajorOutage {
		t.Fatalf("component[2] = %#v", components[2])
	}
}

func TestProviderFetchIncidentFixture(t *testing.T) {
	t.Parallel()

	provider := providerWithFixture(t, "slack_current_incident.json")

	snapshot, err := provider.Fetch(context.Background())
	if err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}

	if snapshot.State != status.StateDegraded {
		t.Fatalf("State = %q, want %q", snapshot.State, status.StateDegraded)
	}

	if len(snapshot.Incidents) != 1 {
		t.Fatalf("incidents len = %d, want 1", len(snapshot.Incidents))
	}

	if snapshot.Incidents[0].Name != "Messages are delayed" {
		t.Fatalf("incident name = %q", snapshot.Incidents[0].Name)
	}
}

func providerWithFixture(t *testing.T, fixtureName string) *Provider {
	t.Helper()

	// #nosec G304 -- fixtureName is controlled by tests.
	data, err := os.ReadFile(filepath.Join("..", "testdata", fixtureName))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(data)
	}))
	t.Cleanup(server.Close)

	return NewWithEndpoint(fetch.NewClient(5*time.Second), server.URL, server.URL)
}
