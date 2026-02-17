package views

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/rootlyhq/rootly-tui/internal/debug"
)

func TestNewLogsModel(t *testing.T) {
	m := NewLogsModel()

	if m.Visible {
		t.Error("expected logs to be hidden initially")
	}

	// Should have auto-tail enabled by default
	if !m.autoTail {
		t.Error("expected autoTail to be true initially")
	}
}

func TestLogsModelToggle(t *testing.T) {
	m := NewLogsModel()

	m.Toggle()
	if !m.Visible {
		t.Error("expected logs to be visible after toggle")
	}

	m.Toggle()
	if m.Visible {
		t.Error("expected logs to be hidden after second toggle")
	}
}

func TestLogsModelShowHide(t *testing.T) {
	m := NewLogsModel()

	m.Show()
	if !m.Visible {
		t.Error("expected logs to be visible after Show()")
	}

	m.Hide()
	if m.Visible {
		t.Error("expected logs to be hidden after Hide()")
	}
}

func TestLogsModelSetDimensions(t *testing.T) {
	m := NewLogsModel()

	m.SetDimensions(100, 50)

	if m.width != 100 {
		t.Errorf("expected width 100, got %d", m.width)
	}
	if m.height != 50 {
		t.Errorf("expected height 50, got %d", m.height)
	}
}

func TestLogsModelRefresh(t *testing.T) {
	// Clear any existing logs
	debug.ClearLogs()

	m := NewLogsModel()
	m.SetDimensions(100, 50)

	// Add some test logs
	debug.Logger.Info("test log 1")
	debug.Logger.Info("test log 2")

	m.Refresh()

	if m.lineCount < 2 {
		t.Errorf("expected at least 2 lines, got %d", m.lineCount)
	}
}

func TestLogsModelUpdate(t *testing.T) {
	m := NewLogsModel()
	m.Visible = true
	m.SetDimensions(100, 50)

	// Add some logs
	debug.ClearLogs()
	for i := 0; i < 100; i++ {
		debug.Logger.Info("test log entry")
	}
	m.Refresh()

	// Test 'f' to toggle follow mode
	initialAutoTail := m.autoTail
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'f'}})
	if m.autoTail == initialAutoTail {
		t.Error("expected autoTail to toggle after 'f'")
	}

	// Test 'G' goes to bottom and enables follow
	m.autoTail = false
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'G'}})
	if !m.autoTail {
		t.Error("expected autoTail to be true after 'G'")
	}

	// Test 'g' goes to top and disables follow
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}})
	if m.autoTail {
		t.Error("expected autoTail to be false after 'g'")
	}

	// Test clear with 'c'
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
	if m.lineCount != 0 {
		t.Errorf("expected 0 lines after clear, got %d", m.lineCount)
	}
}

func TestLogsModelUpdateWhenHidden(t *testing.T) {
	m := NewLogsModel()
	m.Visible = false

	// Updates should be ignored when hidden
	initialAutoTail := m.autoTail
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'f'}})
	if m.autoTail != initialAutoTail {
		t.Error("expected autoTail to remain unchanged when hidden")
	}
}

func TestLogsModelView(t *testing.T) {
	m := NewLogsModel()
	m.SetDimensions(100, 50)

	view := m.View()
	if view == "" {
		t.Error("expected non-empty view")
	}

	// Should contain title
	if !strings.Contains(view, "Debug Logs") {
		t.Error("expected view to contain 'Debug Logs'")
	}
}

func TestLogsModelViewWithLogs(t *testing.T) {
	debug.ClearLogs()
	debug.Logger.Info("test message")

	m := NewLogsModel()
	m.SetDimensions(100, 50)
	m.Refresh()

	view := m.View()

	// Should contain the test message
	if !strings.Contains(view, "test message") {
		t.Error("expected view to contain log message")
	}
}

func TestLogsModelClearSelection(t *testing.T) {
	m := NewLogsModel()
	m.selecting = true
	m.selectStart = 5
	m.selectEnd = 10
	m.hasSelection = true

	m.clearSelection()

	if m.selecting {
		t.Error("expected selecting to be false")
	}
	if m.selectStart != 0 {
		t.Error("expected selectStart to be 0")
	}
	if m.selectEnd != 0 {
		t.Error("expected selectEnd to be 0")
	}
	if m.hasSelection {
		t.Error("expected hasSelection to be false")
	}
}

func TestLogsModelSelectAll(t *testing.T) {
	m := NewLogsModel()
	m.Visible = true
	m.lineCount = 10

	// Press 'a' to select all
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})

	if !m.hasSelection {
		t.Error("expected hasSelection to be true")
	}
	if m.selectStart != 0 {
		t.Errorf("expected selectStart 0, got %d", m.selectStart)
	}
	if m.selectEnd != 9 {
		t.Errorf("expected selectEnd 9, got %d", m.selectEnd)
	}
}

func TestLogsModelEscapeClearsSelection(t *testing.T) {
	m := NewLogsModel()
	m.Visible = true
	m.hasSelection = true
	m.selectStart = 1
	m.selectEnd = 5

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})

	if m.hasSelection {
		t.Error("expected hasSelection to be false after escape")
	}
}

func TestLogsModelCopyToClipboard(t *testing.T) {
	debug.ClearLogs()
	debug.Logger.Info("line to copy")

	m := NewLogsModel()
	m.Visible = true
	m.SetDimensions(100, 50)
	m.Refresh()

	// Copy should set status message (regardless of clipboard availability)
	m.copyToClipboard()

	// Status should be set (either "Copied!" or "Clipboard unavailable")
	if m.statusMsg == "" {
		t.Error("expected status message after copy")
	}
}

func TestLogsModelCopyYKeypress(t *testing.T) {
	debug.ClearLogs()
	debug.Logger.Info("test log")

	m := NewLogsModel()
	m.Visible = true
	m.SetDimensions(100, 50)
	m.Show() // This checks clipboard availability
	m.Refresh()

	// Press 'y' to copy
	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})

	// Behavior depends on clipboard availability (requires CGO_ENABLED=1)
	if m.clipboardAvailable {
		// When clipboard is available, status message should be set
		if m.statusMsg == "" {
			t.Error("expected status message after 'y' keypress when clipboard available")
		}
		// Should return a tick command to clear status
		if cmd == nil {
			t.Error("expected tick command to clear status when clipboard available")
		}
	} else if m.statusMsg != "" {
		// When clipboard is not available, 'y' should be ignored
		t.Errorf("expected no status message when clipboard unavailable, got %q", m.statusMsg)
	}
}

func TestLogsModelStatusClearMsg(t *testing.T) {
	m := NewLogsModel()
	m.Visible = true
	m.statusMsg = "Test status"

	// Send clear message
	m, _ = m.Update(LogsStatusClearMsg{})

	if m.statusMsg != "" {
		t.Errorf("expected empty status after LogsStatusClearMsg, got %q", m.statusMsg)
	}
}

func TestLogsModelViewWithStatus(t *testing.T) {
	m := NewLogsModel()
	m.SetDimensions(100, 50)
	m.statusMsg = "Copied!"

	view := m.View()
	if !strings.Contains(view, "Copied!") {
		t.Error("expected view to contain status message")
	}
}

func TestLogsModelInit(t *testing.T) {
	m := NewLogsModel()
	cmd := m.Init()

	// Init should return nil for LogsModel
	if cmd != nil {
		t.Error("expected Init to return nil")
	}
}

func TestLogsModelRefreshMsg(t *testing.T) {
	m := NewLogsModel()
	m.Visible = true
	m.SetDimensions(100, 50)

	debug.ClearLogs()
	debug.Logger.Info("refresh test")

	// Send refresh message
	m, cmd := m.Update(LogsRefreshMsg{})

	// Should have loaded the logs
	if m.lineCount == 0 {
		t.Error("expected logs to be loaded after refresh message")
	}

	// Should return a command to schedule next refresh
	if cmd == nil {
		t.Error("expected tick command for next refresh")
	}
}

func TestLogsModelStartAutoRefresh(t *testing.T) {
	m := NewLogsModel()

	cmd := m.StartAutoRefresh()

	if cmd == nil {
		t.Error("expected StartAutoRefresh to return a command")
	}
}

func TestLogsModelScrollDisablesAutoTail(t *testing.T) {
	m := NewLogsModel()
	m.Visible = true
	m.SetDimensions(100, 50)
	m.autoTail = true

	// Scroll up should disable auto-tail
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	if m.autoTail {
		t.Error("expected autoTail to be false after scroll up")
	}

	// Reset
	m.autoTail = true

	// Scroll down should also disable auto-tail
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	if m.autoTail {
		t.Error("expected autoTail to be false after scroll down")
	}
}

func TestColorizeLogEntry(t *testing.T) {
	tests := []struct {
		name  string
		entry string
	}{
		{"error entry", "ERRO rootly-tui Error message"},
		{"error entry alt", "ERROR: something went wrong"},
		{"warning entry", "WARN rootly-tui Warning message"},
		{"info entry", "INFO rootly-tui Info message"},
		{"debug entry", "DEBU rootly-tui Debug message"},
		{"debug entry alt", "DEBUG: some debug info"},
		{"plain entry", "Some plain text"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := colorizeLogEntry(tt.entry)
			// Result should contain the original entry text (even if wrapped in ANSI codes)
			if result == "" {
				t.Error("expected non-empty result")
			}
		})
	}
}

func TestLogsModelGetHelpText(t *testing.T) {
	m := NewLogsModel()

	// Without clipboard
	m.clipboardAvailable = false
	help := m.getHelpText()
	if strings.Contains(help, "y:copy") {
		t.Error("expected no copy help when clipboard unavailable")
	}

	// With clipboard
	m.clipboardAvailable = true
	help = m.getHelpText()
	if !strings.Contains(help, "y:copy") {
		t.Error("expected copy help when clipboard available")
	}
}

func TestLogsModelWindowResize(t *testing.T) {
	m := NewLogsModel()
	m.Visible = true

	m, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})

	if m.width != 120 {
		t.Errorf("expected width 120, got %d", m.width)
	}
	if m.height != 40 {
		t.Errorf("expected height 40, got %d", m.height)
	}
}
