package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/tuipcli/tuip/internal/providers"
	"github.com/tuipcli/tuip/internal/status"
)

func TestCheckProvidersPreservesOrderAndAllowsDegradedStatus(t *testing.T) {
	t.Parallel()

	registry := providers.NewRegistry()
	mustRegister(t, registry, fakeProvider{
		metadata: providers.Metadata{ID: "github", Name: "GitHub"},
		snapshot: status.Snapshot{ProviderID: "github", Name: "GitHub", State: status.StateDegraded, Summary: "Degraded"},
	})
	mustRegister(t, registry, fakeProvider{
		metadata: providers.Metadata{ID: "slack", Name: "Slack"},
		snapshot: status.Snapshot{ProviderID: "slack", Name: "Slack", State: status.StateOperational, Summary: "OK"},
	})

	response, err := CheckProviders(context.Background(), registry, []string{"slack", "github"}, StatusOptions{})
	if err != nil {
		t.Fatalf("CheckProviders() error = %v", err)
	}
	if got, want := len(response.Results), 2; got != want {
		t.Fatalf("results len = %d, want %d", got, want)
	}
	if response.Results[0].ProviderID != "slack" || response.Results[1].ProviderID != "github" {
		t.Fatalf("result order = [%s, %s], want [slack, github]", response.Results[0].ProviderID, response.Results[1].ProviderID)
	}
	if !HasUnhealthyProvider(response) {
		t.Fatalf("HasUnhealthyProvider() = false, want true")
	}
}

func TestCheckProvidersProviderErrorReturnsSnapshotAndError(t *testing.T) {
	t.Parallel()

	registry := providers.NewRegistry()
	mustRegister(t, registry, fakeProvider{
		metadata: providers.Metadata{ID: "slack", Name: "Slack", SourceURL: "https://slack-status.com/"},
		err:      errors.New("upstream failed"),
	})

	response, err := CheckProviders(context.Background(), registry, []string{"slack"}, StatusOptions{})
	if err == nil {
		t.Fatalf("CheckProviders() error = nil, want error")
	}
	if got, want := len(response.Results), 1; got != want {
		t.Fatalf("results len = %d, want %d", got, want)
	}
	result := response.Results[0]
	if result.State != status.StateError {
		t.Fatalf("State = %q, want %q", result.State, status.StateError)
	}
	if result.Error == "" {
		t.Fatalf("Error is empty, want provider error message")
	}
}

func TestCheckProvidersUnknownProviderFailsBeforeFetch(t *testing.T) {
	t.Parallel()

	response, err := CheckProviders(context.Background(), providers.NewRegistry(), []string{"missing"}, StatusOptions{})
	if err == nil {
		t.Fatalf("CheckProviders() error = nil, want error")
	}
	if got, want := len(response.Results), 1; got != want {
		t.Fatalf("results len = %d, want %d", got, want)
	}
	if !response.Results[0].CheckedAt.IsZero() {
		t.Fatalf("unexpected populated result for unknown provider: %#v", response.Results[0])
	}
}

func TestCheckProvidersOmitsDetailsUnlessRequested(t *testing.T) {
	t.Parallel()

	registry := providers.NewRegistry()
	mustRegister(t, registry, fakeProvider{
		metadata: providers.Metadata{ID: "cloudflare", Name: "Cloudflare"},
		snapshot: status.Snapshot{
			ProviderID: "cloudflare",
			Name:       "Cloudflare",
			State:      status.StateOperational,
			Incidents:  []status.Incident{{Kind: "incident", Name: "test incident"}},
			Components: []status.Component{{Name: "API", State: status.StateOperational}},
		},
	})

	withoutDetails, err := CheckProviders(context.Background(), registry, []string{"cloudflare"}, StatusOptions{})
	if err != nil {
		t.Fatalf("CheckProviders() without details error = %v", err)
	}
	if len(withoutDetails.Results[0].Incidents) != 0 || len(withoutDetails.Results[0].Components) != 0 {
		t.Fatalf("details were not omitted: %#v", withoutDetails.Results[0])
	}

	withDetails, err := CheckProviders(context.Background(), registry, []string{"cloudflare"}, StatusOptions{Details: true})
	if err != nil {
		t.Fatalf("CheckProviders() with details error = %v", err)
	}
	if len(withDetails.Results[0].Incidents) != 1 || len(withDetails.Results[0].Components) != 1 {
		t.Fatalf("details were not preserved: %#v", withDetails.Results[0])
	}
}

func mustRegister(t *testing.T, registry *providers.Registry, provider fakeProvider) {
	t.Helper()
	if err := registry.Register(provider.metadata, func() providers.Provider { return provider }); err != nil {
		t.Fatalf("Register() error = %v", err)
	}
}

type fakeProvider struct {
	metadata providers.Metadata
	snapshot status.Snapshot
	err      error
}

func (p fakeProvider) Metadata() providers.Metadata { return p.metadata }

func (p fakeProvider) Fetch(ctx context.Context) (status.Snapshot, error) {
	if p.err != nil {
		return status.Snapshot{}, p.err
	}
	snapshot := p.snapshot
	if snapshot.CheckedAt.IsZero() {
		snapshot.CheckedAt = time.Now().UTC()
	}
	return snapshot, nil
}
