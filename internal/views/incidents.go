package views

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/list"
	"github.com/evertras/bubble-table/table"

	"github.com/rootlyhq/rootly-tui/internal/api"
	"github.com/rootlyhq/rootly-tui/internal/components"
	"github.com/rootlyhq/rootly-tui/internal/config"
	"github.com/rootlyhq/rootly-tui/internal/i18n"
	"github.com/rootlyhq/rootly-tui/internal/styles"
)

// renderBulletList renders a section with a bold title and bullet list using lipgloss/list
func renderBulletList(icon, title string, items []string) string {
	if len(items) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString(styles.TextBold.Render(icon + " " + title))
	b.WriteString("\n")
	// Convert []string to []any for list.New
	anyItems := make([]any, len(items))
	for i, item := range items {
		anyItems[i] = item
	}
	l := list.New(anyItems...).
		Enumerator(list.Bullet).
		ItemStyle(styles.DetailValue)
	b.WriteString(l.String())
	b.WriteString("\n\n") // Blank line after section
	return b.String()
}

// Column keys for incidents table
const (
	colKeyIndicator = "indicator"
	colKeySev       = "sev"
	colKeyID        = "id"
	colKeyStatus    = "status"
	colKeyTime      = "time"
	colKeyTitle     = "title"
)

// Row indicator for selected row
const rowIndicator = "▶"

// SortField represents the field to sort by
type SortField int

const (
	SortByNone SortField = iota
	SortByCreated
	SortByUpdated
)

type IncidentsModel struct {
	incidents    []api.Incident
	width        int
	height       int
	listWidth    int
	detailWidth  int
	listHeight   int
	detailHeight int
	layout       string // "horizontal" or "vertical"
	loading      bool
	error        string
	// Pagination state
	currentPage int
	totalPages  int
	totalCount  int
	hasNext     bool
	hasPrev     bool
	// Loading spinner (passed from app)
	spinnerView string
	// Detail loading state - tracks which incident ID is currently loading (empty = not loading)
	detailLoadingID string
	// Detail viewport for scrollable content
	detailViewport      viewport.Model
	detailViewportReady bool
	detailFocused       bool // Whether detail pane has focus (for scrolling)
	// Table for list view
	table table.Model
	// Sorting
	sortState *components.SortState
	sortMenu  *components.SortMenuModel
}

// borderNoDividers creates a rounded border without vertical column dividers
func borderNoDividers() table.Border {
	return table.Border{
		Top:    "─",
		Left:   "│",
		Right:  "│",
		Bottom: "─",

		TopRight:    "╮",
		TopLeft:     "╭",
		BottomRight: "╯",
		BottomLeft:  "╰",

		TopJunction:    "─",
		LeftJunction:   "│",
		RightJunction:  "│",
		BottomJunction: "─",
		InnerJunction:  " ",

		InnerDivider: " ", // Space instead of vertical line between columns
	}
}

func NewIncidentsModel() IncidentsModel {
	// Define table columns with i18n headers using evertras/bubble-table
	columns := []table.Column{
		table.NewColumn(colKeyIndicator, "", 2), // Selection indicator column
		table.NewColumn(colKeySev, i18n.T("incidents.col.severity"), 4),
		table.NewColumn(colKeyID, i18n.T("incidents.col.id"), 10),
		table.NewColumn(colKeyStatus, i18n.T("incidents.detail.status"), 12),
		table.NewColumn(colKeyTime, "", 8),                                 // Relative time (e.g., "2d ago", "3h ago")
		table.NewFlexColumn(colKeyTitle, i18n.T("incidents.col.title"), 1), // Flex to fill remaining space
	}

	t := table.New(columns).
		Focused(true).
		Border(borderNoDividers()).
		WithBaseStyle(lipgloss.NewStyle().Foreground(styles.ColorText)).
		HighlightStyle(lipgloss.NewStyle()). // No background highlight, arrow shows selection
		HeaderStyle(lipgloss.NewStyle().Bold(true).Foreground(styles.ColorText))

	// Initialize sort menu with incident-specific options (only API-supported sorts)
	sortOptions := []components.SortOption{
		{Label: i18n.T("sorting.created"), Description: i18n.T("sorting.desc.created"), Value: SortByCreated},
		{Label: i18n.T("sorting.updated"), Description: i18n.T("sorting.desc.updated"), Value: SortByUpdated},
	}

	return IncidentsModel{
		incidents:   []api.Incident{},
		currentPage: 1,
		table:       t,
		sortState:   components.NewSortState(),
		sortMenu:    components.NewSortMenu(sortOptions),
	}
}

func (m IncidentsModel) Init() tea.Cmd {
	return nil
}

func (m IncidentsModel) Update(msg tea.Msg) (IncidentsModel, tea.Cmd) {
	var cmd tea.Cmd
	var cmds []tea.Cmd

	// Track previous cursor position to detect changes
	prevCursor := m.table.GetHighlightedRowIndex()

	switch msg := msg.(type) {
	case tea.KeyMsg:
		// When detail is focused, handle scrolling keys
		if m.detailFocused {
			switch msg.String() {
			case "esc", "q":
				// Return focus to list
				m.detailFocused = false
				return m, nil
			case "j", "down":
				if m.detailViewportReady {
					m.detailViewport.ScrollDown(3)
				}
				return m, nil
			case "k", "up":
				if m.detailViewportReady {
					m.detailViewport.ScrollUp(3)
				}
				return m, nil
			case "g":
				if m.detailViewportReady {
					m.detailViewport.GotoTop()
				}
				return m, nil
			case "G":
				if m.detailViewportReady {
					m.detailViewport.GotoBottom()
				}
				return m, nil
			case "d", "pgdown":
				if m.detailViewportReady {
					m.detailViewport.HalfPageDown()
				}
				return m, nil
			case "u", "pgup":
				if m.detailViewportReady {
					m.detailViewport.HalfPageUp()
				}
				return m, nil
			}
			// Forward other keys to viewport
			if m.detailViewportReady {
				m.detailViewport, cmd = m.detailViewport.Update(msg)
				cmds = append(cmds, cmd)
			}
			return m, tea.Batch(cmds...)
		}

		// Handle navigation keys ourselves to prevent table's wrap-around behavior
		switch msg.String() {
		case "j", "down":
			cursor := m.table.GetHighlightedRowIndex()
			if cursor < len(m.incidents)-1 {
				m.table = m.table.WithHighlightedRow(cursor + 1)
				m.updateRowIndicators()
				m.updateViewportContent()
			}
			return m, nil
		case "k", "up":
			cursor := m.table.GetHighlightedRowIndex()
			if cursor > 0 {
				m.table = m.table.WithHighlightedRow(cursor - 1)
				m.updateRowIndicators()
				m.updateViewportContent()
			}
			return m, nil
		case "g":
			// Go to first row
			m.table = m.table.WithHighlightedRow(0)
			m.updateRowIndicators()
			m.updateViewportContent()
			return m, nil
		case "G":
			// Go to last row
			if len(m.incidents) > 0 {
				m.table = m.table.WithHighlightedRow(len(m.incidents) - 1)
				m.updateRowIndicators()
				m.updateViewportContent()
			}
			return m, nil
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.updateDimensions()
		m.updateViewportContent()
	}

	// Forward other messages to table
	m.table, cmd = m.table.Update(msg)
	cmds = append(cmds, cmd)

	// Update detail viewport if cursor changed
	if m.table.GetHighlightedRowIndex() != prevCursor {
		m.updateViewportContent()
	}

	// Forward mouse messages to viewport for scrolling
	if m.detailViewportReady && m.detailFocused {
		m.detailViewport, cmd = m.detailViewport.Update(msg)
		cmds = append(cmds, cmd)
	}

	return m, tea.Batch(cmds...)
}

// updateRowIndicators updates the arrow indicator to show on the current row
func (m *IncidentsModel) updateRowIndicators() {
	if len(m.incidents) == 0 {
		return
	}
	cursor := m.table.GetHighlightedRowIndex()
	rows := make([]table.Row, len(m.incidents))
	for i, inc := range m.incidents {
		seqID := inc.SequentialID
		if seqID == "" {
			seqID = "INC-?"
		}
		status := inc.Status
		if len(status) > 12 {
			status = status[:12]
		}
		title := inc.Summary
		if title == "" {
			title = inc.Title
		}
		title = strings.ReplaceAll(title, "\n", " ")
		title = strings.ReplaceAll(title, "\r", "")

		sevCell := table.NewStyledCell(severitySignalPlain(inc.Severity), severityStyle(inc.Severity))
		statusCell := table.NewStyledCell(status, statusStyle(status))

		// Use StartedAt if available, otherwise CreatedAt
		timeStr := "-"
		if inc.StartedAt != nil {
			timeStr = formatRelativeTime(*inc.StartedAt)
		} else if !inc.CreatedAt.IsZero() {
			timeStr = formatRelativeTime(inc.CreatedAt)
		}
		timeCell := table.NewStyledCell(timeStr, styles.TextDim)

		indicator := ""
		if i == cursor {
			indicator = rowIndicator
		}

		rows[i] = table.NewRow(table.RowData{
			colKeyIndicator: indicator,
			colKeySev:       sevCell,
			colKeyID:        seqID,
			colKeyStatus:    statusCell,
			colKeyTime:      timeCell,
			colKeyTitle:     title,
		})
	}
	m.table = m.table.WithRows(rows)
}

// updateViewportContent updates the viewport content when data changes
func (m *IncidentsModel) updateViewportContent() {
	if !m.detailViewportReady {
		return
	}
	inc := m.SelectedIncident()
	if inc == nil {
		return
	}
	content := m.generateDetailContent(inc)
	m.detailViewport.SetContent(content)
	m.detailViewport.GotoTop()
}

func (m *IncidentsModel) updateDimensions() {
	if m.width <= 0 {
		return
	}

	// Default to horizontal layout
	if m.layout == "" {
		m.layout = config.LayoutHorizontal
	}

	var tableWidth, tableHeight, viewportWidth, viewportHeight int

	if m.layout == config.LayoutVertical {
		// Vertical layout: use full height, minimal overhead
		// App already subtracts 10 for header/help bar, so we only need minimal adjustment
		totalContentHeight := m.height - 2 // Just account for spacing between panes
		if totalContentHeight < 10 {
			totalContentHeight = 10
		}

		// 45/55 split - give more space to detail pane for scrolling content
		m.listWidth = m.width - 2
		m.detailWidth = m.width - 2
		m.listHeight = (totalContentHeight * 45) / 100
		m.detailHeight = totalContentHeight - m.listHeight

		tableWidth = m.listWidth - 4
		// Account for: title (2 lines), footer (1 line), container borders (2)
		tableHeight = m.listHeight - 5
		if tableHeight < 3 {
			tableHeight = 3
		}

		viewportWidth = m.detailWidth - 4
		viewportHeight = m.detailHeight - 4
	} else {
		// Horizontal layout: 50/50 split left/right
		// Account for header, help bar, borders
		totalContentHeight := m.height - 8
		if totalContentHeight < 5 {
			totalContentHeight = 5
		}

		m.listWidth = (m.width - 6) / 2 // -6 for gap between panes
		m.detailWidth = m.width - m.listWidth - 6
		m.listHeight = totalContentHeight
		m.detailHeight = totalContentHeight

		tableWidth = m.listWidth - 4
		// Account for: title (2 lines), footer (2 lines), container borders (2)
		tableHeight = totalContentHeight - 6
		if tableHeight < 3 {
			tableHeight = 3
		}

		viewportWidth = m.detailWidth - 4
		viewportHeight = totalContentHeight - 4
	}

	// Ensure minimum dimensions
	if viewportHeight < 1 {
		viewportHeight = 1
	}
	if viewportWidth < 20 {
		viewportWidth = 20
	}

	// Calculate page size based on available table height
	// Account for header row (1 line) and some padding
	pageSize := tableHeight - 2
	if pageSize < 3 {
		pageSize = 3
	}
	if pageSize > 25 {
		pageSize = 25 // Cap at API page size
	}

	// Update table dimensions and page size
	m.table = m.table.WithTargetWidth(tableWidth).WithMinimumHeight(tableHeight).WithPageSize(pageSize)

	// Update or create viewport
	if !m.detailViewportReady {
		m.detailViewport = viewport.New(viewportWidth, viewportHeight)
		m.detailViewport.MouseWheelEnabled = true
		m.detailViewportReady = true
	} else {
		m.detailViewport.Width = viewportWidth
		m.detailViewport.Height = viewportHeight
	}
}

func (m *IncidentsModel) SetIncidents(incidents []api.Incident, pagination api.PaginationInfo) {
	m.incidents = incidents
	m.loading = false
	m.error = ""
	m.currentPage = pagination.CurrentPage
	m.totalPages = pagination.TotalPages
	m.totalCount = pagination.TotalCount
	m.hasNext = pagination.HasNext
	m.hasPrev = pagination.HasPrev

	// Build table rows from incidents with styled cells
	rows := make([]table.Row, len(m.incidents))
	cursor := m.table.GetHighlightedRowIndex()
	for i, inc := range m.incidents {
		seqID := inc.SequentialID
		if seqID == "" {
			seqID = "INC-?"
		}
		status := inc.Status
		if len(status) > 12 {
			status = status[:12]
		}
		title := inc.Summary
		if title == "" {
			title = inc.Title
		}
		title = strings.ReplaceAll(title, "\n", " ")
		title = strings.ReplaceAll(title, "\r", "")

		// Create styled cells using evertras/bubble-table
		sevCell := table.NewStyledCell(severitySignalPlain(inc.Severity), severityStyle(inc.Severity))
		statusCell := table.NewStyledCell(status, statusStyle(status))

		// Use StartedAt if available, otherwise CreatedAt
		timeStr := "-"
		if inc.StartedAt != nil {
			timeStr = formatRelativeTime(*inc.StartedAt)
		} else if !inc.CreatedAt.IsZero() {
			timeStr = formatRelativeTime(inc.CreatedAt)
		}
		timeCell := table.NewStyledCell(timeStr, styles.TextDim)

		// Show indicator for highlighted row
		indicator := ""
		if i == cursor {
			indicator = rowIndicator
		}

		rows[i] = table.NewRow(table.RowData{
			colKeyIndicator: indicator,
			colKeySev:       sevCell,
			colKeyID:        seqID,
			colKeyStatus:    statusCell,
			colKeyTime:      timeCell,
			colKeyTitle:     title,
		})
	}
	m.table = m.table.WithRows(rows)

	// Set custom footer with pagination info
	footer := m.buildPaginationFooter()
	m.table = m.table.WithStaticFooter(footer)

	// Adjust cursor if needed
	if cursor >= len(incidents) && len(incidents) > 0 {
		m.table = m.table.WithHighlightedRow(len(incidents) - 1)
	}
	m.updateViewportContent()
}

// buildPaginationFooter creates a footer string showing pagination info
func (m *IncidentsModel) buildPaginationFooter() string {
	if m.totalPages > 0 && m.totalCount > 0 {
		return fmt.Sprintf("Page %d/%d (%d total)", m.currentPage, m.totalPages, m.totalCount)
	}
	if m.currentPage > 0 {
		return fmt.Sprintf("Page %d", m.currentPage)
	}
	return ""
}

func (m *IncidentsModel) SetLoading(loading bool) {
	m.loading = loading
}

func (m *IncidentsModel) SetSpinner(spinner string) {
	m.spinnerView = spinner
}

func (m *IncidentsModel) SetError(err string) {
	m.error = err
	m.loading = false
}

func (m *IncidentsModel) SetDimensions(width, height int) {
	m.width = width
	m.height = height
	m.updateDimensions()
}

// SetLayout sets the layout direction (horizontal or vertical)
func (m *IncidentsModel) SetLayout(layout string) {
	m.layout = layout
	m.updateDimensions()
}

// Pagination methods
func (m IncidentsModel) CurrentPage() int {
	return m.currentPage
}

func (m IncidentsModel) HasNextPage() bool {
	return m.hasNext
}

func (m IncidentsModel) HasPrevPage() bool {
	return m.hasPrev
}

func (m IncidentsModel) TotalPages() int {
	return m.totalPages
}

func (m IncidentsModel) TotalCount() int {
	return m.totalCount
}

func (m *IncidentsModel) NextPage() {
	// Check both hasNext flag and totalPages to prevent going beyond the last page
	if m.hasNext && (m.totalPages == 0 || m.currentPage < m.totalPages) {
		m.currentPage++
		m.table = m.table.WithHighlightedRow(0)
	}
}

func (m *IncidentsModel) PrevPage() {
	if m.hasPrev && m.currentPage > 1 {
		m.currentPage--
		m.table = m.table.WithHighlightedRow(0)
	}
}

func (m IncidentsModel) SelectedIncident() *api.Incident {
	cursor := m.table.GetHighlightedRowIndex()
	if cursor >= 0 && cursor < len(m.incidents) {
		return &m.incidents[cursor]
	}
	return nil
}

func (m IncidentsModel) SelectedIndex() int {
	return m.table.GetHighlightedRowIndex()
}

func (m *IncidentsModel) SetDetailLoading(id string) {
	m.detailLoadingID = id
}

func (m *IncidentsModel) ClearDetailLoading() {
	m.detailLoadingID = ""
}

func (m IncidentsModel) IsDetailLoading() bool {
	return m.detailLoadingID != ""
}

// IsLoadingIncident returns true if the specified incident ID is currently loading
func (m IncidentsModel) IsLoadingIncident(id string) bool {
	return m.detailLoadingID == id
}

// SetDetailFocused sets focus on the detail pane for scrolling
func (m *IncidentsModel) SetDetailFocused(focused bool) {
	m.detailFocused = focused
}

// IsDetailFocused returns whether the detail pane has focus
func (m IncidentsModel) IsDetailFocused() bool {
	return m.detailFocused
}

func (m *IncidentsModel) UpdateIncidentDetail(index int, incident *api.Incident) {
	if index >= 0 && index < len(m.incidents) && incident != nil {
		m.incidents[index] = *incident
		// Update viewport content without resetting scroll (detail just loaded)
		if m.detailViewportReady && index == m.table.GetHighlightedRowIndex() {
			content := m.generateDetailContent(incident)
			m.detailViewport.SetContent(content)
		}
	}
}

func (m IncidentsModel) View() string {
	if m.loading {
		// Show loading within the layout structure to prevent jarring shift
		loadingMsg := fmt.Sprintf("%s %s", m.spinnerView, i18n.Tf("incidents.loading_page", map[string]any{"Page": m.currentPage}))
		listContent := styles.TextBold.Render(i18n.T("incidents.title")) + "\n\n" + styles.TextDim.Render(loadingMsg)
		listView := styles.ListContainer.Width(m.listWidth).Height(m.listHeight).Render(listContent)
		detailView := styles.DetailContainer.Width(m.detailWidth).Height(m.detailHeight).Render("")
		return m.joinPanes(listView, detailView)
	}

	if m.error != "" {
		return styles.Error.Render(i18n.T("common.error") + ": " + m.error)
	}

	if len(m.incidents) == 0 {
		return styles.TextDim.Render(i18n.T("incidents.none_found"))
	}

	// Build list view
	listView := m.renderList(m.listHeight)

	// Build detail view
	detailView := m.renderDetail(m.detailHeight)

	// Join based on layout
	return m.joinPanes(listView, detailView)
}

// joinPanes joins the list and detail panes based on the current layout
func (m IncidentsModel) joinPanes(listView, detailView string) string {
	if m.layout == config.LayoutVertical {
		return lipgloss.JoinVertical(lipgloss.Left, listView, detailView)
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, listView, "  ", detailView)
}

func (m IncidentsModel) renderList(height int) string {
	var b strings.Builder

	// Title
	title := styles.TextBold.Render(i18n.T("incidents.title"))
	b.WriteString(title)
	b.WriteString("\n\n")

	// Render table
	b.WriteString(m.table.View())
	b.WriteString("\n")

	// Page navigation footer
	var footer strings.Builder
	if m.hasPrev {
		footer.WriteString(styles.TextDim.Render("← ["))
	} else {
		footer.WriteString(styles.TextDim.Render("  "))
	}
	fmt.Fprintf(&footer, " %s %d ", i18n.T("common.page"), m.currentPage)
	if m.hasNext {
		footer.WriteString(styles.TextDim.Render("] →"))
	}

	// Item count
	if len(m.incidents) > 0 {
		footer.WriteString(styles.TextDim.Render(fmt.Sprintf("  (%d-%d)", m.table.GetHighlightedRowIndex()+1, len(m.incidents))))
	}

	// Sort indicator
	if sortInfo := m.GetSortInfo(); sortInfo != "" {
		footer.WriteString(styles.TextDim.Render("  " + sortInfo))
	}

	b.WriteString(footer.String())

	content := b.String()
	return styles.ListContainer.Width(m.listWidth).Height(height).Render(content)
}

func (m IncidentsModel) renderDetail(height int) string {
	inc := m.SelectedIncident()
	if inc == nil {
		return styles.DetailContainer.Width(m.detailWidth).Height(height).Render(
			styles.TextDim.Render(i18n.T("incidents.select_prompt")),
		)
	}

	// Render with or without viewport
	if !m.detailViewportReady {
		content := m.generateDetailContent(inc)
		return styles.DetailContainer.Width(m.detailWidth).Height(height).Render(content)
	}

	// Add scroll indicator if content is scrollable
	var footer string
	if m.detailViewport.TotalLineCount() > m.detailViewport.VisibleLineCount() {
		scrollPercent := int(m.detailViewport.ScrollPercent() * 100)
		if m.detailFocused {
			footer = styles.Primary.Render(fmt.Sprintf("─── %d%% (j/k scroll, Esc to exit) ───", scrollPercent))
		} else {
			footer = styles.TextDim.Render(fmt.Sprintf("─── %d%% (Enter to scroll) ───", scrollPercent))
		}
	}

	// Use viewport for rendering
	viewportContent := m.detailViewport.View()
	if footer != "" {
		viewportContent = viewportContent + "\n" + footer
	}

	// Use focused style when detail has focus
	containerStyle := styles.DetailContainer
	if m.detailFocused {
		containerStyle = styles.DetailContainerFocused
	}
	return containerStyle.Width(m.detailWidth).Height(height).Render(viewportContent)
}

//nolint:gocyclo // View rendering function with many optional fields to display
func (m IncidentsModel) generateDetailContent(inc *api.Incident) string {
	var b strings.Builder

	// Title line: [INC-XXX] Title (strip newlines for single-line display)
	title := inc.Title
	if title == "" {
		title = inc.Summary
	}
	title = strings.ReplaceAll(title, "\n", " ")
	title = strings.ReplaceAll(title, "\r", "")
	if inc.SequentialID != "" {
		b.WriteString(styles.Primary.Bold(true).Render("[" + inc.SequentialID + "]"))
		b.WriteString(" ")
	}
	b.WriteString(styles.DetailTitle.Render(title))
	b.WriteString("\n\n")

	// Severity, Status, and Kind row
	sevSignal := styles.RenderSeveritySignal(inc.Severity)
	sevBadge := styles.RenderSeverity(inc.Severity)
	statusBadge := styles.RenderStatus(inc.Status)
	b.WriteString(fmt.Sprintf("%s: %s %s  %s: %s", i18n.T("incidents.detail.severity"), sevSignal, sevBadge, i18n.T("incidents.detail.status"), statusBadge))

	// Show incident kind if it's scheduled maintenance
	if inc.Kind == "scheduled" || inc.Kind == "scheduled_maintenance" {
		b.WriteString("  ")
		b.WriteString(styles.DetailLabel.Render(i18n.T("incidents.detail.kind") + ":"))
		b.WriteString(" ")
		b.WriteString(styles.RenderScheduledMaintenance())
	}

	// Show creator if available (from detail view)
	if inc.CreatedByName != "" {
		creatorInfo := styles.RenderNameWithEmail(inc.CreatedByName, inc.CreatedByEmail)
		relTime := formatRelativeTime(inc.CreatedAt)
		fmt.Fprintf(&b, "  %s %s by %s", i18n.T("incidents.timeline.created"), relTime, creatorInfo)
	}
	b.WriteString("\n\n")

	// Links section (high up for quick access)
	rootlyURL := inc.ShortURL
	if rootlyURL == "" {
		rootlyURL = inc.URL
	}
	if rootlyURL == "" && inc.ID != "" {
		rootlyURL = fmt.Sprintf("https://rootly.com/account/incidents/%s", inc.ID)
	}
	if inc.SlackChannelURL != "" || inc.JiraIssueURL != "" || rootlyURL != "" {
		b.WriteString(styles.TextBold.Render("🔗 " + i18n.T("incidents.detail.links")))
		b.WriteString("\n")
		if rootlyURL != "" {
			b.WriteString(m.renderLinkRow(i18n.T("incidents.links.rootly"), rootlyURL))
		}
		if inc.SlackChannelURL != "" {
			if inc.SlackChannelName != "" {
				displayName := "#" + inc.SlackChannelName
				if inc.SlackChannelArchived {
					displayName += " (archived)"
				}
				b.WriteString(m.renderLinkRowCustom(i18n.T("incidents.links.slack"), inc.SlackChannelURL, displayName))
			} else {
				b.WriteString(m.renderLinkRow(i18n.T("incidents.links.slack"), inc.SlackChannelURL))
			}
		}
		if inc.JiraIssueURL != "" {
			b.WriteString(m.renderLinkRow(i18n.T("incidents.links.jira"), inc.JiraIssueURL))
		}
		b.WriteString("\n")
	}

	// Description (shows the summary if different from title, rendered as markdown)
	summaryClean := strings.ReplaceAll(inc.Summary, "\r", "")
	titleClean := strings.ReplaceAll(title, "\n", " ")
	titleClean = strings.ReplaceAll(titleClean, "\r", "")
	if summaryClean != "" && strings.TrimSpace(summaryClean) != strings.TrimSpace(titleClean) {
		b.WriteString(styles.TextBold.Render("📝 " + i18n.T("incidents.detail.description")))
		b.WriteString("\n")
		// Render as markdown, use detail width minus padding
		descWidth := m.detailWidth - 4
		if descWidth < 40 {
			descWidth = 40
		}
		b.WriteString(styles.RenderMarkdown(summaryClean, descWidth))
		b.WriteString("\n\n")
	}

	// Timeline
	b.WriteString(styles.TextBold.Render("📅 " + i18n.T("incidents.timeline.title")))
	b.WriteString("\n")

	if !inc.CreatedAt.IsZero() {
		b.WriteString(m.renderDetailRow(i18n.T("incidents.timeline.created"), formatTime(inc.CreatedAt)))
	}
	if inc.StartedAt != nil {
		b.WriteString(m.renderDetailRow(i18n.T("incidents.timeline.started"), formatTime(*inc.StartedAt)))
	}
	if inc.DetectedAt != nil {
		b.WriteString(m.renderDetailRow(i18n.T("incidents.timeline.detected"), formatTime(*inc.DetectedAt)))
	}
	if inc.AcknowledgedAt != nil {
		b.WriteString(m.renderDetailRow(i18n.T("incidents.timeline.acknowledged"), formatTime(*inc.AcknowledgedAt)))
	}
	if inc.MitigatedAt != nil {
		b.WriteString(m.renderDetailRow(i18n.T("incidents.timeline.mitigated"), formatTime(*inc.MitigatedAt)))
	}
	if inc.ResolvedAt != nil {
		b.WriteString(m.renderDetailRow(i18n.T("incidents.timeline.resolved"), formatTime(*inc.ResolvedAt)))
	}
	if inc.ClosedAt != nil {
		b.WriteString(m.renderDetailRow(i18n.T("incidents.timeline.closed"), formatTime(*inc.ClosedAt)))
	}
	if inc.CancelledAt != nil {
		b.WriteString(m.renderDetailRow(i18n.T("incidents.timeline.cancelled"), formatTime(*inc.CancelledAt)))
	}
	// Scheduled maintenance times
	if inc.ScheduledFor != nil {
		b.WriteString(m.renderDetailRow(i18n.T("incidents.timeline.scheduled_for"), formatTime(*inc.ScheduledFor)))
	}
	if inc.ScheduledUntil != nil {
		b.WriteString(m.renderDetailRow(i18n.T("incidents.timeline.scheduled_until"), formatTime(*inc.ScheduledUntil)))
	}
	b.WriteString("\n")

	// Duration Metrics section
	hasMetrics := inc.Duration() > 0 || inc.TimeToMitigation() > 0 || inc.TimeToResolution() > 0
	if hasMetrics {
		b.WriteString(styles.TextBold.Render("⏳ " + i18n.T("incidents.metrics.title")))
		b.WriteString("\n")

		// Total duration
		if duration := inc.Duration(); duration > 0 {
			b.WriteString(m.renderMetricRow(i18n.T("incidents.metrics.duration"), formatDuration(duration)))
		}

		// Time to Detection (TTD)
		if ttd := inc.TimeToDetection(); ttd > 0 {
			b.WriteString(m.renderMetricRow(i18n.T("incidents.metrics.ttd"), formatHours(ttd)))
		}

		// Time to Acknowledge (TTA)
		if tta := inc.TimeToAcknowledge(); tta > 0 {
			b.WriteString(m.renderMetricRow(i18n.T("incidents.metrics.tta"), formatHours(tta)))
		}

		// Time to Mitigation (TTM)
		if ttm := inc.TimeToMitigation(); ttm > 0 {
			b.WriteString(m.renderMetricRow(i18n.T("incidents.metrics.ttm"), formatHours(ttm)))
		}

		// Time to Resolution (TTR)
		if ttr := inc.TimeToResolution(); ttr > 0 {
			b.WriteString(m.renderMetricRow(i18n.T("incidents.metrics.ttr"), formatHours(ttr)))
		}

		// Time to Close
		if ttc := inc.TimeToClose(); ttc > 0 {
			b.WriteString(m.renderMetricRow(i18n.T("incidents.metrics.ttc"), formatHours(ttc)))
		}

		// Time in Triage
		if triage := inc.InTriageDuration(); triage > 0 {
			b.WriteString(m.renderMetricRow(i18n.T("incidents.metrics.triage"), formatDuration(triage)))
		}

		// Maintenance duration (for scheduled incidents)
		if maint := inc.MaintenanceDuration(); maint > 0 {
			b.WriteString(m.renderMetricRow(i18n.T("incidents.metrics.maintenance"), formatDuration(maint)))
		}

		b.WriteString("\n")
	}

	// Services, Environments, Teams
	b.WriteString(renderBulletList("🛠 ", i18n.T("incidents.detail.services"), inc.Services))
	b.WriteString(renderBulletList("🌐 ", i18n.T("incidents.detail.environments"), inc.Environments))
	b.WriteString(renderBulletList("👥 ", i18n.T("incidents.detail.teams"), inc.Teams))

	// Extended info (populated when DetailLoaded is true)
	if inc.DetailLoaded {
		// Mitigation message
		if inc.MitigationMessage != "" {
			b.WriteString(styles.TextBold.Render("🛡 " + i18n.T("incidents.detail.mitigation_message")))
			b.WriteString("\n")
			descWidth := m.detailWidth - 4
			if descWidth < 40 {
				descWidth = 40
			}
			b.WriteString(styles.RenderMarkdown(inc.MitigationMessage, descWidth))
			b.WriteString("\n\n")
		}

		// Resolution message
		if inc.ResolutionMessage != "" {
			b.WriteString(styles.TextBold.Render("✅ " + i18n.T("incidents.detail.resolution_message")))
			b.WriteString("\n")
			descWidth := m.detailWidth - 4
			if descWidth < 40 {
				descWidth = 40
			}
			b.WriteString(styles.RenderMarkdown(inc.ResolutionMessage, descWidth))
			b.WriteString("\n\n")
		}

		// Who performed actions
		if inc.StartedByName != "" || inc.MitigatedByName != "" || inc.ResolvedByName != "" {
			b.WriteString(styles.TextBold.Render("👤 " + i18n.T("incidents.detail.responders")))
			b.WriteString("\n")
			if inc.StartedByName != "" {
				b.WriteString(styles.DetailLabel.Render(i18n.T("incidents.detail.started_by") + ":"))
				b.WriteString(" ")
				b.WriteString(styles.RenderNameWithEmail(inc.StartedByName, inc.StartedByEmail))
				b.WriteString("\n")
			}
			if inc.MitigatedByName != "" {
				b.WriteString(styles.DetailLabel.Render(i18n.T("incidents.detail.mitigated_by") + ":"))
				b.WriteString(" ")
				b.WriteString(styles.RenderNameWithEmail(inc.MitigatedByName, inc.MitigatedByEmail))
				b.WriteString("\n")
			}
			if inc.ResolvedByName != "" {
				b.WriteString(styles.DetailLabel.Render(i18n.T("incidents.detail.resolved_by") + ":"))
				b.WriteString(" ")
				b.WriteString(styles.RenderNameWithEmail(inc.ResolvedByName, inc.ResolvedByEmail))
				b.WriteString("\n")
			}
			b.WriteString("\n")
		}

		// Roles (Commander, Communicator, etc.)
		if len(inc.Roles) > 0 {
			b.WriteString(styles.TextBold.Render("🎭 " + i18n.T("incidents.detail.roles")))
			b.WriteString("\n")
			for _, role := range inc.Roles {
				userName := strings.TrimSpace(role.UserName)
				if userName == "" {
					continue
				}
				roleName := strings.TrimSpace(role.Name)
				userEmail := strings.TrimSpace(role.UserEmail)
				b.WriteString(styles.DetailLabel.Render(roleName + ":"))
				b.WriteString(" ")
				b.WriteString(styles.RenderNameWithEmail(userName, userEmail))
				b.WriteString("\n")
			}
			b.WriteString("\n")
		}

		// Causes, Types, Functionalities
		b.WriteString(renderBulletList("🔍 ", i18n.T("incidents.detail.causes"), inc.Causes))
		b.WriteString(renderBulletList("📋 ", i18n.T("incidents.detail.types"), inc.IncidentTypes))
		b.WriteString(renderBulletList("⚙️ ", i18n.T("incidents.detail.functionalities"), inc.Functionalities))

		// Integration links
		integrationLinks := m.collectIntegrationLinks(inc)
		if len(integrationLinks) > 0 {
			b.WriteString(styles.TextBold.Render("🔌 " + i18n.T("incidents.detail.integrations")))
			b.WriteString("\n")
			for _, link := range integrationLinks {
				b.WriteString(m.renderLinkRow(link.label, link.url))
			}
			b.WriteString("\n")
		}

		// Labels
		if len(inc.Labels) > 0 {
			b.WriteString(styles.TextBold.Render("🏷  " + i18n.T("incidents.detail.labels")))
			b.WriteString("\n")
			// Sort keys for consistent display
			keys := make([]string, 0, len(inc.Labels))
			for k := range inc.Labels {
				keys = append(keys, k)
			}
			for _, k := range keys {
				b.WriteString(styles.DetailLabel.Render(k + ":"))
				b.WriteString(" ")
				b.WriteString(m.renderLabelValue(inc.Labels[k]))
				b.WriteString("\n")
			}
			b.WriteString("\n")
		}

		// Metadata (source, private, retrospective status)
		hasMetadata := inc.Source != "" || inc.Private || inc.RetrospectiveProgressStatus != ""
		if hasMetadata {
			b.WriteString(styles.TextBold.Render("ℹ️  " + i18n.T("incidents.detail.metadata")))
			b.WriteString("\n")
			if inc.Source != "" {
				b.WriteString(m.renderDetailRow(i18n.T("incidents.detail.source"), inc.Source))
			}
			if inc.Private {
				b.WriteString(m.renderDetailRow(i18n.T("incidents.detail.private"), "Yes"))
			}
			if inc.RetrospectiveProgressStatus != "" {
				b.WriteString(m.renderDetailRow(i18n.T("incidents.detail.retrospective"), formatRetroStatus(inc.RetrospectiveProgressStatus)))
			}
			b.WriteString("\n")
		}
	}

	// Show loading spinner or hint if detail not loaded
	if m.IsLoadingIncident(inc.ID) {
		b.WriteString("\n")
		fmt.Fprintf(&b, "%s %s", m.spinnerView, i18n.T("incidents.loading_details"))
	} else if !inc.DetailLoaded {
		b.WriteString("\n")
		b.WriteString(styles.TextDim.Render(i18n.T("incidents.press_enter")))
	}

	return b.String()
}

func (m IncidentsModel) renderDetailRow(label, value string) string {
	return styles.DetailLabel.Render(label+":") + " " + styles.DetailValue.Render(value) + "\n"
}

func (m IncidentsModel) renderMetricRow(label, value string) string {
	return styles.DetailLabel.Render(label+":") + " " + styles.RenderMetric(value) + "\n"
}

func (m IncidentsModel) renderLinkRow(label, url string) string {
	// Calculate available width for URL display
	// Account for label, colon, space, container padding, and border (~20 chars)
	maxURLLen := m.detailWidth - len(label) - 20
	if maxURLLen < 20 {
		maxURLLen = 20
	}

	displayURL := url
	if len(displayURL) > maxURLLen {
		displayURL = displayURL[:maxURLLen-3] + "..."
	}

	return styles.DetailLabel.Render(label+":") + " " + styles.RenderLink(url, displayURL) + "\n"
}

func (m IncidentsModel) renderLinkRowCustom(label, url, displayText string) string {
	return styles.DetailLabel.Render(label+":") + " " + styles.RenderLink(url, displayText) + "\n"
}

// severitySignalPlain returns plain signal bars without color styling
func severitySignalPlain(severity string) string {
	switch severity {
	case "critical", "Critical", "CRITICAL", "sev0", "SEV0":
		return "▁▃▅▇"
	case "high", "High", "HIGH", "sev1", "SEV1":
		return "▁▃▅░"
	case "medium", "Medium", "MEDIUM", "sev2", "SEV2":
		return "▁▃░░"
	case "low", "Low", "LOW", "sev3", "SEV3":
		return "▁░░░"
	default:
		return "░░░░"
	}
}

// severityStyle returns the lipgloss style for a severity level
func severityStyle(severity string) lipgloss.Style {
	switch severity {
	case "critical", "Critical", "CRITICAL", "sev0", "SEV0":
		return lipgloss.NewStyle().Foreground(styles.ColorCritical).Bold(true)
	case "high", "High", "HIGH", "sev1", "SEV1":
		return lipgloss.NewStyle().Foreground(styles.ColorHigh).Bold(true)
	case "medium", "Medium", "MEDIUM", "sev2", "SEV2":
		return lipgloss.NewStyle().Foreground(styles.ColorMedium).Bold(true)
	case "low", "Low", "LOW", "sev3", "SEV3":
		return lipgloss.NewStyle().Foreground(styles.ColorLow).Bold(true)
	default:
		return lipgloss.NewStyle().Foreground(styles.ColorMuted)
	}
}

// statusStyle returns the lipgloss style for a status
func statusStyle(status string) lipgloss.Style {
	s := strings.ToLower(strings.TrimSpace(status))
	switch s {
	case "open", "triggered", "firing", "critical":
		return lipgloss.NewStyle().Foreground(styles.ColorPastelRed)
	case "started", "in_progress", "acknowledged", "investigating", "identified", "monitoring", "mitigated":
		return lipgloss.NewStyle().Foreground(styles.ColorPastelYellow)
	case "resolved", "fixed":
		return lipgloss.NewStyle().Foreground(styles.ColorPastelGreen)
	default:
		return lipgloss.NewStyle().Foreground(styles.ColorPastelGray)
	}
}

// SetSort sets the sort field and direction
// Returns true if the field changed (requiring a reload from API)
func (m *IncidentsModel) SetSort(field SortField) bool {
	fieldChanged := m.sortState.Toggle(field)
	return fieldChanged
}

// GetSortParam returns the API sort parameter string based on current sort state
// Returns empty string if sorting is disabled
func (m IncidentsModel) GetSortParam() string {
	if !m.sortState.IsEnabled() {
		return ""
	}

	var fieldName string
	switch m.sortState.Field {
	case SortByCreated:
		fieldName = "created_at"
	case SortByUpdated:
		fieldName = "updated_at"
	default:
		return ""
	}

	// Add minus prefix for descending order
	if m.sortState.Direction == components.SortDesc {
		return "-" + fieldName
	}
	return fieldName
}

// GetSortInfo returns a string describing the current sort
func (m IncidentsModel) GetSortInfo() string {
	if !m.sortState.IsEnabled() {
		return ""
	}

	var fieldName string
	switch m.sortState.Field {
	case SortByCreated:
		fieldName = i18n.T("sorting.created")
	case SortByUpdated:
		fieldName = i18n.T("sorting.updated")
	default:
		return ""
	}

	// Show direction as "Newest First" or "Oldest First"
	var directionLabel string
	if m.sortState.Direction == components.SortDesc {
		directionLabel = i18n.T("sorting.newest_first")
	} else {
		directionLabel = i18n.T("sorting.oldest_first")
	}

	return fmt.Sprintf("%s (%s)", fieldName, directionLabel)
}

// ToggleSortMenu toggles the visibility of the sort menu
func (m *IncidentsModel) ToggleSortMenu() {
	m.sortMenu.Toggle()
}

// IsSortMenuVisible returns whether the sort menu is visible
func (m IncidentsModel) IsSortMenuVisible() bool {
	return m.sortMenu.IsVisible()
}

// HandleSortMenuKey handles keyboard input for the sort menu
// Returns true if sorting changed and a reload is needed
func (m *IncidentsModel) HandleSortMenuKey(key string) bool {
	if selected, shouldApply := m.sortMenu.HandleKey(key); shouldApply {
		if field, ok := selected.(SortField); ok {
			return m.SetSort(field)
		}
	}
	return false
}

// RenderSortMenu renders the sort menu overlay
func (m IncidentsModel) RenderSortMenu() string {
	return m.sortMenu.Render(m.sortState.Field, m.sortState.Direction)
}

// isIncidentURL checks if a string looks like a URL
func isIncidentURL(s string) bool {
	return strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://")
}

// renderLabelValue renders a label value, making URLs clickable
func (m IncidentsModel) renderLabelValue(value string) string {
	if isIncidentURL(value) {
		// Truncate long URLs for display
		displayURL := value
		maxLen := m.detailWidth - 30
		if maxLen < 30 {
			maxLen = 30
		}
		if len(displayURL) > maxLen {
			displayURL = displayURL[:maxLen-3] + "..."
		}
		return styles.RenderLink(value, displayURL)
	}
	return styles.DetailValue.Render(value)
}

// integrationLink represents a labeled URL for display
type integrationLink struct {
	label string
	url   string
}

// collectIntegrationLinks gathers all non-empty integration URLs
func (m IncidentsModel) collectIntegrationLinks(inc *api.Incident) []integrationLink {
	var links []integrationLink

	if inc.GoogleMeetingURL != "" {
		links = append(links, integrationLink{i18n.T("incidents.integrations.google_meet"), inc.GoogleMeetingURL})
	}
	if inc.ZoomMeetingJoinURL != "" {
		links = append(links, integrationLink{i18n.T("incidents.integrations.zoom"), inc.ZoomMeetingJoinURL})
	}
	if inc.LinearIssueURL != "" {
		links = append(links, integrationLink{i18n.T("incidents.integrations.linear"), inc.LinearIssueURL})
	}
	if inc.GithubIssueURL != "" {
		links = append(links, integrationLink{i18n.T("incidents.integrations.github"), inc.GithubIssueURL})
	}
	if inc.GitlabIssueURL != "" {
		links = append(links, integrationLink{i18n.T("incidents.integrations.gitlab"), inc.GitlabIssueURL})
	}
	if inc.PagerdutyIncidentURL != "" {
		links = append(links, integrationLink{i18n.T("incidents.integrations.pagerduty"), inc.PagerdutyIncidentURL})
	}
	if inc.OpsgenieIncidentURL != "" {
		links = append(links, integrationLink{i18n.T("incidents.integrations.opsgenie"), inc.OpsgenieIncidentURL})
	}
	if inc.AsanaTaskURL != "" {
		links = append(links, integrationLink{i18n.T("incidents.integrations.asana"), inc.AsanaTaskURL})
	}
	if inc.TrelloCardURL != "" {
		links = append(links, integrationLink{i18n.T("incidents.integrations.trello"), inc.TrelloCardURL})
	}
	if inc.ConfluencePageURL != "" {
		links = append(links, integrationLink{i18n.T("incidents.integrations.confluence"), inc.ConfluencePageURL})
	}
	if inc.DatadogNotebookURL != "" {
		links = append(links, integrationLink{i18n.T("incidents.integrations.datadog"), inc.DatadogNotebookURL})
	}
	if inc.ServiceNowIncidentURL != "" {
		links = append(links, integrationLink{i18n.T("incidents.integrations.servicenow"), inc.ServiceNowIncidentURL})
	}
	if inc.FreshserviceTicketURL != "" {
		links = append(links, integrationLink{i18n.T("incidents.integrations.freshservice"), inc.FreshserviceTicketURL})
	}

	return links
}

// formatRetroStatus formats the retrospective progress status for display
func formatRetroStatus(status string) string {
	switch status {
	case "not_started":
		return i18n.T("incidents.retro.not_started")
	case "in_progress":
		return i18n.T("incidents.retro.in_progress")
	case "completed":
		return i18n.T("incidents.retro.completed")
	default:
		return status
	}
}

// GetDetailPlainText returns the detail panel content as plain text for clipboard
func (m IncidentsModel) GetDetailPlainText() string {
	inc := m.SelectedIncident()
	if inc == nil {
		return ""
	}
	return m.generatePlainTextDetail(inc)
}

// generatePlainTextDetail generates plain text detail for copying to clipboard
func (m IncidentsModel) generatePlainTextDetail(inc *api.Incident) string {
	var b strings.Builder

	// Title line
	title := inc.Title
	if title == "" {
		title = inc.Summary
	}
	title = strings.ReplaceAll(title, "\n", " ")
	title = strings.ReplaceAll(title, "\r", "")
	if inc.SequentialID != "" {
		b.WriteString("[" + inc.SequentialID + "] ")
	}
	b.WriteString(title)
	b.WriteString("\n\n")

	// Severity, Status
	b.WriteString("Severity: " + inc.Severity + "  Status: " + inc.Status)
	if inc.Kind == "scheduled" || inc.Kind == "scheduled_maintenance" {
		b.WriteString("  Type: Scheduled Maintenance")
	}
	if inc.CreatedByName != "" {
		relTime := formatRelativeTime(inc.CreatedAt)
		b.WriteString(fmt.Sprintf("  Created %s by %s", relTime, inc.CreatedByName))
		if inc.CreatedByEmail != "" {
			b.WriteString(" <" + inc.CreatedByEmail + ">")
		}
	}
	b.WriteString("\n\n")

	// Links
	rootlyURL := inc.ShortURL
	if rootlyURL == "" {
		rootlyURL = inc.URL
	}
	if rootlyURL == "" && inc.ID != "" {
		rootlyURL = fmt.Sprintf("https://rootly.com/account/incidents/%s", inc.ID)
	}
	if inc.SlackChannelURL != "" || inc.JiraIssueURL != "" || rootlyURL != "" {
		b.WriteString("Links\n")
		if rootlyURL != "" {
			b.WriteString("  Rootly: " + rootlyURL + "\n")
		}
		if inc.SlackChannelURL != "" {
			b.WriteString("  Slack: " + inc.SlackChannelURL + "\n")
		}
		if inc.JiraIssueURL != "" {
			b.WriteString("  Jira: " + inc.JiraIssueURL + "\n")
		}
		b.WriteString("\n")
	}

	// Description
	summaryClean := strings.ReplaceAll(inc.Summary, "\r", "")
	titleClean := strings.ReplaceAll(title, "\n", " ")
	titleClean = strings.ReplaceAll(titleClean, "\r", "")
	if summaryClean != "" && strings.TrimSpace(summaryClean) != strings.TrimSpace(titleClean) {
		b.WriteString("Description\n")
		b.WriteString(summaryClean)
		b.WriteString("\n\n")
	}

	// Timeline
	b.WriteString("Timeline\n")
	if !inc.CreatedAt.IsZero() {
		b.WriteString("  Created: " + formatTime(inc.CreatedAt) + "\n")
	}
	if inc.StartedAt != nil {
		b.WriteString("  Started: " + formatTime(*inc.StartedAt) + "\n")
	}
	if inc.DetectedAt != nil {
		b.WriteString("  Detected: " + formatTime(*inc.DetectedAt) + "\n")
	}
	if inc.AcknowledgedAt != nil {
		b.WriteString("  Acknowledged: " + formatTime(*inc.AcknowledgedAt) + "\n")
	}
	if inc.MitigatedAt != nil {
		b.WriteString("  Mitigated: " + formatTime(*inc.MitigatedAt) + "\n")
	}
	if inc.ResolvedAt != nil {
		b.WriteString("  Resolved: " + formatTime(*inc.ResolvedAt) + "\n")
	}
	if inc.ClosedAt != nil {
		b.WriteString("  Closed: " + formatTime(*inc.ClosedAt) + "\n")
	}
	b.WriteString("\n")

	// Services, Environments, Teams
	if len(inc.Services) > 0 {
		b.WriteString("Services: " + strings.Join(inc.Services, ", ") + "\n")
	}
	if len(inc.Environments) > 0 {
		b.WriteString("Environments: " + strings.Join(inc.Environments, ", ") + "\n")
	}
	if len(inc.Teams) > 0 {
		b.WriteString("Teams: " + strings.Join(inc.Teams, ", ") + "\n")
	}

	// Extended info
	if inc.DetailLoaded {
		if len(inc.Roles) > 0 {
			b.WriteString("\nRoles\n")
			for _, role := range inc.Roles {
				userName := strings.TrimSpace(role.UserName)
				if userName == "" {
					continue
				}
				roleName := strings.TrimSpace(role.Name)
				b.WriteString("  " + roleName + ": " + userName)
				if role.UserEmail != "" {
					b.WriteString(" <" + role.UserEmail + ">")
				}
				b.WriteString("\n")
			}
		}

		if len(inc.Labels) > 0 {
			b.WriteString("\nLabels\n")
			for k, v := range inc.Labels {
				b.WriteString("  " + k + ": " + v + "\n")
			}
		}
	}

	return b.String()
}
