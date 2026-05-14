package providers

import (
	"context"
	"reflect"
	"testing"

	"github.com/tuipcli/tuip/internal/status"
)

func TestRegistryAliasesResolveToCanonicalProvider(t *testing.T) {
	t.Parallel()

	registry := NewRegistry()
	provider := staticProvider{metadata: Metadata{
		ID:      "github-enterprise-cloud-eu",
		Aliases: []string{"github-eu", "ghec-eu"},
		Name:    "GitHub Enterprise Cloud - EU",
	}}
	if err := registry.Register(provider.metadata, func() Provider { return provider }); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	canonicalID, ok := registry.CanonicalID("github-eu")
	if !ok {
		t.Fatalf("CanonicalID() ok = false, want true")
	}
	if canonicalID != "github-enterprise-cloud-eu" {
		t.Fatalf("CanonicalID() = %q", canonicalID)
	}

	resolved, ok := registry.Get("ghec-eu")
	if !ok {
		t.Fatalf("Get() ok = false, want true")
	}
	if resolved.Metadata().ID != "github-enterprise-cloud-eu" {
		t.Fatalf("resolved provider ID = %q", resolved.Metadata().ID)
	}

	canonicalIDs, err := registry.CanonicalIDs([]string{"github-eu", "ghec-eu"})
	if err != nil {
		t.Fatalf("CanonicalIDs() error = %v", err)
	}
	if want := []string{"github-enterprise-cloud-eu", "github-enterprise-cloud-eu"}; !reflect.DeepEqual(canonicalIDs, want) {
		t.Fatalf("CanonicalIDs() = %#v, want %#v", canonicalIDs, want)
	}
}

func TestRegistryRejectsAliasConflicts(t *testing.T) {
	t.Parallel()

	registry := NewRegistry()
	first := staticProvider{metadata: Metadata{ID: "first", Aliases: []string{"shared"}, Name: "First"}}
	second := staticProvider{metadata: Metadata{ID: "second", Aliases: []string{"shared"}, Name: "Second"}}
	if err := registry.Register(first.metadata, func() Provider { return first }); err != nil {
		t.Fatalf("Register(first) error = %v", err)
	}
	if err := registry.Register(second.metadata, func() Provider { return second }); err == nil {
		t.Fatalf("Register(second) error = nil, want conflict")
	}
}

type staticProvider struct {
	metadata Metadata
}

func (p staticProvider) Metadata() Metadata { return p.metadata }

func (p staticProvider) Fetch(ctx context.Context) (status.Snapshot, error) {
	return status.Snapshot{ProviderID: p.metadata.ID, Name: p.metadata.Name, State: status.StateOperational}, nil
}
