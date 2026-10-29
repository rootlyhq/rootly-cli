package agent

import (
	"strings"
	"testing"

	"charm.land/glamour/v2"
	"github.com/charmbracelet/x/ansi"
)

func TestGlamourPreservesOSC8LinksAndWrapsVisibleText(t *testing.T) {
	const target = "https://rootly.com/docs/a-long-path-that-should-not-be-visible"
	linkStart := ansi.SetHyperlink(target)
	linkEnd := ansi.ResetHyperlink()
	renderer, err := glamour.NewTermRenderer(glamour.WithWordWrap(20))
	if err != nil {
		t.Fatal(err)
	}

	rendered, err := renderer.Render(linkStart + "Read the guide" + linkEnd + " now")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(rendered, linkStart) || !strings.Contains(rendered, linkEnd) {
		t.Fatalf("rendered output did not preserve OSC-8 controls: %q", rendered)
	}
	visible := strings.TrimSpace(ansi.Strip(rendered))
	if strings.Contains(visible, target) || !strings.Contains(visible, "Read the guide now") {
		t.Errorf("visible output = %q, want link text without its target", visible)
	}
	for _, line := range strings.Split(visible, "\n") {
		if width := ansi.StringWidth(line); width > 20 {
			t.Errorf("visible line width = %d, want at most 20: %q", width, line)
		}
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
	if !strings.Contains(processed, ansi.SetHyperlink(url)) || !strings.Contains(processed, ansi.ResetHyperlink()) {
		t.Errorf("processed Markdown = %q, want OSC-8 around the link text", processed)
	}
	visible := ansi.Strip(processed)
	if strings.Contains(visible, "]("+url+")") {
		t.Errorf("processed Markdown still contains the rendered destination: %q", visible)
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
