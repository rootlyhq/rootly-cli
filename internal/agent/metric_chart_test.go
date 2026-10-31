package agent

import (
	"bytes"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/google/uuid"
)

const metricPieSource = "```metric-chart\n" +
	`{"type":"pie","label":"Incident count","unit":"count","dimension":"Severity","labels":["SEV2","SEV3","SEV1"],"values":[13,11,1],"displays":["13","11","1"],"options":[{"color":"#7C5CE6"}]}` +
	"\n```"

func TestPrepareAssistantMarkdownRendersPieChart(t *testing.T) {
	for _, noColor := range []bool{false, true} {
		prepared, charts := prepareAssistantMarkdown(metricPieSource, markdownOpts{width: 100, noColor: noColor})
		got := expandAssistantMarkdownCharts(prepared, charts)
		for _, text := range []string{"SEV2", "13 · 52%", "SEV3", "11 · 44%", "SEV1", "1 · 4%"} {
			if !strings.Contains(ansi.Strip(got), text) {
				t.Errorf("pie output is missing %q: %q", text, got)
			}
		}
		if strings.Contains(got, `{"type"`) {
			t.Errorf("pie output contains raw JSON: %q", got)
		}
		for _, line := range strings.Split(got, "\n") {
			if width := ansi.StringWidth(line); width > 100 {
				t.Errorf("pie line width = %d, want at most 100: %q", width, line)
			}
		}
		if noColor && strings.Contains(got, "\x1b") {
			t.Errorf("no-color pie output contains ANSI: %q", got)
		}
	}
}

func TestPrepareAssistantMarkdownRendersLineChart(t *testing.T) {
	source := "```metric-chart\n" +
		`{"type":"line","label":"Incidents","unit":"count","labels":["2026-01-01","2026-01-02","2026-01-03","2026-01-04","2026-01-05"],"values":[2,5,3,7,4],"displays":["2","5","3","7","4"]}` +
		"\n```"
	prepared, charts := prepareAssistantMarkdown(source, markdownOpts{width: 100, noColor: true})
	got := expandAssistantMarkdownCharts(prepared, charts)
	if !strings.Contains(got, "╭") {
		t.Errorf("line output does not contain asciigraph axes: %q", got)
	}
	if !strings.Contains(got, "2026-01-01 … 2026-01-05") {
		t.Errorf("line output is missing its date caption: %q", got)
	}
}

func TestPrepareAssistantMarkdownLeavesInvalidMetricFence(t *testing.T) {
	source := "```metric-chart\n{not json}\n```"
	prepared, charts := prepareAssistantMarkdown(source, markdownOpts{width: 100})
	if prepared != source || len(charts) != 0 {
		t.Errorf("invalid metric fence = %q, %d charts; want original code fence", prepared, len(charts))
	}
}

func TestPrepareAssistantMarkdownLeavesIndentedMetricFenceLiteral(t *testing.T) {
	source := "    ```metric-chart\n" +
		"    {\"type\":\"pie\",\"labels\":[\"SEV2\"],\"values\":[13]}\n" +
		"    ```"
	prepared, charts := prepareAssistantMarkdown(source, markdownOpts{width: 100})
	if len(charts) != 0 || !strings.Contains(prepared, `{"type"`) {
		t.Errorf("indented fence was converted to a chart: %q, %d charts", prepared, len(charts))
	}
}

func TestPrepareAssistantMarkdownRendersStepAsVerticalTransitions(t *testing.T) {
	source := "```metric-chart\n" +
		`{"type":"step","label":"Incidents","unit":"count","labels":["a","b","c","d"],"values":[1,1,5,5]}` +
		"\n```"
	prepared, charts := prepareAssistantMarkdown(source, markdownOpts{width: 100, noColor: true})
	got := expandAssistantMarkdownCharts(prepared, charts)
	cornerColumns := make(map[int]struct{})
	hasHorizontalRun := false
	for _, line := range strings.Split(got, "\n") {
		if strings.Contains(line, "──") {
			hasHorizontalRun = true
		}
		for column, character := range []rune(line) {
			if strings.ContainsRune("╭╮╰╯", character) {
				cornerColumns[column] = struct{}{}
			}
		}
	}
	if !hasHorizontalRun {
		t.Errorf("step output has no horizontal plateau: %q", got)
	}
	if len(cornerColumns) != 1 {
		t.Errorf("step transitions span %d columns, want one vertical jump: %q", len(cornerColumns), got)
	}
}

func TestPrepareAssistantMarkdownUsesNumericDisplayFallback(t *testing.T) {
	source := "```metric-chart\n" +
		`{"type":"pie","labels":["SEV2","SEV3","SEV1"],"values":[13,11,1]}` +
		"\n```"
	prepared, charts := prepareAssistantMarkdown(source, markdownOpts{width: 100, noColor: true})
	got := expandAssistantMarkdownCharts(prepared, charts)
	for _, want := range []string{"13 · 52%", "11 · 44%", "1 · 4%"} {
		if !strings.Contains(got, want) {
			t.Errorf("output = %q, want numeric fallback %q", got, want)
		}
	}
}

func TestPrepareAssistantMarkdownLineChartLeavesNullGap(t *testing.T) {
	source := "```metric-chart\n" +
		`{"type":"line","labels":["a","b","c"],"values":[1,null,3]}` +
		"\n```"
	prepared, charts := prepareAssistantMarkdown(source, markdownOpts{width: 100, noColor: true})
	got := expandAssistantMarkdownCharts(prepared, charts)
	if !strings.Contains(got, "╴") || !strings.Contains(got, "╶") {
		t.Errorf("line output has no separated null-gap endpoints: %q", got)
	}
}

func TestRewriteDashboardLinks(t *testing.T) {
	for _, test := range []struct {
		name      string
		href      string
		sessionID string
		want      string
	}{
		{
			name:      "with session",
			href:      "#AI-add-to-dashboard%3aref",
			sessionID: testSessionID,
			want:      "https://rootly.com/account?chat=" + testSessionID,
		},
		{
			name: "without session",
			href: "#ai-add-to-dashboard:ref",
			want: "https://rootly.com/account",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			source := "[Add to dashboard](<" + test.href + ">)"
			prepared, charts := prepareAssistantMarkdown(source, markdownOpts{
				width: 100, noColor: true, webBase: "https://rootly.com", sessionID: test.sessionID,
			})
			got := expandAssistantMarkdownCharts(prepared, charts)
			if !strings.Contains(got, "[Add to dashboard: open in web]("+test.want+")") {
				t.Errorf("rewritten link = %q, want destination %q", got, test.want)
			}
			if strings.Contains(strings.ToLower(got), "ai-add-to-dashboard") {
				t.Errorf("dashboard marker remains in output: %q", got)
			}
		})
	}
}

func TestLiveMetricFenceShowsDrawingPlaceholder(t *testing.T) {
	model := NewTUIModel(t.Context(), &fakeStreamer{}, TUIOptions{NoColor: true})
	model.state = tuiStreaming
	model.answer.WriteString("```metric-chart\n{\"type\":\"pie\",\"values\":[")

	got := ansi.Strip(model.renderLiveAssistantAnswerPart(model.answer.String(), true))
	if !strings.Contains(got, "◌ drawing chart…") {
		t.Errorf("live output = %q, want drawing placeholder", got)
	}
	if strings.Contains(got, `"type"`) || strings.Contains(got, `"values"`) {
		t.Errorf("live output exposed partial metric JSON: %q", got)
	}
}

func TestMetricChartsReflowOnTranscriptResize(t *testing.T) {
	for _, width := range []int{60, 120} {
		t.Run(strconv.Itoa(width), func(t *testing.T) {
			model := NewTUIModel(t.Context(), &fakeStreamer{}, TUIOptions{NoColor: true})
			model.width = width
			model.appendAssistantTranscriptBlock(metricPieSource, true, false)
			model.rebuildTranscriptCache()
			if !strings.Contains(model.transcriptCache, "13 · 52%") {
				t.Fatalf("width %d transcript does not contain rendered chart: %q", width, model.transcriptCache)
			}
			for _, line := range strings.Split(ansi.Strip(model.transcriptCache), "\n") {
				if lineWidth := ansi.StringWidth(line); lineWidth > width-1 {
					t.Errorf("width %d transcript line width = %d: %q", width, lineWidth, line)
				}
			}
		})
	}
}

func TestWriteAnswerPlainRendersChartsAndDashboardLinks(t *testing.T) {
	sessionID, err := uuid.Parse(testSessionID)
	if err != nil {
		t.Fatal(err)
	}
	markdown := metricPieSource + "\n\n[Add to dashboard](#ai-add-to-dashboard:ref)"
	var stdout bytes.Buffer
	err = writeAnswer(PrintOptions{
		Stdout:      &stdout,
		APIHost:     "api.rootly.com",
		SessionID:   &sessionID,
		IsStdoutTTY: func() bool { return false },
	}, markdown)
	if err != nil {
		t.Fatal(err)
	}
	got := stdout.String()
	if strings.Contains(got, `{"type"`) || strings.Contains(got, "```metric-chart") {
		t.Errorf("plain output contains the raw chart fence: %q", got)
	}
	if !strings.Contains(got, "[Add to dashboard: open in web](https://rootly.com/account?chat="+testSessionID+")") {
		t.Errorf("plain output did not rewrite dashboard link: %q", got)
	}
	if strings.Contains(got, "\x1b") {
		t.Errorf("plain output contains ANSI: %q", got)
	}

	var terminalOutput bytes.Buffer
	err = writeAnswer(PrintOptions{
		Stdout:      &terminalOutput,
		APIHost:     "api.rootly.com",
		SessionID:   &sessionID,
		IsStdoutTTY: func() bool { return true },
	}, markdown)
	if err != nil {
		t.Fatal(err)
	}
	terminalText := ansi.Strip(terminalOutput.String())
	if strings.Contains(terminalText, `{"type"`) || strings.Contains(terminalText, "```metric-chart") ||
		strings.Contains(strings.ToLower(terminalText), "ai-add-to-dashboard") {
		t.Errorf("terminal output contains an unrendered chart or dashboard marker: %q", terminalText)
	}
	if !strings.Contains(terminalText, "Add to dashboard: open in web") ||
		!strings.Contains(terminalOutput.String(), "https://rootly.com/account?chat="+testSessionID) {
		t.Errorf("terminal output did not render dashboard link: %q", terminalText)
	}
}

func TestExpandStepSeriesKeepsEveryMeasurement(t *testing.T) {
	for _, tc := range []struct{ count, width int }{{19, 20}, {7, 60}, {3, 60}, {20, 20}, {90, 20}} {
		values := make([]float64, tc.count)
		for index := range values {
			values[index] = float64(index)
		}
		expanded := expandStepSeries(values, tc.width)
		if len(expanded) != tc.width {
			t.Fatalf("%d values at width %d: got %d columns", tc.count, tc.width, len(expanded))
		}
		if expanded[0] != 0 || expanded[len(expanded)-1] != float64(tc.count-1) {
			t.Errorf("%d values at width %d: first/last = %v/%v", tc.count, tc.width, expanded[0], expanded[len(expanded)-1])
		}
		if tc.count <= tc.width {
			seen := map[float64]bool{}
			for _, value := range expanded {
				seen[value] = true
			}
			if len(seen) != tc.count {
				t.Errorf("%d values at width %d: %d distinct measurements plotted", tc.count, tc.width, len(seen))
			}
		}
	}
}

func TestLineChartKeepsPeakAtEveryWidth(t *testing.T) {
	values := make([]string, 31)
	for index := range values {
		values[index] = "0"
	}
	for _, index := range []int{3, 11, 19, 27} {
		values[index] = "1"
	}
	labels := make([]string, len(values))
	for index := range labels {
		labels[index] = fmt.Sprintf("%q", fmt.Sprintf("Oct %d", index+1))
	}
	source := fmt.Sprintf(`{"type":"line","labels":[%s],"values":[%s]}`, strings.Join(labels, ","), strings.Join(values, ","))
	for _, width := range []int{139, 110, 70, 40} {
		lines, ok := renderMetricChart(source, markdownOpts{width: width, noColor: true})
		if !ok {
			t.Fatalf("width %d: chart not rendered", width)
		}
		graph := strings.Join(lines, "\n")
		if !strings.Contains(graph, "1.00") || strings.Contains(graph, "0.90") || strings.Contains(graph, "0.79") {
			t.Errorf("width %d: peak not preserved:\n%s", width, graph)
		}
	}
}

func TestFitLineSeriesBucketsLongSeriesByPeak(t *testing.T) {
	values := make([]float64, 90)
	values[44] = 7
	series, width := fitLineSeries(values, 30)
	if width != 30 || len(series) != 30 {
		t.Fatalf("got %d values at width %d, want 30", len(series), width)
	}
	if peak := slices.Max(series); peak != 7 {
		t.Errorf("peak = %v, want 7", peak)
	}
}
