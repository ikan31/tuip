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

	err := registry.Register(provider.metadata, func() Provider { return provider })
	if err != nil {
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

func TestRegistrySearchMatchesProviderMetadata(t *testing.T) {
	t.Parallel()

	registry := NewRegistry()

	items := []staticProvider{
		{metadata: Metadata{ID: "slack", Name: "Slack", Category: "Communication", Description: "Team messaging"}},
		{metadata: Metadata{ID: "cloudflare", Name: "Cloudflare", Category: "Infrastructure", Description: "Edge network"}},
		{metadata: Metadata{ID: "github-enterprise-cloud-eu", Aliases: []string{"github-eu", "ghec-eu"}, Name: "GitHub Enterprise Cloud - EU", Category: "Developer Tools"}},
	}
	for _, item := range items {
		provider := item

		err := registry.Register(provider.metadata, func() Provider { return provider })
		if err != nil {
			t.Fatalf("Register() error = %v", err)
		}
	}

	tests := []struct {
		name  string
		query string
		want  []string
	}{
		{name: "id substring", query: "cloud", want: []string{"cloudflare", "github-enterprise-cloud-eu"}},
		{name: "alias compact", query: "gheceu", want: []string{"github-enterprise-cloud-eu"}},
		{name: "category", query: "comm", want: []string{"slack"}},
		{name: "fuzzy subsequence", query: "gheeu", want: []string{"github-enterprise-cloud-eu"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := registry.Search(tt.query)

			gotIDs := make([]string, 0, len(got))

			for _, item := range got {
				gotIDs = append(gotIDs, item.ID)
			}

			if !reflect.DeepEqual(gotIDs, tt.want) {
				t.Fatalf("Search(%q) = %#v, want %#v", tt.query, gotIDs, tt.want)
			}
		})
	}
}

func TestRegistrySearchEmptyQueryReturnsMetadataOrder(t *testing.T) {
	t.Parallel()

	registry := NewRegistry()

	for _, provider := range []staticProvider{
		{metadata: Metadata{ID: "slack", Name: "Slack"}},
		{metadata: Metadata{ID: "cloudflare", Name: "Cloudflare"}},
	} {
		err := registry.Register(provider.metadata, func() Provider { return provider })
		if err != nil {
			t.Fatalf("Register() error = %v", err)
		}
	}

	got := registry.Search("")

	gotIDs := make([]string, 0, len(got))

	for _, item := range got {
		gotIDs = append(gotIDs, item.ID)
	}

	if want := []string{"cloudflare", "slack"}; !reflect.DeepEqual(gotIDs, want) {
		t.Fatalf("Search(empty) = %#v, want %#v", gotIDs, want)
	}
}

func TestRegistryRejectsAliasConflicts(t *testing.T) {
	t.Parallel()

	registry := NewRegistry()
	first := staticProvider{metadata: Metadata{ID: "first", Aliases: []string{"shared"}, Name: "First"}}

	second := staticProvider{metadata: Metadata{ID: "second", Aliases: []string{"shared"}, Name: "Second"}}

	err := registry.Register(first.metadata, func() Provider { return first })
	if err != nil {
		t.Fatalf("Register(first) error = %v", err)
	}

	err = registry.Register(second.metadata, func() Provider { return second })
	if err == nil {
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
