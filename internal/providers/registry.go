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
}

type registryEntry struct {
	metadata Metadata
	factory  Factory
}

// NewRegistry creates an empty provider registry.
func NewRegistry() *Registry {
	return &Registry{entries: map[string]registryEntry{}}
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

	id := strings.ToLower(strings.TrimSpace(metadata.ID))
	metadata.ID = id
	if _, exists := r.entries[id]; exists {
		return fmt.Errorf("provider %q is already registered", id)
	}
	r.entries[id] = registryEntry{metadata: metadata, factory: factory}
	return nil
}

// Get returns a provider by ID.
func (r *Registry) Get(id string) (Provider, bool) {
	entry, ok := r.entries[normalizeID(id)]
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

// Has reports whether a provider ID is registered.
func (r *Registry) Has(id string) bool {
	_, ok := r.entries[normalizeID(id)]
	return ok
}

// ValidateIDs returns an error listing unknown provider IDs, if any.
func (r *Registry) ValidateIDs(ids []string) error {
	var unknown []string
	for _, id := range ids {
		if !r.Has(id) {
			unknown = append(unknown, id)
		}
	}
	if len(unknown) > 0 {
		return fmt.Errorf("unknown provider(s): %s", strings.Join(unknown, ", "))
	}
	return nil
}

func normalizeID(id string) string {
	return strings.ToLower(strings.TrimSpace(id))
}
