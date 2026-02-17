package views

import (
	"runtime"
	"strings"

	"github.com/rootlyhq/rootly-tui/internal/i18n"
	"github.com/rootlyhq/rootly-tui/internal/styles"
)

type AboutModel struct {
	Visible bool
	version string
}

func NewAboutModel(version string) AboutModel {
	return AboutModel{
		Visible: false,
		version: version,
	}
}

func (m *AboutModel) Toggle() {
	m.Visible = !m.Visible
}

func (m *AboutModel) Show() {
	m.Visible = true
}

func (m *AboutModel) Hide() {
	m.Visible = false
}

func (m AboutModel) View() string {
	var b strings.Builder

	b.WriteString(styles.DialogTitle.Render(i18n.T("about.title")))
	b.WriteString("\n\n")

	// App name and version
	b.WriteString(styles.TextBold.Render("Rootly TUI"))
	b.WriteString("\n")
	b.WriteString(styles.Text.Render("v" + m.version))
	b.WriteString("\n\n")

	// Description
	b.WriteString(styles.Text.Render(i18n.T("about.description")))
	b.WriteString("\n\n")

	// System info
	b.WriteString(styles.TextBold.Render(i18n.T("about.system")))
	b.WriteString("\n")
	b.WriteString(renderAboutLine(i18n.T("about.go_version"), runtime.Version()))
	b.WriteString(renderAboutLine(i18n.T("about.platform"), runtime.GOOS+"/"+runtime.GOARCH))
	b.WriteString("\n")

	// Links
	b.WriteString(styles.TextBold.Render(i18n.T("about.links")))
	b.WriteString("\n")
	b.WriteString(renderAboutLine(i18n.T("about.docs"), "https://docs.rootly.com/integrations/tui"))
	b.WriteString(renderAboutLine("GitHub", "https://github.com/rootlyhq/rootly-tui"))
	b.WriteString("\n")

	// Credits
	b.WriteString(styles.TextDim.Render("Built with vibe coding"))
	b.WriteString("\n")
	b.WriteString(styles.TextDim.Render("Thanks Claude Opus 4.5"))
	b.WriteString("\n\n")

	// Close hint
	b.WriteString(styles.TextDim.Render(i18n.T("about.press_to_close")))

	return styles.Dialog.Render(b.String())
}

func renderAboutLine(label, value string) string {
	labelStyle := styles.TextDim.Width(14)
	return labelStyle.Render(label+":") + " " + styles.Text.Render(value) + "\n"
}
