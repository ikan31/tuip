package uptimekuma

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/ikan31/tuip/internal/fetch"
	"github.com/ikan31/tuip/internal/providers"
	"github.com/ikan31/tuip/internal/status"
)

// Options configures a reusable Uptime Kuma public status page provider.
type Options struct {
	ID           string
	Aliases      []string
	Name         string
	Description  string
	Category     string
	SourceURL    string
	APIURL       string
	StatusURL    string
	HeartbeatURL string
}

// Provider fetches Uptime Kuma public status page JSON endpoints.
type Provider struct {
	client  *fetch.Client
	options Options
}

const (
	heartbeatDown        = 0
	heartbeatUp          = 1
	heartbeatPending     = 2
	heartbeatMaintenance = 3

	severityOperational   = 0
	severityUnknown       = 1
	severityMaintenance   = 2
	severityDegraded      = 3
	severityPartialOutage = 4
	severityMajorOutage   = 5
)

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
	var page pageResponse

	err := p.client.GetJSON(ctx, p.options.StatusURL, &page)
	if err != nil {
		return status.Snapshot{}, fmt.Errorf("fetch %s status page: %w", p.options.ID, err)
	}

	var heartbeats heartbeatResponse

	err = p.client.GetJSON(ctx, p.options.HeartbeatURL, &heartbeats)
	if err != nil {
		return status.Snapshot{}, fmt.Errorf("fetch %s heartbeats: %w", p.options.ID, err)
	}

	components, overall, updatedAt := mapComponents(page.PublicGroupList, heartbeats.HeartbeatList)
	incidents := mapIncidents(page.Incidents)
	summary := summaryForState(overall, len(components))

	return status.Snapshot{
		ProviderID: p.options.ID,
		Name:       p.options.Name,
		State:      overall,
		Summary:    summary,
		SourceURL:  p.options.SourceURL,
		CheckedAt:  time.Now().UTC(),
		UpdatedAt:  updatedAt,
		Incidents:  incidents,
		Components: components,
	}, nil
}

func mapComponents(groups []publicGroup, heartbeatList map[string][]heartbeat) ([]status.Component, status.State, *time.Time) {
	components := make([]status.Component, 0)
	overall := status.StateOperational

	var updatedAt *time.Time

	for _, group := range groups {
		for _, monitor := range group.MonitorList {
			latest, ok := latestHeartbeat(heartbeatList[strconv.Itoa(monitor.ID)])
			state := status.StateUnknown
			statusValue := "unknown"

			if ok {
				state = mapHeartbeatStatus(latest.Status)
				statusValue = heartbeatStatusText(latest.Status)

				if parsed := parseUptimeKumaTime(latest.Time); parsed != nil && (updatedAt == nil || parsed.After(*updatedAt)) {
					updatedAt = parsed
				}
			}

			components = append(components, status.Component{
				Name:   strings.TrimSpace(monitor.Name),
				Status: statusValue,
				State:  state,
				Group:  strings.TrimSpace(group.Name),
			})

			overall = worseState(overall, state)
		}
	}

	if len(components) == 0 {
		overall = status.StateUnknown
	}

	return components, overall, updatedAt
}

func latestHeartbeat(items []heartbeat) (heartbeat, bool) {
	if len(items) == 0 {
		return heartbeat{}, false
	}

	return items[len(items)-1], true
}

func mapHeartbeatStatus(value int) status.State {
	switch value {
	case heartbeatUp:
		return status.StateOperational
	case heartbeatMaintenance:
		return status.StateMaintenance
	case heartbeatDown:
		return status.StateMajorOutage
	case heartbeatPending:
		return status.StateUnknown
	default:
		return status.StateUnknown
	}
}

func heartbeatStatusText(value int) string {
	switch value {
	case heartbeatDown:
		return "down"
	case heartbeatUp:
		return "up"
	case heartbeatPending:
		return "pending"
	case heartbeatMaintenance:
		return "maintenance"
	default:
		return "unknown"
	}
}

func worseState(left, right status.State) status.State {
	if severity(right) > severity(left) {
		return right
	}

	return left
}

func severity(state status.State) int {
	switch state {
	case status.StateMajorOutage, status.StateError:
		return severityMajorOutage
	case status.StatePartialOutage:
		return severityPartialOutage
	case status.StateDegraded:
		return severityDegraded
	case status.StateMaintenance:
		return severityMaintenance
	case status.StateUnknown:
		return severityUnknown
	case status.StateOperational:
		return severityOperational
	default:
		return severityUnknown
	}
}

func summaryForState(state status.State, componentCount int) string {
	if componentCount == 0 {
		return "No components exposed"
	}

	if state == status.StateOperational {
		return "All monitored services are operational"
	}

	return state.Display()
}

func mapIncidents(items []incident) []status.Incident {
	incidents := make([]status.Incident, 0, len(items))
	for _, item := range items {
		incidents = append(incidents, status.Incident{
			Kind:      "incident",
			Name:      strings.TrimSpace(item.Title),
			Status:    strings.TrimSpace(item.Style),
			Summary:   strings.TrimSpace(item.Content),
			StartedAt: parseUptimeKumaTime(item.CreatedDate),
			UpdatedAt: parseUptimeKumaTime(item.LastUpdatedDate),
		})
	}

	return incidents
}

func parseUptimeKumaTime(value string) *time.Time {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}

	layouts := []string{
		"2006-01-02 15:04:05.000",
		"2006-01-02 15:04:05",
		time.RFC3339,
	}

	for _, layout := range layouts {
		parsed, err := time.Parse(layout, value)
		if err == nil {
			utc := parsed.UTC()

			return &utc
		}
	}

	return nil
}

//nolint:tagliatelle // Uptime Kuma uses camelCase public JSON fields.
type pageResponse struct {
	Config          pageConfig    `json:"config"`
	Incidents       []incident    `json:"incidents"`
	PublicGroupList []publicGroup `json:"publicGroupList"`
}

type pageConfig struct {
	Title string `json:"title"`
}

//nolint:tagliatelle // Uptime Kuma uses camelCase public JSON fields.
type publicGroup struct {
	Name        string    `json:"name"`
	MonitorList []monitor `json:"monitorList"`
}

type monitor struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

//nolint:tagliatelle // Uptime Kuma uses camelCase public JSON fields.
type incident struct {
	Title           string `json:"title"`
	Content         string `json:"content"`
	Style           string `json:"style"`
	CreatedDate     string `json:"createdDate"`
	LastUpdatedDate string `json:"lastUpdatedDate"`
}

//nolint:tagliatelle // Uptime Kuma uses camelCase public JSON fields.
type heartbeatResponse struct {
	HeartbeatList map[string][]heartbeat `json:"heartbeatList"`
}

type heartbeat struct {
	Status int    `json:"status"`
	Time   string `json:"time"`
	Msg    string `json:"msg"`
}
