package pagerdutystatus

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/ikan31/tuip/internal/fetch"
	"github.com/ikan31/tuip/internal/providers"
	"github.com/ikan31/tuip/internal/status"
)

const (
	pagerDutyAPIPrefix = "/api/"
	pagerDutyDataPath  = "/api/data"
)

var htmlTagPattern = regexp.MustCompile(`<[^>]+>`)

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
	components, serviceNames := mapBusinessServices(payload.Layout.LayoutSettings.BusinessServices, state)
	incidents, impacts := p.fetchActivePosts(ctx, serviceNames)
	components = applyComponentImpacts(components, impacts, serviceNames)

	return status.Snapshot{
		ProviderID: p.options.ID,
		Name:       firstNonEmpty(p.options.Name, payload.Layout.LayoutSettings.Name),
		State:      state,
		Summary:    summary,
		SourceURL:  p.options.SourceURL,
		CheckedAt:  checkedAt,
		Incidents:  incidents,
		Components: components,
	}, nil
}

// MapHeadline maps a PagerDuty status page headline into tuip's normalized
// status model.
func MapHeadline(headline string) status.State {
	normalized := strings.ToLower(strings.TrimSpace(headline))

	switch {
	case normalized == "":
		return status.StateUnknown
	case strings.Contains(normalized, "all systems operational") || strings.Contains(normalized, "running smoothly") || strings.Contains(normalized, "paddling smoothly"):
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

func mapBusinessServices(services []businessServiceResponse, headlineState status.State) ([]status.Component, map[string]serviceInfo) {
	groupNames := map[string]string{}

	for _, service := range services {
		if service.GroupingElement && service.ID != "" {
			groupNames[service.ID] = firstNonEmpty(service.DisplayName, service.Summary, service.Name)
		}
	}

	baseState := status.StateUnknown
	if headlineState == status.StateOperational {
		baseState = status.StateOperational
	}

	components := make([]status.Component, 0, len(services))
	serviceNames := map[string]serviceInfo{}
	seen := map[string]bool{}

	for _, service := range services {
		if service.GroupingElement {
			continue
		}

		name := firstNonEmpty(service.DisplayName, service.Summary, service.Name)
		if name == "" {
			continue
		}

		key := firstNonEmpty(service.StatusPageServiceID, service.ID, name)
		if seen[key] {
			continue
		}

		seen[key] = true
		group := groupNames[normalizeRawID(service.HeadID)]
		component := status.Component{
			Name:   name,
			Status: componentStatus(baseState),
			State:  baseState,
			Group:  group,
		}

		components = append(components, component)

		info := serviceInfo{Name: name, Group: group}

		for _, id := range []string{service.StatusPageServiceID, service.ID} {
			if strings.TrimSpace(id) != "" {
				serviceNames[id] = info
			}
		}
	}

	return components, serviceNames
}

func (p *Provider) fetchActivePosts(ctx context.Context, services map[string]serviceInfo) ([]status.Incident, map[string]componentImpact) {
	enums, err := p.fetchPostEnums(ctx)
	if err != nil || len(enums) == 0 {
		return []status.Incident{}, nil
	}

	statusIDs := activePostStatusIDs(enums)
	if len(statusIDs) == 0 {
		return []status.Incident{}, nil
	}

	posts, err := p.fetchPosts(ctx, statusIDs)
	if err != nil {
		return []status.Incident{}, nil
	}

	return mapPosts(posts, enums, services)
}

func (p *Provider) fetchPostEnums(ctx context.Context) (map[string]postEnumResponse, error) {
	endpoint := siblingEndpoint(p.options.DataURL, "post_enums")
	if endpoint == "" {
		return map[string]postEnumResponse{}, nil
	}

	var response postEnumsResponse

	err := p.client.GetJSON(ctx, endpoint, &response)
	if err != nil {
		return nil, fmt.Errorf("fetch PagerDuty post enums: %w", err)
	}

	enums := make(map[string]postEnumResponse, len(response.PostEnums))
	for _, item := range response.PostEnums {
		if item.ID != "" {
			enums[item.ID] = item
		}
	}

	return enums, nil
}

func (p *Provider) fetchPosts(ctx context.Context, statusIDs []string) ([]postResponse, error) {
	endpoint := siblingEndpoint(p.options.DataURL, "posts")
	if endpoint == "" {
		return []postResponse{}, nil
	}

	parsed, err := url.Parse(endpoint)
	if err != nil {
		return nil, fmt.Errorf("parse PagerDuty posts endpoint: %w", err)
	}

	query := parsed.Query()
	for _, statusID := range statusIDs {
		query.Add("statuses[]", statusID)
	}

	query.Set("limit", "500")
	parsed.RawQuery = query.Encode()

	var response postsResponse

	err = p.client.GetJSON(ctx, parsed.String(), &response)
	if err != nil {
		return nil, fmt.Errorf("fetch PagerDuty posts: %w", err)
	}

	return response.Posts, nil
}

func activePostStatusIDs(enums map[string]postEnumResponse) []string {
	ids := make([]string, 0)

	for _, item := range enums {
		postType := strings.ToLower(strings.TrimSpace(item.PostType))
		name := strings.ToLower(strings.TrimSpace(item.Name))
		enumType := strings.ToLower(strings.TrimSpace(item.PostEnumType))

		if enumType != "status" {
			continue
		}

		if postType == "incident" && (name == "detected" || name == "investigating") {
			ids = append(ids, item.ID)
		}

		if postType == "maintenance" && name == "in progress" {
			ids = append(ids, item.ID)
		}
	}

	return ids
}

func mapPosts(posts []postResponse, enums map[string]postEnumResponse, services map[string]serviceInfo) ([]status.Incident, map[string]componentImpact) {
	incidents := make([]status.Incident, 0, len(posts))
	impacts := map[string]componentImpact{}

	for _, post := range posts {
		latest := post.LatestUpdate
		incident := status.Incident{
			Kind:      firstNonEmpty(post.PostType, "incident"),
			Name:      post.Title,
			Status:    enumName(enums, latest.StatusID),
			Impact:    enumName(enums, latest.SeverityID),
			Summary:   cleanText(latest.Message),
			StartedAt: parseTime(firstNonEmpty(post.StartsAt, post.FirstUpdateAt)),
			UpdatedAt: parseTime(latest.ReportedAt),
		}

		if incident.Kind == "maintenance" {
			incident.ScheduledFor = parseTime(post.StartsAt)
			incident.ScheduledUntil = parseTime(post.EndsAt)
		}

		impactedServices := make([]string, 0, len(latest.Impacts))
		for _, impact := range latest.Impacts {
			state := MapImpact(enumName(enums, impact.SeverityID), post.PostType)
			service := services[impact.ServiceID]
			serviceName := firstNonEmpty(service.Name, impact.ServiceID)

			if serviceName != "" {
				impactedServices = append(impactedServices, serviceName)
			}

			if impact.ServiceID != "" {
				impacts[impact.ServiceID] = componentImpact{
					Name:   serviceName,
					Group:  service.Group,
					Status: firstNonEmpty(enumName(enums, impact.SeverityID), componentStatus(state)),
					State:  state,
				}
			}
		}

		if incident.Summary == "" && len(impactedServices) > 0 {
			incident.Summary = "Affected services: " + strings.Join(impactedServices, ", ")
		}

		incidents = append(incidents, incident)
	}

	return incidents, impacts
}

// MapImpact maps PagerDuty status-page impact/severity enum names into tuip's
// normalized component states.
func MapImpact(impact, postType string) status.State {
	normalized := strings.ToLower(strings.TrimSpace(impact))
	postType = strings.ToLower(strings.TrimSpace(postType))

	switch normalized {
	case "operational", "all good":
		return status.StateOperational
	case "maintenance":
		return status.StateMaintenance
	case "partial outage":
		return status.StatePartialOutage
	case "outage", "major":
		return status.StateMajorOutage
	case "minor":
		return status.StateDegraded
	}

	if postType == "maintenance" {
		return status.StateMaintenance
	}

	return status.StateUnknown
}

func applyComponentImpacts(components []status.Component, impacts map[string]componentImpact, services map[string]serviceInfo) []status.Component {
	if len(impacts) == 0 {
		return components
	}

	index := map[string]int{}

	for idx, component := range components {
		for serviceID, service := range services {
			if service.Name == component.Name && service.Group == component.Group {
				index[serviceID] = idx
			}
		}
	}

	for serviceID, impact := range impacts {
		idx, ok := index[serviceID]
		if !ok {
			components = append(components, status.Component{
				Name:   firstNonEmpty(impact.Name, serviceID),
				Status: impact.Status,
				State:  impact.State,
				Group:  impact.Group,
			})

			continue
		}

		components[idx].Status = impact.Status
		components[idx].State = impact.State
	}

	return components
}

func enumName(enums map[string]postEnumResponse, id string) string {
	if item, ok := enums[id]; ok {
		return item.Name
	}

	return ""
}

func componentStatus(state status.State) string {
	if state == "" {
		return string(status.StateUnknown)
	}

	return string(state)
}

func siblingEndpoint(dataURL, endpoint string) string {
	parsed, err := url.Parse(dataURL)
	if err != nil {
		return ""
	}

	switch {
	case strings.HasSuffix(parsed.Path, pagerDutyDataPath):
		parsed.Path = strings.TrimSuffix(parsed.Path, "data") + endpoint
	case strings.HasSuffix(parsed.Path, "/data"):
		parsed.Path = strings.TrimSuffix(parsed.Path, "data") + endpoint
	default:
		parsed.Path = strings.TrimRight(parsed.Path, "/") + pagerDutyAPIPrefix + endpoint
	}

	parsed.RawQuery = ""
	parsed.Fragment = ""

	return parsed.String()
}

func normalizeRawID(raw json.RawMessage) string {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" {
		return ""
	}

	var stringValue string

	err := json.Unmarshal(raw, &stringValue)
	if err == nil {
		return stringValue
	}

	var numberValue json.Number

	err = json.Unmarshal(raw, &numberValue)
	if err == nil {
		return numberValue.String()
	}

	return strings.Trim(trimmed, `"`)
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

	return &parsed
}

func cleanText(value string) string {
	cleaned := htmlTagPattern.ReplaceAllString(value, " ")
	cleaned = html.UnescapeString(cleaned)

	return strings.Join(strings.Fields(cleaned), " ")
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}

	return ""
}

type serviceInfo struct {
	Name  string
	Group string
}

type componentImpact struct {
	Name   string
	Group  string
	Status string
	State  status.State
}

type dataResponse struct {
	Layout layoutResponse `json:"layout"`
}

type layoutResponse struct {
	LayoutSettings layoutSettingsResponse `json:"layout_settings"`
}

type layoutSettingsResponse struct {
	Name             string                    `json:"name"`
	StatusPage       statusPageResponse        `json:"statusPage"`
	BusinessServices []businessServiceResponse `json:"business_services"`
}

type statusPageResponse struct {
	GlobalStatusHeadline string `json:"globalStatusHeadline"`
}

//nolint:tagliatelle // PagerDuty Status Pages use mixed public JSON field names.
type businessServiceResponse struct {
	Summary                 string          `json:"summary"`
	StatusPageServiceID     string          `json:"status_page_service_id"`
	DisplayName             string          `json:"displayName"`
	Name                    string          `json:"name"`
	Description             string          `json:"description"`
	ID                      string          `json:"id"`
	Type                    string          `json:"type"`
	GroupingElement         bool            `json:"grouping_element"`
	HeadID                  json.RawMessage `json:"Head_ID"`
	ChildBusinessServiceIDs []string        `json:"child_business_services"`
}

type postEnumsResponse struct {
	PostEnums []postEnumResponse `json:"post_enums"`
}

type postEnumResponse struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	PostType     string `json:"post_type"`
	PostEnumType string `json:"post_enum_type"`
}

type postsResponse struct {
	Posts []postResponse `json:"posts"`
}

type postResponse struct {
	ID            string             `json:"id"`
	Title         string             `json:"title"`
	PostType      string             `json:"post_type"`
	StartsAt      string             `json:"starts_at"`
	EndsAt        string             `json:"ends_at"`
	FirstUpdateAt string             `json:"first_update_at"`
	LatestUpdate  postUpdateResponse `json:"latest_update"`
}

type postUpdateResponse struct {
	ReportedAt      string               `json:"reported_at"`
	Message         string               `json:"message"`
	StatusID        string               `json:"status_id"`
	SeverityID      string               `json:"severity_id"`
	UpdateFrequency int                  `json:"update_frequency_ms"`
	Impacts         []postImpactResponse `json:"impacts"`
}

type postImpactResponse struct {
	ServiceID  string `json:"service_id"`
	SeverityID string `json:"severity_id"`
}
