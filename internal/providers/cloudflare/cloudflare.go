package cloudflare

import (
	"github.com/tuipcli/tuip/internal/fetch"
	"github.com/tuipcli/tuip/internal/providers/statuspage"
)

const (
	sourceURL  = "https://www.cloudflarestatus.com/"
	apiURL     = "https://www.cloudflarestatus.com/api"
	summaryURL = "https://www.cloudflarestatus.com/api/v2/summary.json"
)

// New creates a Cloudflare provider backed by Cloudflare's Statuspage JSON API.
func New(client *fetch.Client) *statuspage.Provider {
	return statuspage.NewProvider(client, statuspage.Options{
		ID:          "cloudflare",
		Name:        "Cloudflare",
		Description: "Cloudflare service status",
		Category:    "Infrastructure",
		SourceURL:   sourceURL,
		APIURL:      apiURL,
		SummaryURL:  summaryURL,
	})
}
