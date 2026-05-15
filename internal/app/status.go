package app

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/tuipcli/tuip/internal/providers"
	"github.com/tuipcli/tuip/internal/status"
)

const ProviderTimeout = 5 * time.Second

// StatusOptions controls status fetch behavior.
type StatusOptions struct {
	Details bool
}

// CheckProviders fetches all requested providers concurrently and returns
// results in the same order as the requested IDs.
func CheckProviders(ctx context.Context, registry *providers.Registry, providerIDs []string, opts StatusOptions) (status.Response, error) {
	checkedAt := time.Now().UTC()
	response := status.Response{
		CheckedAt: checkedAt,
		Results:   make([]status.Snapshot, len(providerIDs)),
	}

	if len(providerIDs) == 0 {
		return response, fmt.Errorf("at least one provider is required")
	}
	if err := registry.ValidateIDs(providerIDs); err != nil {
		return response, err
	}

	var wg sync.WaitGroup
	var mu sync.Mutex
	var hadRuntimeError bool

	for i, providerID := range providerIDs {
		provider, _ := registry.Get(providerID)
		metadata := provider.Metadata()

		wg.Add(1)
		go func() {
			defer wg.Done()

			providerCtx, cancel := context.WithTimeout(ctx, ProviderTimeout)
			defer cancel()

			snapshot, err := provider.Fetch(providerCtx)
			if err != nil {
				snapshot = status.Snapshot{
					ProviderID: metadata.ID,
					Name:       metadata.Name,
					State:      status.StateError,
					Summary:    "Failed to fetch status",
					SourceURL:  metadata.SourceURL,
					CheckedAt:  time.Now().UTC(),
					Incidents:  []status.Incident{},
					Components: []status.Component{},
					Error:      err.Error(),
				}
				mu.Lock()
				hadRuntimeError = true
				mu.Unlock()
			}

			if snapshot.CheckedAt.IsZero() {
				snapshot.CheckedAt = time.Now().UTC()
			}
			if !opts.Details {
				snapshot.Incidents = []status.Incident{}
				snapshot.Components = []status.Component{}
			} else {
				if snapshot.Incidents == nil {
					snapshot.Incidents = []status.Incident{}
				}
				if snapshot.Components == nil {
					snapshot.Components = []status.Component{}
				}
			}

			response.Results[i] = snapshot
		}()
	}

	wg.Wait()
	if hadRuntimeError {
		return response, fmt.Errorf("one or more providers failed")
	}
	return response, nil
}

// HasUnhealthyProvider reports whether any successfully fetched provider is not
// operational. Runtime failures are intentionally excluded because callers
// should handle CheckProviders' returned error separately.
func HasUnhealthyProvider(response status.Response) bool {
	for _, result := range response.Results {
		if result.State != status.StateError && !result.State.IsHealthy() {
			return true
		}
	}
	return false
}
