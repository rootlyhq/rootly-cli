package agent

import (
	"strings"

	"charm.land/lipgloss/v2"
	rootly "github.com/rootlyhq/rootly-go"
)

func (m *TUIModel) welcomeBanner() string {
	logo := " ▄▀▀▀▄ \n█ ● ● █\n ▀▄▄▄▀ "
	if !m.noColor {
		logo = lipgloss.NewStyle().Foreground(lipgloss.Color("#8B5CF6")).Render(logo)
	}

	title := "Rootly AI"
	if !m.noColor {
		title = lipgloss.NewStyle().Bold(true).Render(title)
	}
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
	return lipgloss.JoinHorizontal(lipgloss.Top, logo, "  ", rows) + "\n"
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
