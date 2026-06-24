package gcp

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/ikan31/tuip/internal/fetch"
	"github.com/ikan31/tuip/internal/providers"
	"github.com/ikan31/tuip/internal/status"
)

const (
	incidentsURL = "https://status.cloud.google.com/incidents.json"
	sourceURL    = "https://status.cloud.google.com/"
)

const (
	stateRankOperational = iota
	stateRankUnknown
	stateRankMaintenance
	stateRankDegraded
	stateRankPartialOutage
	stateRankMajorOutage
)

// Provider fetches Google Cloud's public structured incident JSON.
type Provider struct {
	client   *fetch.Client
	endpoint string
	metadata providers.Metadata
}

// New creates a Google Cloud provider using the public incidents JSON endpoint.
func New(client *fetch.Client) *Provider {
	return NewWithEndpoint(client, incidentsURL, sourceURL)
}

// NewWithEndpoint creates a Google Cloud provider with an override endpoint. It
// is used by tests with httptest fixtures.
func NewWithEndpoint(client *fetch.Client, endpoint string, source string) *Provider {
	return &Provider{
		client:   client,
		endpoint: endpoint,
		metadata: providers.Metadata{
			ID:          "gcp",
			Aliases:     []string{"google-cloud", "google-cloud-platform"},
			Name:        "Google Cloud",
			Description: "Google Cloud public status",
			Category:    "Cloud & Hosting",
			SourceURL:   source,
			APIURL:      endpoint,
		},
	}
}

func (p *Provider) Metadata() providers.Metadata { return p.metadata }

func (p *Provider) Fetch(ctx context.Context) (status.Snapshot, error) {
	var payload []incidentResponse

	err := p.client.GetJSON(ctx, p.endpoint, &payload)
	if err != nil {
		return status.Snapshot{}, fmt.Errorf("fetch Google Cloud status: %w", err)
	}

	active := activeIncidents(payload)
	state := MapIncidentsState(active)
	incidents := mapIncidents(active)
	components := mapComponents(active)

	return status.Snapshot{
		ProviderID: p.metadata.ID,
		Name:       p.metadata.Name,
		State:      state,
		Summary:    summaryForActiveIncidents(len(active)),
		SourceURL:  p.metadata.SourceURL,
		CheckedAt:  time.Now().UTC(),
		UpdatedAt:  latestModified(active),
		Incidents:  incidents,
		Components: components,
	}, nil
}

func activeIncidents(items []incidentResponse) []incidentResponse {
	active := make([]incidentResponse, 0, len(items))
	for _, item := range items {
		if strings.TrimSpace(item.End) != "" {
			continue
		}

		if MapImpact(firstNonEmpty(item.MostRecentUpdate.Status, item.StatusImpact)) == status.StateOperational {
			continue
		}

		active = append(active, item)
	}

	return active
}

// MapIncidentsState maps active Google Cloud incidents into tuip's normalized
// overall state.
func MapIncidentsState(items []incidentResponse) status.State {
	if len(items) == 0 {
		return status.StateOperational
	}

	state := status.StateOperational
	for _, item := range items {
		state = worseState(state, mapIncidentState(item))
	}

	if state == status.StateOperational {
		return status.StateDegraded
	}

	return state
}

// MapImpact maps Google Cloud's status impact values into normalized states.
func MapImpact(value string) status.State {
	normalized := strings.ToUpper(strings.TrimSpace(value))

	switch {
	case normalized == "":
		return status.StateUnknown
	case strings.Contains(normalized, "AVAILABLE"):
		return status.StateOperational
	case strings.Contains(normalized, "MAINTENANCE"):
		return status.StateMaintenance
	case strings.Contains(normalized, "OUTAGE"):
		return status.StateMajorOutage
	case strings.Contains(normalized, "DISRUPTION") || strings.Contains(normalized, "ISSUE") || strings.Contains(normalized, "INFORMATION"):
		return status.StateDegraded
	default:
		return status.StateUnknown
	}
}

func mapIncidentState(item incidentResponse) status.State {
	state := MapImpact(firstNonEmpty(item.MostRecentUpdate.Status, item.StatusImpact))
	severity := strings.ToLower(strings.TrimSpace(item.Severity))

	if severity == "high" && state == status.StateDegraded {
		return status.StateMajorOutage
	}

	if state == status.StateUnknown && severity != "" {
		return status.StateDegraded
	}

	return state
}

func mapIncidents(items []incidentResponse) []status.Incident {
	incidents := make([]status.Incident, 0, len(items))
	for _, item := range items {
		name := strings.TrimSpace(item.ExternalDesc)
		if name == "" {
			name = "Google Cloud incident " + strings.TrimSpace(item.ID)
		}

		incidents = append(incidents, status.Incident{
			Kind:      "incident",
			Name:      name,
			Status:    firstNonEmpty(item.MostRecentUpdate.Status, item.StatusImpact),
			Impact:    strings.TrimSpace(item.Severity),
			Summary:   cleanText(firstNonEmpty(item.MostRecentUpdate.Text, item.ExternalDesc)),
			URL:       incidentURL(item.URI),
			StartedAt: parseGCPTime(item.Begin),
			UpdatedAt: parseGCPTime(firstNonEmpty(item.MostRecentUpdate.Modified, item.MostRecentUpdate.When, item.Modified)),
		})
	}

	return incidents
}

func mapComponents(items []incidentResponse) []status.Component {
	components := make([]status.Component, 0)
	componentIndexes := map[string]int{}

	for _, item := range items {
		state := mapIncidentState(item)
		statusText := componentStatusText(item)
		locations := joinRefs(item.CurrentlyAffectedLocations)

		for _, product := range item.AffectedProducts {
			name := firstNonEmpty(product.Title, product.ID)
			if name == "" {
				continue
			}

			key := firstNonEmpty(product.ID, name)
			if index, ok := componentIndexes[key]; ok {
				components[index].State = worseState(components[index].State, state)
				components[index].Status = mergeStatusText(components[index].Status, statusText)
				components[index].Group = mergeStatusText(components[index].Group, locations)

				continue
			}

			componentIndexes[key] = len(components)
			components = append(components, status.Component{
				Name:   name,
				Status: statusText,
				State:  state,
				Group:  locations,
			})
		}
	}

	return components
}

func componentStatusText(item incidentResponse) string {
	statusText := firstNonEmpty(item.MostRecentUpdate.Status, item.StatusImpact)

	severity := strings.TrimSpace(item.Severity)
	if severity != "" {
		statusText += " (" + severity + ")"
	}

	return strings.TrimSpace(statusText)
}

func mergeStatusText(left, right string) string {
	left = strings.TrimSpace(left)
	right = strings.TrimSpace(right)

	if left == "" || left == right {
		return right
	}

	if right == "" {
		return left
	}

	return left + "; " + right
}

func joinRefs(refs []namedRef) string {
	names := make([]string, 0, len(refs))
	seen := map[string]bool{}

	for _, ref := range refs {
		name := firstNonEmpty(ref.Title, ref.ID)
		if name == "" || seen[name] {
			continue
		}

		seen[name] = true
		names = append(names, name)
	}

	return strings.Join(names, ", ")
}

func summaryForActiveIncidents(activeIncidentCount int) string {
	if activeIncidentCount == 0 {
		return "No active Google Cloud incidents"
	}

	if activeIncidentCount == 1 {
		return "1 active Google Cloud incident"
	}

	return strconv.Itoa(activeIncidentCount) + " active Google Cloud incidents"
}

func latestModified(items []incidentResponse) *time.Time {
	var latest *time.Time

	for _, item := range items {
		parsed := parseGCPTime(firstNonEmpty(item.MostRecentUpdate.Modified, item.MostRecentUpdate.When, item.Modified))
		if parsed != nil && (latest == nil || parsed.After(*latest)) {
			latest = parsed
		}
	}

	return latest
}

func incidentURL(uri string) string {
	uri = strings.TrimSpace(uri)
	if uri == "" {
		return sourceURL
	}

	parsed, err := url.Parse(uri)
	if err == nil && parsed.IsAbs() {
		return uri
	}

	return sourceURL + strings.TrimPrefix(uri, "/")
}

func cleanText(value string) string {
	value = strings.ReplaceAll(value, "**", "")
	value = strings.ReplaceAll(value, "__", "")

	return strings.Join(strings.Fields(value), " ")
}

func parseGCPTime(value string) *time.Time {
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

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}

	return ""
}

func worseState(left, right status.State) status.State {
	if stateRank(right) > stateRank(left) {
		return right
	}

	return left
}

func stateRank(state status.State) int {
	switch state {
	case status.StateMajorOutage, status.StateError:
		return stateRankMajorOutage
	case status.StatePartialOutage:
		return stateRankPartialOutage
	case status.StateDegraded:
		return stateRankDegraded
	case status.StateMaintenance:
		return stateRankMaintenance
	case status.StateUnknown:
		return stateRankUnknown
	case status.StateOperational:
		return stateRankOperational
	}

	return stateRankUnknown
}

type incidentResponse struct {
	ID                          string           `json:"id"`
	Number                      string           `json:"number"`
	Begin                       string           `json:"begin"`
	Created                     string           `json:"created"`
	End                         string           `json:"end"`
	Modified                    string           `json:"modified"`
	ExternalDesc                string           `json:"external_desc"`
	Updates                     []updateResponse `json:"updates"`
	MostRecentUpdate            updateResponse   `json:"most_recent_update"`
	StatusImpact                string           `json:"status_impact"`
	Severity                    string           `json:"severity"`
	AffectedProducts            []namedRef       `json:"affected_products"`
	CurrentlyAffectedLocations  []namedRef       `json:"currently_affected_locations"`
	PreviouslyAffectedLocations []namedRef       `json:"previously_affected_locations"`
	ServiceKey                  string           `json:"service_key"`
	ServiceName                 string           `json:"service_name"`
	URI                         string           `json:"uri"`
}

type updateResponse struct {
	Created           string     `json:"created"`
	Modified          string     `json:"modified"`
	Text              string     `json:"text"`
	When              string     `json:"when"`
	Status            string     `json:"status"`
	AffectedLocations []namedRef `json:"affected_locations"`
}

type namedRef struct {
	Title string `json:"title"`
	ID    string `json:"id"`
}
