package statusio

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/ikan31/tuip/internal/fetch"
	"github.com/ikan31/tuip/internal/providers"
	"github.com/ikan31/tuip/internal/status"
)

// Options configures a reusable Status.io public JSON API provider.
type Options struct {
	ID          string
	Aliases     []string
	Name        string
	Description string
	Category    string
	SourceURL   string
	APIURL      string
	StatusURL   string
}

// Provider fetches a Status.io /1.0/status/<page-id> endpoint.
type Provider struct {
	client  *fetch.Client
	options Options
}

// NewProvider creates a Status.io public status API provider.
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
	var payload statusResponse

	err := p.client.GetJSON(ctx, p.options.StatusURL, &payload)
	if err != nil {
		return status.Snapshot{}, fmt.Errorf("fetch %s status: %w", p.options.ID, err)
	}

	checkedAt := time.Now().UTC()
	overall := payload.Result.StatusOverall
	state := MapStatus(overall.Status, overall.StatusCode)
	summary := strings.TrimSpace(overall.Status)
	if summary == "" {
		summary = state.Display()
	}

	components := mapComponents(payload.Result.Status)
	updatedAt := parseTime(overall.Updated)
	if updatedAt == nil {
		updatedAt = latestComponentUpdate(payload.Result.Status)
	}

	return status.Snapshot{
		ProviderID: p.options.ID,
		Name:       p.options.Name,
		State:      state,
		Summary:    summary,
		SourceURL:  p.options.SourceURL,
		CheckedAt:  checkedAt,
		UpdatedAt:  updatedAt,
		Incidents:  []status.Incident{},
		Components: components,
	}, nil
}

// MapStatus maps Status.io status labels/codes into tuip's normalized states.
func MapStatus(label string, code int) status.State {
	switch code {
	case 100:
		return status.StateOperational
	case 200:
		return status.StateMaintenance
	case 300:
		return status.StateDegraded
	case 400:
		return status.StatePartialOutage
	case 500:
		return status.StateMajorOutage
	}

	normalized := strings.ToLower(strings.TrimSpace(label))
	switch {
	case normalized == "":
		return status.StateUnknown
	case strings.Contains(normalized, "operational"):
		return status.StateOperational
	case strings.Contains(normalized, "maintenance"):
		return status.StateMaintenance
	case strings.Contains(normalized, "degraded") || strings.Contains(normalized, "performance"):
		return status.StateDegraded
	case strings.Contains(normalized, "partial") || strings.Contains(normalized, "disruption"):
		return status.StatePartialOutage
	case strings.Contains(normalized, "outage") || strings.Contains(normalized, "down"):
		return status.StateMajorOutage
	default:
		return status.StateUnknown
	}
}

func mapComponents(items []componentResponse) []status.Component {
	components := make([]status.Component, 0, len(items))
	for _, item := range items {
		name := strings.TrimSpace(item.Name)
		if name == "" {
			continue
		}

		components = append(components, status.Component{
			Name:   name,
			Status: item.Status,
			State:  MapStatus(item.Status, item.StatusCode),
			Group:  containerNames(item.Containers),
		})
	}

	return components
}

func containerNames(containers []containerResponse) string {
	names := make([]string, 0, len(containers))
	seen := map[string]bool{}
	for _, container := range containers {
		name := strings.TrimSpace(container.Name)
		if name == "" || seen[name] {
			continue
		}

		seen[name] = true
		names = append(names, name)
	}

	return strings.Join(names, ", ")
}

func latestComponentUpdate(items []componentResponse) *time.Time {
	var latest *time.Time
	for _, item := range items {
		updatedAt := parseTime(item.Updated)
		if updatedAt != nil && (latest == nil || updatedAt.After(*latest)) {
			latest = updatedAt
		}
	}

	return latest
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

type statusResponse struct {
	Result resultResponse `json:"result"`
}

type resultResponse struct {
	StatusOverall overallResponse     `json:"status_overall"`
	Status        []componentResponse `json:"status"`
}

type overallResponse struct {
	Updated    string `json:"updated"`
	Status     string `json:"status"`
	StatusCode int    `json:"status_code"`
}

type componentResponse struct {
	ID         string              `json:"id"`
	Name       string              `json:"name"`
	Status     string              `json:"status"`
	StatusCode int                 `json:"status_code"`
	Containers []containerResponse `json:"containers"`
	Updated    string              `json:"updated"`
}

type containerResponse struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Updated    string `json:"updated"`
	Status     string `json:"status"`
	StatusCode int    `json:"status_code"`
}
