package providers

import (
	"fmt"
	"sort"
	"strings"
)

// Factory constructs a provider. Factories are used so providers can be
// instantiated with shared dependencies while the registry remains a stable
// source of provider metadata.
type Factory func() Provider

// Registry maps stable provider IDs, such as "slack" or "github", to provider
// implementations and metadata.
type Registry struct {
	entries map[string]registryEntry
	aliases map[string]string
}

type registryEntry struct {
	metadata Metadata
	factory  Factory
}

// NewRegistry creates an empty provider registry.
func NewRegistry() *Registry {
	return &Registry{
		entries: map[string]registryEntry{},
		aliases: map[string]string{},
	}
}

// Register adds a provider factory to the registry.
func (r *Registry) Register(metadata Metadata, factory Factory) error {
	if metadata.ID == "" {
		return fmt.Errorf("provider id is required")
	}
	if metadata.Name == "" {
		return fmt.Errorf("provider %q name is required", metadata.ID)
	}
	if factory == nil {
		return fmt.Errorf("provider %q factory is required", metadata.ID)
	}

	id := normalizeID(metadata.ID)
	metadata.ID = id
	if _, exists := r.entries[id]; exists {
		return fmt.Errorf("provider %q is already registered", id)
	}
	if canonical, exists := r.aliases[id]; exists {
		return fmt.Errorf("provider id %q conflicts with alias for %q", id, canonical)
	}

	aliases := make([]string, 0, len(metadata.Aliases))
	seenAliases := map[string]bool{}
	for _, alias := range metadata.Aliases {
		alias = normalizeID(alias)
		if alias == "" || alias == id || seenAliases[alias] {
			continue
		}
		if _, exists := r.entries[alias]; exists {
			return fmt.Errorf("provider alias %q conflicts with registered provider", alias)
		}
		if canonical, exists := r.aliases[alias]; exists {
			return fmt.Errorf("provider alias %q conflicts with alias for %q", alias, canonical)
		}
		seenAliases[alias] = true
		aliases = append(aliases, alias)
	}
	metadata.Aliases = aliases

	r.entries[id] = registryEntry{metadata: metadata, factory: factory}
	for _, alias := range aliases {
		r.aliases[alias] = id
	}
	return nil
}

// Get returns a provider by ID or alias.
func (r *Registry) Get(id string) (Provider, bool) {
	canonicalID, ok := r.CanonicalID(id)
	if !ok {
		return nil, false
	}
	entry, ok := r.entries[canonicalID]
	if !ok {
		return nil, false
	}
	return entry.factory(), true
}

// Metadata returns all registered provider metadata sorted by provider ID.
func (r *Registry) Metadata() []Metadata {
	items := make([]Metadata, 0, len(r.entries))
	for _, entry := range r.entries {
		items = append(items, entry.metadata)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].ID < items[j].ID })
	return items
}

// Has reports whether a provider ID or alias is registered.
func (r *Registry) Has(id string) bool {
	_, ok := r.CanonicalID(id)
	return ok
}

// CanonicalID resolves a provider ID or alias to the canonical provider ID.
func (r *Registry) CanonicalID(id string) (string, bool) {
	id = normalizeID(id)
	if _, ok := r.entries[id]; ok {
		return id, true
	}
	canonicalID, ok := r.aliases[id]
	return canonicalID, ok
}

// CanonicalIDs resolves provider IDs and aliases to canonical provider IDs.
func (r *Registry) CanonicalIDs(ids []string) ([]string, error) {
	canonicalIDs := make([]string, 0, len(ids))
	var unknown []string
	for _, id := range ids {
		canonicalID, ok := r.CanonicalID(id)
		if !ok {
			unknown = append(unknown, id)
			continue
		}
		canonicalIDs = append(canonicalIDs, canonicalID)
	}
	if len(unknown) > 0 {
		return nil, fmt.Errorf("unknown provider(s): %s", strings.Join(unknown, ", "))
	}
	return canonicalIDs, nil
}

// ValidateIDs returns an error listing unknown provider IDs, if any.
func (r *Registry) ValidateIDs(ids []string) error {
	_, err := r.CanonicalIDs(ids)
	return err
}

func normalizeID(id string) string {
	return strings.ToLower(strings.TrimSpace(id))
}
