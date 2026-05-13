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
			if got := MapStatus(tt.apiStatus, tt.activeIncidentCount); got != tt.want {
				t.Fatalf("MapStatus(%q, %d) = %q, want %q", tt.apiStatus, tt.activeIncidentCount, got, tt.want)
			}
		})
	}
}

func TestProviderFetchOKFixture(t *testing.T) {
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

func TestProviderFetchIncidentFixture(t *testing.T) {
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
