package pagerdutystatus

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/ikan31/tuip/internal/fetch"
	"github.com/ikan31/tuip/internal/providers"
	"github.com/ikan31/tuip/internal/status"
)

// Options configures a provider for PagerDuty-hosted public status pages that
// expose /api/data.
type Options struct {
	ID          string
	Aliases     []string
	Name        string
	Description string
	Category    string
	SourceURL   string
	APIURL      string
	DataURL     string
}

// Provider fetches a PagerDuty public status page /api/data endpoint.
type Provider struct {
	client  *fetch.Client
	options Options
}

// NewProvider creates a PagerDuty public status page provider.
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
	var payload dataResponse

	err := p.client.GetJSON(ctx, p.options.DataURL, &payload)
	if err != nil {
		return status.Snapshot{}, fmt.Errorf("fetch %s status: %w", p.options.ID, err)
	}

	checkedAt := time.Now().UTC()

	summary := strings.TrimSpace(payload.Layout.LayoutSettings.StatusPage.GlobalStatusHeadline)
	if summary == "" {
		summary = "Unknown status"
	}

	state := MapHeadline(summary)

	return status.Snapshot{
		ProviderID: p.options.ID,
		Name:       firstNonEmpty(payload.Layout.LayoutSettings.Name, p.options.Name),
		State:      state,
		Summary:    summary,
		SourceURL:  p.options.SourceURL,
		CheckedAt:  checkedAt,
		Incidents:  []status.Incident{},
		Components: []status.Component{},
	}, nil
}

// MapHeadline maps a PagerDuty status page headline into tuip's normalized
// status model.
func MapHeadline(headline string) status.State {
	normalized := strings.ToLower(strings.TrimSpace(headline))

	switch {
	case normalized == "":
		return status.StateUnknown
	case strings.Contains(normalized, "all systems operational") || strings.Contains(normalized, "running smoothly"):
		return status.StateOperational
	case strings.Contains(normalized, "maintenance"):
		return status.StateMaintenance
	case strings.Contains(normalized, "partial"):
		return status.StatePartialOutage
	case strings.Contains(normalized, "degraded"):
		return status.StateDegraded
	case strings.Contains(normalized, "major") || strings.Contains(normalized, "outage") || strings.Contains(normalized, "incident"):
		return status.StateMajorOutage
	default:
		return status.StateUnknown
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}

	return ""
}

type dataResponse struct {
	Layout layoutResponse `json:"layout"`
}

type layoutResponse struct {
	LayoutSettings layoutSettingsResponse `json:"layout_settings"`
}

type layoutSettingsResponse struct {
	Name       string             `json:"name"`
	StatusPage statusPageResponse `json:"statusPage"`
}

type statusPageResponse struct {
	GlobalStatusHeadline string `json:"globalStatusHeadline"`
}
