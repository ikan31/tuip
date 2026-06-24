package azure

import (
	"context"
	"encoding/xml"
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
	feedURL   = "https://rssfeed.azure.status.microsoft/en-us/status/feed/"
	sourceURL = "https://azure.status.microsoft/en-us/status"
)

const (
	stateRankOperational = iota
	stateRankUnknown
	stateRankMaintenance
	stateRankDegraded
	stateRankPartialOutage
	stateRankMajorOutage
)

// Provider fetches Microsoft's public Azure Status RSS feed.
type Provider struct {
	client   *fetch.Client
	endpoint string
	metadata providers.Metadata
}

// New creates an Azure provider using Microsoft's public Azure Status RSS feed.
func New(client *fetch.Client) *Provider {
	return NewWithEndpoint(client, feedURL, sourceURL)
}

// NewWithEndpoint creates an Azure provider with an override endpoint. It is
// used by tests with httptest fixtures.
func NewWithEndpoint(client *fetch.Client, endpoint string, source string) *Provider {
	return &Provider{
		client:   client,
		endpoint: endpoint,
		metadata: providers.Metadata{
			ID:          "azure",
			Aliases:     []string{"microsoft-azure", "msazure"},
			Name:        "Microsoft Azure",
			Description: "Microsoft Azure public status",
			Category:    "Cloud & Hosting",
			SourceURL:   source,
			APIURL:      endpoint,
		},
	}
}

func (p *Provider) Metadata() providers.Metadata { return p.metadata }

func (p *Provider) Fetch(ctx context.Context) (status.Snapshot, error) {
	text, err := p.client.GetText(ctx, p.endpoint)
	if err != nil {
		return status.Snapshot{}, fmt.Errorf("fetch Azure status: %w", err)
	}

	var payload feedResponse

	err = xml.Unmarshal([]byte(text), &payload)
	if err != nil {
		return status.Snapshot{}, fmt.Errorf("decode Azure status feed: %w", err)
	}

	checkedAt := time.Now().UTC()
	updatedAt := parseAzureTime(payload.Channel.LastBuildDate)
	incidents := mapFeedItems(payload.Channel.Items)
	state := MapFeedState(payload.Channel.Items)

	return status.Snapshot{
		ProviderID: p.metadata.ID,
		Name:       p.metadata.Name,
		State:      state,
		Summary:    azureSummary(len(payload.Channel.Items)),
		SourceURL:  p.metadata.SourceURL,
		CheckedAt:  checkedAt,
		UpdatedAt:  updatedAt,
		Incidents:  incidents,
		Components: []status.Component{},
	}, nil
}

// MapFeedState maps the active Azure Status RSS items into tuip's normalized
// state model. The feed is empty when Azure has no public active events.
func MapFeedState(items []feedItem) status.State {
	if len(items) == 0 {
		return status.StateOperational
	}

	state := status.StateOperational
	for _, item := range items {
		state = maxState(state, MapEventState(item.Title+" "+item.Description+" "+item.Content))
	}

	if state == status.StateOperational {
		return status.StateDegraded
	}

	return state
}

// MapEventState maps Azure RSS item text into a normalized state.
func MapEventState(value string) status.State {
	text := strings.ToLower(strings.TrimSpace(value))
	if text == "" {
		return status.StateDegraded
	}

	majorTerms := []string{"critical", "major", "outage", "unavailable", "down", "widespread", "multiple regions", "multiple services"}
	for _, term := range majorTerms {
		if strings.Contains(text, term) {
			return status.StateMajorOutage
		}
	}

	if strings.Contains(text, "maintenance") {
		return status.StateMaintenance
	}

	degradedTerms := []string{"degraded", "issue", "issues", "impact", "warning", "intermittent", "latency", "failure", "failures", "error", "errors"}
	for _, term := range degradedTerms {
		if strings.Contains(text, term) {
			return status.StateDegraded
		}
	}

	return status.StateDegraded
}

func mapFeedItems(items []feedItem) []status.Incident {
	incidents := make([]status.Incident, 0, len(items))
	for _, item := range items {
		name := strings.TrimSpace(item.Title)
		if name == "" {
			name = "Azure status event"
		}

		updatedAt := parseAzureTime(firstNonEmpty(item.PubDate, item.Updated))
		url := firstNonEmpty(strings.TrimSpace(item.Link), strings.TrimSpace(item.GUID.Value))
		summary := cleanText(firstNonEmpty(item.Description, item.Content))

		incidents = append(incidents, status.Incident{
			Kind:      "incident",
			Name:      name,
			Status:    "active",
			Impact:    string(MapEventState(name + " " + summary)),
			Summary:   summary,
			URL:       url,
			UpdatedAt: updatedAt,
		})
	}

	return incidents
}

func azureSummary(activeEventCount int) string {
	if activeEventCount == 0 {
		return "No active Azure status events"
	}

	if activeEventCount == 1 {
		return "1 active Azure status event"
	}

	return strconv.Itoa(activeEventCount) + " active Azure status events"
}

func maxState(left status.State, right status.State) status.State {
	if stateRank(right) > stateRank(left) {
		return right
	}

	return left
}

func stateRank(state status.State) int {
	switch state {
	case status.StateMajorOutage:
		return stateRankMajorOutage
	case status.StatePartialOutage:
		return stateRankPartialOutage
	case status.StateDegraded:
		return stateRankDegraded
	case status.StateMaintenance:
		return stateRankMaintenance
	case status.StateUnknown, status.StateError:
		return stateRankUnknown
	case status.StateOperational:
		return stateRankOperational
	}

	return stateRankOperational
}

func parseAzureTime(value string) *time.Time {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}

	layouts := []string{
		time.RFC1123Z,
		time.RFC1123,
		"Mon, 02 Jan 2006 15:04:05 Z",
		"Mon, 2 Jan 2006 15:04:05 Z",
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

func cleanText(value string) string {
	value = html.UnescapeString(value)
	value = tagPattern.ReplaceAllString(value, " ")

	return strings.Join(strings.Fields(value), " ")
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}

	return ""
}

var tagPattern = regexp.MustCompile(`(?is)<[^>]*>`)

type feedResponse struct {
	XMLName xml.Name    `xml:"rss"`
	Channel feedChannel `xml:"channel"`
}

type feedChannel struct {
	Title         string     `xml:"title"`
	Link          string     `xml:"link"`
	Description   string     `xml:"description"`
	LastBuildDate string     `xml:"lastBuildDate"`
	Items         []feedItem `xml:"item"`
}

type feedItem struct {
	Title       string   `xml:"title"`
	Link        string   `xml:"link"`
	Description string   `xml:"description"`
	Content     string   `xml:"encoded"`
	PubDate     string   `xml:"pubDate"`
	Updated     string   `xml:"updated"`
	GUID        feedGUID `xml:"guid"`
}

type feedGUID struct {
	Value string `xml:",chardata"`
}
