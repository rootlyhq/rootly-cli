package agent

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	rootly "github.com/rootlyhq/rootly-go"
)

const (
	welcomeLogoWidth  = 10
	welcomeLogoHeight = 4
)

var welcomeLogoLines = []string{
	"  ▄ ██ ▄  ",
	" ▄█▀▄▄▀█▄ ",
	"▄██▄▀▀▄██▄",
	"▀▀▀▀██▀▀▀▀",
}

func (m *TUIModel) welcomeBanner() string {
	return m.welcomeBannerContent(sproutLogo(m.noColor)) + "\n"
}

func (m *TUIModel) welcomeBannerFrame() string {
	return m.welcomeBannerContent(renderSproutFrame(m.welcomeFrame))
}

func (m *TUIModel) welcomeBannerContent(logo string) string {
	text := m.welcomeTextRows()
	if m.width < 50 {
		return text
	}
	textWidth := max(0, m.width-welcomeLogoWidth-2)
	lines := strings.Split(text, "\n")
	for index, line := range lines {
		lines[index] = ansi.Truncate(line, textWidth, "")
	}
	return lipgloss.JoinHorizontal(lipgloss.Center, logo, "  ", strings.Join(lines, "\n"))
}

func (m *TUIModel) welcomeTextRows() string {
	title := "✦ Rootly AI"
	version := strings.TrimPrefix(sanitizeTerminalText(m.version), "v")
	if version != "" && version != "dev" {
		title += " v" + version
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
		userLine = name
	}

	host := m.apiHost
	if host == "" && m.api != nil {
		host = m.api.APIHost()
	}
	hostLine := host
	contextLine := m.sessionHeaderContext()
	lines := []string{title, userLine, hostLine, contextLine}
	for index, line := range lines {
		line = sanitizeTerminalText(line)
		if !m.noColor {
			line = faint(false, line)
		}
		lines[index] = line
	}
	return strings.Join(lines, "\n")
}

func (m *TUIModel) sessionHeaderContext() string {
	if m.incident != nil {
		id := m.incident.SequentialID
		if id == "" {
			id = shortSessionID(m.incident.ID)
		}
		return id + " · " + sanitizeTerminalText(m.incident.Title)
	}
	if m.alert != nil {
		id := m.alert.ShortID
		if id == "" {
			id = shortSessionID(m.alert.ID)
		}
		return id + " · " + sanitizeTerminalText(m.alert.Summary)
	}
	if m.resumedSession {
		if title := normalizeSessionTitle(m.sessionTitle); title != "" {
			return title
		}
		if m.sessionID != nil {
			return shortSessionID(m.sessionID.String())
		}
	}
	return "New session"
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
