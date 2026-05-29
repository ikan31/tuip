package tui

import (
	"errors"
	"strings"
	"testing"

	"github.com/tuipcli/tuip/internal/providers"
	"github.com/tuipcli/tuip/internal/status"
)

func TestRenderGridCardWrapsProviderName(t *testing.T) {
	t.Parallel()

	card := renderGridCard(status.Snapshot{
		Name:  "GitHub Enterprise Cloud - EU",
		State: status.StateOperational,
	}, 32, minGridCardHeight, false)

	for _, want := range []string{"GitHub Enterprise", "Cloud - EU", "Operational"} {
		if !strings.Contains(card, want) {
			t.Fatalf("card missing %q:\n%s", want, card)
		}
	}

	if strings.Contains(card, "…") {
		t.Fatalf("card truncated provider name:\n%s", card)
	}
}

func TestRenderSidebarProviderKeepsMarkerWithWrappedName(t *testing.T) {
	t.Parallel()

	m := model{width: 96, focus: focusSidebar}
	line := m.renderSidebarItem(0, sidebarItem{
		kind:       sidebarProvider,
		label:      "github-enterprise-cloud-eu",
		configured: true,
	})

	lines := strings.Split(line, "\n")

	if len(lines) < 2 {
		t.Fatalf("sidebar item did not wrap: %q", line)
	}

	if !strings.HasPrefix(lines[0], "› * github") {
		t.Fatalf("first wrapped line = %q, want marker and provider name together", lines[0])
	}

	for _, wrappedLine := range lines {
		if strings.TrimSpace(wrappedLine) == "*" {
			t.Fatalf("marker rendered on its own line: %q", line)
		}
	}
}

func TestStatusScrollKeepsSelectedCardInViewport(t *testing.T) {
	t.Parallel()

	results := make([]status.Snapshot, 30)
	for idx := range results {
		results[idx] = status.Snapshot{Name: "Provider", State: status.StateOperational}
	}

	m := model{
		width:          100,
		height:         20,
		focus:          focusStatus,
		selectedStatus: len(results) - 1,
		response:       status.Response{Results: results},
	}

	m.statusScroll = m.scrollForSelectedStatus()
	top, bottom := m.selectedStatusLineRange()

	if top < m.statusScroll || bottom >= m.statusScroll+m.bodyHeight() {
		t.Fatalf("selected range %d-%d outside viewport %d-%d", top, bottom, m.statusScroll, m.statusScroll+m.bodyHeight()-1)
	}
}

func TestSidebarScrollKeepsSelectedItemInViewport(t *testing.T) {
	t.Parallel()

	dashboardNames := []string{"work", "ops", "sales", "hr"}
	providerIDs := []string{
		"accelo", "affinity", "asana", "ashby", "bitbucket", "box", "capsule", "confluence", "dropbox", "freshbooks",
		"github", "greenhouse", "gusto", "hubspot", "jira", "monday", "nutshell", "quickbooks-online", "trello", "xero",
	}

	registry := providers.NewRegistry()
	for _, providerID := range providerIDs {
		err := registry.Register(providers.Metadata{ID: providerID, Name: providerID}, func() providers.Provider { return nil })
		if err != nil {
			t.Fatalf("Register() error = %v", err)
		}
	}

	m := model{
		width:          100,
		height:         12,
		focus:          focusSidebar,
		registry:       registry,
		dashboardNames: dashboardNames,
	}

	m.sidebarIndex = len(m.sidebarItems()) - 1
	m.sidebarScroll = m.scrollForSelectedSidebar()
	line := m.selectedSidebarLine()

	if line < m.sidebarScroll || line >= m.sidebarScroll+m.bodyHeight() {
		t.Fatalf("selected sidebar line %d outside viewport %d-%d", line, m.sidebarScroll, m.sidebarScroll+m.bodyHeight()-1)
	}
}

func TestStatusScrollCanHideErrorPrefixToShowFullCardRows(t *testing.T) {
	t.Parallel()

	m := model{
		width:  100,
		height: 22,
		focus:  focusStatus,
		err:    errors.New("one provider failed"),
	}

	results := make([]status.Snapshot, m.gridColumns()*2)
	for idx := range results {
		results[idx] = status.Snapshot{Name: "Provider", State: status.StateOperational}
	}

	m.response = status.Response{Results: results}
	m.selectedStatus = m.gridColumns()
	m.statusScroll = m.scrollForSelectedStatus()

	if m.statusScroll != m.gridStartLine() {
		t.Fatalf("statusScroll = %d, want error prefix height %d", m.statusScroll, m.gridStartLine())
	}

	visibleHeight := m.mainVisibleHeight(m.bodyHeight(), m.statusScroll)

	wantVisibleHeight := m.visibleGridRowsWithoutPrefix(m.bodyHeight())*m.gridRowStride() - 1
	if visibleHeight != wantVisibleHeight {
		t.Fatalf("visibleHeight = %d, want %d", visibleHeight, wantVisibleHeight)
	}

	top, bottom := m.selectedStatusLineRange()
	if top < m.statusScroll || bottom >= m.statusScroll+visibleHeight {
		t.Fatalf("selected range %d-%d outside rendered viewport %d-%d", top, bottom, m.statusScroll, m.statusScroll+visibleHeight-1)
	}
}

func TestCloudflareCountry(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		want string
		ok   bool
	}{
		{name: "Amsterdam, Netherlands - (AMS)", want: "Netherlands", ok: true},
		{name: "Charlotte, NC, United States - (CLT)", want: "United States", ok: true},
		{name: "Bot Management", ok: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, ok := cloudflareCountry(tt.name)
			if ok != tt.ok || got != tt.want {
				t.Fatalf("cloudflareCountry(%q) = %q, %t; want %q, %t", tt.name, got, ok, tt.want, tt.ok)
			}
		})
	}
}

func TestCloudflareDetailLinesSummarizeImpact(t *testing.T) {
	t.Parallel()

	snapshot := status.Snapshot{
		ProviderID: "cloudflare",
		Name:       "Cloudflare",
		SourceURL:  "https://www.cloudflarestatus.com/",
		Incidents:  []status.Incident{{Name: "Network issue"}},
		Components: []status.Component{
			{Name: "Amsterdam, Netherlands - (AMS)", Group: "Europe", Status: "under_maintenance", State: status.StateMaintenance},
			{Name: "Rotterdam, Netherlands - (RTM)", Group: "Europe", Status: "partial_outage", State: status.StatePartialOutage},
			{Name: "Bot Management", Group: "Cloudflare Sites and Services", Status: "degraded_performance", State: status.StateDegraded},
			{Name: "Paris, France - (CDG)", Group: "Europe", Status: "operational", State: status.StateOperational},
		},
	}

	joined := strings.Join(cloudflareDetailLines(snapshot), "\n")
	for _, want := range []string{
		"Cloudflare quick impact summary",
		"Affected components: 3 / 4",
		"Europe: 2",
		"Netherlands: 2",
		"Bot Management",
		"Full details: https://www.cloudflarestatus.com/",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("cloudflare detail summary missing %q:\n%s", want, joined)
		}
	}
}
