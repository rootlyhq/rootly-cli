package agent

import (
	"bytes"
	"strings"
	"testing"

	"charm.land/glamour/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

func TestGlamourRendersLinksWithPrivateUseMarkers(t *testing.T) {
	const target = "http://localhost:3000/account/incidents/8-iam-credentials-has-been-leaked?tab=follow_ups&ai_slack_unfurl_resource=action_item:e052c503-dc3b-447e-8a5e-e4dd4d2a266a"
	markdown := "Created: [Rotate the keys](" + target + ") on INC-8.\n\n- [Review the action](https://rootly.com/docs/actions)"
	renderer, err := glamour.NewTermRenderer(glamour.WithEnvironmentConfig(), glamour.WithWordWrap(80))
	if err != nil {
		t.Fatal(err)
	}

	source, urls := preprocessMarkdownLinksWithURLs(markdown, "api.rootly.com")
	rendered, err := renderer.Render(source)
	if err != nil {
		t.Fatal(err)
	}
	output := restoreMarkdownLinks(rendered, urls)
	visible := ansi.Strip(output)
	if !strings.Contains(visible, "Rotate the keys") {
		t.Errorf("visible output = %q, want link label", visible)
	}
	for _, unwanted := range []string{"follow_ups", "ai_slack_unfurl", "\uE000", "\uE001"} {
		if strings.Contains(visible, unwanted) {
			t.Errorf("visible output = %q, want no %q", visible, unwanted)
		}
	}
	if got := strings.Count(output, ansi.SetHyperlink(target)); got != 1 {
		t.Errorf("hyperlink start count = %d, want exactly one", got)
	}
}

func TestRestoreMarkdownLinksStylesHyperlinkText(t *testing.T) {
	const destination = "https://rootly.com/account?chat=x"
	rendered := "\uE000\x1b[1mopen in web\x1b[0m\uE001"
	output := restoreMarkdownLinks(rendered, []markdownLink{{
		destination: destination,
		hyperlink:   true,
	}})

	start := ansi.SetHyperlink(destination)
	end := ansi.ResetHyperlink()
	linkStart := strings.Index(output, start)
	if linkStart < 0 {
		t.Fatalf("output = %q, want OSC-8 open sequence", output)
	}
	linkEnd := strings.Index(output[linkStart+len(start):], end)
	if linkEnd < 0 {
		t.Fatalf("output = %q, want OSC-8 close sequence", output)
	}
	styledText := output[linkStart+len(start) : linkStart+len(start)+linkEnd]
	if !strings.Contains(styledText, "\x1b[4") {
		t.Errorf("link text = %q, want underline SGR", styledText)
	}
	wantStyledText := lipgloss.NewStyle().
		Underline(true).
		Foreground(lipgloss.Color(metricChartColor)).
		Render("open in web")
	if !strings.Contains(styledText, wantStyledText) {
		t.Errorf("link text = %q, want foreground %s and underlined label", styledText, metricChartColor)
	}
	if got := strings.Count(ansi.Strip(output), "open in web"); got != 1 {
		t.Errorf("visible link label count = %d, want 1: %q", got, ansi.Strip(output))
	}
}

func TestRestoreMarkdownLinksDoesNotUnderlineWrappedIndentation(t *testing.T) {
	rendered := "\uE000first line\n    \x1b[1msecond line\x1b[0m\uE001"
	output := restoreMarkdownLinks(rendered, []markdownLink{{
		destination: "https://rootly.com/wrapped",
		hyperlink:   true,
	}})
	lines := strings.Split(output, "\n")
	if len(lines) != 2 {
		t.Fatalf("output lines = %d, want 2: %q", len(lines), output)
	}
	if !strings.HasPrefix(lines[1], "    ") {
		t.Fatalf("wrapped line = %q, want leading indentation before style", lines[1])
	}
	if !strings.HasPrefix(strings.TrimLeft(lines[1], " "), "\x1b[") {
		t.Errorf("wrapped line = %q, want style after its indentation", lines[1])
	}
	if got := ansi.Strip(output); got != "first line\n    second line" {
		t.Errorf("visible output = %q, want wrapped label with indentation", got)
	}
}

func TestGlamourMapsMultipleLinksToURLsInOrder(t *testing.T) {
	const firstURL = "https://rootly.com/incidents/first"
	const secondURL = "https://rootly.com/incidents/second"
	renderer, err := glamour.NewTermRenderer(glamour.WithEnvironmentConfig(), glamour.WithWordWrap(80))
	if err != nil {
		t.Fatal(err)
	}
	markdown := "[First](https://rootly.com/incidents/first) and [Second](https://rootly.com/incidents/second)."
	source, urls := preprocessMarkdownLinksWithURLs(markdown, "api.rootly.com")
	rendered, err := renderer.Render(source)
	if err != nil {
		t.Fatal(err)
	}
	output := restoreMarkdownLinks(rendered, urls)
	firstIndex := strings.Index(output, ansi.SetHyperlink(firstURL))
	secondIndex := strings.Index(output, ansi.SetHyperlink(secondURL))
	if firstIndex < 0 || secondIndex <= firstIndex ||
		strings.Count(output, ansi.SetHyperlink(firstURL)) != 1 ||
		strings.Count(output, ansi.SetHyperlink(secondURL)) != 1 {
		t.Errorf("hyperlinks are missing or reordered: %q", output)
	}
}

func TestGlamourDisclosesUntrustedLinkHostsAndTrustsRootlyDomain(t *testing.T) {
	markdown := "[Open INC-8](https://evil.example/x) and [Open Rootly](https://rootly.com/incidents/8)"
	renderer, err := glamour.NewTermRenderer(glamour.WithEnvironmentConfig(), glamour.WithWordWrap(80))
	if err != nil {
		t.Fatal(err)
	}
	source, links := preprocessMarkdownLinksWithURLs(markdown, "api.rootly.com")
	rendered, err := renderer.Render(source)
	if err != nil {
		t.Fatal(err)
	}
	output := restoreMarkdownLinks(rendered, links)
	visible := ansi.Strip(output)
	if !strings.Contains(visible, "Open INC-8 (evil.example)") {
		t.Errorf("visible output = %q, want external host after link label", visible)
	}
	if !strings.Contains(visible, "Open Rootly") || strings.Contains(visible, "(rootly.com)") {
		t.Errorf("visible output = %q, want trusted Rootly link without host disclosure", visible)
	}
}

func TestGlamourDoesNotHyperlinkUnsupportedSchemes(t *testing.T) {
	markdown := "[x](javascript:alert(1))"
	renderer, err := glamour.NewTermRenderer(glamour.WithEnvironmentConfig(), glamour.WithWordWrap(80))
	if err != nil {
		t.Fatal(err)
	}
	source, links := preprocessMarkdownLinksWithURLs(markdown, "api.rootly.com")
	rendered, err := renderer.Render(source)
	if err != nil {
		t.Fatal(err)
	}
	output := restoreMarkdownLinks(rendered, links)
	if strings.Contains(output, "\x1b]8;;") {
		t.Errorf("output = %q, want no OSC-8 hyperlink", output)
	}
	if visible := ansi.Strip(output); !strings.Contains(visible, "x <javascript:alert(1)>") {
		t.Errorf("visible output = %q, want label followed by plain URL", visible)
	}
}

func TestGlamourPreservesPrivateUseLinkMarkers(t *testing.T) {
	const (
		start = "\uE000"
		end   = "\uE001"
	)
	source := "Rotate " + start + "the keys" + end + " today.\n\n- " + start + "review access" + end
	renderer, err := glamour.NewTermRenderer(glamour.WithEnvironmentConfig(), glamour.WithWordWrap(80))
	if err != nil {
		t.Fatal(err)
	}
	rendered, err := renderer.Render(source)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(rendered, start) != 2 || strings.Count(rendered, end) != 2 {
		t.Fatalf("rendered output = %q, want both pairs of private-use markers", rendered)
	}
	firstStart := strings.Index(rendered, start)
	firstEnd := strings.Index(rendered, end)
	secondStart := strings.LastIndex(rendered, start)
	secondEnd := strings.LastIndex(rendered, end)
	if firstStart >= firstEnd || firstEnd >= secondStart || secondStart >= secondEnd {
		t.Errorf("rendered marker order is incorrect: %q", rendered)
	}
}

func TestPreprocessMarkdownLinksUsesASTAndHonorsNoColor(t *testing.T) {
	const (
		url      = "https://rootly.com/docs"
		code     = "`[code](https://rootly.com/code)`"
		block    = "[block](https://rootly.com/block)"
		autolink = "<https://rootly.com/auto>"
		bare     = "https://rootly.com/bare"
	)
	markdown := "[Read the docs](" + url + ") " + code + "\n\n```md\n" + block + "\n```\n\n" + autolink + " " + bare

	processed := terminalMarkdown(markdown, false)
	if !strings.Contains(processed, "\uE000Read the docs\uE001") {
		t.Errorf("processed Markdown = %q, want private-use markers around the link text", processed)
	}
	if strings.Contains(processed, ansi.SetHyperlink(url)) || strings.Contains(processed, ansi.ResetHyperlink()) {
		t.Errorf("processed Markdown = %q, must not contain OSC-8 controls before rendering", processed)
	}
	for _, unchanged := range []string{code, block, autolink, bare} {
		if !strings.Contains(processed, unchanged) {
			t.Errorf("processed Markdown = %q, want unchanged %q", processed, unchanged)
		}
	}
	if got := terminalMarkdown(markdown, true); got != markdown {
		t.Errorf("no-color Markdown = %q, want raw Markdown %q", got, markdown)
	}
}

func TestRenderMarkdownStylesRewrittenDashboardLink(t *testing.T) {
	model := NewTUIModel(t.Context(), &fakeStreamer{}, TUIOptions{APIHost: "api.rootly.com"})
	output := model.renderMarkdown("[Add to dashboard](#ai-add-to-dashboard:ref)", 80, false)
	if !strings.Contains(output, ansi.SetHyperlink("https://rootly.com/account")) {
		t.Errorf("rendered link = %q, want rewritten OSC-8 destination", output)
	}
	if !strings.Contains(output, "\x1b[4") {
		t.Errorf("rendered link = %q, want underlined clickable text", output)
	}
	if visible := ansi.Strip(output); !strings.Contains(visible, "Add to dashboard: open in web") {
		t.Errorf("rendered link = %q, want rewritten label", visible)
	}
}

func TestRenderMarkdownStylesOpenInWebLink(t *testing.T) {
	const destination = "https://rootly.com/account?chat=x"
	model := NewTUIModel(t.Context(), &fakeStreamer{}, TUIOptions{APIHost: "api.rootly.com"})
	output := model.renderMarkdown("[open in web]("+destination+")", 80, false)
	if !strings.Contains(output, ansi.SetHyperlink(destination)) ||
		!strings.Contains(output, ansi.ResetHyperlink()) {
		t.Errorf("rendered link = %q, want OSC-8 open and close sequences", output)
	}
	if !strings.Contains(output, "\x1b[4") {
		t.Errorf("rendered link = %q, want underlined clickable text", output)
	}
	if got := strings.Count(ansi.Strip(output), "open in web"); got != 1 {
		t.Errorf("visible link label count = %d, want 1: %q", got, ansi.Strip(output))
	}
}

func TestNoColorAndPipedMarkdownSkipLinkStyling(t *testing.T) {
	const markdown = "[Add to dashboard](#ai-add-to-dashboard:ref)"
	model := NewTUIModel(t.Context(), &fakeStreamer{}, TUIOptions{
		APIHost: "api.rootly.com",
		NoColor: true,
	})
	tuiOutput := model.renderMarkdown(markdown, 80, false)
	if strings.Contains(tuiOutput, "\x1b") {
		t.Errorf("no-color TUI output = %q, want no ANSI", tuiOutput)
	}

	for _, test := range []struct {
		name        string
		noColor     bool
		stdoutIsTTY bool
	}{
		{name: "no-color terminal", noColor: true, stdoutIsTTY: true},
		{name: "piped output", stdoutIsTTY: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			var stdout bytes.Buffer
			err := writeAnswer(PrintOptions{
				Stdout:      &stdout,
				APIHost:     "api.rootly.com",
				NoColor:     test.noColor,
				IsStdoutTTY: func() bool { return test.stdoutIsTTY },
			}, markdown)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(stdout.String(), "\x1b") {
				t.Errorf("print output = %q, want no ANSI", stdout.String())
			}
		})
	}
}
