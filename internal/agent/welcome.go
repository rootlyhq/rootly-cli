package agent

import (
	"os"
	"path/filepath"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	rootly "github.com/rootlyhq/rootly-go"
)

const (
	welcomeLogoWidth  = 14
	welcomeLogoHeight = 4
)

var welcomeLogoLines = []string{
	"     ▄█       ",
	"    ▄▀ █      ",
	"▀▀▀▀▀  █ ▄▀▀▀▀",
	"       ▀█     ",
}

func (m *TUIModel) welcomeBanner() string {
	return m.welcomeBannerContent(pulseLogo(m.noColor)) + "\n"
}

func (m *TUIModel) welcomeBannerFrame() string {
	return m.welcomeBannerContent(renderPulseFrame(m.welcomeFrame))
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
	version := strings.TrimPrefix(sanitizeTerminalText(m.version), "v")
	versionText := ""
	if version != "" && version != "dev" {
		versionText = " v" + version
	}
	title := sanitizeTerminalText("✦") + " " + sanitizeTerminalText("Rootly AI") + sanitizeTerminalText(versionText)
	if !m.noColor {
		title = lipgloss.NewStyle().Foreground(lipgloss.Color("#F5A24F")).Render(sanitizeTerminalText("✦")) +
			" " + lipgloss.NewStyle().Bold(true).Render(sanitizeTerminalText("Rootly AI"))
		if versionText != "" {
			title += faint(false, sanitizeTerminalText(versionText))
		}
	}

	userLine := ""
	if m.currentUser != nil {
		attributes := m.currentUser.Data.Attributes
		name := currentUserFirstName(attributes)
		team := currentUserTeamName(attributes)
		if name == "" {
			name = sanitizeTerminalText(attributes.Email)
		}
		if name != "" {
			name = "Welcome back, " + name
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
	lines := []string{title, userLine}
	if dir := displayWorkingDir(m.workingDir); dir != "" {
		lines = append(lines, dir)
	}
	lines = append(lines, hostLine)
	if capability := m.welcomeCapabilityLine(); capability != "" {
		lines = append(lines, capability)
	}
	lines = append(lines, contextLine)
	for index := 1; index < len(lines); index++ {
		line := lines[index]
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

func pulseLogo(noColor bool) string {
	logo := strings.Join(welcomeLogoLines, "\n")
	if noColor {
		return logo
	}
	return lipgloss.NewStyle().Foreground(lipgloss.Color("#7C5CE6")).Render(logo)
}

const welcomeCapabilityText = "Ask me about incidents, alerts and on-call"

func (m *TUIModel) welcomeCapabilityLine() string {
	available := m.width
	if m.width >= 50 {
		available = m.width - welcomeLogoWidth - 2
	}
	if available <= 0 || ansi.StringWidth(welcomeCapabilityText) > available {
		return ""
	}
	return welcomeCapabilityText
}

func displayWorkingDir(dir string) string {
	if dir == "" {
		return ""
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		if dir == home {
			return "~"
		}
		if rest, ok := strings.CutPrefix(dir, home+string(os.PathSeparator)); ok {
			return filepath.Join("~", rest)
		}
	}
	return dir
}

func currentUserFirstName(attributes rootly.CurrentUser) string {
	if first := strings.TrimSpace(attributes.FirstName.GetOrEmpty()); first != "" {
		return sanitizeTerminalText(first)
	}
	if fields := strings.Fields(currentUserName(attributes)); len(fields) > 0 {
		return fields[0]
	}
	return ""
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
