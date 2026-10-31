package agent

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/guptarohit/asciigraph"
)

const metricChartColor = "#7C5CE6"

type markdownOpts struct {
	width     int
	indent    int
	noColor   bool
	streaming bool
	webBase   string
	sessionID string
}

type markdownChart struct {
	token  string
	indent string
	lines  []string
}

type metricChartSpec struct {
	Type     string     `json:"type"`
	Labels   []string   `json:"labels"`
	Values   []*float64 `json:"values"`
	Displays []string   `json:"displays"`
}

func prepareAssistantMarkdown(markdown string, opts markdownOpts) (string, []markdownChart) {
	markdown = sanitizeTerminalText(markdown)
	if opts.width <= 0 {
		opts.width = 80
	}
	markdown = rewriteDashboardLinks(markdown, opts.webBase, opts.sessionID)

	lines := strings.Split(markdown, "\n")
	prepared := make([]string, 0, len(lines))
	charts := make([]markdownChart, 0)
	usedTokens := make(map[string]struct{})
	var closingFence string

	for index := 0; index < len(lines); index++ {
		line := lines[index]
		if closingFence != "" {
			prepared = append(prepared, line)
			if isClosingMarkdownFence(line, closingFence) {
				closingFence = ""
			}
			continue
		}

		if isMetricChartFence(line) {
			end := findMetricChartFenceEnd(lines, index+1)
			if end < 0 {
				if opts.streaming {
					token := nextMetricChartToken(markdown, usedTokens, len(charts))
					charts = append(charts, markdownChart{
						token:  token,
						indent: markdownLineIndent(line),
						lines:  []string{drawingChartLine(opts.noColor)},
					})
					prepared = append(prepared, markdownLineIndent(line)+token)
					break
				}
				prepared = append(prepared, lines[index:]...)
				break
			}

			chart, valid := renderMetricChart(strings.Join(lines[index+1:end], "\n"), opts)
			if !valid {
				prepared = append(prepared, lines[index:end+1]...)
			} else {
				token := nextMetricChartToken(markdown, usedTokens, len(charts))
				indent := markdownLineIndent(line)
				charts = append(charts, markdownChart{token: token, indent: indent, lines: chart})
				prepared = append(prepared, indent+token)
			}
			index = end
			continue
		}

		prepared = append(prepared, line)
		if fence, ok := openingMarkdownFence(line); ok {
			closingFence = fence
		}
	}

	return strings.Join(prepared, "\n"), charts
}

func expandAssistantMarkdownCharts(markdown string, charts []markdownChart) string {
	if len(charts) == 0 {
		return markdown
	}

	byToken := make(map[string]markdownChart, len(charts))
	for _, chart := range charts {
		byToken[chart.token] = chart
	}
	lines := strings.Split(markdown, "\n")
	expanded := make([]string, 0, len(lines))
	for _, line := range lines {
		plain := strings.TrimSpace(ansi.Strip(line))
		if chart, ok := byToken[plain]; ok {
			indent := chart.indent
			stripped := ansi.Strip(line)
			if rendered := stripped[:len(stripped)-len(strings.TrimLeft(stripped, " "))]; len(rendered) > len(indent) {
				indent = rendered
			}
			for _, chartLine := range chart.lines {
				expanded = append(expanded, indent+chartLine)
			}
			continue
		}
		expanded = append(expanded, line)
	}
	return strings.Join(expanded, "\n")
}

func renderMetricChart(source string, opts markdownOpts) ([]string, bool) {
	var spec metricChartSpec
	if err := json.Unmarshal([]byte(source), &spec); err != nil ||
		len(spec.Values) == 0 || len(spec.Labels) != len(spec.Values) {
		return nil, false
	}
	for _, value := range spec.Values {
		if value != nil && (math.IsNaN(*value) || math.IsInf(*value, 0)) {
			return nil, false
		}
	}

	switch spec.Type {
	case "pie":
		return renderMetricBars(spec, opts, true), true
	case "bar":
		return renderMetricBars(spec, opts, false), true
	case "line", "step":
		if metricValueCount(spec.Values) < 2 {
			return renderMetricBars(spec, opts, false), true
		}
		width := clamp(opts.width-12, 20, 60)
		values := metricSeries(spec, spec.Type == "step")
		var graph string
		if spec.Type == "step" {
			graph = asciigraph.Plot(expandStepSeries(values, width), asciigraph.Height(8))
		} else {
			graph = asciigraph.Plot(values, asciigraph.Height(8), asciigraph.Width(width))
		}
		if !opts.noColor {
			graph = lipgloss.NewStyle().Foreground(lipgloss.Color(metricChartColor)).Render(graph)
		}
		caption := metricChartCaption(spec.Labels, opts.width-opts.indent, opts.noColor)
		return append(strings.Split(graph, "\n"), caption), true
	default:
		return nil, false
	}
}

func renderMetricBars(spec metricChartSpec, opts markdownOpts, pie bool) []string {
	labelWidth := 0
	for _, label := range spec.Labels {
		labelWidth = max(labelWidth, ansi.StringWidth(sanitizeTerminalText(label)))
	}
	labelWidth = min(labelWidth, 20)

	sum, maximum := 0.0, 0.0
	for _, value := range spec.Values {
		if value == nil {
			continue
		}
		sum += *value
		maximum = max(maximum, *value)
	}

	suffixes := make([]string, len(spec.Values))
	maxSuffixWidth := 0
	for index, value := range spec.Values {
		if value == nil {
			suffixes[index] = "–"
		} else {
			suffix := metricDisplay(spec.Displays, index, *value)
			if pie && sum > 0 {
				suffix += fmt.Sprintf(" · %d%%", int(math.Round(*value/sum*100)))
			}
			suffixes[index] = suffix
		}
		maxSuffixWidth = max(maxSuffixWidth, ansi.StringWidth(suffixes[index]))
	}

	labelColumn := labelWidth + 1
	suffixWidth := maxSuffixWidth + 1
	availableWidth := opts.width - labelColumn - suffixWidth - opts.indent
	barWidth := clamp(availableWidth, 10, 40)

	lines := make([]string, len(spec.Values))
	for index, value := range spec.Values {
		label := ansi.Truncate(sanitizeTerminalText(spec.Labels[index]), labelWidth, "…")
		label = ansi.Truncate(label, labelWidth, "")
		label += strings.Repeat(" ", max(0, labelWidth-ansi.StringWidth(label)))
		bar := ""
		if value != nil && *value > 0 && maximum > 0 {
			bar = metricBar(*value/maximum, barWidth)
			if !opts.noColor {
				bar = lipgloss.NewStyle().Foreground(lipgloss.Color(metricChartColor)).Render(bar)
			}
		}
		line := label + " " + bar + " " + suffixes[index]
		lines[index] = strings.TrimRight(line, " ")
	}
	return lines
}

func metricSeries(spec metricChartSpec, carryNulls bool) []float64 {
	values := make([]float64, 0, len(spec.Values))
	var previous float64
	hasPrevious := false
	for _, value := range spec.Values {
		if value == nil {
			if carryNulls && hasPrevious {
				values = append(values, previous)
			} else if !carryNulls {
				values = append(values, math.NaN())
			}
			continue
		}
		previous = *value
		hasPrevious = true
		values = append(values, previous)
	}
	return values
}

func metricValueCount(values []*float64) int {
	count := 0
	for _, value := range values {
		if value != nil {
			count++
		}
	}
	return count
}

func expandStepSeries(values []float64, width int) []float64 {
	if len(values) == 0 {
		return nil
	}
	width = max(1, width)
	expanded := make([]float64, width)
	for index := range expanded {
		position := index * len(values) / width
		if len(values) > width && width > 1 {
			position = int(math.Round(float64(index) * float64(len(values)-1) / float64(width-1)))
		}
		expanded[index] = values[position]
	}
	return expanded
}

func metricChartCaption(labels []string, width int, noColor bool) string {
	if len(labels) == 0 {
		return ""
	}
	caption := sanitizeTerminalText(labels[0]) + " … " + sanitizeTerminalText(labels[len(labels)-1])
	caption = ansi.Truncate(caption, max(1, width), "…")
	if noColor {
		return caption
	}
	return lipgloss.NewStyle().Faint(true).Render(caption)
}

func metricBar(ratio float64, width int) string {
	if ratio <= 0 || width <= 0 {
		return ""
	}
	fractions := []string{"▏", "▎", "▍", "▌", "▋", "▊", "▉"}
	eighths := int(math.Round(min(1, ratio) * float64(width*8)))
	if eighths == 0 {
		eighths = 1
	}
	full := eighths / 8
	remainder := eighths % 8
	bar := strings.Repeat("█", full)
	if remainder > 0 {
		bar += fractions[remainder-1]
	}
	return bar
}

func metricDisplay(displays []string, index int, value float64) string {
	if index >= len(displays) || strings.TrimSpace(displays[index]) == "" {
		return strconv.FormatFloat(value, 'f', -1, 64)
	}
	return sanitizeTerminalText(displays[index])
}

func drawingChartLine(noColor bool) string {
	line := "◌ drawing chart…"
	if noColor {
		return line
	}
	return lipgloss.NewStyle().Faint(true).Render(line)
}

func findMetricChartFenceEnd(lines []string, start int) int {
	for index := start; index < len(lines); index++ {
		if strings.TrimSpace(lines[index]) == "```" {
			return index
		}
	}
	return -1
}

func isMetricChartFence(line string) bool {
	if _, ok := openingMarkdownFence(line); !ok {
		return false
	}
	return strings.TrimSpace(strings.TrimLeft(line, " ")) == "```metric-chart"
}

func openingMarkdownFence(line string) (string, bool) {
	trimmed := strings.TrimLeft(line, " ")
	if len(line)-len(trimmed) > 3 || len(trimmed) < 3 {
		return "", false
	}
	character := trimmed[0]
	if character != '`' && character != '~' {
		return "", false
	}
	count := 0
	for count < len(trimmed) && trimmed[count] == character {
		count++
	}
	if count < 3 {
		return "", false
	}
	return strings.Repeat(string(character), count), true
}

func markdownLineIndent(line string) string {
	trimmed := strings.TrimLeft(line, " \t")
	return line[:len(line)-len(trimmed)]
}

func isClosingMarkdownFence(line, opening string) bool {
	trimmed := strings.TrimSpace(line)
	if len(trimmed) < len(opening) || !strings.HasPrefix(trimmed, opening) {
		return false
	}
	return strings.TrimSpace(strings.TrimPrefix(trimmed, opening)) == ""
}

func nextMetricChartToken(markdown string, used map[string]struct{}, index int) string {
	for {
		token := fmt.Sprintf("RLYCHART%d", index)
		if !strings.Contains(markdown, token) {
			if _, exists := used[token]; !exists {
				used[token] = struct{}{}
				return token
			}
		}
		index++
	}
}

func clamp(value, minimum, maximum int) int {
	return min(max(value, minimum), maximum)
}
