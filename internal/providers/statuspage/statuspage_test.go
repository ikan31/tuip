package statuspage

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ikan31/tuip/internal/fetch"
	"github.com/ikan31/tuip/internal/status"
)

func TestMapIndicator(t *testing.T) {
	t.Parallel()

	tests := map[string]status.State{
		"none":        status.StateOperational,
		"minor":       status.StateDegraded,
		"major":       status.StateMajorOutage,
		"critical":    status.StateMajorOutage,
		"maintenance": status.StateMaintenance,
		"":            status.StateUnknown,
		"surprise":    status.StateUnknown,
	}
	for input, want := range tests {
		t.Run(input, func(t *testing.T) {
			t.Parallel()

			if got := MapIndicator(input); got != want {
				t.Fatalf("MapIndicator(%q) = %q, want %q", input, got, want)
			}
		})
	}
}

func TestMapComponentStatus(t *testing.T) {
	t.Parallel()

	tests := map[string]status.State{
		"operational":          status.StateOperational,
		"degraded_performance": status.StateDegraded,
		"partial_outage":       status.StatePartialOutage,
		"major_outage":         status.StateMajorOutage,
		"under_maintenance":    status.StateMaintenance,
		"weird":                status.StateUnknown,
	}
	for input, want := range tests {
		t.Run(input, func(t *testing.T) {
			t.Parallel()

			if got := MapComponentStatus(input); got != want {
				t.Fatalf("MapComponentStatus(%q) = %q, want %q", input, got, want)
			}
		})
	}
}

func TestProviderFetchGitHubFixture(t *testing.T) {
	t.Parallel()

	provider := providerWithFixture(t, "github_summary_operational.json", "github", "GitHub")

	snapshot, err := provider.Fetch(context.Background())
	if err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}

	if snapshot.ProviderID != "github" {
		t.Fatalf("ProviderID = %q", snapshot.ProviderID)
	}

	if snapshot.State != status.StateOperational {
		t.Fatalf("State = %q, want %q", snapshot.State, status.StateOperational)
	}

	if snapshot.Summary != "All Systems Operational" {
		t.Fatalf("Summary = %q", snapshot.Summary)
	}

	if len(snapshot.Components) != 2 {
		t.Fatalf("components len = %d, want 2", len(snapshot.Components))
	}
}

func TestProviderFetchCloudflareFixture(t *testing.T) {
	t.Parallel()

	provider := providerWithFixture(t, "cloudflare_summary_minor.json", "cloudflare", "Cloudflare")

	snapshot, err := provider.Fetch(context.Background())
	if err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}

	if snapshot.State != status.StateDegraded {
		t.Fatalf("State = %q, want %q", snapshot.State, status.StateDegraded)
	}

	if len(snapshot.Incidents) != 2 {
		t.Fatalf("incidents len = %d, want 2", len(snapshot.Incidents))
	}

	if snapshot.Incidents[0].Summary != "A fix has been implemented and we are monitoring the results." {
		t.Fatalf("unexpected incident summary: %q", snapshot.Incidents[0].Summary)
	}

	if len(snapshot.Components) != 1 {
		t.Fatalf("components len = %d, want 1", len(snapshot.Components))
	}

	if snapshot.Components[0].State != status.StateDegraded {
		t.Fatalf("component state = %q", snapshot.Components[0].State)
	}

	if snapshot.Components[0].Group != "Core Services" {
		t.Fatalf("component group = %q", snapshot.Components[0].Group)
	}
}

func providerWithFixture(t *testing.T, fixtureName, id, name string) *Provider {
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

	return NewProvider(fetch.NewClient(5*time.Second), Options{
		ID:          id,
		Name:        name,
		Description: name + " service status",
		SourceURL:   server.URL,
		APIURL:      server.URL,
		SummaryURL:  server.URL,
	})
}
