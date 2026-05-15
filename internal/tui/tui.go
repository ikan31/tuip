package tui

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/tuipcli/tuip/internal/app"
	"github.com/tuipcli/tuip/internal/config"
	"github.com/tuipcli/tuip/internal/fetch"
	"github.com/tuipcli/tuip/internal/providers"
	"github.com/tuipcli/tuip/internal/providers/builtin"
	"github.com/tuipcli/tuip/internal/status"
)

const (
	allDashboardName  = config.AllDashboard
	minGridCardHeight = 5
)

// Run starts the TUI dashboard spike.
func Run(ctx context.Context, configPath string) error {
	client := fetch.NewClient(5 * time.Second)
	registry, err := builtin.NewRegistry(client)
	if err != nil {
		return err
	}

	model := newModel(ctx, registry, configPath)
	_, err = tea.NewProgram(model, tea.WithAltScreen()).Run()
	return err
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
	ctx              context.Context
	registry         *providers.Registry
	configPath       string
	providerIDs      []string
	dashboard        string
	dashboardNames   []string
	defaultDashboard string
	response         status.Response
	loading          bool
	err              error
	lastRefreshed    time.Time
	width            int
	height           int
	statusScroll     int
	detailScroll     int
	detailsLoaded    bool
	inspect          bool
	selectedStatus   int

	focus            focusArea
	mode             inputMode
	providerFind     string
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

type mutationMsg struct {
	dashboard        string
	dashboardNames   []string
	defaultDashboard string
	refresh          bool
	err              error
}

func newModel(ctx context.Context, registry *providers.Registry, configPath string) model {
	return model{
		ctx:        ctx,
		registry:   registry,
		configPath: configPath,
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
		m.providerIDs = msg.providerIDs
		m.dashboard = msg.dashboard
		m.dashboardNames = msg.dashboardNames
		m.defaultDashboard = msg.defaultDashboard
		m.response = msg.response
		m.detailsLoaded = msg.detailsLoaded
		m.err = msg.err
		m.lastRefreshed = time.Now().UTC()
		m.selectedStatus = m.clampedStatusIndex()
		m.statusScroll = m.clampedStatusScroll()
		m.detailScroll = m.clampedDetailScroll()
		m.sidebarIndex = m.clampedSidebarIndex()
		m.sidebarScroll = m.clampedSidebarScroll()
	}
	return m, nil
}

func (m model) updateInput(msg tea.KeyMsg) (model, tea.Cmd, bool) {
	switch m.mode {
	case inputProviderSearch:
		switch msg.String() {
		case "esc":
			m.mode = inputNone
		case "enter":
			m.mode = inputNone
		case "backspace", "ctrl+h":
			m.providerFind = trimLastRune(m.providerFind)
		case "ctrl+u":
			m.providerFind = ""
		default:
			if msg.Type == tea.KeyRunes {
				m.providerFind += msg.String()
			}
		}
		m.sidebarIndex = m.clampedSidebarIndex()
		m.sidebarScroll = m.scrollForSelectedSidebar()
		return m, nil, true
	case inputDashboardCreate:
		switch msg.String() {
		case "esc":
			m.mode = inputNone
			m.createName = ""
		case "enter":
			name := strings.TrimSpace(m.createName)
			m.mode = inputNone
			m.createName = ""
			if name == "" {
				return m, nil, true
			}
			return m, m.createDashboard(name), true
		case "backspace", "ctrl+h":
			m.createName = trimLastRune(m.createName)
		case "ctrl+u":
			m.createName = ""
		default:
			if msg.Type == tea.KeyRunes {
				m.createName += msg.String()
			}
		}
		return m, nil, true
	case inputDashboardRename:
		switch msg.String() {
		case "esc":
			m.mode = inputNone
			m.renameName = ""
		case "enter":
			oldName, _ := m.dashboardActionTarget()
			newName := strings.TrimSpace(m.renameName)
			m.mode = inputNone
			m.renameName = ""
			if newName == "" || oldName == allDashboardName {
				return m, nil, true
			}
			return m, m.renameDashboard(oldName, newName), true
		case "backspace", "ctrl+h":
			m.renameName = trimLastRune(m.renameName)
		case "ctrl+u":
			m.renameName = ""
		default:
			if msg.Type == tea.KeyRunes {
				m.renameName += msg.String()
			}
		}
		return m, nil, true
	case inputDashboardDeleteConfirm:
		switch msg.String() {
		case "y", "Y":
			name, _ := m.dashboardActionTarget()
			m.mode = inputNone
			if name == allDashboardName {
				return m, nil, true
			}
			return m, m.deleteDashboard(name), true
		case "n", "N", "esc", "enter":
			m.mode = inputNone
		}
		return m, nil, true
	default:
		return m, nil, false
	}
}

func (m model) updateKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.inspect {
		switch msg.String() {
		case "esc", "backspace", "enter":
			m.inspect = false
			m.statusScroll = m.scrollForSelectedStatus()
			return m, nil
		case "q", "ctrl+c":
			return m, tea.Quit
		}
	}

	switch msg.String() {
	case "q", "ctrl+c", "esc":
		return m, tea.Quit
	case "tab":
		m.toggleFocus()
		return m, nil
	case "c":
		if m.focus == focusSidebar {
			return m.activateSidebarAction(actionNewDashboard)
		}
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
	case "/":
		m.focus = focusSidebar
		return m.activateSidebarAction(actionSearchProviders)
	case "n":
		m.focus = focusSidebar
		m.mode = inputDashboardCreate
		return m, nil
	}

	if m.focus == focusSidebar {
		return m.updateSidebarKey(msg)
	}
	return m.updateStatusKey(msg)
}

func (m model) updateSidebarKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "down", "j":
		m.sidebarIndex++
	case "up", "k":
		m.sidebarIndex--
	case "home", "g":
		m.sidebarIndex = 0
	case "end", "G":
		m.sidebarIndex = len(m.sidebarItems()) - 1
	case "right", "l":
		m.focus = focusStatus
	case "enter":
		return m.activateSidebarItem()
	case "a":
		return m.addSelectedProvider()
	case "x", "backspace":
		return m.removeSelectedProvider()
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
		case "down", "j":
			m.detailScroll++
		case "pgup", "b":
			m.detailScroll -= m.bodyHeight()
		case "pgdown", "f", " ":
			m.detailScroll += m.bodyHeight()
		case "home", "g":
			m.detailScroll = 0
		case "end", "G":
			m.detailScroll = m.maxScroll()
		}
		m.detailScroll = m.clampedDetailScroll()
		return m, nil
	}

	columns := m.gridColumns()
	switch msg.String() {
	case "left", "h":
		if m.selectedStatus%columns == 0 {
			m.focus = focusSidebar
		} else {
			m.selectedStatus--
			m.selectedStatus = m.clampedStatusIndex()
			m.statusScroll = m.scrollForSelectedStatus()
		}
	case "right", "l":
		m.selectedStatus++
		m.selectedStatus = m.clampedStatusIndex()
		m.statusScroll = m.scrollForSelectedStatus()
	case "enter":
		return m.openSelectedStatusDetails()
	case "up", "k":
		m.selectedStatus -= columns
		m.selectedStatus = m.clampedStatusIndex()
		m.statusScroll = m.scrollForSelectedStatus()
	case "down", "j":
		m.selectedStatus += columns
		m.selectedStatus = m.clampedStatusIndex()
		m.statusScroll = m.scrollForSelectedStatus()
	case "pgup", "b":
		m.statusScroll -= m.bodyHeight()
	case "pgdown", "f", " ":
		m.statusScroll += m.bodyHeight()
	case "home", "g":
		m.selectedStatus = 0
		m.statusScroll = m.scrollForSelectedStatus()
	case "end", "G":
		m.selectedStatus = len(m.response.Results) - 1
		m.selectedStatus = m.clampedStatusIndex()
		m.statusScroll = m.scrollForSelectedStatus()
	}
	m.statusScroll = m.clampedStatusScroll()
	return m, nil
}

func (m model) openSelectedStatusDetails() (tea.Model, tea.Cmd) {
	if len(m.response.Results) == 0 {
		return m, nil
	}
	m.inspect = true
	m.detailScroll = 0
	if !m.detailsLoaded {
		m.loading = true
		m.err = nil
		return m, m.refresh()
	}
	return m, nil
}

func (m *model) toggleFocus() {
	if m.focus == focusSidebar {
		m.focus = focusStatus
		return
	}
	m.focus = focusSidebar
}

func (m model) View() string {
	if m.height <= 0 {
		bodyLines := m.bodyLines()
		return strings.Join(append(m.headerLines(), bodyLines...), "\n") + "\n"
	}

	contentHeight := m.contentHeight()
	scroll := m.activeScroll()
	maxScroll := maxScroll(len(m.bodyLines()), contentHeight)
	main := m.renderMain(contentHeight, scroll)
	sidebar := m.renderSidebar(contentHeight)
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
		return "provider details • j/k scroll • esc/enter close • q quit"
	}
	switch m.mode {
	case inputProviderSearch:
		return "provider search • type query • enter/esc done • ctrl+u clear"
	case inputDashboardCreate:
		return "new dashboard: " + m.createName + "_  • enter create • esc cancel"
	case inputDashboardRename:
		return "rename dashboard: " + m.renameName + "_  • enter rename • esc cancel"
	case inputDashboardDeleteConfirm:
		return "delete dashboard " + m.activeDashboard() + "?  y confirm • n/esc cancel"
	default:
		return "tab focus • r refresh • q quit • / search providers • enter select action/provider/details"
	}
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
	contentWidth := max(8, m.sidebarWidth()-6)

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
	labelWidth := max(8, contentWidth-runeLen(firstPrefix))
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
	visibleBody := sliceLines(bodyLines, scroll, height)
	if m.inspect {
		visibleBody = truncateLines(visibleBody, max(8, m.mainWidth()-6))
	}
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
	if m.loading {
		return []string{"Loading statuses..."}
	}

	lines := make([]string, 0)
	if m.err != nil {
		lines = append(lines, errorStyle.Render("Error: "+m.err.Error()), "")
	}

	if len(m.response.Results) == 0 {
		return append(lines, "No providers configured for this dashboard.")
	}

	if m.inspect {
		return append(lines, m.inspectLines()...)
	}

	return m.gridLines()
}

func (m model) gridLines() []string {
	if len(m.response.Results) == 0 {
		return nil
	}

	columns := m.gridColumns()
	rows := make([]string, 0)
	for start := 0; start < len(m.response.Results); start += columns {
		end := min(len(m.response.Results), start+columns)
		cards := make([]string, 0, columns)
		cardHeight := m.gridCardHeight()
		for idx := start; idx < end; idx++ {
			selected := idx == m.selectedStatus && m.focus == focusStatus
			cards = append(cards, renderGridCard(m.response.Results[idx], m.gridCardWidth(), cardHeight, selected))
		}
		for len(cards) < columns {
			cards = append(cards, lipgloss.NewStyle().Width(m.gridCardWidth()).Height(cardHeight).Render(""))
		}
		rows = append(rows, strings.Split(lipgloss.JoinHorizontal(lipgloss.Top, cards...), "\n")...)
		rows = append(rows, "")
	}
	return rows
}

func (m model) footerLine(scroll, maxScroll int) string {
	parts := []string{fmt.Sprintf("scroll %d/%d", scroll, maxScroll)}
	if len(m.response.Results) > 0 && !m.inspect {
		parts = append(parts, fmt.Sprintf("selected %d/%d", m.selectedStatus+1, len(m.response.Results)))
	}
	if !m.lastRefreshed.IsZero() {
		parts = append(parts, "last refreshed: "+m.lastRefreshed.Format(time.RFC3339))
	}
	return subtleStyle.Render(strings.Join(parts, " • "))
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
		for _, line := range strings.Split(m.renderSidebarItem(idx, item), "\n") {
			rows = append(rows, sidebarRow{line: line, itemIndex: idx})
		}
	}
	return rows
}

func (m model) inputPromptRows() []sidebarRow {
	switch m.mode {
	case inputDashboardCreate:
		return []sidebarRow{
			{line: "", itemIndex: -1},
			{line: selectedStyle.Render("Create dashboard"), itemIndex: -1},
			{line: "Name: " + m.createName + "_", itemIndex: -1},
			{line: subtleStyle.Render("enter save • esc cancel"), itemIndex: -1},
		}
	case inputDashboardRename:
		target, _ := m.dashboardActionTarget()
		return []sidebarRow{
			{line: "", itemIndex: -1},
			{line: selectedStyle.Render("Rename " + target), itemIndex: -1},
			{line: "Name: " + m.renameName + "_", itemIndex: -1},
			{line: subtleStyle.Render("enter save • esc cancel"), itemIndex: -1},
		}
	case inputDashboardDeleteConfirm:
		target, _ := m.dashboardActionTarget()
		return []sidebarRow{
			{line: "", itemIndex: -1},
			{line: errorStyle.Render("Delete " + target + "?"), itemIndex: -1},
			{line: subtleStyle.Render("y delete • n/esc cancel"), itemIndex: -1},
		}
	default:
		return nil
	}
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
	scroll := m.sidebarScroll
	if line < scroll {
		scroll = line
	}
	if line >= scroll+m.contentHeight() {
		scroll = line - m.contentHeight() + 1
	}
	return clamp(scroll, 0, maxScroll(len(m.sidebarRows()), m.contentHeight()))
}

func (m model) sidebarItems() []sidebarItem {
	items := []sidebarItem{
		{kind: sidebarAction, id: actionNewDashboard},
		{kind: sidebarAction, id: actionRenameDashboard},
		{kind: sidebarAction, id: actionDeleteDashboard},
		{kind: sidebarAction, id: actionSetDefaultDashboard},
		{kind: sidebarAction, id: actionToggleProviderGrouping},
		{
			kind:      sidebarDashboard,
			id:        allDashboardName,
			label:     allDashboardName,
			active:    m.activeDashboard() == allDashboardName,
			isDefault: m.isDefaultDashboard(allDashboardName),
		},
	}
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
	case actionNewDashboard:
		return "(c)reate dashboard"
	case actionRenameDashboard:
		return "(r)ename dashboard"
	case actionDeleteDashboard:
		return "(d)elete dashboard"
	case actionSetDefaultDashboard:
		return "(s)et dashboard default"
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
	case actionNewDashboard:
		m.mode = inputDashboardCreate
		m.createName = ""
		return m, nil
	case actionRenameDashboard:
		target, ok := m.dashboardActionTarget()
		if !ok || target == allDashboardName {
			m.err = fmt.Errorf("select a dashboard before renaming")
			return m, nil
		}
		m.mode = inputDashboardRename
		m.renameName = target
		return m, nil
	case actionDeleteDashboard:
		target, ok := m.dashboardActionTarget()
		if !ok || target == allDashboardName {
			m.err = fmt.Errorf("select a dashboard before deleting")
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

func (m model) dashboardActionTarget() (string, bool) {
	if m.focus == focusSidebar {
		if item, ok := m.selectedSidebarItem(); ok && item.kind == sidebarDashboard {
			return item.id, true
		}
	}
	return m.activeDashboard(), true
}

func (m model) addSelectedProvider() (tea.Model, tea.Cmd) {
	item, ok := m.selectedSidebarItem()
	if !ok || item.kind != sidebarProvider {
		return m, nil
	}
	if m.activeDashboard() == allDashboardName {
		m.err = fmt.Errorf("select or create a dashboard before adding providers")
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
		m.err = fmt.Errorf("select a configured dashboard before removing providers")
		return m, nil
	}
	return m, mutateConfig(m.configPath, m.activeDashboard(), true, func(cfg *config.Config) error {
		return cfg.RemoveProviders(m.activeDashboard(), []string{item.id})
	})
}

func (m model) setDefaultDashboard() (tea.Model, tea.Cmd) {
	target, ok := m.dashboardActionTarget()
	if !ok {
		return m, nil
	}
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
	return func() tea.Msg {
		includeDetails := m.inspect
		providerIDs, dashboard, dashboardNames, defaultDashboard, err := resolveDashboardProviderIDs(m.configPath, m.registry, m.dashboard)
		if err != nil {
			return refreshMsg{dashboard: dashboard, dashboardNames: dashboardNames, defaultDashboard: defaultDashboard, err: err}
		}

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

		response, checkErr := app.CheckProviders(m.ctx, m.registry, providerIDs, app.StatusOptions{Details: includeDetails})
		return refreshMsg{
			providerIDs:      providerIDs,
			dashboard:        dashboard,
			dashboardNames:   dashboardNames,
			defaultDashboard: defaultDashboard,
			detailsLoaded:    includeDetails,
			response:         response,
			err:              checkErr,
		}
	}
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
		if err := mutate(cfg); err != nil {
			return mutationMsg{dashboard: dashboard, refresh: refresh, err: err}
		}
		if err := config.Save(path, cfg); err != nil {
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
		return nil, allDashboardName, nil, "", err
	}

	cfg, err := config.Load(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return allProviderIDs(registry), allDashboardName, nil, "", nil
		}
		return nil, allDashboardName, nil, "", err
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
		return nil, activeDashboard, dashboardNames, cfg.DefaultDashboard, err
	}
	return providerIDs, activeDashboard, dashboardNames, cfg.DefaultDashboard, nil
}

func (m model) inspectLines() []string {
	if len(m.response.Results) == 0 {
		return []string{"No provider selected."}
	}
	snapshot := m.response.Results[m.clampedStatusIndex()]
	lines := []string{
		titleStyle.Render(snapshot.Name),
		fmt.Sprintf("Provider: %s", snapshot.ProviderID),
		fmt.Sprintf("State:    %s", lipgloss.NewStyle().Foreground(stateColor(snapshot.State)).Bold(true).Render(snapshot.State.Display())),
		fmt.Sprintf("Summary:  %s", snapshot.Summary),
		fmt.Sprintf("Checked:  %s", formatTime(snapshot.CheckedAt)),
	}
	if snapshot.UpdatedAt != nil {
		lines = append(lines, fmt.Sprintf("Updated:  %s", formatTime(*snapshot.UpdatedAt)))
	}
	if snapshot.SourceURL != "" {
		lines = append(lines, fmt.Sprintf("Source:   %s", snapshot.SourceURL))
	}
	if snapshot.Error != "" {
		lines = append(lines, "Error:    "+snapshot.Error)
	}
	lines = append(lines, detailLines(snapshot)...)
	lines = append(lines, "", subtleStyle.Render("enter/esc closes details"))
	return lines
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
		limit := min(len(snapshot.Incidents), 10)
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
				lines = append(lines, fmt.Sprintf("    %s", truncate(incident.Summary, 120)))
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
		lines = append(lines, formatNamedCounts(regions, 8)...)
	}
	if len(countries) > 0 {
		lines = append(lines, "", "Affected countries/areas:")
		lines = append(lines, formatCloudflareCountries(countries, 14)...)
	}
	if len(services) > 0 {
		lines = append(lines, "", "Affected Cloudflare services:")
		lines = append(lines, formatComponents(services, 10)...)
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
	if len(parts) < 2 {
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
		lines = append(lines, formatComponents(nonOperational, 30)...)
		return lines
	}

	if len(components) <= 20 {
		lines = append(lines, "All components:")
		lines = append(lines, formatComponents(components, 20)...)
		return lines
	}

	groups := componentGroupCounts(components)
	lines = append(lines, fmt.Sprintf("All components operational across %d groups", len(groups)))
	lines = append(lines, formatGroupCounts(groups, 20)...)
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
	top, bottom := m.selectedStatusLineRange()
	scroll := m.statusScroll
	if top < scroll {
		scroll = top
	}
	if bottom >= scroll+m.bodyHeight() {
		scroll = bottom - m.bodyHeight() + 1
	}
	return clamp(scroll, 0, m.maxScroll())
}

func (m model) selectedStatusLineRange() (int, int) {
	if len(m.response.Results) == 0 {
		return 0, 0
	}
	cardHeight := m.gridCardHeight()
	row := m.clampedStatusIndex() / m.gridColumns()
	top := row * (cardHeight + 1)
	return top, top + cardHeight - 1
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
	return max(1, m.height-len(m.headerLines())-2)
}

func (m model) bodyHeight() int {
	return m.contentHeight()
}

func (m model) sidebarWidth() int {
	if m.width <= 0 {
		return 32
	}
	return clamp(m.width/3, 28, 38)
}

func (m model) mainWidth() int {
	if m.width <= 0 {
		return 80
	}
	return max(32, m.width-m.sidebarWidth()-2)
}

func (m model) gridColumns() int {
	available := max(24, m.mainWidth()-6)
	switch {
	case available >= 112:
		return 4
	case available >= 84:
		return 3
	case available >= 54:
		return 2
	default:
		return 1
	}
}

func (m model) gridCardWidth() int {
	columns := m.gridColumns()
	gapWidth := columns - 1
	available := max(24, m.mainWidth()-6-gapWidth)
	return max(18, available/columns)
}

func (m model) gridCardHeight() int {
	nameWidth := max(8, m.gridCardWidth()-6)
	maxNameLines := 1
	for _, snapshot := range m.response.Results {
		maxNameLines = max(maxNameLines, len(wrapText(snapshot.Name, nameWidth)))
	}
	return max(minGridCardHeight, maxNameLines+1)
}

func (m model) maxScroll() int {
	return maxScroll(len(m.bodyLines()), m.bodyHeight())
}

func (m model) activeScroll() int {
	if m.inspect {
		return m.clampedDetailScroll()
	}
	return m.clampedStatusScroll()
}

func (m model) clampedStatusScroll() int {
	return clamp(m.statusScroll, 0, m.maxScroll())
}

func (m model) clampedDetailScroll() int {
	return clamp(m.detailScroll, 0, m.maxScroll())
}

func (m model) clampedSidebarIndex() int {
	return clamp(m.sidebarIndex, 0, max(0, len(m.sidebarItems())-1))
}

func (m model) clampedSidebarScroll() int {
	return clamp(m.sidebarScroll, 0, maxScroll(len(m.sidebarRows()), m.contentHeight()))
}

func (m model) clampedStatusIndex() int {
	return clamp(m.selectedStatus, 0, max(0, len(m.response.Results)-1))
}

func renderGridCard(snapshot status.Snapshot, width, height int, selected bool) string {
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

	nameLines := wrapText(snapshot.Name, max(8, width-4))
	if selected {
		name := wrapWithPrefix(snapshot.Name, "› ", "  ", max(8, width-6))
		nameLines = strings.Split(selectedStyle.Render(name), "\n")
	} else {
		nameLines = strings.Split(lipgloss.NewStyle().Bold(true).Render(strings.Join(nameLines, "\n")), "\n")
	}
	stateText := lipgloss.NewStyle().Foreground(stateColor(snapshot.State)).Bold(true).Render(snapshot.State.Display())
	lines := append(nameLines, stateText)
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

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
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

func truncateLines(lines []string, width int) []string {
	truncated := make([]string, len(lines))
	for idx, line := range lines {
		truncated[idx] = truncate(line, width)
	}
	return truncated
}

func truncate(value string, width int) string {
	if width <= 1 || len([]rune(value)) <= width {
		return value
	}
	runes := []rune(value)
	return string(runes[:width-1]) + "…"
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
