package providers

import (
	"context"

	"github.com/ikan31/tuip/internal/status"
)

// Metadata describes a built-in provider. The registry uses this for lookup,
// provider listing, config validation, and future TUI search.
type Metadata struct {
	ID          string
	Aliases     []string
	Name        string
	Description string
	Category    string
	SourceURL   string
	APIURL      string
}

// Provider fetches one SaaS service's status and maps it into tuip's normalized
// status model.
type Provider interface {
	Metadata() Metadata
	Fetch(ctx context.Context) (status.Snapshot, error)
}
