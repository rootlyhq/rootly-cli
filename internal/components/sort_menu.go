package components

import (
	"fmt"
	"strings"

	"github.com/rootlyhq/rootly-tui/internal/i18n"
	"github.com/rootlyhq/rootly-tui/internal/styles"
)

// SortMenuModel provides a reusable sort menu overlay that can be used by any view.
// Each view defines its own sort options and handles the selected field value.

type SortMenuModel struct {
	visible bool
	cursor  int
	options []SortOption
}

type SortOption struct {
	Label       string
	Description string
	Value       interface{}
}

func NewSortMenu(options []SortOption) *SortMenuModel {
	return &SortMenuModel{
		visible: false,
		cursor:  0,
		options: options,
	}
}

func (m *SortMenuModel) Toggle() {
	m.visible = !m.visible
	if m.visible {
		m.cursor = 0
	}
}

func (m *SortMenuModel) IsVisible() bool {
	return m.visible
}

func (m *SortMenuModel) Close() {
	m.visible = false
}

func (m *SortMenuModel) HandleKey(key string) (selected interface{}, shouldApply bool) {
	switch key {
	case "j", "down":
		if m.cursor < len(m.options)-1 {
			m.cursor++
		}
	case "k", "up":
		if m.cursor > 0 {
			m.cursor--
		}
	case "enter":
		selected = m.options[m.cursor].Value
		m.visible = false
		return selected, true
	case "esc", "q":
		m.visible = false
		return nil, false
	}
	return nil, false
}

func (m *SortMenuModel) Render(currentSortField interface{}, sortDirection SortDirection) string {
	if !m.visible {
		return ""
	}

	var b strings.Builder
	b.WriteString(styles.DialogTitle.Render(i18n.T("sort_by")))
	b.WriteString("\n\n")

	for i, opt := range m.options {
		cursor := "  "
		if i == m.cursor {
			cursor = "▶ "
		}

		sortIndicator := ""
		if currentSortField == opt.Value {
			if sortDirection == SortDesc {
				sortIndicator = " (" + i18n.T("sorting.newest_first") + ")"
			} else {
				sortIndicator = " (" + i18n.T("sorting.oldest_first") + ")"
			}
		}

		line := fmt.Sprintf("%s%s%s", cursor, opt.Label, sortIndicator)
		if i == m.cursor {
			b.WriteString(styles.Primary.Render(line))
		} else {
			b.WriteString(styles.Text.Render(line))
		}
		b.WriteString("\n")

		if i == m.cursor {
			b.WriteString(styles.TextDim.Render("  " + opt.Description))
			b.WriteString("\n")
		}
	}

	b.WriteString("\n")
	b.WriteString(styles.TextDim.Render(i18n.T("sort_menu_help")))

	return styles.Dialog.Render(b.String())
}
