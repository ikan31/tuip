package docker

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
	rssURL    = "https://www.dockerstatus.com/pages/533c6539221ae15e3f000031/rss"
	sourceURL = "https://www.dockerstatus.com/"
)

const (
	stateRankOperational = iota
	stateRankUnknown
	stateRankMaintenance
	stateRankDegraded
	stateRankPartialOutage
	stateRankMajorOutage
)

// Provider fetches Docker's public Status.io RSS feed.
type Provider struct {
	client   *fetch.Client
	endpoint string
	metadata providers.Metadata
}

// New creates a Docker provider using Docker's public Status.io RSS feed.
func New(client *fetch.Client) *Provider {
	return NewWithEndpoint(client, rssURL, sourceURL)
}

// NewWithEndpoint creates a Docker provider with an override endpoint. It is
// used by tests with httptest fixtures.
func NewWithEndpoint(client *fetch.Client, endpoint string, source string) *Provider {
	return &Provider{
		client:   client,
		endpoint: endpoint,
		metadata: providers.Metadata{
			ID:          "docker",
			Aliases:     []string{"docker-hub", "dockerhub"},
			Name:        "Docker",
			Description: "Docker service status",
			Category:    "Package Registries",
			SourceURL:   source,
			APIURL:      endpoint,
		},
	}
}

func (p *Provider) Metadata() providers.Metadata { return p.metadata }

func (p *Provider) Fetch(ctx context.Context) (status.Snapshot, error) {
	text, err := p.client.GetText(ctx, p.endpoint)
	if err != nil {
		return status.Snapshot{}, fmt.Errorf("fetch Docker status: %w", err)
	}

	var payload feedResponse

	err = xml.Unmarshal([]byte(text), &payload)
	if err != nil {
		return status.Snapshot{}, fmt.Errorf("decode Docker status feed: %w", err)
	}

	items := activeItems(payload.Channel.Items)
	state := MapFeedState(items)

	return status.Snapshot{
		ProviderID: p.metadata.ID,
		Name:       p.metadata.Name,
		State:      state,
		Summary:    summaryForItems(len(items)),
		SourceURL:  p.metadata.SourceURL,
		CheckedAt:  time.Now().UTC(),
		UpdatedAt:  latestPubDate(items, payload.Channel.LastBuildDate),
		Incidents:  mapIncidents(items),
		Components: []status.Component{},
	}, nil
}

// MapFeedState maps active Docker RSS items into tuip's normalized overall
// state. Docker's Status.io RSS feed includes history, so callers should pass
// only items whose latest update is not resolved or completed.
func MapFeedState(items []feedItem) status.State {
	if len(items) == 0 {
		return status.StateOperational
	}

	state := status.StateOperational
	for _, item := range items {
		state = worseState(state, mapItemState(item))
	}

	if state == status.StateOperational {
		return status.StateDegraded
	}

	return state
}

func activeItems(items []feedItem) []feedItem {
	active := make([]feedItem, 0, len(items))
	for _, item := range items {
		latest := latestUpdate(item.Description)
		if !isTerminalStatus(latest.Status) {
			active = append(active, item)
		}
	}

	return active
}

func mapIncidents(items []feedItem) []status.Incident {
	incidents := make([]status.Incident, 0, len(items))
	for _, item := range items {
		latest := latestUpdate(item.Description)

		name := strings.TrimSpace(item.Title)
		if name == "" {
			name = "Docker status event"
		}

		incidents = append(incidents, status.Incident{
			Kind:      incidentKind(item, latest),
			Name:      name,
			Status:    latest.Status,
			Impact:    string(mapItemState(item)),
			Summary:   firstNonEmpty(latest.Text, cleanText(item.Description)),
			URL:       firstNonEmpty(strings.TrimSpace(item.Link), sourceURL),
			UpdatedAt: parseDockerTime(item.PubDate),
		})
	}

	return incidents
}

func incidentKind(item feedItem, latest feedUpdate) string {
	if strings.Contains(strings.ToLower(item.Link+" "+item.Title+" "+latest.Status), "maintenance") {
		return "maintenance"
	}

	return "incident"
}

func mapItemState(item feedItem) status.State {
	latest := latestUpdate(item.Description)
	statusText := strings.ToLower(strings.TrimSpace(latest.Status))
	body := strings.ToLower(item.Title + " " + latest.Text + " " + item.Description)

	switch statusText {
	case "resolved", "completed":
		return status.StateOperational
	case "active":
		if strings.Contains(body, "maintenance") {
			return status.StateMaintenance
		}

		return status.StateDegraded
	case "scheduled":
		return status.StateMaintenance
	case "investigating", "identified", "monitoring":
		if strings.Contains(body, "outage") || strings.Contains(body, "unavailable") {
			return status.StateMajorOutage
		}

		return status.StateDegraded
	case "":
		return status.StateUnknown
	default:
		return status.StateDegraded
	}
}

func latestUpdate(description string) feedUpdate {
	matches := updatePattern.FindAllStringSubmatch(description, -1)
	if len(matches) == 0 {
		return feedUpdate{}
	}

	latest := matches[len(matches)-1]

	return feedUpdate{
		Status: cleanText(latest[1]),
		Text:   cleanText(latest[2]),
	}
}

func isTerminalStatus(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "resolved", "completed":
		return true
	default:
		return false
	}
}

func summaryForItems(itemCount int) string {
	if itemCount == 0 {
		return "No active Docker status events"
	}

	if itemCount == 1 {
		return "1 active Docker status event"
	}

	return strconv.Itoa(itemCount) + " active Docker status events"
}

func latestPubDate(items []feedItem, fallback string) *time.Time {
	var latest *time.Time

	for _, item := range items {
		parsed := parseDockerTime(item.PubDate)
		if parsed != nil && (latest == nil || parsed.After(*latest)) {
			latest = parsed
		}
	}

	if latest != nil {
		return latest
	}

	return parseDockerTime(fallback)
}

func parseDockerTime(value string) *time.Time {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}

	layouts := []string{
		time.RFC1123Z,
		time.RFC1123,
		"Mon, 02 Jan 2006 15:04:05 MST",
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
	value = htmlTagPattern.ReplaceAllString(value, " ")

	return strings.Join(strings.Fields(value), " ")
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

var (
	htmlTagPattern = regexp.MustCompile(`(?is)<[^>]*>`)
	updatePattern  = regexp.MustCompile(`(?is)<b[^>]*>\s*(.*?)\s*</b>\s*-\s*(.*?)(?:<br\s*/?>\s*<br\s*/?>|$)`)
)

type feedResponse struct {
	XMLName xml.Name    `xml:"rss"`
	Channel feedChannel `xml:"channel"`
}

type feedChannel struct {
	Title         string     `xml:"title"`
	Link          string     `xml:"link"`
	LastBuildDate string     `xml:"lastBuildDate"`
	Description   string     `xml:"description"`
	Items         []feedItem `xml:"item"`
}

type feedItem struct {
	Title       string   `xml:"title"`
	Link        string   `xml:"link"`
	PubDate     string   `xml:"pubDate"`
	GUID        feedGUID `xml:"guid"`
	Description string   `xml:"description"`
}

type feedGUID struct {
	Value string `xml:",chardata"`
}

type feedUpdate struct {
	Status string
	Text   string
}
