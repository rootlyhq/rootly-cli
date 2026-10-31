package agent

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
)

func bulletizeWideTables(markdown string, width int) string {
	if width <= 0 || !strings.Contains(markdown, "|") {
		return markdown
	}
	lines := strings.Split(markdown, "\n")
	output := make([]string, 0, len(lines))
	fence := ""
	for index := 0; index < len(lines); index++ {
		line := lines[index]
		if fence != "" {
			output = append(output, line)
			if closesFence(line, fence) {
				fence = ""
			}
			continue
		}
		if marker := markdownFenceMarker(line); marker != "" {
			fence = marker
			output = append(output, line)
			continue
		}
		if index+1 < len(lines) && isTableRow(line) && isTableSeparator(lines[index+1]) {
			end := index + 2
			for end < len(lines) && isTableRow(lines[end]) {
				end++
			}
			table := lines[index:end]
			if len(table) > 2 && markdownTableWidth(table) > width {
				output = append(output, tableAsBullets(table)...)
			} else {
				output = append(output, table...)
			}
			index = end - 1
			continue
		}
		output = append(output, line)
	}
	return strings.Join(output, "\n")
}

func markdownFenceMarker(line string) string {
	trimmed := strings.TrimSpace(line)
	for _, char := range []string{"`", "~"} {
		run := len(trimmed) - len(strings.TrimLeft(trimmed, char))
		if run >= 3 {
			return trimmed[:run]
		}
	}
	return ""
}

func closesFence(line, fence string) bool {
	trimmed := strings.TrimSpace(line)
	return strings.HasPrefix(trimmed, fence) && strings.Trim(trimmed, fence[:1]) == ""
}

func isTableRow(line string) bool {
	trimmed := strings.TrimSpace(line)
	return strings.Count(trimmed, "|") >= 1 && strings.Contains(trimmed, "|") && trimmed != ""
}

func isTableSeparator(line string) bool {
	cells := tableCells(line)
	if len(cells) == 0 {
		return false
	}
	for _, cell := range cells {
		cell = strings.Trim(cell, ":")
		if cell == "" || strings.Trim(cell, "-") != "" {
			return false
		}
	}
	return true
}

func tableCells(line string) []string {
	trimmed := strings.TrimSpace(line)
	trimmed = strings.TrimPrefix(trimmed, "|")
	if !strings.HasSuffix(trimmed, `\|`) {
		trimmed = strings.TrimSuffix(trimmed, "|")
	}
	var cells []string
	var cell strings.Builder
	for index := 0; index < len(trimmed); index++ {
		switch {
		case trimmed[index] == '\\' && index+1 < len(trimmed) && trimmed[index+1] == '|':
			cell.WriteByte('|')
			index++
		case trimmed[index] == '|':
			cells = append(cells, strings.TrimSpace(cell.String()))
			cell.Reset()
		default:
			cell.WriteByte(trimmed[index])
		}
	}
	return append(cells, strings.TrimSpace(cell.String()))
}

func markdownTableWidth(table []string) int {
	widths := []int{}
	for index, line := range table {
		if index == 1 {
			continue
		}
		for column, cell := range tableCells(line) {
			if column >= len(widths) {
				widths = append(widths, 0)
			}
			widths[column] = max(widths[column], ansi.StringWidth(cell))
		}
	}
	total := 1
	for _, width := range widths {
		total += width + 3
	}
	return total
}

func tableAsBullets(table []string) []string {
	headers := tableCells(table[0])
	bullets := make([]string, 0, len(table)-2)
	for _, line := range table[2:] {
		cells := tableCells(line)
		parts := make([]string, 0, len(cells))
		for column, cell := range cells {
			if cell == "" {
				continue
			}
			if column == 0 {
				parts = append(parts, "**"+cell+"**")
				continue
			}
			if column < len(headers) && headers[column] != "" {
				cell = headers[column] + ": " + cell
			}
			parts = append(parts, cell)
		}
		if len(parts) > 0 {
			bullets = append(bullets, "- "+strings.Join(parts, " · "))
		}
	}
	return bullets
}
