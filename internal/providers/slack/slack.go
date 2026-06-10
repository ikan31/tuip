package slack

import (
	"context"
	"fmt"
	"html"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/ikan31/tuip/internal/fetch"
	"github.com/ikan31/tuip/internal/providers"
	"github.com/ikan31/tuip/internal/status"
)

const (
	currentStatusURL = "https://status.slack.com/api/v2.0.0/current"
	sourceURL        = "https://slack-status.com/"
	apiURL           = "https://docs.slack.dev/reference/slack-status-api/"
	paragraphMatches = 2
)

// Provider fetches Slack's custom status API.
type Provider struct {
	client     *fetch.Client
	endpoint   string
	statusPage string
	metadata   providers.Metadata
}

// New creates a Slack provider using Slack's current status API.
func New(client *fetch.Client) *Provider {
	return NewWithEndpoint(client, currentStatusURL, sourceURL)
}

// NewWithEndpoint creates a Slack provider with an override endpoint. It is
// used by tests with httptest fixtures.
func NewWithEndpoint(client *fetch.Client, endpoint string, source string) *Provider {
	return &Provider{
		client:     client,
		endpoint:   endpoint,
		statusPage: source,
		metadata: providers.Metadata{
			ID:          "slack",
			Name:        "Slack",
			Description: "Slack service status",
			Category:    "Communication",
			SourceURL:   source,
			APIURL:      apiURL,
		},
	}
}

func (p *Provider) Metadata() providers.Metadata { return p.metadata }

func (p *Provider) Fetch(ctx context.Context) (status.Snapshot, error) {
	var payload currentResponse

	err := p.client.GetJSON(ctx, p.endpoint, &payload)
	if err != nil {
		return status.Snapshot{}, fmt.Errorf("fetch Slack status: %w", err)
	}

	checkedAt := time.Now().UTC()
	updatedAt := parseSlackTime(payload.DateUpdated)
	state := MapStatus(payload.Status, len(payload.ActiveIncidents))
	summary := slackSummary(payload.Status, len(payload.ActiveIncidents))

	incidents := make([]status.Incident, 0, len(payload.ActiveIncidents))
	for _, incident := range payload.ActiveIncidents {
		incidents = append(incidents, status.Incident{
			Kind:      incident.TypeOrDefault(),
			Name:      incident.Title,
			Status:    incident.Status,
			URL:       incident.URL,
			StartedAt: parseSlackTime(incident.DateCreated),
			UpdatedAt: parseSlackTime(incident.DateUpdated),
		})
	}

	components := []status.Component{}

	page, textErr := p.client.GetText(ctx, p.statusPage)
	if textErr == nil {
		components = parseSlackComponents(page)
	}

	return status.Snapshot{
		ProviderID: p.metadata.ID,
		Name:       p.metadata.Name,
		State:      state,
		Summary:    summary,
		SourceURL:  p.metadata.SourceURL,
		CheckedAt:  checkedAt,
		UpdatedAt:  updatedAt,
		Incidents:  incidents,
		Components: components,
	}, nil
}

// MapStatus maps Slack's status API fields into tuip's normalized state.
func MapStatus(apiStatus string, activeIncidentCount int) status.State {
	if activeIncidentCount > 0 {
		return status.StateDegraded
	}

	switch strings.ToLower(strings.TrimSpace(apiStatus)) {
	case "ok", "active", "resolved":
		return status.StateOperational
	case "maintenance":
		return status.StateMaintenance
	case "", "unknown":
		return status.StateUnknown
	default:
		return status.StateDegraded
	}
}

func slackSummary(apiStatus string, activeIncidentCount int) string {
	if activeIncidentCount == 1 {
		return "1 active incident"
	}

	if activeIncidentCount > 1 {
		return strconv.Itoa(activeIncidentCount) + " active incidents"
	}

	if strings.EqualFold(apiStatus, "ok") || apiStatus == "" {
		return "All systems operational"
	}

	return "Slack status: " + apiStatus
}

func parseSlackComponents(page string) []status.Component {
	servicesStart := strings.Index(page, `<div id="services">`)
	if servicesStart >= 0 {
		page = page[servicesStart:]
	}

	parts := strings.Split(page, `<div class="service header align_center">`)
	if len(parts) <= 1 {
		return []status.Component{}
	}

	components := make([]status.Component, 0, len(parts)-1)

	for _, part := range parts[1:] {
		matches := slackParagraphPattern.FindAllStringSubmatch(part, -1)
		if len(matches) < paragraphMatches {
			continue
		}

		name := cleanSlackHTML(matches[0][1])

		statusText := cleanSlackHTML(matches[1][1])

		if name == "" || statusText == "" {
			continue
		}

		components = append(components, status.Component{
			Name:   name,
			Status: statusText,
			State:  mapSlackServiceStatus(statusText),
		})
	}

	return components
}

func mapSlackServiceStatus(value string) status.State {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "no issues", "ok", "operational":
		return status.StateOperational
	case "maintenance":
		return status.StateMaintenance
	case "notice", "incident":
		return status.StateDegraded
	case "outage":
		return status.StateMajorOutage
	case "":
		return status.StateUnknown
	default:
		return status.StateDegraded
	}
}

func cleanSlackHTML(value string) string {
	value = html.UnescapeString(value)
	value = slackTagPattern.ReplaceAllString(value, " ")

	return strings.Join(strings.Fields(value), " ")
}

func parseSlackTime(value string) *time.Time {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}

	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return nil
	}

	utc := parsed.UTC()

	return &utc
}

var (
	slackParagraphPattern = regexp.MustCompile(`(?is)<p[^>]*>(.*?)</p>`)
	slackTagPattern       = regexp.MustCompile(`(?is)<[^>]*>`)
)

type currentResponse struct {
	Status          string          `json:"status"`
	DateCreated     string          `json:"date_created"`
	DateUpdated     string          `json:"date_updated"`
	ActiveIncidents []slackIncident `json:"active_incidents"`
}

type slackIncident struct {
	ID          int    `json:"id"`
	Title       string `json:"title"`
	Type        string `json:"type"`
	Status      string `json:"status"`
	URL         string `json:"url"`
	DateCreated string `json:"date_created"`
	DateUpdated string `json:"date_updated"`
}

func (i slackIncident) TypeOrDefault() string {
	if strings.TrimSpace(i.Type) == "" {
		return "incident"
	}

	return i.Type
}
