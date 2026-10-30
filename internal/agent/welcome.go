package agent

import (
	"strings"

	"charm.land/lipgloss/v2"
	rootly "github.com/rootlyhq/rootly-go"
)

const (
	welcomeLogoWidth  = 17
	welcomeLogoHeight = 10
)

var welcomeLogoLines = []string{
	"        ▄        ",
	"       ███       ",
	"   ███ ▀█▀ ███   ",
	"    ▀█▀   ▀█▀    ",
	" ▄██▄   ▄   ▄██▄ ",
	"  ▀█▀  ███  ▀█▀  ",
	" ▄▄▄    ▀    ▄▄▄ ",
	"███████▄ ▄███████",
	"     ▀▀███▀▀     ",
	"       ███       ",
}

func (m *TUIModel) welcomeBanner() string {
	return m.welcomeBannerContent(sproutLogo(m.noColor)) + "\n"
}

func (m *TUIModel) welcomeBannerFrame() string {
	return m.welcomeBannerContent(renderSproutFrame(m.welcomeFrame))
}

func (m *TUIModel) welcomeBannerContent(logo string) string {
	return lipgloss.JoinHorizontal(lipgloss.Center, logo, "  ", m.welcomeTextRows())
}

func (m *TUIModel) welcomeTextRows() string {
	mark := "✦"
	name := "Rootly AI"
	if !m.noColor {
		mark = lipgloss.NewStyle().Foreground(lipgloss.Color("#F5A24F")).Render(mark)
		name = lipgloss.NewStyle().Bold(true).Render(name)
	}
	title := mark + " " + name
	version := strings.TrimPrefix(sanitizeTerminalText(m.version), "v")
	if version != "" && version != "dev" {
		title += " " + faint(m.noColor, "v"+version)
	}

	userLine := ""
	if m.currentUser != nil {
		attributes := m.currentUser.Data.Attributes
		name := currentUserName(attributes)
		team := currentUserTeamName(attributes)
		if name == "" {
			name = sanitizeTerminalText(attributes.Email)
		}
		if name != "" && team != "" {
			name += " · " + team
		}
		userLine = faint(m.noColor, name)
	}

	host := m.apiHost
	if host == "" && m.api != nil {
		host = m.api.APIHost()
	}
	hostLine := faint(m.noColor, sanitizeTerminalText(host))
	return strings.Join([]string{title, userLine, hostLine}, "\n")
}

func sproutLogo(noColor bool) string {
	logo := strings.Join(welcomeLogoLines, "\n")
	if noColor {
		return logo
	}
	return lipgloss.NewStyle().Foreground(lipgloss.Color("#7C5CE6")).Render(logo)
}

func currentUserName(attributes rootly.CurrentUser) string {
	name := attributes.FullName.GetOrEmpty()
	if name == "" {
		name = strings.TrimSpace(attributes.FirstName.GetOrEmpty() + " " + attributes.LastName.GetOrEmpty())
	}
	return sanitizeTerminalText(name)
}

func currentUserTeamName(attributes rootly.CurrentUser) string {
	fullNameWithTeam := sanitizeTerminalText(attributes.FullNameWithTeam.GetOrEmpty())
	if !strings.HasPrefix(fullNameWithTeam, "[") {
		return ""
	}
	end := strings.Index(fullNameWithTeam, "] ")
	if end <= 1 {
		return ""
	}
	return strings.TrimSpace(fullNameWithTeam[1:end])
}
