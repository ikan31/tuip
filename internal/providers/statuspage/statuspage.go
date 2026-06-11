package statuspage

import (
	"context"
	"fmt"
	"html"
	"regexp"
	"strings"
	"time"

	"github.com/ikan31/tuip/internal/fetch"
	"github.com/ikan31/tuip/internal/providers"
	"github.com/ikan31/tuip/internal/status"
)

// Options configures a reusable Atlassian Statuspage JSON provider.
type Options struct {
	ID          string
	Aliases     []string
	Name        string
	Description string
	Category    string
	SourceURL   string
	APIURL      string
	SummaryURL  string
}

// Provider fetches a Statuspage /api/v2/summary.json endpoint.
type Provider struct {
	client  *fetch.Client
	options Options
}

func NewProvider(client *fetch.Client, options Options) *Provider {
	return &Provider{client: client, options: options}
}

func (p *Provider) Metadata() providers.Metadata {
	return providers.Metadata{
		ID:          p.options.ID,
		Aliases:     p.options.Aliases,
		Name:        p.options.Name,
		Description: p.options.Description,
		Category:    p.options.Category,
		SourceURL:   p.options.SourceURL,
		APIURL:      p.options.APIURL,
	}
}

func (p *Provider) Fetch(ctx context.Context) (status.Snapshot, error) {
	var payload summaryResponse

	err := p.client.GetJSON(ctx, p.options.SummaryURL, &payload)
	if err != nil {
		return status.Snapshot{}, fmt.Errorf("fetch %s status: %w", p.options.ID, err)
	}

	checkedAt := time.Now().UTC()
	updatedAt := parseTime(payload.Page.UpdatedAt)

	state := MapIndicator(payload.Status.Indicator)
	if strings.TrimSpace(payload.Status.Indicator) == "" && strings.TrimSpace(payload.Page.Status) != "" {
		state = MapPageStatus(payload.Page.Status)
	}

	summary := strings.TrimSpace(payload.Status.Description)

	if summary == "" {
		summary = state.Display()
	}

	incidents := make([]status.Incident, 0, len(payload.Incidents)+len(payload.ScheduledMaintenances))
	for _, incident := range payload.Incidents {
		incidents = append(incidents, mapIncident(incident))
	}

	for _, maintenance := range payload.ScheduledMaintenances {
		mapped := mapIncident(maintenance)
		mapped.Kind = "maintenance"
		mapped.ScheduledFor = parseTime(maintenance.ScheduledFor)
		mapped.ScheduledUntil = parseTime(maintenance.ScheduledUntil)
		incidents = append(incidents, mapped)
	}

	if state == status.StateOperational && hasInProgressMaintenance(payload.ScheduledMaintenances) {
		state = status.StateMaintenance
		summary = "Scheduled maintenance in progress"
	}

	components := mapComponents(payload.Components)

	return status.Snapshot{
		ProviderID: p.options.ID,
		Name:       p.options.Name,
		State:      state,
		Summary:    summary,
		SourceURL:  p.options.SourceURL,
		CheckedAt:  checkedAt,
		UpdatedAt:  updatedAt,
		Incidents:  incidents,
		Components: components,
	}, nil
}

// MapIndicator maps Statuspage's top-level indicator into tuip's normalized
// status model.
func MapIndicator(indicator string) status.State {
	switch strings.ToLower(strings.TrimSpace(indicator)) {
	case "none":
		return status.StateOperational
	case "minor":
		return status.StateDegraded
	case "major":
		return status.StateMajorOutage
	case "critical":
		return status.StateMajorOutage
	case "maintenance":
		return status.StateMaintenance
	case "", "unknown":
		return status.StateUnknown
	default:
		return status.StateUnknown
	}
}

// MapPageStatus maps lightweight Statuspage-compatible page statuses into
// normalized states. Some providers expose only page.status instead of the
// Atlassian status.indicator object.
func MapPageStatus(pageStatus string) status.State {
	switch strings.ToLower(strings.TrimSpace(pageStatus)) {
	case "up", "operational", "ok", "active":
		return status.StateOperational
	case "hasissues", "has_issues", "degraded", "degraded_performance":
		return status.StateDegraded
	case "partialoutage", "partial_outage":
		return status.StatePartialOutage
	case "down", "majoroutage", "major_outage", "critical":
		return status.StateMajorOutage
	case "maintenance", "under_maintenance":
		return status.StateMaintenance
	case "", "unknown":
		return status.StateUnknown
	default:
		return status.StateUnknown
	}
}

// MapComponentStatus maps Statuspage component statuses into normalized states.
func MapComponentStatus(componentStatus string) status.State {
	switch strings.ToLower(strings.TrimSpace(componentStatus)) {
	case "operational":
		return status.StateOperational
	case "degraded_performance":
		return status.StateDegraded
	case "partial_outage":
		return status.StatePartialOutage
	case "major_outage":
		return status.StateMajorOutage
	case "under_maintenance":
		return status.StateMaintenance
	case "", "unknown":
		return status.StateUnknown
	default:
		return status.StateUnknown
	}
}

func mapIncident(incident incidentResponse) status.Incident {
	return status.Incident{
		Kind:      "incident",
		Name:      incident.Name,
		Status:    incident.Status,
		Impact:    incident.Impact,
		Summary:   latestIncidentUpdate(incident.IncidentUpdates),
		URL:       incident.Shortlink,
		StartedAt: parseTime(incident.StartedAt),
		UpdatedAt: parseTime(incident.UpdatedAt),
	}
}

func latestIncidentUpdate(updates []incidentUpdateResponse) string {
	if len(updates) == 0 {
		return ""
	}

	return cleanHTML(updates[0].Body)
}

func mapComponents(components []componentResponse) []status.Component {
	groupNames := map[string]string{}

	for _, component := range components {
		if component.Group {
			groupNames[component.ID] = component.Name
		}
	}

	mapped := make([]status.Component, 0, len(components))

	for _, component := range components {
		if component.Group {
			continue
		}

		mapped = append(mapped, status.Component{
			Name:   component.Name,
			Status: component.Status,
			State:  MapComponentStatus(component.Status),
			Group:  groupNames[component.GroupID],
		})
	}

	return mapped
}

func hasInProgressMaintenance(maintenances []incidentResponse) bool {
	for _, maintenance := range maintenances {
		statusValue := strings.ToLower(strings.TrimSpace(maintenance.Status))
		if statusValue == "in_progress" || statusValue == "verifying" {
			return true
		}
	}

	return false
}

func parseTime(value string) *time.Time {
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

var htmlTagPattern = regexp.MustCompile(`<[^>]*>`)

func cleanHTML(value string) string {
	value = html.UnescapeString(value)
	value = htmlTagPattern.ReplaceAllString(value, " ")

	return strings.Join(strings.Fields(value), " ")
}

type summaryResponse struct {
	Page                  pageResponse        `json:"page"`
	Components            []componentResponse `json:"components"`
	Incidents             []incidentResponse  `json:"incidents"`
	ScheduledMaintenances []incidentResponse  `json:"scheduled_maintenances"`
	Status                statusResponse      `json:"status"`
}

type pageResponse struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	URL       string `json:"url"`
	Status    string `json:"status"`
	UpdatedAt string `json:"updated_at"`
}

type statusResponse struct {
	Indicator   string `json:"indicator"`
	Description string `json:"description"`
}

type componentResponse struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Status   string `json:"status"`
	GroupID  string `json:"group_id"`
	Group    bool   `json:"group"`
	Position int    `json:"position"`
}

type incidentResponse struct {
	ID              string                   `json:"id"`
	Name            string                   `json:"name"`
	Status          string                   `json:"status"`
	Impact          string                   `json:"impact"`
	Shortlink       string                   `json:"shortlink"`
	StartedAt       string                   `json:"started_at"`
	UpdatedAt       string                   `json:"updated_at"`
	ScheduledFor    string                   `json:"scheduled_for"`
	ScheduledUntil  string                   `json:"scheduled_until"`
	IncidentUpdates []incidentUpdateResponse `json:"incident_updates"`
}

type incidentUpdateResponse struct {
	Body string `json:"body"`
}
