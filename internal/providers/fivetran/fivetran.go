package fivetran

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/ikan31/tuip/internal/fetch"
	"github.com/ikan31/tuip/internal/providers"
	"github.com/ikan31/tuip/internal/status"
)

const (
	providerID        = "fivetran"
	providerName      = "Fivetran"
	sourceURL         = "https://status.fivetran.com/"
	majorOutageRank   = 5
	partialOutageRank = 4
	maintenanceRank   = 3
	degradedRank      = 2
	unknownRank       = 1
	operationalRank   = 0

	apiURL    = "https://status.fivetran.com/api/v1/status"
	activeURL = "https://status.fivetran.com/api/v1/active"
)

// Provider fetches Fivetran's current public status API.
type Provider struct {
	client    *fetch.Client
	statusURL string
	activeURL string
}

// New creates a Fivetran provider.
func New(client *fetch.Client) *Provider {
	return &Provider{
		client:    client,
		statusURL: apiURL,
		activeURL: activeURL,
	}
}

// NewWithURLs creates a Fivetran provider with overridden API URLs.
func NewWithURLs(client *fetch.Client, statusURL, activeURL string) *Provider {
	return &Provider{
		client:    client,
		statusURL: statusURL,
		activeURL: activeURL,
	}
}

func (p *Provider) Metadata() providers.Metadata {
	return providers.Metadata{
		ID:          providerID,
		Name:        providerName,
		Description: "Fivetran service status",
		Category:    "Data Integration",
		SourceURL:   sourceURL,
		APIURL:      p.statusURL,
	}
}

func (p *Provider) Fetch(ctx context.Context) (status.Snapshot, error) {
	var statusPayload currentStatusResponse

	err := p.client.GetJSON(ctx, p.statusURL, &statusPayload)
	if err != nil {
		return status.Snapshot{}, fmt.Errorf("fetch fivetran status: %w", err)
	}

	var activePayload activeResponse

	err = p.client.GetJSON(ctx, p.activeURL, &activePayload)
	if err != nil {
		return status.Snapshot{}, fmt.Errorf("fetch fivetran active incidents: %w", err)
	}

	checkedAt := time.Now().UTC()
	components := mapComponents(statusPayload.Groups)
	incidents := mapActiveIncidents(activePayload)
	state := overallState(components, incidents)

	return status.Snapshot{
		ProviderID: providerID,
		Name:       providerName,
		State:      state,
		Summary:    summary(state, incidents),
		SourceURL:  sourceURL,
		CheckedAt:  checkedAt,
		UpdatedAt:  latestUpdate(statusPayload.Groups, incidents),
		Incidents:  incidents,
		Components: components,
	}, nil
}

func mapComponents(groups []groupResponse) []status.Component {
	componentCount := 0
	for _, group := range groups {
		componentCount += 1 + len(group.Services)
	}

	components := make([]status.Component, 0, componentCount)
	for _, group := range groups {
		components = append(components, status.Component{
			Name:   group.Name,
			Status: group.Status,
			State:  mapServiceStatus(group.Status),
		})

		for _, service := range group.Services {
			components = append(components, status.Component{
				Name:   service.Name,
				Status: service.Status,
				State:  mapServiceStatus(service.Status),
				Group:  group.Name,
			})
		}
	}

	return components
}

func mapActiveIncidents(active activeResponse) []status.Incident {
	incidents := make([]status.Incident, 0, len(active.Incidents)+len(active.Maintenances))
	for _, incident := range active.Incidents {
		incidents = append(incidents, status.Incident{
			Kind:      "incident",
			Name:      incident.Title,
			Status:    incident.Status,
			Impact:    incident.Impact,
			Summary:   strings.TrimSpace(incident.Message),
			StartedAt: parseTime(incident.CreatedAt),
			UpdatedAt: parseTime(incident.UpdatedAt),
		})
	}

	for _, maintenance := range active.Maintenances {
		incidents = append(incidents, status.Incident{
			Kind:           "maintenance",
			Name:           maintenance.Title,
			Status:         maintenance.Status,
			Impact:         maintenance.Impact,
			Summary:        strings.TrimSpace(maintenance.Message),
			StartedAt:      parseTime(maintenance.CreatedAt),
			UpdatedAt:      parseTime(maintenance.UpdatedAt),
			ScheduledFor:   parseTime(maintenance.ScheduledFor),
			ScheduledUntil: parseTime(maintenance.ScheduledUntil),
		})
	}

	return incidents
}

func overallState(components []status.Component, incidents []status.Incident) status.State {
	state := status.StateOperational
	for _, component := range components {
		state = worstState(state, component.State)
	}

	for _, incident := range incidents {
		if incident.Kind == "maintenance" {
			state = worstState(state, status.StateMaintenance)

			continue
		}

		state = worstState(state, mapImpact(incident.Impact))
	}

	return state
}

func summary(state status.State, incidents []status.Incident) string {
	for _, incident := range incidents {
		if strings.TrimSpace(incident.Name) != "" {
			return incident.Name
		}
	}

	if state == status.StateOperational {
		return "All Systems Operational"
	}

	return state.Display()
}

func latestUpdate(groups []groupResponse, incidents []status.Incident) *time.Time {
	var latest *time.Time
	for _, group := range groups {
		latest = maxTime(latest, parseTime(group.UpdatedAt))
	}

	for _, incident := range incidents {
		latest = maxTime(latest, incident.UpdatedAt)
		latest = maxTime(latest, incident.StartedAt)
	}

	return latest
}

func maxTime(current, candidate *time.Time) *time.Time {
	if candidate == nil {
		return current
	}

	if current == nil || candidate.After(*current) {
		return candidate
	}

	return current
}

func mapServiceStatus(value string) status.State {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "operational", "resolved":
		return status.StateOperational
	case "degraded", "degraded_performance", "partial", "partial_outage":
		return status.StateDegraded
	case "major", "major_outage", "outage", "down":
		return status.StateMajorOutage
	case "maintenance", "under_maintenance":
		return status.StateMaintenance
	case "unknown", "":
		return status.StateUnknown
	default:
		return status.StateUnknown
	}
}

func mapImpact(value string) status.State {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "none", "":
		return status.StateUnknown
	case "minor":
		return status.StateDegraded
	case "major":
		return status.StatePartialOutage
	case "critical":
		return status.StateMajorOutage
	default:
		return status.StateDegraded
	}
}

func worstState(left, right status.State) status.State {
	if stateRank(right) > stateRank(left) {
		return right
	}

	return left
}

func stateRank(state status.State) int {
	switch state {
	case status.StateError, status.StateMajorOutage:
		return majorOutageRank
	case status.StatePartialOutage:
		return partialOutageRank
	case status.StateMaintenance:
		return maintenanceRank
	case status.StateDegraded:
		return degradedRank
	case status.StateUnknown:
		return unknownRank
	case status.StateOperational:
		return operationalRank
	default:
		return unknownRank
	}
}

func parseTime(value string) *time.Time {
	if strings.TrimSpace(value) == "" {
		return nil
	}

	for _, layout := range []string{time.RFC3339Nano, time.RFC3339} {
		parsed, err := time.Parse(layout, value)
		if err == nil {
			return &parsed
		}
	}

	return nil
}

type currentStatusResponse struct {
	Groups []groupResponse `json:"groups"`
}

type groupResponse struct {
	Name      string            `json:"group_name"`
	Status    string            `json:"status"`
	UpdatedAt string            `json:"updated_at"`
	Services  []serviceResponse `json:"services"`
}

type serviceResponse struct {
	Name   string `json:"name"`
	Status string `json:"status"`
}

type activeResponse struct {
	Incidents    []activeItemResponse `json:"incidents"`
	Maintenances []activeItemResponse `json:"maintenances"`
}

type activeItemResponse struct {
	Title          string `json:"title"`
	Message        string `json:"message"`
	Status         string `json:"status"`
	Impact         string `json:"impact"`
	CreatedAt      string `json:"created_at"`
	UpdatedAt      string `json:"updated_at"`
	ScheduledFor   string `json:"scheduled_for"`
	ScheduledUntil string `json:"scheduled_until"`
}
