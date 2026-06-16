package tui

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"slices"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/ikan31/tuip/internal/app"
	"github.com/ikan31/tuip/internal/config"
	"github.com/ikan31/tuip/internal/fetch"
	"github.com/ikan31/tuip/internal/providers"
	"github.com/ikan31/tuip/internal/providers/builtin"
	"github.com/ikan31/tuip/internal/status"
	"github.com/ikan31/tuip/internal/statuscache"
)

const (
	allDashboardName    = config.AllDashboard
	minGridCardHeight   = 5
	providerTimeout     = 5 * time.Second
	statusCacheTTL      = 60 * time.Second
	statusErrorCacheTTL = 10 * time.Second

	keyEsc       = "esc"
	keyEnter     = "enter"
	keyBackspace = "backspace"
	keyCtrlH     = "ctrl+h"
	keyDown      = "down"

	minSidebarContentWidth    = 8
	mainBorderHorizontalSize  = 6
	statusIncidentLimit       = 10
	loadingStatusSummary      = "Loading status…"
	minimumSplitParts         = 2
	componentDisplayLimit     = 20
	defaultSidebarWidth       = 32
	maxFallbackMainWidth      = 80
	minMainWidth              = 32
	minDetailAvailableWidth   = 24
	wideGridBreakpoint        = 112
	mediumGridBreakpoint      = 84
	narrowGridBreakpoint      = 54
	wideGridColumns           = 4
	mediumGridColumns         = 3
	narrowGridColumns         = 2
	fallbackSidebarRatio      = 3
	minSidebarWidth           = 28
	maxSidebarWidth           = 38
	minGridCardWidth          = 18
	footerHeight              = 2
	borderWidth               = 2
	statusFilterLineCount     = 2
	gridCardHorizontalPadding = 6
	gridCardNamePadding       = 4
)

// Run starts the TUI dashboard.
func Run(ctx context.Context, configPath string, logger *slog.Logger) error {
	client := fetch.NewClient(providerTimeout)

	registry, err := builtin.NewRegistry(client)
	if err != nil {
		return fmt.Errorf("create built-in provider registry: %w", err)
	}

	cachePath, err := config.StatusCachePath(configPath)
	if err != nil {
		return fmt.Errorf("resolve status cache path: %w", err)
	}

	cache, err := statuscache.LoadOrNew(cachePath)
	if err != nil {
		logWarn(logger, "status_cache_load_error", slog.String("path", cachePath), slog.String("error", err.Error()))
		cache = statuscache.New(cachePath)
	}

	model := newModel(ctx, registry, configPath, cache, logger)

	_, err = tea.NewProgram(model, tea.WithAltScreen()).Run()
	if err != nil {
		return fmt.Errorf("run TUI program: %w", err)
	}

	return nil
}

type focusArea int

const (
	focusSidebar focusArea = iota
	focusStatus
)

type inputMode int

const (
	inputNone inputMode = iota
	inputProviderSearch
	inputStatusFilter
	inputDashboardCreate
	inputDashboardRename
	inputDashboardDeleteConfirm
)

type sidebarItemKind int

const (
	sidebarAction sidebarItemKind = iota
	sidebarDashboard
	sidebarProviderAction
	sidebarProvider
)

const (
	actionRefreshDashboard       = "refresh-dashboard"
	actionNewDashboard           = "new-dashboard"
	actionRenameDashboard        = "rename-dashboard"
	actionDeleteDashboard        = "delete-dashboard"
	actionSetDefaultDashboard    = "set-default-dashboard"
	actionToggleProviderGrouping = "toggle-provider-grouping"
	actionSearchProviders        = "search-providers"
)

type providerListMode int

const (
	providerListAlphabetical providerListMode = iota
	providerListCategory
)

type sidebarItem struct {
	kind       sidebarItemKind
	id         string
	label      string
	category   string
	configured bool
	isDefault  bool
	active     bool
}

type sidebarRow struct {
	line      string
	itemIndex int
}

type model struct {
	ctx                    context.Context //nolint:containedctx // Bubble Tea commands need app context across updates.
	registry               *providers.Registry
	configPath             string
	cache                  *statuscache.Cache
	logger                 *slog.Logger
	providerIDs            []string
	dashboard              string
	dashboardNames         []string
	defaultDashboard       string
	response               status.Response
	loading                bool
	err                    error
	lastRefreshed          time.Time
	width                  int
	height                 int
	statusScroll           int
	detailScroll           int
	preInspectStatusScroll int
	detailsLoaded          bool
	inspect                bool
	selectedStatus         int
	selectedProviderID     string
	activeRefreshID        int64
	loadingTotal           int

	focus            focusArea
	mode             inputMode
	providerFind     string
	statusFind       string
	createName       string
	renameName       string
	providerListMode providerListMode
	sidebarIndex     int
	sidebarScroll    int
}

type refreshMsg struct {
	providerIDs      []string
	dashboard        string
	dashboardNames   []string
	defaultDashboard string
	response         status.Response
	detailsLoaded    bool
	err              error
}

type refreshStartedMsg struct {
	refreshID        int64
	providerIDs      []string
	dashboard        string
	dashboardNames   []string
	defaultDashboard string
	detailsLoaded    bool
	results          <-chan app.ProviderStatusResult
}

type providerStatusMsg struct {
	refreshID int64
	result    app.ProviderStatusResult
	results   <-chan app.ProviderStatusResult
}

type mutationMsg struct {
	dashboard        string
	dashboardNames   []string
	defaultDashboard string
	refresh          bool
	err              error
}

func newModel(ctx context.Context, registry *providers.Registry, configPath string, cache *statuscache.Cache, logger *slog.Logger) model {
	return model{
		ctx:        ctx,
		registry:   registry,
		configPath: configPath,
		cache:      cache,
		logger:     logger,
		loading:    true,
		focus:      focusSidebar,
	}
}

func (m model) Init() tea.Cmd {
	return m.refresh()
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.statusScroll = m.clampedStatusScroll()
		m.detailScroll = m.clampedDetailScroll()
		m.sidebarIndex = m.clampedSidebarIndex()
		m.sidebarScroll = m.clampedSidebarScroll()
	case tea.KeyMsg:
		if updated, cmd, handled := m.updateInput(msg); handled {
			return updated, cmd
		}

		return m.updateKey(msg)
	case mutationMsg:
		m.err = msg.err
		if msg.err != nil {
			return m, nil
		}

		m.dashboardNames = msg.dashboardNames

		m.defaultDashboard = msg.defaultDashboard

		if msg.dashboard != "" {
			m.dashboard = msg.dashboard
		}

		m.sidebarIndex = m.clampedSidebarIndex()

		m.sidebarScroll = m.clampedSidebarScroll()

		if !msg.refresh {
			return m, nil
		}

		m.loading = true

		return m, m.refresh()
	case refreshMsg:
		m.loading = false
		m.loadingTotal = 0
		m.providerIDs = msg.providerIDs
		m.dashboard = msg.dashboard
		m.dashboardNames = msg.dashboardNames
		m.defaultDashboard = msg.defaultDashboard
		m.response = msg.response
		m.detailsLoaded = msg.detailsLoaded
		m.err = msg.err
		m.lastRefreshed = time.Now().UTC()
		m = m.syncSelectedStatus()
		m.statusScroll = m.clampedStatusScroll()
		m.detailScroll = m.clampedDetailScroll()
		m.sidebarIndex = m.clampedSidebarIndex()
		m.sidebarScroll = m.clampedSidebarScroll()
	case refreshStartedMsg:
		if msg.refreshID < m.activeRefreshID {
			return m, nil
		}

		m.activeRefreshID = msg.refreshID
		m.loading = true
		m.loadingTotal = len(msg.providerIDs)
		m.providerIDs = msg.providerIDs
		m.dashboard = msg.dashboard
		m.dashboardNames = msg.dashboardNames
		m.defaultDashboard = msg.defaultDashboard
		m.response = status.Response{CheckedAt: time.Now().UTC(), Results: placeholderSnapshots(m.registry, msg.providerIDs)}
		m.detailsLoaded = msg.detailsLoaded
		m.err = nil
		m = m.syncSelectedStatus()
		m.statusScroll = m.clampedStatusScroll()
		m.detailScroll = m.clampedDetailScroll()
		m.sidebarIndex = m.clampedSidebarIndex()
		m.sidebarScroll = m.clampedSidebarScroll()

		return m, waitForProviderStatus(msg.refreshID, msg.results)
	case providerStatusMsg:
		if msg.refreshID != m.activeRefreshID {
			return m, nil
		}

		if msg.result.Done {
			m.loading = false
			m.loadingTotal = 0
			m.err = msg.result.Err
			m.lastRefreshed = time.Now().UTC()
			m = m.syncSelectedStatus()
			m.statusScroll = m.clampedStatusScroll()

			return m, nil
		}

		m.response.CheckedAt = time.Now().UTC()
		m.response.Results = upsertOrderedSnapshot(m.response.Results, msg.result.Snapshot, m.providerIDs)

		if msg.result.RuntimeError {
			m.err = errors.New("one or more providers failed")
		}

		m = m.syncSelectedStatus()
		m.statusScroll = m.clampedStatusScroll()

		return m, waitForProviderStatus(msg.refreshID, msg.results)
	}

	return m, nil
}

func (m model) updateInput(msg tea.KeyMsg) (model, tea.Cmd, bool) {
	switch m.mode {
	case inputNone:
		return m, nil, false
	case inputProviderSearch:
		switch msg.String() {
		case keyEsc:
			m.mode = inputNone
		case keyEnter:
			m.mode = inputNone
		case keyBackspace, keyCtrlH:
			m.providerFind = trimLastRune(m.providerFind)
		default:
			if msg.Type == tea.KeyRunes {
				m.providerFind += msg.String()
			}
		}

		m.sidebarIndex = m.clampedSidebarIndex()
		m.sidebarScroll = m.scrollForSelectedSidebar()

		return m, nil, true
	case inputStatusFilter:
		switch msg.String() {
		case keyEsc:
			m.mode = inputNone
		case keyEnter:
			m.mode = inputNone
		case keyBackspace, keyCtrlH:
			m.statusFind = trimLastRune(m.statusFind)
		default:
			if msg.Type == tea.KeyRunes {
				m.statusFind += msg.String()
			}
		}

		m = m.syncSelectedStatus()
		m.statusScroll = m.scrollForSelectedStatus()

		return m, nil, true
	case inputDashboardCreate:
		switch msg.String() {
		case keyEsc:
			m.mode = inputNone
			m.createName = ""
		case keyEnter:
			name := strings.TrimSpace(m.createName)
			m.mode = inputNone

			m.createName = ""

			if name == "" {
				return m, nil, true
			}

			return m, m.createDashboard(name), true
		case keyBackspace, keyCtrlH:
			m.createName = trimLastRune(m.createName)
		default:
			if msg.Type == tea.KeyRunes {
				m.createName += msg.String()
			}
		}

		return m, nil, true
	case inputDashboardRename:
		switch msg.String() {
		case keyEsc:
			m.mode = inputNone
			m.renameName = ""
		case keyEnter:
			oldName := m.dashboardActionTarget()
			newName := strings.TrimSpace(m.renameName)
			m.mode = inputNone

			m.renameName = ""

			if newName == "" || oldName == allDashboardName {
				return m, nil, true
			}

			return m, m.renameDashboard(oldName, newName), true
		case keyBackspace, keyCtrlH:
			m.renameName = trimLastRune(m.renameName)
		default:
			if msg.Type == tea.KeyRunes {
				m.renameName += msg.String()
			}
		}

		return m, nil, true
	case inputDashboardDeleteConfirm:
		switch msg.String() {
		case "y", "Y":
			name := m.dashboardActionTarget()

			m.mode = inputNone

			if name == allDashboardName {
				return m, nil, true
			}

			return m, m.deleteDashboard(name), true
		case "n", "N", keyEsc, keyEnter:
			m.mode = inputNone
		}

		return m, nil, true
	}

	return m, nil, false
}

func (m model) updateKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.inspect {
		switch msg.String() {
		case keyEsc, keyEnter:
			m.inspect = false
			m.statusScroll = clamp(m.preInspectStatusScroll, 0, m.statusMaxScroll())

			return m, nil
		case "ctrl+c":
			return m, tea.Quit
		}
	}

	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case keyEsc:
		if m.focus == focusStatus {
			m.focus = focusSidebar
		}

		return m, nil
	case "/":
		if !m.inspect && m.focus == focusStatus {
			m = m.startStatusFilter()
		}

		return m, nil
	case "c":
		if m.focus == focusSidebar {
			return m.activateSidebarAction(actionNewDashboard)
		}
	case "R":
		m.loading = true
		m.err = nil

		return m, m.forceRefresh()
	case "r":
		if m.focus == focusSidebar {
			return m.activateSidebarAction(actionRenameDashboard)
		}

		m.loading = true
		m.err = nil

		return m, m.refresh()
	case "d":
		if m.focus == focusStatus {
			return m.openSelectedStatusDetails()
		}

		if m.focus == focusSidebar {
			return m.activateSidebarAction(actionDeleteDashboard)
		}

		return m, nil
	case "s":
		if m.focus == focusSidebar {
			return m.activateSidebarAction(actionSetDefaultDashboard)
		}
	}

	if m.focus == focusSidebar {
		return m.updateSidebarKey(msg)
	}

	return m.updateStatusKey(msg)
}

func (m model) updateSidebarKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case keyDown, "j":
		m.sidebarIndex++
	case "up", "k":
		m.sidebarIndex--
	case "right", "l":
		m.focus = focusStatus
	case keyEnter:
		return m.activateSidebarItem()
	}

	m.sidebarIndex = m.clampedSidebarIndex()
	m.sidebarScroll = m.scrollForSelectedSidebar()

	return m, nil
}

func (m model) updateStatusKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.inspect {
		switch msg.String() {
		case "up", "k":
			m.detailScroll--
		case keyDown, "j":
			m.detailScroll++
		}

		m.detailScroll = m.clampedDetailScroll()

		return m, nil
	}

	columns := m.gridColumns()
	currentIndex := m.selectedStatusIndex()

	switch msg.String() {
	case "left", "h":
		if currentIndex%columns == 0 {
			m.focus = focusSidebar
		} else {
			m = m.setSelectedStatusIndex(currentIndex - 1)
			m.statusScroll = m.scrollForSelectedStatus()
		}
	case "right", "l":
		m = m.setSelectedStatusIndex(currentIndex + 1)
		m.statusScroll = m.scrollForSelectedStatus()
	case keyEnter:
		return m.openSelectedStatusDetails()
	case "up", "k":
		m = m.setSelectedStatusIndex(currentIndex - columns)
		m.statusScroll = m.scrollForSelectedStatus()
	case keyDown, "j":
		m = m.setSelectedStatusIndex(currentIndex + columns)
		m.statusScroll = m.scrollForSelectedStatus()
	}

	m.statusScroll = m.clampedStatusScroll()

	return m, nil
}

func (m model) openSelectedStatusDetails() (tea.Model, tea.Cmd) {
	if len(m.filteredStatusResults()) == 0 {
		return m, nil
	}

	m.preInspectStatusScroll = m.statusScroll
	m.inspect = true

	m.detailScroll = 0

	if !m.detailsLoaded {
		m.loading = true
		m.err = nil

		return m, m.refresh()
	}

	return m, nil
}

func (m model) View() string {
	if m.height <= 0 {
		bodyLines := m.bodyLines()

		return strings.Join(append(m.headerLines(), bodyLines...), "\n") + "\n"
	}

	bodyHeight := m.bodyHeight()
	scroll := m.activeScroll()
	maxScroll := m.activeMaxScroll()
	main := m.renderMain(bodyHeight, scroll)
	sidebar := m.renderSidebar(bodyHeight)
	content := lipgloss.JoinHorizontal(lipgloss.Top, sidebar, main)

	lines := append(m.headerLines(), content, "")
	lines = append(lines, m.footerLine(scroll, maxScroll))

	return strings.Join(lines, "\n")
}

func (m model) headerLines() []string {
	dashboard := m.dashboard
	if dashboard == "" {
		dashboard = allDashboardName
	}

	focus := "management"
	if m.focus == focusStatus {
		focus = "status"
	}

	return []string{
		titleStyle.Render("tuip") + "  " + subtleStyle.Render("dashboard: "+dashboard+" • focus: "+focus),
		subtleStyle.Render(m.helpText()),
		"",
	}
}

func (m model) helpText() string {
	if m.inspect {
		return "provider details • ↑/↓ or j/k scroll • esc/enter close • R refresh • ctrl+c quit"
	}

	switch m.mode {
	case inputNone:
		return "↑/↓/←/→ or h/j/k/l move • enter select/details • / filter • R refresh • c/r/d/s actions • ctrl+c quit"
	case inputProviderSearch:
		return "provider search • type query • enter/esc done"
	case inputStatusFilter:
		return "dashboard filter • type query • backspace clear • enter/esc done"
	case inputDashboardCreate:
		return "new dashboard: " + m.createName + "_  • enter create • esc cancel"
	case inputDashboardRename:
		return "rename dashboard: " + m.renameName + "_  • enter rename • esc cancel"
	case inputDashboardDeleteConfirm:
		return "delete dashboard " + m.activeDashboard() + "?  y confirm • n/esc cancel"
	}

	return "↑/↓/←/→ or h/j/k/l move • enter select/details • / filter • R refresh • c/r/d/s actions • ctrl+c quit"
}

func (m model) renderSidebar(height int) string {
	rows := m.sidebarRows()

	lines := make([]string, 0, len(rows))

	for _, row := range rows {
		lines = append(lines, row.line)
	}

	visibleLines := sliceLines(lines, m.sidebarScroll, height)
	for len(visibleLines) < height {
		visibleLines = append(visibleLines, "")
	}

	borderColor := lipgloss.Color("238")
	if m.focus == focusSidebar {
		borderColor = lipgloss.Color("205")
	}

	return lipgloss.NewStyle().
		Width(m.sidebarWidth()).
		Height(height).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(borderColor).
		Padding(0, 1).
		Render(strings.Join(visibleLines, "\n"))
}

func (m model) renderSidebarItem(idx int, item sidebarItem) string {
	prefix := "  "
	if idx == m.sidebarIndex && m.focus == focusSidebar {
		prefix = "› "
	}

	contentWidth := max(minSidebarContentWidth, m.sidebarWidth()-mainBorderHorizontalSize)

	label := item.label
	firstPrefix := prefix

	switch item.kind {
	case sidebarAction, sidebarProviderAction:
		label = m.actionLabel(item.id)
	case sidebarDashboard:
		if item.active {
			label = "● " + label
		} else {
			label = "○ " + label
		}

		if item.isDefault {
			label += " *"
		}
	case sidebarProvider:
		marker := "  "
		if item.configured {
			marker = "* "
		}

		firstPrefix = prefix + marker
	}

	nextPrefix := strings.Repeat(" ", runeLen(firstPrefix))
	labelWidth := max(minSidebarContentWidth, contentWidth-runeLen(firstPrefix))

	wrapped := wrapWithPrefix(label, firstPrefix, nextPrefix, labelWidth)
	if idx == m.sidebarIndex && m.focus == focusSidebar {
		return selectedStyle.Render(wrapped)
	}

	if item.active {
		return activeStyle.Render(wrapped)
	}

	return wrapped
}

func (m model) renderMain(height, scroll int) string {
	bodyLines := m.bodyLines()
	visibleBody := m.visibleMainBodyLines(bodyLines, height, scroll)

	for len(visibleBody) < height {
		visibleBody = append(visibleBody, "")
	}

	borderColor := lipgloss.Color("238")
	if m.focus == focusStatus {
		borderColor = lipgloss.Color("205")
	}

	return lipgloss.NewStyle().
		Width(m.mainWidth()).
		Height(height).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(borderColor).
		Padding(0, 1).
		Render(strings.Join(visibleBody, "\n"))
}

func (m model) bodyLines() []string {
	if m.loading && len(m.response.Results) == 0 {
		return []string{"Loading statuses..."}
	}

	lines := make([]string, 0)

	if len(m.response.Results) == 0 {
		return append(lines, "No providers configured for this dashboard.")
	}

	if m.inspect {
		return append(lines, m.inspectLines()...)
	}

	return append(lines, m.gridLines()...)
}

func (m model) gridLines() []string {
	results := m.filteredStatusResults()
	rows := m.statusFilterLines(len(results))

	if len(results) == 0 {
		query := strings.TrimSpace(m.statusFind)
		if query == "" {
			return rows
		}

		return append(rows, fmt.Sprintf("No providers match %q.", query))
	}

	columns := m.gridColumns()

	for start := 0; start < len(results); start += columns {
		end := min(len(results), start+columns)
		cards := make([]string, 0, columns)

		cardHeight := m.gridCardHeight()

		selectedIndex := m.selectedStatusIndex()
		for idx := start; idx < end; idx++ {
			selected := idx == selectedIndex && m.focus == focusStatus
			cards = append(cards, renderGridCard(results[idx], m.gridCardWidth(), cardHeight, selected))
		}

		for len(cards) < columns {
			cards = append(cards, lipgloss.NewStyle().Width(m.gridCardWidth()).Height(cardHeight).Render(""))
		}

		rows = append(rows, strings.Split(lipgloss.JoinHorizontal(lipgloss.Top, cards...), "\n")...)
		rows = append(rows, "")
	}

	return rows
}

func (m model) visibleMainBodyLines(bodyLines []string, height, scroll int) []string {
	if m.stickyStatusFilter() {
		return m.stickyStatusBodyLines(bodyLines, height, scroll)
	}

	visibleHeight := m.mainVisibleHeight(height, scroll)

	return sliceLines(bodyLines, scroll, visibleHeight)
}

func (m model) stickyStatusFilter() bool {
	return !m.inspect && len(m.response.Results) > 0 && (m.mode == inputStatusFilter || strings.TrimSpace(m.statusFind) != "")
}

func (m model) stickyStatusBodyLines(bodyLines []string, height, scroll int) []string {
	filterLines := m.statusFilterLines(len(m.filteredStatusResults()))
	if height <= len(filterLines) {
		return sliceLines(filterLines, 0, height)
	}

	bodyScroll := max(scroll, m.gridStartLine())
	visibleHeight := m.stickyStatusGridVisibleHeight(height - len(filterLines))
	visibleBody := append([]string{}, filterLines...)

	return append(visibleBody, sliceLines(bodyLines, bodyScroll, visibleHeight)...)
}

func (m model) stickyStatusGridVisibleHeight(height int) int {
	if len(m.filteredStatusResults()) == 0 {
		return height
	}

	return min(height, m.visibleGridRowsForHeight(height)*m.gridRowStride()-1)
}

func (m model) statusFilterLines(matchCount int) []string {
	query := strings.TrimSpace(m.statusFind)

	value := "all providers"
	if query != "" {
		value = query
	}

	if m.mode == inputStatusFilter {
		value = m.statusFind + "_"
	}

	line := ""
	if len(m.response.Results) > 0 {
		line = fmt.Sprintf("(%d/%d)", matchCount, len(m.response.Results))
	}

	hint := "press / to search"

	if m.mode == inputStatusFilter {
		line = "Search: " + value
		hint = "enter/esc done • backspace clears"
	}

	style := subtleStyle
	if m.mode == inputStatusFilter {
		style = selectedStyle
	}

	return []string{style.Render(line) + "  " + subtleStyle.Render(hint), ""}
}

func (m model) filteredStatusResults() []status.Snapshot {
	results := m.response.Results

	query := strings.TrimSpace(m.statusFind)
	if query == "" || len(results) == 0 {
		return results
	}

	matchedIDs := map[string]bool{}

	if m.registry != nil {
		for _, metadata := range m.registry.Search(query) {
			matchedIDs[metadata.ID] = true
		}
	}

	filtered := make([]status.Snapshot, 0, len(results))
	for _, snapshot := range results {
		if matchedIDs[snapshot.ProviderID] || statusSnapshotMatches(snapshot, query) {
			filtered = append(filtered, snapshot)
		}
	}

	return filtered
}

func statusSnapshotMatches(snapshot status.Snapshot, query string) bool {
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return true
	}

	fields := []string{
		snapshot.ProviderID,
		snapshot.Name,
		string(snapshot.State),
		snapshot.State.Display(),
		snapshot.Summary,
	}

	for _, field := range fields {
		if strings.Contains(strings.ToLower(field), query) {
			return true
		}
	}

	return false
}

func (m model) footerLine(_, _ int) string {
	parts := []string{}

	filtered := m.filteredStatusResults()
	if len(filtered) > 0 && !m.inspect {
		parts = append(parts, fmt.Sprintf("provider %d/%d", m.selectedStatusIndex()+1, len(filtered)))
	}

	if m.loading && m.loadingTotal > 0 {
		parts = append(parts, fmt.Sprintf("loaded %d/%d", loadedStatusCount(m.response.Results), m.loadingTotal))
	}

	if !m.lastRefreshed.IsZero() {
		parts = append(parts, "last refreshed: "+m.lastRefreshed.Format(time.RFC3339))
	}

	parts = append(parts, "cache ttl: "+statusCacheTTL.String())

	footer := subtleStyle.Render(strings.Join(parts, " • "))
	if m.err != nil {
		footer += "  " + errorStyle.Render("Error: "+m.err.Error())
	}

	return footer
}

func (m model) sidebarRows() []sidebarRow {
	items := m.sidebarItems()
	rows := []sidebarRow{{line: sidebarTitleStyle.Render("Management"), itemIndex: -1}}
	rows = append(rows, m.inputPromptRows()...)

	section := ""
	lastCategory := ""

	for idx, item := range items {
		switch item.kind {
		case sidebarAction:
			if section != "actions" {
				rows = append(rows, sidebarRow{line: subtleStyle.Render("actions"), itemIndex: -1})
				section = "actions"
			}
		case sidebarDashboard:
			if section != "dashboards" {
				rows = append(rows, sidebarRow{line: "", itemIndex: -1}, sidebarRow{line: subtleStyle.Render("dashboards"), itemIndex: -1})
				section = "dashboards"
			}
		case sidebarProviderAction, sidebarProvider:
			if section != "providers" {
				rows = append(rows, sidebarRow{line: "", itemIndex: -1}, sidebarRow{line: subtleStyle.Render("providers — " + m.providerListModeLabel()), itemIndex: -1})
				section = "providers"
				lastCategory = ""
			}

			if item.kind == sidebarProvider && m.providerListMode == providerListCategory && item.category != lastCategory {
				rows = append(rows, sidebarRow{line: subtleStyle.Render("  " + item.category), itemIndex: -1})
				lastCategory = item.category
			}
		}

		for line := range strings.SplitSeq(m.renderSidebarItem(idx, item), "\n") {
			rows = append(rows, sidebarRow{line: line, itemIndex: idx})
		}
	}

	return rows
}

func (m model) inputPromptRows() []sidebarRow {
	switch m.mode {
	case inputNone, inputProviderSearch, inputStatusFilter:
		return nil
	case inputDashboardCreate:
		return []sidebarRow{
			{line: "", itemIndex: -1},
			{line: selectedStyle.Render("Create dashboard"), itemIndex: -1},
			{line: "Name: " + m.createName + "_", itemIndex: -1},
			{line: subtleStyle.Render("enter save • esc cancel"), itemIndex: -1},
		}
	case inputDashboardRename:
		target := m.dashboardActionTarget()

		return []sidebarRow{
			{line: "", itemIndex: -1},
			{line: selectedStyle.Render("Rename " + target), itemIndex: -1},
			{line: "Name: " + m.renameName + "_", itemIndex: -1},
			{line: subtleStyle.Render("enter save • esc cancel"), itemIndex: -1},
		}
	case inputDashboardDeleteConfirm:
		target := m.dashboardActionTarget()

		return []sidebarRow{
			{line: "", itemIndex: -1},
			{line: errorStyle.Render("Delete " + target + "?"), itemIndex: -1},
			{line: subtleStyle.Render("y delete • n/esc cancel"), itemIndex: -1},
		}
	}

	return nil
}

func (m model) selectedSidebarLine() int {
	for idx, row := range m.sidebarRows() {
		if row.itemIndex == m.clampedSidebarIndex() {
			return idx
		}
	}

	return 0
}

func (m model) scrollForSelectedSidebar() int {
	line := m.selectedSidebarLine()

	scroll := min(line, m.sidebarScroll)

	if line >= scroll+m.bodyHeight() {
		scroll = line - m.bodyHeight() + 1
	}

	return clamp(scroll, 0, maxScroll(len(m.sidebarRows()), m.bodyHeight()))
}

func (m model) sidebarItems() []sidebarItem {
	items := make([]sidebarItem, 0, 8+len(m.dashboardNames)+1+len(m.filteredProviderMetadata()))

	items = append(items,
		sidebarItem{kind: sidebarAction, id: actionRefreshDashboard},
		sidebarItem{kind: sidebarAction, id: actionNewDashboard},
		sidebarItem{kind: sidebarAction, id: actionRenameDashboard},
		sidebarItem{kind: sidebarAction, id: actionDeleteDashboard},
		sidebarItem{kind: sidebarAction, id: actionSetDefaultDashboard},
		sidebarItem{kind: sidebarAction, id: actionToggleProviderGrouping},
		sidebarItem{
			kind:      sidebarDashboard,
			id:        allDashboardName,
			label:     allDashboardName,
			active:    m.activeDashboard() == allDashboardName,
			isDefault: m.isDefaultDashboard(allDashboardName),
		},
	)
	for _, name := range m.dashboardNames {
		items = append(items, sidebarItem{
			kind:      sidebarDashboard,
			id:        name,
			label:     name,
			active:    m.activeDashboard() == name,
			isDefault: m.isDefaultDashboard(name),
		})
	}

	items = append(items, sidebarItem{kind: sidebarProviderAction, id: actionSearchProviders})

	configured := m.configuredProviderSet()
	for _, metadata := range m.filteredProviderMetadata() {
		items = append(items, sidebarItem{
			kind:       sidebarProvider,
			id:         metadata.ID,
			label:      metadata.ID,
			category:   providerCategory(metadata),
			configured: configured[metadata.ID],
		})
	}

	return items
}

func (m model) filteredProviderMetadata() []providers.Metadata {
	filtered := m.registry.Search(m.providerFind)
	if m.providerListMode == providerListCategory {
		sort.SliceStable(filtered, func(i, j int) bool {
			leftCategory := providerCategory(filtered[i])

			rightCategory := providerCategory(filtered[j])

			if leftCategory == rightCategory {
				return filtered[i].ID < filtered[j].ID
			}

			return leftCategory < rightCategory
		})
	}

	return filtered
}

func providerCategory(metadata providers.Metadata) string {
	if strings.TrimSpace(metadata.Category) == "" {
		return "Other"
	}

	return metadata.Category
}

func (m model) providerListModeLabel() string {
	if m.providerListMode == providerListCategory {
		return "category"
	}

	return "A-Z"
}

func (m model) actionLabel(action string) string {
	switch action {
	case actionRefreshDashboard:
		return "(R)efresh dashboard"
	case actionNewDashboard:
		return "(c)reate dashboard"
	case actionRenameDashboard:
		return "(r)ename dashboard"
	case actionDeleteDashboard:
		return "(d)elete dashboard"
	case actionSetDefaultDashboard:
		return "(s)et as default dashboard"
	case actionToggleProviderGrouping:
		if m.providerListMode == providerListCategory {
			return "Providers: category"
		}

		return "Providers: A-Z"
	case actionSearchProviders:
		if m.mode == inputProviderSearch {
			return "Search: " + m.providerFind + "_"
		}

		if strings.TrimSpace(m.providerFind) != "" {
			return "Search: " + m.providerFind
		}

		return "Search providers"
	default:
		return action
	}
}

func (m model) configuredProviderSet() map[string]bool {
	configured := map[string]bool{}
	if m.activeDashboard() == allDashboardName {
		return configured
	}

	for _, providerID := range m.providerIDs {
		configured[providerID] = true
	}

	return configured
}

func (m model) activeDashboard() string {
	if m.dashboard == "" {
		return allDashboardName
	}

	return m.dashboard
}

func (m model) isDefaultDashboard(name string) bool {
	return m.defaultDashboard == name
}

func (m model) selectedSidebarItem() (sidebarItem, bool) {
	items := m.sidebarItems()
	if m.sidebarIndex < 0 || m.sidebarIndex >= len(items) {
		return sidebarItem{}, false
	}

	return items[m.sidebarIndex], true
}

func (m model) sidebarItemIndex(kind sidebarItemKind, id string) (int, bool) {
	for idx, item := range m.sidebarItems() {
		if item.kind == kind && item.id == id {
			return idx, true
		}
	}

	return 0, false
}

func (m model) activateSidebarItem() (tea.Model, tea.Cmd) {
	item, ok := m.selectedSidebarItem()
	if !ok {
		return m, nil
	}

	if item.kind == sidebarAction || item.kind == sidebarProviderAction {
		return m.activateSidebarAction(item.id)
	}

	if item.kind == sidebarDashboard {
		m.dashboard = item.id
		m.loading = true
		m.err = nil
		m.inspect = false
		m.detailsLoaded = false
		m.selectedStatus = 0
		m.selectedProviderID = ""
		m.statusScroll = 0
		m.detailScroll = 0

		return m, m.refresh()
	}

	if item.configured {
		return m.removeSelectedProvider()
	}

	return m.addSelectedProvider()
}

func (m model) activateSidebarAction(action string) (tea.Model, tea.Cmd) {
	switch action {
	case actionRefreshDashboard:
		m.loading = true
		m.err = nil

		return m, m.forceRefresh()
	case actionNewDashboard:
		m.mode = inputDashboardCreate
		m.createName = ""

		return m, nil
	case actionRenameDashboard:
		target := m.dashboardActionTarget()
		if target == allDashboardName {
			m.err = errors.New("select a dashboard before renaming")

			return m, nil
		}

		m.mode = inputDashboardRename
		m.renameName = target

		return m, nil
	case actionDeleteDashboard:
		target := m.dashboardActionTarget()
		if target == allDashboardName {
			m.err = errors.New("select a dashboard before deleting")

			return m, nil
		}

		m.mode = inputDashboardDeleteConfirm

		return m, nil
	case actionSetDefaultDashboard:
		return m.setDefaultDashboard()
	case actionToggleProviderGrouping:
		if m.providerListMode == providerListCategory {
			m.providerListMode = providerListAlphabetical
		} else {
			m.providerListMode = providerListCategory
		}

		m.sidebarIndex = m.clampedSidebarIndex()
		m.sidebarScroll = m.scrollForSelectedSidebar()

		return m, nil
	case actionSearchProviders:
		m.mode = inputProviderSearch
		if idx, ok := m.sidebarItemIndex(sidebarProviderAction, actionSearchProviders); ok {
			m.sidebarIndex = idx
			m.sidebarScroll = m.scrollForSelectedSidebar()
		}

		return m, nil
	default:
		return m, nil
	}
}

func (m model) startStatusFilter() model {
	m.mode = inputStatusFilter
	m.focus = focusStatus
	m.inspect = false
	m = m.syncSelectedStatus()
	m.statusScroll = m.scrollForSelectedStatus()

	return m
}

func (m model) dashboardActionTarget() string {
	if m.focus == focusSidebar {
		if item, ok := m.selectedSidebarItem(); ok && item.kind == sidebarDashboard {
			return item.id
		}
	}

	return m.activeDashboard()
}

func (m model) addSelectedProvider() (tea.Model, tea.Cmd) {
	item, ok := m.selectedSidebarItem()
	if !ok || item.kind != sidebarProvider {
		return m, nil
	}

	if m.activeDashboard() == allDashboardName {
		m.err = errors.New("select or create a dashboard before adding providers")

		return m, nil
	}

	return m, mutateConfig(m.configPath, m.activeDashboard(), true, func(cfg *config.Config) error {
		return cfg.AddProviders(m.activeDashboard(), []string{item.id})
	})
}

func (m model) removeSelectedProvider() (tea.Model, tea.Cmd) {
	item, ok := m.selectedSidebarItem()
	if !ok || item.kind != sidebarProvider {
		return m, nil
	}

	if m.activeDashboard() == allDashboardName {
		m.err = errors.New("select a configured dashboard before removing providers")

		return m, nil
	}

	return m, mutateConfig(m.configPath, m.activeDashboard(), true, func(cfg *config.Config) error {
		return cfg.RemoveProviders(m.activeDashboard(), []string{item.id})
	})
}

func (m model) setDefaultDashboard() (tea.Model, tea.Cmd) {
	target := m.dashboardActionTarget()

	return m, mutateConfig(m.configPath, "", false, func(cfg *config.Config) error {
		return cfg.SetDefaultDashboard(target)
	})
}

func (m model) createDashboard(name string) tea.Cmd {
	return mutateConfig(m.configPath, name, true, func(cfg *config.Config) error {
		return cfg.CreateDashboard(name)
	})
}

func (m model) renameDashboard(oldName, newName string) tea.Cmd {
	return mutateConfig(m.configPath, newName, true, func(cfg *config.Config) error {
		return cfg.RenameDashboard(oldName, newName)
	})
}

func (m model) deleteDashboard(name string) tea.Cmd {
	return mutateConfig(m.configPath, allDashboardName, true, func(cfg *config.Config) error {
		return cfg.DeleteDashboard(name)
	})
}

func (m model) refresh() tea.Cmd {
	return m.refreshWithCache(false)
}

func (m model) forceRefresh() tea.Cmd {
	return m.refreshWithCache(true)
}

func (m model) refreshWithCache(forceRefresh bool) tea.Cmd {
	return func() tea.Msg {
		includeDetails := m.inspect

		providerIDs, dashboard, dashboardNames, defaultDashboard, err := resolveDashboardProviderIDs(m.configPath, m.registry, m.dashboard)
		if err != nil {
			return refreshMsg{dashboard: dashboard, dashboardNames: dashboardNames, defaultDashboard: defaultDashboard, err: err}
		}

		logDebug(m.logger, "tui_refresh_start",
			slog.String("dashboard", dashboard),
			slog.Int("provider_count", len(providerIDs)),
			slog.Bool("details", includeDetails),
			slog.Bool("force_refresh", forceRefresh),
		)

		if len(providerIDs) == 0 {
			return refreshMsg{
				providerIDs:      providerIDs,
				dashboard:        dashboard,
				dashboardNames:   dashboardNames,
				defaultDashboard: defaultDashboard,
				detailsLoaded:    includeDetails,
				response:         status.Response{CheckedAt: time.Now().UTC(), Results: []status.Snapshot{}},
			}
		}

		results, streamErr := app.StreamProviders(m.ctx, m.registry, providerIDs, app.StatusOptions{
			Details:       includeDetails,
			Cache:         m.cache,
			CacheTTL:      statusCacheTTL,
			ErrorCacheTTL: statusErrorCacheTTL,
			ForceRefresh:  forceRefresh,
			Logger:        m.logger,
		})
		if streamErr != nil {
			return refreshMsg{dashboard: dashboard, dashboardNames: dashboardNames, defaultDashboard: defaultDashboard, err: streamErr}
		}

		return refreshStartedMsg{
			refreshID:        time.Now().UnixNano(),
			providerIDs:      providerIDs,
			dashboard:        dashboard,
			dashboardNames:   dashboardNames,
			defaultDashboard: defaultDashboard,
			detailsLoaded:    includeDetails,
			results:          results,
		}
	}
}

func waitForProviderStatus(refreshID int64, results <-chan app.ProviderStatusResult) tea.Cmd {
	return func() tea.Msg {
		result, ok := <-results
		if !ok {
			return providerStatusMsg{refreshID: refreshID, result: app.ProviderStatusResult{Done: true}}
		}

		return providerStatusMsg{refreshID: refreshID, result: result, results: results}
	}
}

func placeholderSnapshots(registry *providers.Registry, providerIDs []string) []status.Snapshot {
	snapshots := make([]status.Snapshot, 0, len(providerIDs))
	checkedAt := time.Now().UTC()

	for _, providerID := range providerIDs {
		name := providerID
		sourceURL := ""

		if registry != nil {
			if provider, ok := registry.Get(providerID); ok {
				metadata := provider.Metadata()
				name = metadata.Name
				sourceURL = metadata.SourceURL
			}
		}

		snapshots = append(snapshots, status.Snapshot{
			ProviderID: providerID,
			Name:       name,
			State:      status.StateUnknown,
			Summary:    loadingStatusSummary,
			SourceURL:  sourceURL,
			CheckedAt:  checkedAt,
			Incidents:  []status.Incident{},
			Components: []status.Component{},
		})
	}

	return snapshots
}

func isLoadingSnapshot(snapshot status.Snapshot) bool {
	return snapshot.Summary == loadingStatusSummary && snapshot.State == status.StateUnknown
}

func loadedStatusCount(results []status.Snapshot) int {
	count := 0

	for _, snapshot := range results {
		if !isLoadingSnapshot(snapshot) {
			count++
		}
	}

	return count
}

func upsertOrderedSnapshot(results []status.Snapshot, snapshot status.Snapshot, providerIDs []string) []status.Snapshot {
	updated := make([]status.Snapshot, 0, len(results)+1)
	replaced := false

	for _, item := range results {
		if item.ProviderID == snapshot.ProviderID {
			updated = append(updated, snapshot)
			replaced = true

			continue
		}

		updated = append(updated, item)
	}

	if !replaced {
		updated = append(updated, snapshot)
	}

	order := make(map[string]int, len(providerIDs))
	for idx, providerID := range providerIDs {
		order[providerID] = idx
	}

	sort.SliceStable(updated, func(i, j int) bool {
		return order[updated[i].ProviderID] < order[updated[j].ProviderID]
	})

	return updated
}

func mutateConfig(configPath, dashboard string, refresh bool, mutate func(*config.Config) error) tea.Cmd {
	return func() tea.Msg {
		path, err := config.ResolvePath(configPath)
		if err != nil {
			return mutationMsg{dashboard: dashboard, refresh: refresh, err: err}
		}

		cfg, err := config.LoadOrNew(path)
		if err != nil {
			return mutationMsg{dashboard: dashboard, refresh: refresh, err: err}
		}

		err = mutate(cfg)
		if err != nil {
			return mutationMsg{dashboard: dashboard, refresh: refresh, err: err}
		}

		err = config.Save(path, cfg)
		if err != nil {
			return mutationMsg{dashboard: dashboard, refresh: refresh, err: err}
		}

		return mutationMsg{
			dashboard:        dashboard,
			dashboardNames:   cfg.DashboardNames(),
			defaultDashboard: cfg.DefaultDashboard,
			refresh:          refresh,
		}
	}
}

func resolveDashboardProviderIDs(configPath string, registry *providers.Registry, requestedDashboard string) ([]string, string, []string, string, error) {
	path, err := config.ResolvePath(configPath)
	if err != nil {
		return nil, allDashboardName, nil, "", fmt.Errorf("resolve config path: %w", err)
	}

	cfg, err := config.Load(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return allProviderIDs(registry), allDashboardName, nil, "", nil
		}

		return nil, allDashboardName, nil, "", fmt.Errorf("load config: %w", err)
	}

	dashboardNames := cfg.DashboardNames()

	activeDashboard := requestedDashboard
	if activeDashboard == "" {
		activeDashboard = cfg.DefaultDashboard
	}

	if activeDashboard == "" || activeDashboard == allDashboardName {
		return allProviderIDs(registry), allDashboardName, dashboardNames, cfg.DefaultDashboard, nil
	}

	dashboard, ok := cfg.GetDashboard(activeDashboard)
	if !ok {
		return allProviderIDs(registry), allDashboardName, dashboardNames, cfg.DefaultDashboard, nil
	}

	providerIDs, err := registry.CanonicalIDs(dashboard.ProviderIDs())
	if err != nil {
		return nil, activeDashboard, dashboardNames, cfg.DefaultDashboard, fmt.Errorf("resolve dashboard providers: %w", err)
	}

	return providerIDs, activeDashboard, dashboardNames, cfg.DefaultDashboard, nil
}

func (m model) detailLineWidth() int {
	return max(minSidebarContentWidth, m.mainWidth()-mainBorderHorizontalSize)
}

func (m model) inspectLines() []string {
	results := m.filteredStatusResults()
	if len(results) == 0 {
		return []string{"No provider selected."}
	}

	snapshot := results[m.selectedStatusIndex()]

	lines := []string{
		titleStyle.Render(snapshot.Name),
		"Provider: " + snapshot.ProviderID,
		"State:    " + lipgloss.NewStyle().Foreground(stateColor(snapshot.State)).Bold(true).Render(snapshot.State.Display()),
		"Summary:  " + snapshot.Summary,
		"Checked:  " + formatTime(snapshot.CheckedAt),
	}

	if snapshot.UpdatedAt != nil {
		lines = append(lines, "Updated:  "+formatTime(*snapshot.UpdatedAt))
	}

	if snapshot.SourceURL != "" {
		lines = append(lines, "Source:   "+snapshot.SourceURL)
	}

	if snapshot.Error != "" {
		lines = append(lines, "Error:    "+snapshot.Error)
	}

	lines = append(lines, detailLines(snapshot)...)
	lines = append(lines, "", subtleStyle.Render("enter/esc closes details • ctrl+c quits"))

	return wrapDetailLines(lines, m.detailLineWidth())
}

func detailLines(snapshot status.Snapshot) []string {
	if snapshot.ProviderID == "cloudflare" {
		return cloudflareDetailLines(snapshot)
	}

	lines := []string{""}

	if len(snapshot.Incidents) == 0 {
		lines = append(lines, "Incidents: none")
	} else {
		lines = append(lines, "Incidents:")

		limit := min(len(snapshot.Incidents), statusIncidentLimit)
		for _, incident := range snapshot.Incidents[:limit] {
			label := incident.Kind
			if label == "" {
				label = "incident"
			}

			name := incident.Name
			if incident.Status != "" {
				name += " (" + incident.Status + ")"
			}

			if incident.Impact != "" {
				name += " [" + incident.Impact + "]"
			}

			lines = append(lines, fmt.Sprintf("  - %s: %s", label, name))
			if incident.Summary != "" {
				lines = append(lines, "    "+incident.Summary)
			}

			if incident.URL != "" {
				lines = append(lines, "    "+incident.URL)
			}
		}

		if len(snapshot.Incidents) > limit {
			lines = append(lines, fmt.Sprintf("  ... %d more", len(snapshot.Incidents)-limit))
		}
	}

	return append(lines, componentLines(snapshot.Components)...)
}

func cloudflareDetailLines(snapshot status.Snapshot) []string {
	lines := []string{"", "Cloudflare quick impact summary:"}

	affected := nonOperationalComponents(snapshot.Components)

	if len(snapshot.Incidents) == 0 {
		lines = append(lines, "Incidents: none")
	} else {
		lines = append(lines, fmt.Sprintf("Incidents: %d active/maintenance items; open source for timeline details", len(snapshot.Incidents)))
	}

	if len(snapshot.Components) == 0 {
		lines = append(lines, "Components: none exposed")

		return lines
	}

	if len(affected) == 0 {
		lines = append(lines, fmt.Sprintf("Affected components: none (%d total operational)", len(snapshot.Components)))

		return lines
	}

	lines = append(lines, fmt.Sprintf("Affected components: %d / %d", len(affected), len(snapshot.Components)))
	lines = append(lines, "By state: "+componentCountSummary(componentStateCounts(affected)))

	regions, countries, services := cloudflareImpact(affected)
	if len(regions) > 0 {
		lines = append(lines, "", "Affected regions:")
		lines = append(lines, formatNamedCounts(regions, len(regions))...)
	}

	if len(countries) > 0 {
		lines = append(lines, "", "Affected countries/areas:")
		lines = append(lines, formatCloudflareCountries(countries, len(countries))...)
	}

	if len(services) > 0 {
		lines = append(lines, "", "Affected Cloudflare services:")
		lines = append(lines, formatComponents(services, len(services))...)
	}

	if snapshot.SourceURL != "" {
		lines = append(lines, "", "Full details: "+snapshot.SourceURL)
	}

	return lines
}

func cloudflareImpact(components []status.Component) (map[string]int, map[string]map[status.State]int, []status.Component) {
	regions := map[string]int{}
	countries := map[string]map[status.State]int{}
	services := make([]status.Component, 0)

	for _, component := range components {
		country, ok := cloudflareCountry(component.Name)
		if !ok || strings.EqualFold(component.Group, "Cloudflare Sites and Services") {
			services = append(services, component)

			continue
		}

		region := component.Group
		if region == "" {
			region = "Unknown region"
		}

		regions[region]++

		if countries[country] == nil {
			countries[country] = map[status.State]int{}
		}

		countries[country][component.State]++
	}

	return regions, countries, services
}

func cloudflareCountry(name string) (string, bool) {
	location := strings.TrimSpace(name)
	if beforeCode, _, ok := strings.Cut(location, " - "); ok {
		location = beforeCode
	}

	parts := strings.Split(location, ",")
	if len(parts) < minimumSplitParts {
		return "", false
	}

	country := strings.TrimSpace(parts[len(parts)-1])
	if country == "" {
		return "", false
	}

	return country, true
}

func nonOperationalComponents(components []status.Component) []status.Component {
	affected := make([]status.Component, 0)

	for _, component := range components {
		if component.State != status.StateOperational {
			affected = append(affected, component)
		}
	}

	return affected
}

func componentLines(components []status.Component) []string {
	if len(components) == 0 {
		return []string{"Components: none exposed"}
	}

	counts := componentStateCounts(components)
	nonOperational := nonOperationalComponents(components)

	lines := []string{fmt.Sprintf("Components: %d total (%s)", len(components), componentCountSummary(counts))}
	if len(nonOperational) > 0 {
		lines = append(lines, fmt.Sprintf("Affected components: %d", len(nonOperational)))
		lines = append(lines, formatComponents(nonOperational, len(nonOperational))...)

		return lines
	}

	if len(components) <= componentDisplayLimit {
		lines = append(lines, "All components:")
		lines = append(lines, formatComponents(components, componentDisplayLimit)...)

		return lines
	}

	groups := componentGroupCounts(components)
	lines = append(lines, fmt.Sprintf("All components operational across %d groups", len(groups)))
	lines = append(lines, formatGroupCounts(groups, componentDisplayLimit)...)

	return lines
}

func componentStateCounts(components []status.Component) map[status.State]int {
	counts := map[status.State]int{}
	for _, component := range components {
		counts[component.State]++
	}

	return counts
}

func componentCountSummary(counts map[status.State]int) string {
	states := []status.State{
		status.StateOperational,
		status.StateMaintenance,
		status.StateDegraded,
		status.StatePartialOutage,
		status.StateMajorOutage,
		status.StateUnknown,
	}

	parts := make([]string, 0, len(states))

	for _, state := range states {
		if counts[state] > 0 {
			parts = append(parts, fmt.Sprintf("%s %d", state, counts[state]))
		}
	}

	if len(parts) == 0 {
		return "none"
	}

	return strings.Join(parts, ", ")
}

func formatNamedCounts(counts map[string]int, limit int) []string {
	items := sortedCounts(counts)
	shown := min(len(items), limit)

	lines := make([]string, 0, shown+1)

	for _, item := range items[:shown] {
		lines = append(lines, fmt.Sprintf("  - %s: %d", item.name, item.count))
	}

	if len(items) > limit {
		lines = append(lines, fmt.Sprintf("  ... %d more", len(items)-limit))
	}

	return lines
}

func formatCloudflareCountries(countries map[string]map[status.State]int, limit int) []string {
	totals := map[string]int{}

	for country, counts := range countries {
		for _, count := range counts {
			totals[country] += count
		}
	}

	items := sortedCounts(totals)
	shown := min(len(items), limit)

	lines := make([]string, 0, shown+1)

	for _, item := range items[:shown] {
		lines = append(lines, fmt.Sprintf("  - %s: %d (%s)", item.name, item.count, componentCountSummary(countries[item.name])))
	}

	if len(items) > limit {
		lines = append(lines, fmt.Sprintf("  ... %d more", len(items)-limit))
	}

	return lines
}

type namedCount struct {
	name  string
	count int
}

func sortedCounts(counts map[string]int) []namedCount {
	items := make([]namedCount, 0, len(counts))
	for name, count := range counts {
		items = append(items, namedCount{name: name, count: count})
	}

	sort.Slice(items, func(i, j int) bool {
		if items[i].count == items[j].count {
			return items[i].name < items[j].name
		}

		return items[i].count > items[j].count
	})

	return items
}

func formatComponents(components []status.Component, limit int) []string {
	lines := make([]string, 0, min(len(components), limit)+1)

	shown := min(len(components), limit)
	for _, component := range components[:shown] {
		name := component.Name
		if component.Group != "" {
			name = component.Group + " / " + name
		}

		lines = append(lines, fmt.Sprintf("  - %s: %s", name, component.Status))
	}

	if len(components) > limit {
		lines = append(lines, fmt.Sprintf("  ... %d more", len(components)-limit))
	}

	return lines
}

func componentGroupCounts(components []status.Component) map[string]int {
	groups := map[string]int{}

	for _, component := range components {
		group := component.Group
		if group == "" {
			group = "Ungrouped"
		}

		groups[group]++
	}

	return groups
}

func formatGroupCounts(groups map[string]int, limit int) []string {
	names := make([]string, 0, len(groups))
	for name := range groups {
		names = append(names, name)
	}

	sort.Strings(names)

	shown := min(len(names), limit)

	lines := make([]string, 0, shown+1)

	for _, name := range names[:shown] {
		lines = append(lines, fmt.Sprintf("  - %s: %d", name, groups[name]))
	}

	if len(names) > limit {
		lines = append(lines, fmt.Sprintf("  ... %d more groups", len(names)-limit))
	}

	return lines
}

func (m model) scrollForSelectedStatus() int {
	if len(m.filteredStatusResults()) == 0 {
		return 0
	}

	currentScroll := m.clampedStatusScroll()
	selectedRow := m.selectedStatusIndex() / m.gridColumns()
	firstVisibleRow := m.firstVisibleGridRow(currentScroll)
	visibleRows := m.visibleGridRowsAtScroll(m.bodyHeight(), currentScroll)

	if selectedRow >= firstVisibleRow && selectedRow < firstVisibleRow+visibleRows {
		return currentScroll
	}

	if selectedRow < firstVisibleRow {
		firstVisibleRow = selectedRow
	} else {
		firstVisibleRow = selectedRow - m.visibleGridRowsWithoutPrefix(m.bodyHeight()) + 1
	}

	if firstVisibleRow <= 0 {
		if selectedRow < m.visibleGridRowsAtScroll(m.bodyHeight(), 0) {
			return 0
		}

		return clamp(m.gridStartLine(), 0, m.statusMaxScroll())
	}

	return clamp(m.gridStartLine()+firstVisibleRow*m.gridRowStride(), 0, m.statusMaxScroll())
}

func (m model) selectedStatusLineRange() (int, int) {
	if len(m.filteredStatusResults()) == 0 {
		return 0, 0
	}

	row := m.selectedStatusIndex() / m.gridColumns()
	top := m.gridStartLine() + row*m.gridRowStride()

	return top, top + m.gridCardOuterHeight() - 1
}

func allProviderIDs(registry *providers.Registry) []string {
	metadata := registry.Metadata()

	ids := make([]string, 0, len(metadata))

	for _, item := range metadata {
		ids = append(ids, item.ID)
	}

	sort.Strings(ids)

	return ids
}

func (m model) contentHeight() int {
	if m.height <= 0 {
		return 0
	}

	return max(1, m.height-len(m.headerLines())-footerHeight)
}

func (m model) bodyHeight() int {
	return max(1, m.contentHeight()-borderWidth)
}

func (m model) mainVisibleHeight(height, scroll int) int {
	if m.inspect || len(m.response.Results) == 0 {
		return height
	}

	prefixHeight := 0
	if scroll == 0 {
		prefixHeight = m.gridStartLine()
	}

	return min(height, prefixHeight+m.visibleGridRowsForHeight(height-prefixHeight)*m.gridRowStride()-1)
}

func (m model) gridStartLine() int {
	return statusFilterLineCount
}

func (m model) gridCardOuterHeight() int {
	return m.gridCardHeight() + borderWidth
}

func (m model) gridRowStride() int {
	return m.gridCardOuterHeight() + 1
}

func (m model) gridRowCount() int {
	resultCount := len(m.filteredStatusResults())
	if resultCount == 0 {
		return 0
	}

	return (resultCount + m.gridColumns() - 1) / m.gridColumns()
}

func (m model) visibleGridRowsAtScroll(height, scroll int) int {
	if scroll == 0 {
		return m.visibleGridRowsForHeight(height - m.gridStartLine())
	}

	return m.visibleGridRowsWithoutPrefix(height)
}

func (m model) visibleGridRowsWithoutPrefix(height int) int {
	return m.visibleGridRowsForHeight(height)
}

func (m model) visibleGridRowsForHeight(height int) int {
	height = max(1, height)

	return max(1, (height+1)/m.gridRowStride())
}

func (m model) firstVisibleGridRow(scroll int) int {
	if scroll <= m.gridStartLine() {
		return 0
	}

	return (scroll - m.gridStartLine()) / m.gridRowStride()
}

func (m model) sidebarWidth() int {
	if m.width <= 0 {
		return defaultSidebarWidth
	}

	return clamp(m.width/fallbackSidebarRatio, minSidebarWidth, maxSidebarWidth)
}

func (m model) mainWidth() int {
	if m.width <= 0 {
		return maxFallbackMainWidth
	}

	return max(minMainWidth, m.width-m.sidebarWidth()-borderWidth*2)
}

func (m model) gridColumns() int {
	available := max(minDetailAvailableWidth, m.mainWidth()-mainBorderHorizontalSize)

	switch {
	case available >= wideGridBreakpoint:
		return wideGridColumns
	case available >= mediumGridBreakpoint:
		return mediumGridColumns
	case available >= narrowGridBreakpoint:
		return narrowGridColumns
	default:
		return 1
	}
}

func (m model) gridCardWidth() int {
	columns := m.gridColumns()
	gapWidth := columns - 1
	available := max(minDetailAvailableWidth, m.mainWidth()-mainBorderHorizontalSize-gapWidth)

	return max(minGridCardWidth, available/columns)
}

func (m model) gridCardHeight() int {
	nameWidth := max(minSidebarContentWidth, m.gridCardWidth()-gridCardHorizontalPadding)

	maxNameLines := 1

	for _, snapshot := range m.filteredStatusResults() {
		maxNameLines = max(maxNameLines, len(wrapText(snapshot.Name, nameWidth)))
	}

	return max(minGridCardHeight, maxNameLines+1)
}

func (m model) maxScroll() int {
	if !m.inspect && !m.loading && len(m.response.Results) > 0 {
		return m.statusMaxScroll()
	}

	return maxScroll(len(m.bodyLines()), m.bodyHeight())
}

func (m model) statusMaxScroll() int {
	if m.gridRowCount() <= m.visibleGridRowsAtScroll(m.bodyHeight(), 0) {
		return 0
	}

	return m.gridStartLine() + max(0, m.gridRowCount()-m.visibleGridRowsWithoutPrefix(m.bodyHeight()))*m.gridRowStride()
}

func (m model) activeMaxScroll() int {
	if m.inspect {
		return maxScroll(len(m.bodyLines()), m.bodyHeight())
	}

	return m.maxScroll()
}

func (m model) activeScroll() int {
	if m.inspect {
		return m.clampedDetailScroll()
	}

	return m.clampedStatusScroll()
}

func (m model) clampedStatusScroll() int {
	if m.inspect || m.loading || len(m.response.Results) == 0 {
		return clamp(m.statusScroll, 0, m.maxScroll())
	}

	scroll := clamp(m.statusScroll, 0, m.statusMaxScroll())
	if scroll == 0 {
		return 0
	}

	if scroll <= m.gridStartLine() {
		return m.gridStartLine()
	}

	return m.gridStartLine() + ((scroll - m.gridStartLine()) / m.gridRowStride() * m.gridRowStride())
}

func (m model) clampedDetailScroll() int {
	return clamp(m.detailScroll, 0, m.maxScroll())
}

func (m model) clampedSidebarIndex() int {
	return clamp(m.sidebarIndex, 0, max(0, len(m.sidebarItems())-1))
}

func (m model) clampedSidebarScroll() int {
	return clamp(m.sidebarScroll, 0, maxScroll(len(m.sidebarRows()), m.bodyHeight()))
}

func (m model) selectedStatusIndex() int {
	results := m.filteredStatusResults()
	if len(results) == 0 {
		return 0
	}

	if m.selectedProviderID != "" {
		for idx, snapshot := range results {
			if snapshot.ProviderID == m.selectedProviderID {
				return idx
			}
		}
	}

	return clamp(m.selectedStatus, 0, len(results)-1)
}

func (m model) syncSelectedStatus() model {
	results := m.filteredStatusResults()
	if len(results) == 0 {
		m.selectedStatus = 0

		return m
	}

	if m.selectedProviderID != "" {
		for idx, snapshot := range results {
			if snapshot.ProviderID == m.selectedProviderID {
				m.selectedStatus = idx

				return m
			}
		}

		if m.loading && containsString(m.providerIDs, m.selectedProviderID) {
			m.selectedStatus = clamp(m.selectedStatus, 0, len(results)-1)

			return m
		}
	}

	m.selectedStatus = clamp(m.selectedStatus, 0, len(results)-1)
	m.selectedProviderID = results[m.selectedStatus].ProviderID

	return m
}

func containsString(items []string, target string) bool {
	return slices.Contains(items, target)
}

func (m model) setSelectedStatusIndex(index int) model {
	results := m.filteredStatusResults()
	if len(results) == 0 {
		m.selectedStatus = 0
		m.selectedProviderID = ""

		return m
	}

	m.selectedStatus = clamp(index, 0, len(results)-1)
	m.selectedProviderID = results[m.selectedStatus].ProviderID

	return m
}

func renderGridCard(snapshot status.Snapshot, width, height int, selected bool) string {
	loading := isLoadingSnapshot(snapshot)

	color := stateColor(snapshot.State)
	if selected {
		color = lipgloss.Color("205")
	}

	style := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(color).
		Padding(0, 1).
		Width(width).
		Height(height)

	nameLines := wrapText(snapshot.Name, max(minSidebarContentWidth, width-gridCardNamePadding))

	switch {
	case selected:
		name := wrapWithPrefix(snapshot.Name, "› ", "  ", max(minSidebarContentWidth, width-gridCardHorizontalPadding))
		nameLines = strings.Split(selectedStyle.Render(name), "\n")
	case loading:
		nameLines = strings.Split(subtleStyle.Render(strings.Join(nameLines, "\n")), "\n")
	default:
		nameLines = strings.Split(lipgloss.NewStyle().Bold(true).Render(strings.Join(nameLines, "\n")), "\n")
	}

	stateLabel := snapshot.State.Display()
	if loading {
		stateLabel = loadingStatusSummary
	}

	stateText := lipgloss.NewStyle().Foreground(stateColor(snapshot.State)).Bold(true).Render(stateLabel)

	lines := append([]string{}, nameLines...)
	lines = append(lines, stateText)

	return style.Render(strings.Join(lines, "\n"))
}

func sliceLines(lines []string, start, height int) []string {
	if height <= 0 || start >= len(lines) {
		return nil
	}

	end := min(len(lines), start+height)

	return append([]string(nil), lines[start:end]...)
}

func maxScroll(lineCount, height int) int {
	if height <= 0 || lineCount <= height {
		return 0
	}

	return lineCount - height
}

func clamp(value, low, high int) int {
	if value < low {
		return low
	}

	if value > high {
		return high
	}

	return value
}

func formatTime(value time.Time) string {
	if value.IsZero() {
		return "unknown"
	}

	return value.UTC().Format(time.RFC3339)
}

func trimLastRune(value string) string {
	if value == "" {
		return ""
	}

	runes := []rune(value)

	return string(runes[:len(runes)-1])
}

func wrapDetailLines(lines []string, width int) []string {
	wrapped := make([]string, 0, len(lines))

	for _, line := range lines {
		if strings.Contains(line, "\x1b[") {
			wrapped = append(wrapped, line)

			continue
		}

		wrapped = append(wrapped, wrapIndentedLine(line, width)...)
	}

	return wrapped
}

func wrapIndentedLine(line string, width int) []string {
	if width <= 0 || line == "" || runeLen(line) <= width {
		return []string{line}
	}

	indent := leadingWhitespace(line)
	content := strings.TrimSpace(line)

	continuationPrefix := indent
	if strings.HasPrefix(content, "- ") {
		continuationPrefix = indent + "  "
	}

	return wrapTextWithPrefixes(content, indent, continuationPrefix, width)
}

func leadingWhitespace(value string) string {
	for idx, char := range value {
		if char != ' ' && char != '\t' {
			return value[:idx]
		}
	}

	return value
}

func wrapTextWithPrefixes(value, firstPrefix, nextPrefix string, width int) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return []string{firstPrefix}
	}

	lines := make([]string, 0)
	prefix := firstPrefix
	current := ""
	contentWidth := max(1, width-runeLen(prefix))

	for word := range strings.FieldsSeq(value) {
		for runeLen(word) > contentWidth {
			if current != "" {
				lines = append(lines, prefix+current)
				prefix = nextPrefix
				contentWidth = max(1, width-runeLen(prefix))
				current = ""
			}

			part, rest := splitRunes(word, contentWidth)
			lines = append(lines, prefix+part)
			prefix = nextPrefix
			contentWidth = max(1, width-runeLen(prefix))
			word = rest
		}

		if current == "" {
			current = word

			continue
		}

		if runeLen(current)+1+runeLen(word) <= contentWidth {
			current += " " + word

			continue
		}

		lines = append(lines, prefix+current)
		prefix = nextPrefix
		contentWidth = max(1, width-runeLen(prefix))
		current = word
	}

	if current != "" {
		lines = append(lines, prefix+current)
	}

	return lines
}

func wrapWithPrefix(value, firstPrefix, nextPrefix string, width int) string {
	wrapped := wrapText(value, width)
	for idx, line := range wrapped {
		if idx == 0 {
			wrapped[idx] = firstPrefix + line

			continue
		}

		wrapped[idx] = nextPrefix + line
	}

	return strings.Join(wrapped, "\n")
}

func wrapText(value string, width int) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return []string{""}
	}

	if width <= 0 {
		return []string{value}
	}

	words := strings.Fields(value)
	lines := make([]string, 0)
	current := ""

	for _, word := range words {
		for runeLen(word) > width {
			if current != "" {
				lines = append(lines, current)
				current = ""
			}

			part, rest := splitRunes(word, width)
			lines = append(lines, part)
			word = rest
		}

		if current == "" {
			current = word

			continue
		}

		if runeLen(current)+1+runeLen(word) <= width {
			current += " " + word

			continue
		}

		lines = append(lines, current)
		current = word
	}

	if current != "" {
		lines = append(lines, current)
	}

	return lines
}

func splitRunes(value string, width int) (string, string) {
	runes := []rune(value)
	if len(runes) <= width {
		return value, ""
	}

	return string(runes[:width]), string(runes[width:])
}

func runeLen(value string) int {
	return len([]rune(value))
}

func logDebug(logger *slog.Logger, message string, attrs ...any) {
	if logger != nil {
		logger.Debug(message, attrs...)
	}
}

func logWarn(logger *slog.Logger, message string, attrs ...any) {
	if logger != nil {
		logger.Warn(message, attrs...)
	}
}

func stateColor(state status.State) lipgloss.Color {
	switch state {
	case status.StateOperational:
		return lipgloss.Color("42")
	case status.StateMaintenance:
		return lipgloss.Color("39")
	case status.StateDegraded:
		return lipgloss.Color("214")
	case status.StatePartialOutage:
		return lipgloss.Color("208")
	case status.StateMajorOutage:
		return lipgloss.Color("196")
	case status.StateError:
		return lipgloss.Color("201")
	case status.StateUnknown:
		fallthrough
	default:
		return lipgloss.Color("244")
	}
}

var (
	titleStyle        = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("205"))
	subtleStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
	errorStyle        = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	sidebarTitleStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39"))
	selectedStyle     = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("205"))
	activeStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
)
