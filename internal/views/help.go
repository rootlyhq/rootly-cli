package views

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/rootlyhq/rootly-tui/internal/i18n"
	"github.com/rootlyhq/rootly-tui/internal/styles"
)

type HelpModel struct {
	Visible bool
}

func NewHelpModel() HelpModel {
	return HelpModel{Visible: false}
}

func (m *HelpModel) Toggle() {
	m.Visible = !m.Visible
}

func (m *HelpModel) Show() {
	m.Visible = true
}

func (m *HelpModel) Hide() {
	m.Visible = false
}

func (m HelpModel) View() string {
	var b strings.Builder

	b.WriteString(styles.DialogTitle.Render(i18n.T("help.title")))
	b.WriteString("\n\n")

	// Navigation section
	b.WriteString(styles.TextBold.Render(i18n.T("help.section.navigation")))
	b.WriteString("\n")
	b.WriteString(renderHelpLine("j / Down", i18n.T("help.nav.move_down")))
	b.WriteString(renderHelpLine("k / Up", i18n.T("help.nav.move_up")))
	b.WriteString(renderHelpLine("g", i18n.T("help.nav.first")))
	b.WriteString(renderHelpLine("G", i18n.T("help.nav.last")))
	b.WriteString(renderHelpLine("[", i18n.T("help.nav.prev_page")))
	b.WriteString(renderHelpLine("]", i18n.T("help.nav.next_page")))
	b.WriteString(renderHelpLine("Tab", i18n.T("help.nav.switch_tabs")))
	b.WriteString("\n")

	// Actions section
	b.WriteString(styles.TextBold.Render(i18n.T("help.section.actions")))
	b.WriteString("\n")
	b.WriteString(renderHelpLine("r", i18n.T("help.action.refresh")))
	b.WriteString(renderHelpLine("Enter", i18n.T("help.action.details")))
	b.WriteString(renderHelpLine("o", i18n.T("help.action.open_url")))
	b.WriteString(renderHelpLine("y", i18n.T("help.action.copy")))
	b.WriteString("\n")

	// Sorting section
	b.WriteString(styles.TextBold.Render(i18n.T("sorting.title")))
	b.WriteString("\n")
	b.WriteString(renderHelpLine("S", i18n.T("sorting.open_sort_menu")))
	b.WriteString(styles.TextDim.Render("    Press same key again to toggle newest ⇄ oldest"))
	b.WriteString("\n\n")

	// General section
	b.WriteString(styles.TextBold.Render(i18n.T("help.section.general")))
	b.WriteString("\n")
	b.WriteString(renderHelpLine("l", i18n.T("help.action.logs")))
	b.WriteString(renderHelpLine("s", i18n.T("help.action.setup")))
	b.WriteString(renderHelpLine("A", i18n.T("help.action.about")))
	b.WriteString(renderHelpLine("?", i18n.T("help.action.help")))
	b.WriteString(renderHelpLine("q / Ctrl+C", i18n.T("help.action.quit")))
	b.WriteString("\n\n")

	b.WriteString(styles.TextDim.Render(i18n.T("help.press_to_close")))

	return styles.Dialog.Render(b.String())
}

func renderHelpLine(key, desc string) string {
	keyStyle := styles.HelpKey.Width(12)
	return keyStyle.Render(key) + " " + styles.HelpDesc.Render(desc) + "\n"
}

// RenderHelpBar renders the bottom help bar
// hasSelection indicates whether an incident or alert is currently selected
// isLoading indicates whether data is currently being loaded
// isIncidentsTab indicates whether we're on the incidents tab (for sorting hints)
// currentPage, totalPages, totalCount are for pagination display
func RenderHelpBar(width int, hasSelection, isLoading, isIncidentsTab bool, currentPage, totalPages, totalCount int) string {
	items := []string{
		styles.RenderHelpItem("j/k", i18n.T("helpbar.navigate")),
		styles.RenderHelpItem("[/]", i18n.T("helpbar.page")),
		styles.RenderHelpItem("Tab", i18n.T("helpbar.switch")),
	}
	if !isLoading {
		items = append(items, styles.RenderHelpItem("r", i18n.T("helpbar.refresh")))
		if hasSelection {
			items = append(items,
				styles.RenderHelpItem("o", i18n.T("helpbar.open")),
				styles.RenderHelpItem("y", i18n.T("helpbar.copy")),
			)
		}
	}
	// Show sorting hint only on incidents tab
	if isIncidentsTab {
		items = append(items, styles.RenderHelpItem("S", i18n.T("sorting.sort_by_date")))
	}
	items = append(items,
		styles.RenderHelpItem("l", i18n.T("helpbar.logs")),
		styles.RenderHelpItem("s", i18n.T("helpbar.setup")),
		styles.RenderHelpItem("A", i18n.T("helpbar.about")),
		styles.RenderHelpItem("?", i18n.T("helpbar.help")),
		styles.RenderHelpItem("q", i18n.T("helpbar.quit")),
	)

	helpText := strings.Join(items, "  ")

	// Add pagination info on the right side
	var pageInfo string
	if totalPages > 0 && totalCount > 0 {
		pageInfo = fmt.Sprintf("Page %d/%d (%d total)", currentPage, totalPages, totalCount)
	} else if currentPage > 0 {
		pageInfo = fmt.Sprintf("Page %d", currentPage)
	}
	if pageInfo != "" {
		pageInfoStyled := styles.Muted.Render(pageInfo)
		// Calculate spacing to right-align pagination info
		helpLen := lipgloss.Width(helpText)
		pageInfoLen := lipgloss.Width(pageInfoStyled)
		availableWidth := width - 4 // Account for padding
		if helpLen+pageInfoLen+2 < availableWidth {
			spacing := availableWidth - helpLen - pageInfoLen
			helpText = helpText + strings.Repeat(" ", spacing) + pageInfoStyled
		}
	}

	return styles.HelpBar.Width(width).Render(helpText)
}
