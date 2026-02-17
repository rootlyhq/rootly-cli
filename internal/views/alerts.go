package views

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/list"
	"github.com/evertras/bubble-table/table"

	"github.com/rootlyhq/rootly-tui/internal/api"
	"github.com/rootlyhq/rootly-tui/internal/config"
	"github.com/rootlyhq/rootly-tui/internal/i18n"
	"github.com/rootlyhq/rootly-tui/internal/styles"
)

// renderAlertBulletList renders a section with a bold title and bullet list using lipgloss/list
func renderAlertBulletList(icon, title string, items []string) string {
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

// Column keys for alerts table
const (
	alertColKeyIndicator = "indicator"
	alertColKeySource    = "source"
	alertColKeyID        = "id"
	alertColKeyStatus    = "status"
	alertColKeyTime      = "time"
	alertColKeyTitle     = "title"
)

// Row indicator for selected row (same as incidents)
const alertRowIndicator = "▶"

type AlertsModel struct {
	alerts       []api.Alert
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
	// Detail loading state - tracks which alert ID is currently loading (empty = not loading)
	detailLoadingID string
	// Detail viewport for scrollable content
	detailViewport      viewport.Model
	detailViewportReady bool
	detailFocused       bool // Whether detail pane has focus (for scrolling)
	// Table for list view
	table table.Model
}

func NewAlertsModel() AlertsModel {
	// Define table columns with i18n headers using evertras/bubble-table
	columns := []table.Column{
		table.NewColumn(alertColKeyIndicator, "", 2), // Selection indicator column
		table.NewColumn(alertColKeySource, i18n.T("alerts.detail.source"), 4),
		table.NewColumn(alertColKeyID, i18n.T("incidents.col.id"), 8),
		table.NewColumn(alertColKeyStatus, i18n.T("incidents.detail.status"), 10),
		table.NewColumn(alertColKeyTime, "", 8),                                 // Relative time (e.g., "2d ago", "3h ago")
		table.NewFlexColumn(alertColKeyTitle, i18n.T("incidents.col.title"), 1), // Flex to fill remaining space
	}

	t := table.New(columns).
		Focused(true).
		Border(borderNoDividers()).
		WithBaseStyle(lipgloss.NewStyle().Foreground(styles.ColorText)).
		HighlightStyle(lipgloss.NewStyle()). // No background highlight, arrow shows selection
		HeaderStyle(lipgloss.NewStyle().Bold(true).Foreground(styles.ColorText))

	return AlertsModel{
		alerts:      []api.Alert{},
		currentPage: 1,
		table:       t,
	}
}

func (m AlertsModel) Init() tea.Cmd {
	return nil
}

func (m AlertsModel) Update(msg tea.Msg) (AlertsModel, tea.Cmd) {
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
			if cursor < len(m.alerts)-1 {
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
			if len(m.alerts) > 0 {
				m.table = m.table.WithHighlightedRow(len(m.alerts) - 1)
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

// updateViewportContent updates the viewport content when data changes
func (m *AlertsModel) updateViewportContent() {
	if !m.detailViewportReady {
		return
	}
	alert := m.SelectedAlert()
	if alert == nil {
		return
	}
	content := m.generateDetailContent(alert)
	m.detailViewport.SetContent(content)
	m.detailViewport.GotoTop()
}

// updateRowIndicators updates the arrow indicator to show on the current row
func (m *AlertsModel) updateRowIndicators() {
	if len(m.alerts) == 0 {
		return
	}
	cursor := m.table.GetHighlightedRowIndex()
	rows := make([]table.Row, len(m.alerts))
	for i, alert := range m.alerts {
		shortID := alert.ShortID
		if shortID == "" {
			shortID = "---"
		}
		status := alert.Status
		if len(status) > 10 {
			status = status[:10]
		}
		summary := strings.ReplaceAll(alert.Summary, "\n", " ")
		summary = strings.ReplaceAll(summary, "\r", "")

		statusCell := table.NewStyledCell(status, statusStyle(status))

		// Use StartedAt if available, otherwise CreatedAt
		timeStr := "-"
		if alert.StartedAt != nil {
			timeStr = formatRelativeTime(*alert.StartedAt)
		} else if !alert.CreatedAt.IsZero() {
			timeStr = formatRelativeTime(alert.CreatedAt)
		}
		timeCell := table.NewStyledCell(timeStr, styles.TextDim)

		indicator := ""
		if i == cursor {
			indicator = alertRowIndicator
		}

		rows[i] = table.NewRow(table.RowData{
			alertColKeyIndicator: indicator,
			alertColKeySource:    styles.AlertSourceIcon(alert.Source),
			alertColKeyID:        shortID,
			alertColKeyStatus:    statusCell,
			alertColKeyTime:      timeCell,
			alertColKeyTitle:     summary,
		})
	}
	m.table = m.table.WithRows(rows)
}

// SetDetailFocused sets focus on the detail pane for scrolling
func (m *AlertsModel) SetDetailFocused(focused bool) {
	m.detailFocused = focused
}

// IsDetailFocused returns whether the detail pane has focus
func (m AlertsModel) IsDetailFocused() bool {
	return m.detailFocused
}

func (m *AlertsModel) updateDimensions() {
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

func (m *AlertsModel) SetAlerts(alerts []api.Alert, pagination api.PaginationInfo) {
	m.alerts = alerts
	m.loading = false
	m.error = ""
	m.currentPage = pagination.CurrentPage
	m.totalPages = pagination.TotalPages
	m.totalCount = pagination.TotalCount
	m.hasNext = pagination.HasNext
	m.hasPrev = pagination.HasPrev

	// Build table rows from alerts with styled cells
	rows := make([]table.Row, len(alerts))
	cursor := m.table.GetHighlightedRowIndex()
	for i, alert := range alerts {
		shortID := alert.ShortID
		if shortID == "" {
			shortID = "---"
		}
		status := alert.Status
		if len(status) > 10 {
			status = status[:10]
		}
		summary := strings.ReplaceAll(alert.Summary, "\n", " ")
		summary = strings.ReplaceAll(summary, "\r", "")

		// Create styled cells using evertras/bubble-table
		statusCell := table.NewStyledCell(status, statusStyle(status))

		// Use StartedAt if available, otherwise CreatedAt
		timeStr := "-"
		if alert.StartedAt != nil {
			timeStr = formatRelativeTime(*alert.StartedAt)
		} else if !alert.CreatedAt.IsZero() {
			timeStr = formatRelativeTime(alert.CreatedAt)
		}
		timeCell := table.NewStyledCell(timeStr, styles.TextDim)

		// Show indicator for highlighted row
		indicator := ""
		if i == cursor {
			indicator = alertRowIndicator
		}

		rows[i] = table.NewRow(table.RowData{
			alertColKeyIndicator: indicator,
			alertColKeySource:    styles.AlertSourceIcon(alert.Source),
			alertColKeyID:        shortID,
			alertColKeyStatus:    statusCell,
			alertColKeyTime:      timeCell,
			alertColKeyTitle:     summary,
		})
	}
	m.table = m.table.WithRows(rows)

	// Set custom footer with pagination info
	footer := m.buildPaginationFooter()
	m.table = m.table.WithStaticFooter(footer)

	// Adjust cursor if needed
	if cursor >= len(alerts) && len(alerts) > 0 {
		m.table = m.table.WithHighlightedRow(len(alerts) - 1)
	}
	m.updateViewportContent()
}

// buildPaginationFooter creates a footer string showing pagination info
func (m *AlertsModel) buildPaginationFooter() string {
	if m.totalPages > 0 && m.totalCount > 0 {
		return fmt.Sprintf("Page %d/%d (%d total)", m.currentPage, m.totalPages, m.totalCount)
	}
	if m.currentPage > 0 {
		return fmt.Sprintf("Page %d", m.currentPage)
	}
	return ""
}

func (m *AlertsModel) SetLoading(loading bool) {
	m.loading = loading
}

func (m *AlertsModel) SetSpinner(spinner string) {
	m.spinnerView = spinner
}

func (m *AlertsModel) SetError(err string) {
	m.error = err
	m.loading = false
}

func (m *AlertsModel) SetDimensions(width, height int) {
	m.width = width
	m.height = height
	m.updateDimensions()
}

// SetLayout sets the layout direction (horizontal or vertical)
func (m *AlertsModel) SetLayout(layout string) {
	m.layout = layout
	m.updateDimensions()
}

// Pagination methods
func (m AlertsModel) CurrentPage() int {
	return m.currentPage
}

func (m AlertsModel) HasNextPage() bool {
	return m.hasNext
}

func (m AlertsModel) HasPrevPage() bool {
	return m.hasPrev
}

func (m AlertsModel) TotalPages() int {
	return m.totalPages
}

func (m AlertsModel) TotalCount() int {
	return m.totalCount
}

func (m *AlertsModel) NextPage() {
	// Check both hasNext flag and totalPages to prevent going beyond the last page
	if m.hasNext && (m.totalPages == 0 || m.currentPage < m.totalPages) {
		m.currentPage++
		m.table = m.table.WithHighlightedRow(0)
	}
}

func (m *AlertsModel) PrevPage() {
	if m.hasPrev && m.currentPage > 1 {
		m.currentPage--
		m.table = m.table.WithHighlightedRow(0)
	}
}

func (m AlertsModel) SelectedAlert() *api.Alert {
	cursor := m.table.GetHighlightedRowIndex()
	if cursor >= 0 && cursor < len(m.alerts) {
		return &m.alerts[cursor]
	}
	return nil
}

func (m AlertsModel) SelectedIndex() int {
	return m.table.GetHighlightedRowIndex()
}

func (m *AlertsModel) SetDetailLoading(id string) {
	m.detailLoadingID = id
}

func (m *AlertsModel) ClearDetailLoading() {
	m.detailLoadingID = ""
}

func (m AlertsModel) IsDetailLoading() bool {
	return m.detailLoadingID != ""
}

// IsLoadingAlert returns true if the specified alert ID is currently loading
func (m AlertsModel) IsLoadingAlert(id string) bool {
	return m.detailLoadingID == id
}

func (m *AlertsModel) UpdateAlertDetail(index int, alert *api.Alert) {
	if index >= 0 && index < len(m.alerts) && alert != nil {
		m.alerts[index] = *alert
		// Update viewport content without resetting scroll (detail just loaded)
		if m.detailViewportReady && index == m.table.GetHighlightedRowIndex() {
			content := m.generateDetailContent(alert)
			m.detailViewport.SetContent(content)
		}
	}
}

func (m AlertsModel) View() string {
	if m.loading {
		// Show loading within the layout structure to prevent jarring shift
		loadingMsg := fmt.Sprintf("%s %s", m.spinnerView, i18n.Tf("incidents.loading_page", map[string]any{"Page": m.currentPage}))
		listContent := styles.TextBold.Render(i18n.T("alerts.title")) + "\n\n" + styles.TextDim.Render(loadingMsg)
		listView := styles.ListContainer.Width(m.listWidth).Height(m.listHeight).Render(listContent)
		detailView := styles.DetailContainer.Width(m.detailWidth).Height(m.detailHeight).Render("")
		return m.joinPanes(listView, detailView)
	}

	if m.error != "" {
		return styles.Error.Render(i18n.T("common.error") + ": " + m.error)
	}

	if len(m.alerts) == 0 {
		return styles.TextDim.Render(i18n.T("alerts.none_found"))
	}

	listView := m.renderList(m.listHeight)
	detailView := m.renderDetail(m.detailHeight)

	return m.joinPanes(listView, detailView)
}

// joinPanes joins the list and detail panes based on the current layout
func (m AlertsModel) joinPanes(listView, detailView string) string {
	if m.layout == config.LayoutVertical {
		return lipgloss.JoinVertical(lipgloss.Left, listView, detailView)
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, listView, "  ", detailView)
}

func (m AlertsModel) renderList(height int) string {
	var b strings.Builder

	// Title
	title := styles.TextBold.Render(i18n.T("alerts.title"))
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
	if len(m.alerts) > 0 {
		footer.WriteString(styles.TextDim.Render(fmt.Sprintf("  (%d-%d)", m.table.GetHighlightedRowIndex()+1, len(m.alerts))))
	}

	b.WriteString(footer.String())

	content := b.String()
	return styles.ListContainer.Width(m.listWidth).Height(height).Render(content)
}

func (m AlertsModel) renderDetail(height int) string {
	alert := m.SelectedAlert()
	if alert == nil {
		return styles.DetailContainer.Width(m.detailWidth).Height(height).Render(
			styles.TextDim.Render(i18n.T("alerts.select_prompt")),
		)
	}

	// Render with or without viewport
	if !m.detailViewportReady {
		content := m.generateDetailContent(alert)
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
func (m AlertsModel) generateDetailContent(alert *api.Alert) string {
	var b strings.Builder

	// Title line: [SHORT_ID] Summary (strip newlines for single-line display)
	summaryClean := strings.ReplaceAll(alert.Summary, "\n", " ")
	summaryClean = strings.ReplaceAll(summaryClean, "\r", "")
	if alert.ShortID != "" {
		b.WriteString(styles.Primary.Bold(true).Render("[" + alert.ShortID + "]"))
		b.WriteString(" ")
	}
	b.WriteString(styles.DetailTitle.Render(summaryClean))
	b.WriteString("\n\n")

	// Source and Status row
	sourceIcon := styles.AlertSourceIcon(alert.Source)
	sourceName := styles.AlertSourceName(alert.Source)
	statusBadge := styles.RenderStatus(alert.Status)
	fmt.Fprintf(&b, "%s: %s %s  %s: %s", i18n.T("alerts.detail.source"), sourceIcon, sourceName, i18n.T("incidents.detail.status"), statusBadge)

	// Triggered time
	if !alert.CreatedAt.IsZero() {
		relTime := formatRelativeTime(alert.CreatedAt)
		fmt.Fprintf(&b, "  Triggered %s", relTime)
	}
	b.WriteString("\n\n")

	// Links section (high up for quick access)
	rootlyURL := alert.URL // Use URL from API if available
	if rootlyURL == "" && alert.ShortID != "" {
		rootlyURL = fmt.Sprintf("https://rootly.com/account/alerts/%s", alert.ShortID)
	}
	if rootlyURL != "" || alert.ExternalURL != "" {
		b.WriteString(styles.TextBold.Render("🔗 " + i18n.T("alerts.detail.links")))
		b.WriteString("\n")
		if rootlyURL != "" {
			b.WriteString(m.renderLinkRow(i18n.T("incidents.links.rootly"), rootlyURL))
		}
		if alert.ExternalURL != "" {
			b.WriteString(m.renderLinkRow(i18n.T("alerts.detail.source"), alert.ExternalURL))
		}
		b.WriteString("\n")
	}

	// Description (rendered as markdown)
	if alert.Description != "" {
		b.WriteString(styles.TextBold.Render("📝 " + i18n.T("incidents.detail.description")))
		b.WriteString("\n")
		// Render as markdown, use detail width minus padding
		descWidth := m.detailWidth - 4
		if descWidth < 40 {
			descWidth = 40
		}
		b.WriteString(styles.RenderMarkdown(alert.Description, descWidth))
		b.WriteString("\n\n")
	}

	// Timestamps
	b.WriteString(styles.TextBold.Render("📅 " + i18n.T("incidents.timeline.title")))
	b.WriteString("\n")

	if !alert.CreatedAt.IsZero() {
		b.WriteString(m.renderDetailRow(i18n.T("incidents.timeline.created"), formatAlertTime(alert.CreatedAt)))
	}
	if alert.StartedAt != nil {
		b.WriteString(m.renderDetailRow(i18n.T("incidents.timeline.started"), formatAlertTime(*alert.StartedAt)))
	}
	if alert.EndedAt != nil {
		b.WriteString(m.renderDetailRow(i18n.T("alerts.detail.ended"), formatAlertTime(*alert.EndedAt)))
	}
	b.WriteString("\n")

	// Services, Environments, Teams
	b.WriteString(renderAlertBulletList("🛠 ", i18n.T("incidents.detail.services"), alert.Services))
	b.WriteString(renderAlertBulletList("🌐 ", i18n.T("incidents.detail.environments"), alert.Environments))
	b.WriteString(renderAlertBulletList("👥 ", i18n.T("incidents.detail.teams"), alert.Groups))

	// Extended info (populated when DetailLoaded is true)
	if alert.DetailLoaded {
		// Urgency
		if alert.Urgency != "" {
			b.WriteString(m.renderDetailRow(i18n.T("alerts.detail.urgency"), alert.Urgency))
		}

		// Responders
		if len(alert.Responders) > 0 {
			b.WriteString("\n")
			b.WriteString(styles.TextBold.Render("👤 " + i18n.T("alerts.detail.responders")))
			b.WriteString("\n")
			for _, responder := range alert.Responders {
				b.WriteString(styles.Text.Render("• " + responder + "\n"))
			}
		}

		// Notified users
		if len(alert.NotifiedUsers) > 0 {
			b.WriteString("\n")
			b.WriteString(styles.TextBold.Render("🔔 " + i18n.T("alerts.detail.notified_users")))
			b.WriteString("\n")
			for _, user := range alert.NotifiedUsers {
				b.WriteString(styles.Text.Render("• "))
				b.WriteString(styles.RenderNameWithEmail(user.Name, user.Email))
				b.WriteString("\n")
			}
		}

		// Related incidents
		if len(alert.RelatedIncidents) > 0 {
			b.WriteString("\n")
			b.WriteString(styles.TextBold.Render("🔥 " + i18n.T("alerts.detail.related_incidents")))
			b.WriteString("\n")
			for _, inc := range alert.RelatedIncidents {
				incLabel := inc.SequentialID
				if incLabel == "" {
					incLabel = inc.ID[:8]
				}
				incInfo := fmt.Sprintf("%s - %s (%s)", incLabel, inc.Title, inc.Status)
				b.WriteString(styles.Text.Render("• " + incInfo + "\n"))
			}
		}

		// Metadata section
		hasMetadata := alert.ExternalID != "" || alert.Noise != "" || alert.DeduplicationKey != "" || alert.IsGroupLeaderAlert
		if hasMetadata {
			b.WriteString("\n")
			b.WriteString(styles.TextBold.Render("ℹ️  " + i18n.T("alerts.detail.metadata")))
			b.WriteString("\n")
			if alert.ExternalID != "" {
				b.WriteString(m.renderDetailRow(i18n.T("alerts.detail.external_id"), alert.ExternalID))
			}
			if alert.Noise != "" {
				b.WriteString(m.renderDetailRow(i18n.T("alerts.detail.noise"), formatNoiseStatus(alert.Noise)))
			}
			if alert.IsGroupLeaderAlert {
				b.WriteString(m.renderDetailRow(i18n.T("alerts.detail.group_leader"), i18n.T("alerts.detail.yes")))
			}
			if alert.DeduplicationKey != "" {
				// Truncate long dedup keys
				dedupKey := alert.DeduplicationKey
				if len(dedupKey) > 40 {
					dedupKey = dedupKey[:40] + "..."
				}
				b.WriteString(m.renderDetailRow(i18n.T("alerts.detail.dedup_key"), dedupKey))
			}
		}
	}

	// Labels (sorted for consistent display)
	if len(alert.Labels) > 0 {
		b.WriteString("\n")
		b.WriteString(styles.TextBold.Render("🏷  " + i18n.T("alerts.detail.labels")))
		b.WriteString("\n")
		keys := make([]string, 0, len(alert.Labels))
		for k := range alert.Labels {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			b.WriteString(styles.DetailLabel.Render(k + ":"))
			b.WriteString(" ")
			b.WriteString(m.renderLabelValue(alert.Labels[k]))
			b.WriteString("\n")
		}
	}

	// Data section (raw alert payload from source)
	if len(alert.Data) > 0 {
		b.WriteString("\n")
		b.WriteString(styles.TextBold.Render("📦  " + i18n.T("alerts.detail.data")))
		b.WriteString("\n")
		dataJSON, err := json.MarshalIndent(alert.Data, "", "  ")
		if err == nil {
			b.WriteString(styles.HighlightJSON(string(dataJSON)))
			b.WriteString("\n")
		}
	}

	// Show loading spinner or hint if detail not loaded
	if m.IsLoadingAlert(alert.ID) {
		b.WriteString("\n")
		fmt.Fprintf(&b, "%s %s", m.spinnerView, i18n.T("incidents.loading_details"))
	} else if !alert.DetailLoaded {
		b.WriteString("\n")
		b.WriteString(styles.TextDim.Render(i18n.T("incidents.press_enter")))
	}

	return b.String()
}

func (m AlertsModel) renderDetailRow(label, value string) string {
	return styles.DetailLabel.Render(label+":") + " " + styles.DetailValue.Render(value) + "\n"
}

func (m AlertsModel) renderLinkRow(label, url string) string {
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

// isURL checks if a string looks like a URL
func isURL(s string) bool {
	return strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://")
}

// renderLabelValue renders a label value, making URLs clickable
func (m AlertsModel) renderLabelValue(value string) string {
	if isURL(value) {
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

// formatNoiseStatus formats the noise classification for display
func formatNoiseStatus(noise string) string {
	switch noise {
	case "not_noise":
		return i18n.T("alerts.noise.not_noise")
	case "noise":
		return i18n.T("alerts.noise.noise")
	case "possible_noise":
		return i18n.T("alerts.noise.possible_noise")
	default:
		return noise
	}
}

// GetDetailPlainText returns the detail panel content as plain text for clipboard
func (m AlertsModel) GetDetailPlainText() string {
	alert := m.SelectedAlert()
	if alert == nil {
		return ""
	}
	return m.generatePlainTextDetail(alert)
}

// generatePlainTextDetail generates plain text detail for copying to clipboard
func (m AlertsModel) generatePlainTextDetail(alert *api.Alert) string {
	var b strings.Builder

	// Title line
	summaryClean := strings.ReplaceAll(alert.Summary, "\n", " ")
	summaryClean = strings.ReplaceAll(summaryClean, "\r", "")
	if alert.ShortID != "" {
		b.WriteString("[" + alert.ShortID + "] ")
	}
	b.WriteString(summaryClean)
	b.WriteString("\n\n")

	// Source and Status
	b.WriteString("Source: " + alert.Source + "  Status: " + alert.Status)
	if !alert.CreatedAt.IsZero() {
		relTime := formatRelativeTime(alert.CreatedAt)
		b.WriteString(fmt.Sprintf("  Triggered %s", relTime))
	}
	b.WriteString("\n\n")

	// Links
	rootlyURL := alert.URL
	if rootlyURL == "" && alert.ShortID != "" {
		rootlyURL = fmt.Sprintf("https://rootly.com/account/alerts/%s", alert.ShortID)
	}
	if rootlyURL != "" || alert.ExternalURL != "" {
		b.WriteString("Links\n")
		if rootlyURL != "" {
			b.WriteString("  Rootly: " + rootlyURL + "\n")
		}
		if alert.ExternalURL != "" {
			b.WriteString("  Source: " + alert.ExternalURL + "\n")
		}
		b.WriteString("\n")
	}

	// Description
	if alert.Description != "" {
		b.WriteString("Description\n")
		b.WriteString(alert.Description)
		b.WriteString("\n\n")
	}

	// Timeline
	b.WriteString("Timeline\n")
	if !alert.CreatedAt.IsZero() {
		b.WriteString("  Created: " + formatAlertTime(alert.CreatedAt) + "\n")
	}
	if alert.StartedAt != nil {
		b.WriteString("  Started: " + formatAlertTime(*alert.StartedAt) + "\n")
	}
	if alert.EndedAt != nil {
		b.WriteString("  Ended: " + formatAlertTime(*alert.EndedAt) + "\n")
	}
	b.WriteString("\n")

	// Services, Environments, Teams
	if len(alert.Services) > 0 {
		b.WriteString("Services: " + strings.Join(alert.Services, ", ") + "\n")
	}
	if len(alert.Environments) > 0 {
		b.WriteString("Environments: " + strings.Join(alert.Environments, ", ") + "\n")
	}
	if len(alert.Groups) > 0 {
		b.WriteString("Teams: " + strings.Join(alert.Groups, ", ") + "\n")
	}

	// Extended info
	if alert.DetailLoaded {
		if len(alert.Responders) > 0 {
			b.WriteString("\nResponders: " + strings.Join(alert.Responders, ", ") + "\n")
		}

		if len(alert.Labels) > 0 {
			b.WriteString("\nLabels\n")
			keys := make([]string, 0, len(alert.Labels))
			for k := range alert.Labels {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				b.WriteString("  " + k + ": " + alert.Labels[k] + "\n")
			}
		}

		if len(alert.Data) > 0 {
			b.WriteString("\nData\n")
			dataJSON, err := json.MarshalIndent(alert.Data, "", "  ")
			if err == nil {
				b.WriteString(string(dataJSON))
				b.WriteString("\n")
			}
		}
	}

	return b.String()
}
