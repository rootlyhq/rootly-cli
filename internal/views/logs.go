package views

import (
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"golang.design/x/clipboard"

	"github.com/rootlyhq/rootly-tui/internal/debug"
	"github.com/rootlyhq/rootly-tui/internal/i18n"
	"github.com/rootlyhq/rootly-tui/internal/styles"
)

// LogsRefreshMsg triggers a log refresh
type LogsRefreshMsg struct{}

// LogsStatusClearMsg is sent to clear the status message
type LogsStatusClearMsg struct{}

var (
	logDebugStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))  // Gray
	logInfoStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("12")) // Blue
	logWarnStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("11")) // Yellow
	logErrorStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))  // Red
)

type LogsModel struct {
	Visible  bool
	width    int
	height   int
	viewport viewport.Model

	// Content tracking
	content    string
	lineCount  int
	lastLength int // Track file size for change detection

	// Auto-tail mode
	autoTail bool

	// Mouse selection
	selecting    bool
	selectStart  int
	selectEnd    int
	hasSelection bool

	// Status message
	statusMsg string

	// Clipboard availability
	clipboardChecked   bool
	clipboardAvailable bool
}

func NewLogsModel() LogsModel {
	vp := viewport.New(80, 20)
	vp.Style = lipgloss.NewStyle()
	vp.MouseWheelEnabled = true

	return LogsModel{
		viewport: vp,
		autoTail: true, // Auto-scroll to bottom by default
	}
}

func (m LogsModel) Init() tea.Cmd {
	return nil
}

func (m LogsModel) Update(msg tea.Msg) (LogsModel, tea.Cmd) {
	if !m.Visible {
		return m, nil
	}

	var cmds []tea.Cmd
	var vpCmd tea.Cmd

	switch msg := msg.(type) {
	case LogsRefreshMsg:
		m.loadContent()
		// Continue ticking for auto-refresh
		cmds = append(cmds, m.scheduleRefresh())

	case LogsStatusClearMsg:
		m.statusMsg = ""
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "j", "down":
			m.autoTail = false
			m.viewport, vpCmd = m.viewport.Update(msg)
			cmds = append(cmds, vpCmd)
		case "k", "up":
			m.autoTail = false
			m.viewport, vpCmd = m.viewport.Update(msg)
			cmds = append(cmds, vpCmd)
		case "g":
			m.autoTail = false
			m.viewport.GotoTop()
		case "G":
			m.autoTail = true
			m.viewport.GotoBottom()
		case "f":
			// Toggle auto-tail (follow) mode
			m.autoTail = !m.autoTail
			if m.autoTail {
				m.viewport.GotoBottom()
			}
		case "c":
			debug.ClearLogs()
			m.content = ""
			m.lineCount = 0
			m.lastLength = 0
			m.viewport.SetContent("")
			m.clearSelection()
		case "y":
			if m.clipboardAvailable {
				m.copyToClipboard()
				if m.statusMsg != "" {
					cmds = append(cmds, tea.Tick(2*time.Second, func(t time.Time) tea.Msg {
						return LogsStatusClearMsg{}
					}))
				}
			}
		case "a":
			// Select all
			if m.lineCount > 0 {
				m.selectStart = 0
				m.selectEnd = m.lineCount - 1
				m.hasSelection = true
			}
		case "esc":
			if m.hasSelection {
				m.clearSelection()
			}
		case "ctrl+d", "pgdown":
			m.autoTail = false
			m.viewport, vpCmd = m.viewport.Update(msg)
			cmds = append(cmds, vpCmd)
		case "ctrl+u", "pgup":
			m.autoTail = false
			m.viewport, vpCmd = m.viewport.Update(msg)
			cmds = append(cmds, vpCmd)
		}

	case tea.MouseMsg:
		// Forward scroll events to viewport
		m.viewport, vpCmd = m.viewport.Update(msg)
		cmds = append(cmds, vpCmd)
		// Disable auto-tail on manual scroll
		if msg.Action == tea.MouseActionPress && (msg.Button == tea.MouseButtonWheelUp || msg.Button == tea.MouseButtonWheelDown) {
			m.autoTail = false
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.updateViewportSize()
	}

	return m, tea.Batch(cmds...)
}

func (m *LogsModel) updateViewportSize() {
	// Calculate viewport dimensions (leave room for title, help, borders)
	vpHeight := m.height - 12
	vpWidth := m.width - 8

	if vpHeight < 1 {
		vpHeight = 1
	}
	if vpWidth < 20 {
		vpWidth = 20
	}

	m.viewport.Width = vpWidth
	m.viewport.Height = vpHeight
}

func (m *LogsModel) loadContent() {
	var content string
	var lines []string

	if debug.HasLogFile() {
		// Read from file
		fileContent, err := debug.ReadLogFile()
		if err != nil {
			content = "Error reading log file: " + err.Error()
		} else {
			// Only update if content changed
			if len(fileContent) == m.lastLength {
				return
			}
			m.lastLength = len(fileContent)
			content = fileContent
		}
		lines = strings.Split(strings.TrimSuffix(content, "\n"), "\n")
	} else {
		// Read from memory buffer
		logEntries := debug.GetLogs()
		lines = logEntries
	}

	// Colorize lines
	var colorized []string
	for _, line := range lines {
		line = strings.TrimSuffix(line, "\n")
		if line != "" {
			colorized = append(colorized, colorizeLogEntry(line))
		}
	}

	m.content = strings.Join(colorized, "\n")
	m.lineCount = len(colorized)
	m.viewport.SetContent(m.content)

	// Auto-scroll to bottom if in tail mode
	if m.autoTail {
		m.viewport.GotoBottom()
	}
}

func (m *LogsModel) scheduleRefresh() tea.Cmd {
	return tea.Tick(500*time.Millisecond, func(t time.Time) tea.Msg {
		return LogsRefreshMsg{}
	})
}

func (m *LogsModel) clearSelection() {
	m.selecting = false
	m.selectStart = 0
	m.selectEnd = 0
	m.hasSelection = false
}

func (m *LogsModel) copyToClipboard() {
	text := m.content
	if text == "" {
		return
	}

	if err := clipboard.Init(); err != nil {
		debug.Logger.Error("Failed to initialize clipboard", "error", err)
		m.statusMsg = i18n.T("logs.clipboard_unavailable")
		return
	}

	clipboard.Write(clipboard.FmtText, []byte(text))
	m.statusMsg = i18n.T("logs.copied")
}

func (m *LogsModel) Toggle() {
	m.Visible = !m.Visible
	if m.Visible {
		m.Show()
	}
}

func (m *LogsModel) Show() {
	m.Visible = true
	m.checkClipboard()
	m.loadContent()
}

func (m *LogsModel) checkClipboard() {
	if m.clipboardChecked {
		return
	}
	m.clipboardChecked = true
	if err := clipboard.Init(); err != nil {
		m.clipboardAvailable = false
	} else {
		m.clipboardAvailable = true
	}
}

func (m *LogsModel) Hide() {
	m.Visible = false
}

func (m *LogsModel) Refresh() {
	m.loadContent()
}

func (m *LogsModel) SetDimensions(width, height int) {
	m.width = width
	m.height = height
	m.updateViewportSize()
}

// StartAutoRefresh returns a command to start the auto-refresh ticker
func (m LogsModel) StartAutoRefresh() tea.Cmd {
	return m.scheduleRefresh()
}

func (m LogsModel) View() string {
	var b strings.Builder

	// Title with source indicator
	var titleSuffix string
	if debug.HasLogFile() {
		titleSuffix = " (" + debug.LogFilePath + ")"
	} else {
		titleSuffix = " (" + i18n.T("logs.memory") + ")"
	}
	title := styles.DialogTitle.Render(i18n.T("logs.title") + titleSuffix)
	b.WriteString(title)
	b.WriteString("\n\n")

	// Viewport content
	if m.lineCount == 0 {
		b.WriteString(styles.TextDim.Render(i18n.T("logs.empty")))
		b.WriteString("\n")
	} else {
		b.WriteString(m.viewport.View())
	}

	// Scroll indicator and tail status
	b.WriteString("\n")
	var statusParts []string
	statusParts = append(statusParts, i18n.Tf("logs.line_count", map[string]interface{}{"Count": m.lineCount}))
	if m.autoTail {
		statusParts = append(statusParts, "["+i18n.T("logs.following")+"]")
	}
	if m.viewport.ScrollPercent() < 1.0 {
		statusParts = append(statusParts, i18n.Tf("logs.scroll_percent", map[string]interface{}{"Percent": int(m.viewport.ScrollPercent() * 100)}))
	}
	b.WriteString(styles.TextDim.Render(strings.Join(statusParts, " • ")))

	b.WriteString("\n\n")

	// Status message
	if m.statusMsg != "" {
		b.WriteString(styles.Success.Render(m.statusMsg))
		b.WriteString("\n\n")
	}

	// Help
	help := styles.HelpBar.Render(m.getHelpText())
	b.WriteString(help)

	// Wrap in dialog
	content := b.String()
	dialogWidth := m.width - 4
	if dialogWidth < 40 {
		dialogWidth = 40
	}
	dialog := styles.Dialog.Width(dialogWidth).Render(content)

	return dialog
}

func (m LogsModel) getHelpText() string {
	base := "j/k:scroll g/G:top/bottom f:follow"
	if m.clipboardAvailable {
		base += " y:copy"
	}
	base += " c:clear q:close"
	return base
}

// colorizeLogEntry applies color based on log level
func colorizeLogEntry(entry string) string {
	upperEntry := strings.ToUpper(entry)

	switch {
	case strings.Contains(upperEntry, "ERRO") || strings.Contains(upperEntry, "ERROR"):
		return logErrorStyle.Render(entry)
	case strings.Contains(upperEntry, "WARN"):
		return logWarnStyle.Render(entry)
	case strings.Contains(upperEntry, "INFO"):
		return logInfoStyle.Render(entry)
	case strings.Contains(upperEntry, "DEBU") || strings.Contains(upperEntry, "DEBUG"):
		return logDebugStyle.Render(entry)
	default:
		return styles.Text.Render(entry)
	}
}
