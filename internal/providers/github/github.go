package github

import (
	"github.com/tuipcli/tuip/internal/fetch"
	"github.com/tuipcli/tuip/internal/providers/statuspage"
)

const (
	sourceURL  = "https://www.githubstatus.com/#"
	apiURL     = "https://www.githubstatus.com/api/v2/summary.json"
	summaryURL = "https://www.githubstatus.com/api/v2/summary.json"
)

// New creates a GitHub provider backed by GitHub's Statuspage JSON API.
func New(client *fetch.Client) *statuspage.Provider {
	return statuspage.NewProvider(client, statuspage.Options{
		ID:          "github",
		Name:        "GitHub",
		Description: "GitHub service status",
		SourceURL:   sourceURL,
		APIURL:      apiURL,
		SummaryURL:  summaryURL,
	})
}
