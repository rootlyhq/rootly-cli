package agent

import (
	"strings"

	"charm.land/lipgloss/v2"
	rootly "github.com/rootlyhq/rootly-go"
)

func (m *TUIModel) welcomeBanner() string {
	mark := "✦"
	name := "Rootly AI"
	if !m.noColor {
		mark = lipgloss.NewStyle().Foreground(lipgloss.Color("#F5A24F")).Render(mark)
		name = lipgloss.NewStyle().Bold(true).Render(name)
	}
	title := mark + " " + name
	version := sanitizeTerminalText(m.version)
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
	rows := strings.Join([]string{title, userLine, hostLine}, "\n")
	return lipgloss.JoinHorizontal(lipgloss.Center, sproutLogo(m.noColor), "  ", rows) + "\n"
}

func sproutLogo(noColor bool) string {
	lines := []string{
		"        ▄█▄",
		"    ▄▄  ███  ▄▄",
		"    ▀██  ▀  ██▀",
		" ▄▄▄▄    ▄    ▄▄▄▄",
		"  ▀▀▀   ███   ▀▀▀",
		"▄▄▄▄▄▄▄  ▀  ▄▄▄▄▄▄▄",
		"▀▀▀▀▀▀▀██▄██▀▀▀▀▀▀▀",
		"        ███",
	}
	width := 0
	for _, line := range lines {
		width = max(width, lipgloss.Width(line))
	}
	for index, line := range lines {
		lines[index] = line + strings.Repeat(" ", width-lipgloss.Width(line))
	}
	logo := strings.Join(lines, "\n")
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
