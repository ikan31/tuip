package datadog

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/ikan31/tuip/internal/fetch"
	"github.com/ikan31/tuip/internal/providers"
	"github.com/ikan31/tuip/internal/status"
)

// Provider reads the public config.json used by Datadog's regional status pages.
type Provider struct {
	client   *fetch.Client
	metadata providers.Metadata
}

func NewProvider(client *fetch.Client, metadata providers.Metadata) *Provider {
	return &Provider{client: client, metadata: metadata}
}

func (p *Provider) Metadata() providers.Metadata {
	return p.metadata
}

func (p *Provider) Fetch(ctx context.Context) (status.Snapshot, error) {
	var page pageResponse

	err := p.client.GetJSON(ctx, p.metadata.APIURL, &page)
	if err != nil {
		return status.Snapshot{}, fmt.Errorf("fetch %s status: %w", p.metadata.ID, err)
	}

	components := make([]status.Component, 0, len(page.Components))
	for _, component := range page.Components {
		if component.Type == "ComponentGroup" {
			for _, member := range component.Components {
				components = append(components, mapComponent(member, component.Name))
			}

			continue
		}

		components = append(components, mapComponent(component, ""))
	}

	if len(components) == 0 {
		return status.Snapshot{}, fmt.Errorf("fetch %s status: response has no components", p.metadata.ID)
	}

	state := status.StateUnknown
	for _, component := range components {
		if slices.Index(severity, component.State) > slices.Index(severity, state) {
			state = component.State
		}
	}

	incidents := make([]status.Incident, 0)

	for _, incident := range page.Incidents {
		if incident.Resolved || incident.CurrentStatus == "resolved" {
			continue
		}

		incidents = append(incidents, status.Incident{
			Kind:      "incident",
			Name:      incident.Title,
			Status:    incident.CurrentStatus,
			Summary:   incident.Description,
			StartedAt: incident.PublishedDate,
			UpdatedAt: incident.LastModifiedAt,
		})
	}

	for _, maintenance := range page.Maintenances {
		summary := maintenance.ScheduledDescription
		switch maintenance.CurrentStatus {
		case "in_progress":
			summary = maintenance.InProgressDescription

			if state == status.StateOperational {
				state = status.StateMaintenance
			}
		case "scheduled":
		default:
			continue
		}

		incidents = append(incidents, status.Incident{
			Kind:           "maintenance",
			Name:           maintenance.Title,
			Status:         maintenance.CurrentStatus,
			Summary:        summary,
			ScheduledFor:   maintenance.StartDate,
			ScheduledUntil: maintenance.CompletedDate,
		})
	}

	return status.Snapshot{
		ProviderID: p.metadata.ID,
		Name:       p.metadata.Name,
		State:      state,
		Summary:    state.Display(),
		SourceURL:  p.metadata.SourceURL,
		CheckedAt:  time.Now().UTC(),
		Incidents:  incidents,
		Components: components,
	}, nil
}

// Datadog's component status values match tuip's normalized states.
var severity = []status.State{
	status.StateOperational,
	status.StateMaintenance,
	status.StateDegraded,
	status.StatePartialOutage,
	status.StateMajorOutage,
}

func mapComponent(component componentResponse, group string) status.Component {
	state := status.State(component.Status)
	if !slices.Contains(severity, state) {
		state = status.StateUnknown
	}

	return status.Component{Name: component.Name, Status: component.Status, State: state, Group: group}
}

type pageResponse struct {
	Components   []componentResponse   `json:"components"`
	Incidents    []incidentResponse    `json:"incidents"`
	Maintenances []maintenanceResponse `json:"maintenances"`
}

type componentResponse struct {
	Name       string              `json:"name"`
	Status     string              `json:"status"`
	Type       string              `json:"type"`
	Components []componentResponse `json:"components"`
}

//nolint:tagliatelle // Datadog uses camelCase public JSON fields.
type incidentResponse struct {
	Title          string     `json:"title"`
	CurrentStatus  string     `json:"currentStatus"`
	Description    string     `json:"description"`
	Resolved       bool       `json:"resolved"`
	PublishedDate  *time.Time `json:"publishedDate"`
	LastModifiedAt *time.Time `json:"lastModifiedAt"`
}

//nolint:tagliatelle // Datadog uses camelCase public JSON fields.
type maintenanceResponse struct {
	Title                 string     `json:"title"`
	CurrentStatus         string     `json:"currentStatus"`
	ScheduledDescription  string     `json:"scheduledDescription"`
	InProgressDescription string     `json:"inProgressDescription"`
	StartDate             *time.Time `json:"startDate"`
	CompletedDate         *time.Time `json:"completedDate"`
}
