package github

import (
	"github.com/tuipcli/tuip/internal/fetch"
	"github.com/tuipcli/tuip/internal/providers/pagerdutystatus"
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
		Category:    "Developer Tools",
		SourceURL:   sourceURL,
		APIURL:      apiURL,
		SummaryURL:  summaryURL,
	})
}

// NewEnterpriseCloudAU creates a GitHub Enterprise Cloud Australia regional provider.
func NewEnterpriseCloudAU(client *fetch.Client) *pagerdutystatus.Provider {
	return newEnterpriseCloudPagerDutyProvider(client, "github-enterprise-cloud-au", "Australia", "https://au.githubstatus.com")
}

// NewEnterpriseCloudEU creates a GitHub Enterprise Cloud EU regional provider.
func NewEnterpriseCloudEU(client *fetch.Client) *statuspage.Provider {
	return statuspage.NewProvider(client, statuspage.Options{
		ID:          "github-enterprise-cloud-eu",
		Aliases:     []string{"github-eu", "ghec-eu"},
		Name:        "GitHub Enterprise Cloud - EU",
		Description: "GitHub Enterprise Cloud EU regional status",
		Category:    "Developer Tools",
		SourceURL:   "https://eu.githubstatus.com/",
		APIURL:      "https://eu.githubstatus.com/api/v2/summary.json",
		SummaryURL:  "https://eu.githubstatus.com/api/v2/summary.json",
	})
}

// NewEnterpriseCloudJP creates a GitHub Enterprise Cloud Japan regional provider.
func NewEnterpriseCloudJP(client *fetch.Client) *pagerdutystatus.Provider {
	return newEnterpriseCloudPagerDutyProvider(client, "github-enterprise-cloud-jp", "Japan", "https://jp.githubstatus.com")
}

// NewEnterpriseCloudUS creates a GitHub Enterprise Cloud US regional provider.
func NewEnterpriseCloudUS(client *fetch.Client) *pagerdutystatus.Provider {
	return newEnterpriseCloudPagerDutyProvider(client, "github-enterprise-cloud-us", "US", "https://us.githubstatus.com")
}

func enterpriseCloudAliases(id string) []string {
	switch id {
	case "github-enterprise-cloud-au":
		return []string{"github-au", "ghec-au"}
	case "github-enterprise-cloud-jp":
		return []string{"github-jp", "ghec-jp"}
	case "github-enterprise-cloud-us":
		return []string{"github-us", "ghec-us"}
	default:
		return nil
	}
}

func newEnterpriseCloudPagerDutyProvider(client *fetch.Client, id, region, baseURL string) *pagerdutystatus.Provider {
	return pagerdutystatus.NewProvider(client, pagerdutystatus.Options{
		ID:          id,
		Aliases:     enterpriseCloudAliases(id),
		Name:        "GitHub Enterprise Cloud - " + region,
		Description: "GitHub Enterprise Cloud " + region + " regional status",
		Category:    "Developer Tools",
		SourceURL:   baseURL + "/",
		APIURL:      baseURL + "/api/data",
		DataURL:     baseURL + "/api/data",
	})
}
