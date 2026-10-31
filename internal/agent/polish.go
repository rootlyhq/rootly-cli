package agent

import (
	"errors"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/rootlyhq/rootly-cli/internal/mcp"
)

const (
	sessionNotFoundMessage = "Session not found — starting a new chat"
	blurredCursorGlyph     = "▢"
	mcpReasonMaxWidth      = 120
)

var errSessionNotFound = errors.New("session not found")

type mcpStartupCheckMsg struct{}

func mcpStartupCheck() tea.Msg { return mcpStartupCheckMsg{} }

func (m *TUIModel) updateFocus(message tea.Msg) bool {
	switch message.(type) {
	case tea.FocusMsg:
		m.terminalBlurred = false
		return true
	case tea.BlurMsg:
		m.terminalBlurred = true
		return true
	}
	return false
}

func (m *TUIModel) composerView() string {
	if !m.terminalBlurred || !m.composer.Focused() {
		return m.composer.View()
	}
	positioned := m.composer
	positioned.SetVirtualCursor(false)
	position := positioned.Cursor()
	blurred := m.composer
	blurred.Blur()
	view := blurred.View()
	if position == nil {
		return view
	}
	return overlayBlurredCursor(view, position.X, position.Y)
}

func overlayBlurredCursor(view string, x, y int) string {
	glyph := blurredCursorGlyph
	lines := strings.Split(view, "\n")
	if y < 0 || y >= len(lines) || x < 0 {
		return view
	}
	line := lines[y]
	width := ansi.StringWidth(line)
	if x >= width {
		lines[y] = line + strings.Repeat(" ", x-width) + glyph
	} else {
		lines[y] = ansi.Cut(line, 0, x) + glyph + ansi.Cut(line, x+1, width)
	}
	return strings.Join(lines, "\n")
}

func (m *TUIModel) continueLine() bool {
	value := m.composer.Value()
	trailing := len(value) - len(strings.TrimRight(value, `\`))
	if trailing%2 == 0 {
		return false
	}
	m.composer.SetValue(strings.TrimSuffix(value, `\`) + "\n")
	m.composer.CursorEnd()
	m.historyIndex = -1
	return true
}

func moreLinesHint(hidden int) string {
	if hidden == 1 {
		return "↑ 1 more line"
	}
	return fmt.Sprintf("↑ %d more lines", hidden)
}

func (m *TUIModel) activityLine(line string) string {
	line = sanitizeTerminalText(line)
	rest, failed := strings.CutPrefix(line, "✗")
	if !failed || m.noColor {
		return faint(m.noColor, line)
	}
	return lipgloss.NewStyle().Foreground(lipgloss.Color("1")).Render("✗") + faint(false, rest)
}

func (m *TUIModel) noteClientToolsAccepted(accepted *int) {
	if accepted == nil || m.sentTools == 0 {
		return
	}
	if *accepted == 0 && !m.toolsOffShown {
		m.toolsOffShown = true
		m.println(faint(m.noColor, localToolsOffMessage))
		return
	}
	if *accepted > 0 && !m.toolsOnShown {
		m.toolsOnShown = true
		m.println(faint(m.noColor, localToolsAvailableMessage(*accepted)))
	}
}

func localToolsAvailableMessage(count int) string {
	if count == 1 {
		return "1 local tool available"
	}
	return fmt.Sprintf("%d local tools available", count)
}

func (m *TUIModel) handleHistoryLoaded(msg historyLoadedMsg) {
	if errors.Is(msg.err, errSessionNotFound) {
		m.startNewSession()
		m.println(faint(m.noColor, sessionNotFoundMessage))
		return
	}
	if msg.err != nil {
		m.println(faint(m.noColor, "Could not load session history: "+sanitizeTerminalText(msg.err.Error())))
		return
	}
	m.printHistory(msg.messages)
}

func (m *TUIModel) reportUnreachableMCP() {
	if m.mcp == nil || m.mcpStartupReported || m.mcp.Connecting() > 0 {
		return
	}
	m.mcpStartupReported = true
	for _, view := range m.mcpPage.views {
		if view.SessionOff || view.Status != mcp.StatusError {
			continue
		}
		m.println(faint(m.noColor, unreachableMCPLine(view)))
	}
}

func unreachableMCPLine(view mcp.ServerView) string {
	reason := strings.Join(strings.Fields(sanitizeTerminalText(view.Detail)), " ")
	if reason == "" {
		reason = "connection failed"
	}
	reason = ansi.Truncate(reason, mcpReasonMaxWidth, "…")
	return fmt.Sprintf("MCP '%s' unreachable: %s — open /mcp", sanitizeTerminalText(view.Server.Name), reason)
}
