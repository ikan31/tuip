package builtin

import (
	"fmt"

	"github.com/tuipcli/tuip/internal/fetch"
	"github.com/tuipcli/tuip/internal/providers"
	"github.com/tuipcli/tuip/internal/providers/cloudflare"
	"github.com/tuipcli/tuip/internal/providers/github"
	"github.com/tuipcli/tuip/internal/providers/slack"
	"github.com/tuipcli/tuip/internal/providers/statuspage"
)

const coreProviderCount = 7

type registration struct {
	metadata providers.Metadata
	factory  providers.Factory
}

// NewRegistry returns a registry populated with tuip's built-in providers.
func NewRegistry(client *fetch.Client) (*providers.Registry, error) {
	registry := providers.NewRegistry()

	statuspageRegs := statuspageRegistrations(client)
	registrations := make([]registration, 0, coreProviderCount+len(statuspageRegs))
	registrations = append(registrations,
		registration{
			metadata: slack.New(client).Metadata(),
			factory:  func() providers.Provider { return slack.New(client) },
		},
		registration{
			metadata: github.New(client).Metadata(),
			factory:  func() providers.Provider { return github.New(client) },
		},
		registration{
			metadata: github.NewEnterpriseCloudAU(client).Metadata(),
			factory:  func() providers.Provider { return github.NewEnterpriseCloudAU(client) },
		},
		registration{
			metadata: github.NewEnterpriseCloudEU(client).Metadata(),
			factory:  func() providers.Provider { return github.NewEnterpriseCloudEU(client) },
		},
		registration{
			metadata: github.NewEnterpriseCloudJP(client).Metadata(),
			factory:  func() providers.Provider { return github.NewEnterpriseCloudJP(client) },
		},
		registration{
			metadata: github.NewEnterpriseCloudUS(client).Metadata(),
			factory:  func() providers.Provider { return github.NewEnterpriseCloudUS(client) },
		},
		registration{
			metadata: cloudflare.New(client).Metadata(),
			factory:  func() providers.Provider { return cloudflare.New(client) },
		},
	)

	registrations = append(registrations, statuspageRegs...)

	for _, registration := range registrations {
		err := registry.Register(registration.metadata, registration.factory)
		if err != nil {
			return nil, fmt.Errorf("register provider %s: %w", registration.metadata.ID, err)
		}
	}

	return registry, nil
}

func statuspageRegistrations(client *fetch.Client) []registration {
	options := []statuspage.Options{
		{
			ID:          "accelo",
			Name:        "Accelo",
			Description: "Accelo service status",
			Category:    "CRM & Sales",
			SourceURL:   "https://status.accelo.com/",
			APIURL:      "https://status.accelo.com/api",
			SummaryURL:  "https://status.accelo.com/api/v2/summary.json",
		},
		{
			ID:          "affinity",
			Name:        "Affinity",
			Description: "Affinity service status",
			Category:    "CRM & Sales",
			SourceURL:   "https://status.affinity.co/",
			APIURL:      "https://status.affinity.co/api",
			SummaryURL:  "https://status.affinity.co/api/v2/summary.json",
		},
		{
			ID:          "capsule",
			Aliases:     []string{"capsulecrm"},
			Name:        "Capsule",
			Description: "Capsule CRM service status",
			Category:    "CRM & Sales",
			SourceURL:   "https://status.capsulecrm.com/",
			APIURL:      "https://status.capsulecrm.com/api",
			SummaryURL:  "https://status.capsulecrm.com/api/v2/summary.json",
		},
		{
			ID:          "hubspot",
			Name:        "HubSpot",
			Description: "HubSpot service status",
			Category:    "CRM & Sales",
			SourceURL:   "https://status.hubspot.com/",
			APIURL:      "https://status.hubspot.com/api",
			SummaryURL:  "https://status.hubspot.com/api/v2/summary.json",
		},
		{
			ID:          "nutshell",
			Name:        "Nutshell",
			Description: "Nutshell service status",
			Category:    "CRM & Sales",
			SourceURL:   "https://status.nutshell.com/",
			APIURL:      "https://status.nutshell.com/api",
			SummaryURL:  "https://status.nutshell.com/api/v2/summary.json",
		},
		{
			ID:          "monday",
			Aliases:     []string{"monday.com"},
			Name:        "monday.com",
			Description: "monday.com service status",
			Category:    "Project Management",
			SourceURL:   "https://status.monday.com/",
			APIURL:      "https://status.monday.com/api",
			SummaryURL:  "https://status.monday.com/api/v2/summary.json",
		},
		{
			ID:          "gusto",
			Name:        "Gusto",
			Description: "Gusto service status",
			Category:    "HR & Workforce",
			SourceURL:   "https://status.gusto.com/",
			APIURL:      "https://status.gusto.com/api",
			SummaryURL:  "https://status.gusto.com/api/v2/summary.json",
		},
		{
			ID:          "officient",
			Aliases:     []string{"exact"},
			Name:        "Officient",
			Description: "Officient/Exact HR service status",
			Category:    "HR & Workforce",
			SourceURL:   "https://status.officient.io/",
			APIURL:      "https://status.officient.io/api",
			SummaryURL:  "https://status.officient.io/api/v2/summary.json",
		},
		{
			ID:          "ashby",
			Aliases:     []string{"ashbyhq"},
			Name:        "Ashby",
			Description: "Ashby service status",
			Category:    "HR & Workforce",
			SourceURL:   "https://status.ashbyhq.com/",
			APIURL:      "https://status.ashbyhq.com/api",
			SummaryURL:  "https://status.ashbyhq.com/api/v2/summary.json",
		},
		{
			ID:          "greenhouse",
			Name:        "Greenhouse",
			Description: "Greenhouse service status",
			Category:    "HR & Workforce",
			SourceURL:   "https://status.greenhouse.io/",
			APIURL:      "https://status.greenhouse.io/api",
			SummaryURL:  "https://status.greenhouse.io/api/v2/summary.json",
		},
		{
			ID:          "workable",
			Name:        "Workable",
			Description: "Workable service status",
			Category:    "HR & Workforce",
			SourceURL:   "https://workable.statuspage.io/",
			APIURL:      "https://workable.statuspage.io/api",
			SummaryURL:  "https://workable.statuspage.io/api/v2/summary.json",
		},
		{
			ID:          "freshbooks",
			Name:        "FreshBooks",
			Description: "FreshBooks service status",
			Category:    "Finance & Accounting",
			SourceURL:   "https://status.freshbooks.com/",
			APIURL:      "https://status.freshbooks.com/api",
			SummaryURL:  "https://status.freshbooks.com/api/v2/summary.json",
		},
		{
			ID:          "quickbooks-online",
			Aliases:     []string{"quickbooks", "qbo"},
			Name:        "QuickBooks Online",
			Description: "QuickBooks Online service status",
			Category:    "Finance & Accounting",
			SourceURL:   "https://status.quickbooks.intuit.com/",
			APIURL:      "https://status.quickbooks.intuit.com/api",
			SummaryURL:  "https://status.quickbooks.intuit.com/api/v2/summary.json",
		},
		{
			ID:          "xero",
			Name:        "Xero",
			Description: "Xero service status",
			Category:    "Finance & Accounting",
			SourceURL:   "https://status.xero.com/",
			APIURL:      "https://status.xero.com/api",
			SummaryURL:  "https://status.xero.com/api/v2/summary.json",
		},
		{
			ID:          "box",
			Name:        "Box",
			Description: "Box service status",
			Category:    "File Storage",
			SourceURL:   "https://status.box.com/",
			APIURL:      "https://status.box.com/api",
			SummaryURL:  "https://status.box.com/api/v2/summary.json",
		},
		{
			ID:          "dropbox",
			Name:        "Dropbox",
			Description: "Dropbox service status",
			Category:    "File Storage",
			SourceURL:   "https://status.dropbox.com/",
			APIURL:      "https://status.dropbox.com/api",
			SummaryURL:  "https://status.dropbox.com/api/v2/summary.json",
		},
		{
			ID:          "asana",
			Name:        "Asana",
			Description: "Asana service status",
			Category:    "Project Management",
			SourceURL:   "https://status.asana.com/",
			APIURL:      "https://status.asana.com/api",
			SummaryURL:  "https://status.asana.com/api/v2/summary.json",
		},
		{
			ID:          "bitbucket",
			Name:        "Bitbucket",
			Description: "Atlassian Bitbucket service status",
			Category:    "Developer Tools",
			SourceURL:   "https://bitbucket.status.atlassian.com/",
			APIURL:      "https://bitbucket.status.atlassian.com/api",
			SummaryURL:  "https://bitbucket.status.atlassian.com/api/v2/summary.json",
		},
		{
			ID:          "hive",
			Name:        "Hive",
			Description: "Hive service status",
			Category:    "Project Management",
			SourceURL:   "https://status.hive.com/",
			APIURL:      "https://status.hive.com/api",
			SummaryURL:  "https://status.hive.com/api/v2/summary.json",
		},
		{
			ID:          "jira",
			Aliases:     []string{"jira-software"},
			Name:        "Jira",
			Description: "Jira service status",
			Category:    "Project Management",
			SourceURL:   "https://jira-software.status.atlassian.com/",
			APIURL:      "https://jira-software.status.atlassian.com/api",
			SummaryURL:  "https://jira-software.status.atlassian.com/api/v2/summary.json",
		},
		{
			ID:          "trello",
			Name:        "Trello",
			Description: "Trello service status",
			Category:    "Project Management",
			SourceURL:   "https://trello.status.atlassian.com/",
			APIURL:      "https://trello.status.atlassian.com/api",
			SummaryURL:  "https://trello.status.atlassian.com/api/v2/summary.json",
		},
		{
			ID:          "confluence",
			Name:        "Confluence",
			Description: "Confluence service status",
			Category:    "Collaboration",
			SourceURL:   "https://confluence.status.atlassian.com/",
			APIURL:      "https://confluence.status.atlassian.com/api",
			SummaryURL:  "https://confluence.status.atlassian.com/api/v2/summary.json",
		},
	}

	registrations := make([]registration, 0, len(options))
	for _, option := range options {
		current := option
		provider := statuspage.NewProvider(client, current)
		registrations = append(registrations, registration{
			metadata: provider.Metadata(),
			factory: func() providers.Provider {
				return statuspage.NewProvider(client, current)
			},
		})
	}

	return registrations
}
