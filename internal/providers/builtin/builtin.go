package builtin

import (
	"github.com/tuipcli/tuip/internal/fetch"
	"github.com/tuipcli/tuip/internal/providers"
	"github.com/tuipcli/tuip/internal/providers/cloudflare"
	"github.com/tuipcli/tuip/internal/providers/github"
	"github.com/tuipcli/tuip/internal/providers/slack"
)

// NewRegistry returns a registry populated with tuip's built-in providers.
func NewRegistry(client *fetch.Client) (*providers.Registry, error) {
	registry := providers.NewRegistry()

	registrations := []struct {
		metadata providers.Metadata
		factory  providers.Factory
	}{
		{
			metadata: slack.New(client).Metadata(),
			factory:  func() providers.Provider { return slack.New(client) },
		},
		{
			metadata: github.New(client).Metadata(),
			factory:  func() providers.Provider { return github.New(client) },
		},
		{
			metadata: cloudflare.New(client).Metadata(),
			factory:  func() providers.Provider { return cloudflare.New(client) },
		},
	}

	for _, registration := range registrations {
		if err := registry.Register(registration.metadata, registration.factory); err != nil {
			return nil, err
		}
	}
	return registry, nil
}
