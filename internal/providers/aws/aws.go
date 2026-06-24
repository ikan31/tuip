package aws

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
	rssURL    = "https://status.aws.amazon.com/rss/all.rss"
	sourceURL = "https://health.aws.amazon.com/health/status"
)

const (
	stateRankOperational = iota
	stateRankUnknown
	stateRankMaintenance
	stateRankDegraded
	stateRankPartialOutage
	stateRankMajorOutage
)

// Provider fetches AWS's public Service Health Dashboard RSS feed.
type Provider struct {
	client   *fetch.Client
	endpoint string
	metadata providers.Metadata
}

// New creates an AWS provider using the public all-services RSS feed.
func New(client *fetch.Client) *Provider {
	return NewWithEndpoint(client, rssURL, sourceURL)
}

// NewWithEndpoint creates an AWS provider with an override endpoint. It is used
// by tests with httptest fixtures.
func NewWithEndpoint(client *fetch.Client, endpoint string, source string) *Provider {
	return &Provider{
		client:   client,
		endpoint: endpoint,
		metadata: providers.Metadata{
			ID:          "aws",
			Aliases:     []string{"amazon-web-services"},
			Name:        "Amazon Web Services",
			Description: "AWS public service health",
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
		return status.Snapshot{}, fmt.Errorf("fetch AWS status: %w", err)
	}

	var payload feedResponse

	err = xml.Unmarshal([]byte(text), &payload)
	if err != nil {
		return status.Snapshot{}, fmt.Errorf("decode AWS status feed: %w", err)
	}

	items := latestItemsByEvent(payload.Channel.Items)
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
		Components: mapComponents(items),
	}, nil
}

// MapFeedState maps AWS RSS items into tuip's normalized overall state. AWS's
// public RSS feed is empty when there are no public service events.
func MapFeedState(items []feedItem) status.State {
	if len(items) == 0 {
		return status.StateOperational
	}

	state := status.StateOperational
	for _, item := range items {
		state = worseState(state, MapEventState(item.Title+" "+item.Description))
	}

	if state == status.StateOperational {
		return status.StateDegraded
	}

	return state
}

// MapEventState maps AWS RSS item text into a normalized state.
func MapEventState(value string) status.State {
	text := strings.ToLower(strings.TrimSpace(value))
	if text == "" {
		return status.StateUnknown
	}

	majorTerms := []string{"service disruption", "unavailable", "outage", "data loss", "unable to reliably support"}
	for _, term := range majorTerms {
		if strings.Contains(text, term) {
			return status.StateMajorOutage
		}
	}

	if strings.Contains(text, "maintenance") {
		return status.StateMaintenance
	}

	degradedTerms := []string{"service impact", "increased error", "error rates", "connectivity issues", "latency", "degraded", "impaired"}
	for _, term := range degradedTerms {
		if strings.Contains(text, term) {
			return status.StateDegraded
		}
	}

	if strings.Contains(text, "informational") || strings.Contains(text, "message") {
		return status.StateDegraded
	}

	return status.StateUnknown
}

func latestItemsByEvent(items []feedItem) []feedItem {
	latestByKey := map[string]feedItem{}
	order := make([]string, 0, len(items))

	for _, item := range items {
		key := eventKey(item)
		if key == "" {
			key = strings.TrimSpace(item.GUID.Value)
		}

		if key == "" {
			key = strings.TrimSpace(item.Title)
		}

		if key == "" {
			continue
		}

		current, ok := latestByKey[key]
		if !ok {
			order = append(order, key)
			latestByKey[key] = item

			continue
		}

		if itemIsNewer(item, current) {
			latestByKey[key] = item
		}
	}

	latest := make([]feedItem, 0, len(order))
	for _, key := range order {
		latest = append(latest, latestByKey[key])
	}

	return latest
}

func itemIsNewer(left, right feedItem) bool {
	leftTime := parseAWSTime(left.PubDate)
	rightTime := parseAWSTime(right.PubDate)

	if leftTime == nil {
		return false
	}

	if rightTime == nil {
		return true
	}

	return leftTime.After(*rightTime)
}

func mapIncidents(items []feedItem) []status.Incident {
	incidents := make([]status.Incident, 0, len(items))
	for _, item := range items {
		name := strings.TrimSpace(item.Title)
		if name == "" {
			name = "AWS service health event"
		}

		incidents = append(incidents, status.Incident{
			Kind:      "incident",
			Name:      name,
			Status:    awsStatusLabel(item.Title),
			Impact:    string(MapEventState(item.Title + " " + item.Description)),
			Summary:   cleanText(item.Description),
			URL:       firstNonEmpty(strings.TrimSpace(item.GUID.Value), strings.TrimSpace(item.Link), sourceURL),
			UpdatedAt: parseAWSTime(item.PubDate),
		})
	}

	return incidents
}

func mapComponents(items []feedItem) []status.Component {
	components := make([]status.Component, 0, len(items))
	seen := map[string]bool{}

	for _, item := range items {
		key := eventKey(item)

		name, group := componentFromEventKey(key)
		if name == "" {
			name = awsStatusLabel(item.Title)
		}

		if name == "" {
			name = "AWS service health event"
		}

		dedupeKey := name + "\x00" + group
		if seen[dedupeKey] {
			continue
		}

		seen[dedupeKey] = true

		components = append(components, status.Component{
			Name:   name,
			Status: firstNonEmpty(awsStatusLabel(item.Title), string(MapEventState(item.Title+" "+item.Description))),
			State:  MapEventState(item.Title + " " + item.Description),
			Group:  group,
		})
	}

	return components
}

func componentFromEventKey(key string) (string, string) {
	key = strings.TrimSpace(key)
	if key == "" {
		return "", ""
	}

	region := ""
	service := key

	matches := regionSuffixPattern.FindStringSubmatch(key)
	if len(matches) == regionMatchCount {
		region = matches[1]
		service = strings.TrimSuffix(strings.TrimSuffix(key, region), "-")
	}

	return formatServiceName(service), region
}

func formatServiceName(value string) string {
	value = strings.Trim(value, "-_")
	if value == "" {
		return ""
	}

	if value == "multipleservices" || value == "multiple-services" {
		return "Multiple services"
	}

	parts := strings.FieldsFunc(value, func(r rune) bool { return r == '-' || r == '_' })
	for index, part := range parts {
		if len(part) <= shortServiceNameLength {
			parts[index] = strings.ToUpper(part)

			continue
		}

		parts[index] = strings.ToUpper(part[:1]) + part[1:]
	}

	return strings.Join(parts, " ")
}

func eventKey(item feedItem) string {
	guid := strings.TrimSpace(item.GUID.Value)

	index := strings.LastIndex(guid, "#")
	if index >= 0 {
		guid = guid[index+1:]
	}

	if guid == "" {
		return ""
	}

	underscore := strings.LastIndex(guid, "_")
	if underscore < 0 {
		return guid
	}

	_, err := strconv.ParseInt(guid[underscore+1:], 10, 64)
	if err != nil {
		return guid
	}

	return guid[:underscore]
}

func awsStatusLabel(title string) string {
	before, _, ok := strings.Cut(strings.TrimSpace(title), ":")
	if !ok {
		return strings.TrimSpace(title)
	}

	return strings.TrimSpace(before)
}

func summaryForItems(itemCount int) string {
	if itemCount == 0 {
		return "No active AWS service health events"
	}

	if itemCount == 1 {
		return "1 AWS service health event"
	}

	return strconv.Itoa(itemCount) + " AWS service health events"
}

func latestPubDate(items []feedItem, fallback string) *time.Time {
	var latest *time.Time

	for _, item := range items {
		parsed := parseAWSTime(item.PubDate)
		if parsed != nil && (latest == nil || parsed.After(*latest)) {
			latest = parsed
		}
	}

	if latest != nil {
		return latest
	}

	return parseAWSTime(fallback)
}

func parseAWSTime(value string) *time.Time {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}

	value = normalizeAWSZone(value)
	layouts := []string{
		time.RFC1123Z,
		"Mon, 02 Jan 2006 15:04:05 -0700",
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

func normalizeAWSZone(value string) string {
	replacements := map[string]string{
		" PDT": " -0700",
		" PST": " -0800",
		" UTC": " +0000",
		" GMT": " +0000",
	}

	for old, replacement := range replacements {
		prefix, ok := strings.CutSuffix(value, old)
		if ok {
			return prefix + replacement
		}
	}

	return value
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

const (
	regionMatchCount       = 2
	shortServiceNameLength = 3
)

var (
	htmlTagPattern      = regexp.MustCompile(`(?is)<[^>]*>`)
	regionSuffixPattern = regexp.MustCompile(`([a-z]{2}(?:-gov)?-[a-z]+-\d)$`)
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
