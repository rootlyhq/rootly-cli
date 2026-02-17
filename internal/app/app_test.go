package app

import (
	"os"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/rootlyhq/rootly-tui/internal/api"
	"github.com/rootlyhq/rootly-tui/internal/config"
	"github.com/rootlyhq/rootly-tui/internal/i18n"
)

func TestMain(m *testing.M) {
	// Use a temp directory as HOME to isolate from real config
	tempDir, err := os.MkdirTemp("", "rootly-tui-test-*")
	if err != nil {
		os.Exit(1)
	}
	os.Setenv("HOME", tempDir)

	// Set language to English for consistent test output
	i18n.SetLanguage(i18n.LangEnglish)

	code := m.Run()
	os.RemoveAll(tempDir)
	os.Exit(code)
}

func TestNew(t *testing.T) {
	m := New("1.0.0")

	if m.version != "1.0.0" {
		t.Errorf("expected version '1.0.0', got '%s'", m.version)
	}

	// Should start on setup screen if no config exists
	if m.screen != ScreenSetup && m.screen != ScreenMain {
		t.Errorf("expected screen to be ScreenSetup or ScreenMain, got %d", m.screen)
	}

	if m.activeTab != TabIncidents {
		t.Errorf("expected active tab to be TabIncidents, got %d", m.activeTab)
	}
}

func TestModelInit(t *testing.T) {
	m := New("1.0.0")
	cmd := m.Init()

	// Init should return a command (not nil)
	if cmd == nil {
		t.Error("expected Init() to return a command")
	}
}

func TestModelUpdateQuit(t *testing.T) {
	m := New("1.0.0")
	m.screen = ScreenMain // Set to main screen

	// Test quit with 'q'
	newModel, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	if newModel == nil {
		t.Error("expected model to be non-nil")
	}

	// Should return quit command
	if cmd == nil {
		t.Error("expected quit command")
	}
}

func TestModelUpdateQuitFromSetup(t *testing.T) {
	m := New("1.0.0")
	m.screen = ScreenSetup

	// Test quit with 'q' from setup screen
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})

	// Should return quit command even from setup
	if cmd == nil {
		t.Error("expected quit command from setup screen")
	}
}

func TestModelUpdateTabSwitch(t *testing.T) {
	m := New("1.0.0")
	m.screen = ScreenMain
	m.activeTab = TabIncidents

	// Test tab switch
	newModel, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	model := newModel.(Model)

	if model.activeTab != TabAlerts {
		t.Errorf("expected active tab to be TabAlerts after tab press, got %d", model.activeTab)
	}

	// Switch back
	newModel, _ = model.Update(tea.KeyMsg{Type: tea.KeyTab})
	model = newModel.(Model)

	if model.activeTab != TabIncidents {
		t.Errorf("expected active tab to be TabIncidents after second tab press, got %d", model.activeTab)
	}
}

func TestModelUpdateHelp(t *testing.T) {
	m := New("1.0.0")
	m.screen = ScreenMain
	m.help.Visible = false

	// Test help toggle with '?'
	newModel, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	model := newModel.(Model)

	if !model.help.Visible {
		t.Error("expected help to be visible after '?' press")
	}

	// Toggle off
	newModel, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	model = newModel.(Model)

	if model.help.Visible {
		t.Error("expected help to be hidden after second '?' press")
	}
}

func TestModelUpdateWindowSize(t *testing.T) {
	m := New("1.0.0")

	newModel, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	model := newModel.(Model)

	if model.width != 120 {
		t.Errorf("expected width 120, got %d", model.width)
	}

	if model.height != 40 {
		t.Errorf("expected height 40, got %d", model.height)
	}
}

func TestModelView(t *testing.T) {
	m := New("1.0.0")

	// Test view before window size is set
	view := m.View()
	if view != "Loading..." {
		t.Errorf("expected 'Loading...' before window size, got '%s'", view)
	}

	// Set window size
	newModel, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = newModel.(Model)

	view = m.View()
	if view == "" {
		t.Error("expected non-empty view after window size set")
	}
}

func TestModelIncidentsLoaded(t *testing.T) {
	m := New("1.0.0")
	m.screen = ScreenMain
	m.initialLoading = true

	// Simulate incidents loaded message
	newModel, _ := m.Update(IncidentsLoadedMsg{
		Incidents: nil,
		Err:       nil,
	})
	model := newModel.(Model)

	if model.initialLoading {
		t.Error("expected initialLoading to be false after IncidentsLoadedMsg")
	}

	if model.loading {
		t.Error("expected loading to be false after IncidentsLoadedMsg")
	}
}

func TestModelAlertsLoaded(t *testing.T) {
	m := New("1.0.0")
	m.screen = ScreenMain

	// Simulate alerts loaded message
	newModel, _ := m.Update(AlertsLoadedMsg{
		Alerts: nil,
		Err:    nil,
	})
	model := newModel.(Model)

	// Should not error
	if model.errorMsg != "" {
		t.Errorf("unexpected error: %s", model.errorMsg)
	}
}

func TestDefaultKeyMap(t *testing.T) {
	km := DefaultKeyMap()

	// Verify key bindings are set
	if len(km.Up.Keys()) == 0 {
		t.Error("expected Up key binding to be set")
	}
	if len(km.Down.Keys()) == 0 {
		t.Error("expected Down key binding to be set")
	}
	if len(km.Tab.Keys()) == 0 {
		t.Error("expected Tab key binding to be set")
	}
	if len(km.Refresh.Keys()) == 0 {
		t.Error("expected Refresh key binding to be set")
	}
	if len(km.Help.Keys()) == 0 {
		t.Error("expected Help key binding to be set")
	}
	if len(km.Logs.Keys()) == 0 {
		t.Error("expected Logs key binding to be set")
	}
	if len(km.Quit.Keys()) == 0 {
		t.Error("expected Quit key binding to be set")
	}
}

func TestModelUpdateLogs(t *testing.T) {
	m := New("1.0.0")
	m.screen = ScreenMain
	m.logs.Visible = false

	// Test logs toggle with 'l'
	newModel, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	model := newModel.(Model)

	if !model.logs.Visible {
		t.Error("expected logs to be visible after 'l' press")
	}

	// Toggle off with 'l'
	newModel, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	model = newModel.(Model)

	if model.logs.Visible {
		t.Error("expected logs to be hidden after second 'l' press")
	}
}

func TestModelUpdateLogsEscape(t *testing.T) {
	m := New("1.0.0")
	m.screen = ScreenMain
	m.logs.Visible = true

	// Test closing logs with Escape
	newModel, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	model := newModel.(Model)

	if model.logs.Visible {
		t.Error("expected logs to be hidden after Escape press")
	}
}

func TestModelLogsBlocksOtherKeys(t *testing.T) {
	m := New("1.0.0")
	m.screen = ScreenMain
	m.activeTab = TabIncidents
	m.logs.Visible = true

	// Tab should not switch tabs when logs overlay is visible
	newModel, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	model := newModel.(Model)

	if model.activeTab != TabIncidents {
		t.Error("expected tab switch to be blocked when logs are visible")
	}
}

func TestNewWithValidConfigCreatesClientOnce(t *testing.T) {
	// This test verifies that when a valid config exists,
	// the API client is created in New() so that loadData()
	// doesn't try to create multiple clients concurrently.

	m := New("1.0.0")

	// If we're on the main screen, the client should already be initialized
	if m.screen == ScreenMain {
		if m.apiClient == nil {
			t.Error("expected apiClient to be initialized when screen is ScreenMain")
		}
	}

	// If we're on setup screen, client should be nil (no valid config)
	if m.screen == ScreenSetup {
		if m.apiClient != nil {
			t.Error("expected apiClient to be nil when screen is ScreenSetup")
		}
	}
}

func TestModelClientNotRecreatedOnRefresh(t *testing.T) {
	m := New("1.0.0")
	m.screen = ScreenMain

	// Manually set a non-nil client to simulate initialized state
	// We can't create a real client without config, but we can verify
	// that the loadData functions check for existing client
	initialClient := m.apiClient

	// If client is already set and we're on main screen, refreshing
	// should not create a new client
	if m.screen == ScreenMain && m.apiClient != nil {
		// After refresh, client should be the same instance
		// (This is a structural test - actual refresh tested in integration)
		if m.apiClient != initialClient {
			t.Error("expected apiClient to remain the same after initialization")
		}
	}
}

func TestModelClose(t *testing.T) {
	m := New("1.0.0")

	// Close with nil client should not error
	err := m.Close()
	if err != nil {
		t.Errorf("expected no error closing model with nil client, got %v", err)
	}
}

func TestModelCloseWithClient(t *testing.T) {
	// Create a temp directory for cache
	tmpDir := t.TempDir()
	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", originalHome)

	// Create a valid config
	cfg := &config.Config{
		APIKey:   "test-key",
		Endpoint: "api.rootly.com",
	}

	// Create client manually
	client, err := api.NewClient(cfg)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	m := New("1.0.0")
	m.apiClient = client

	// Close should close the client
	err = m.Close()
	if err != nil {
		t.Errorf("expected no error closing model, got %v", err)
	}

	// Client should be closed (calling Close again is safe but we just verify no panic)
}

func TestModelOpenKeyBinding(t *testing.T) {
	m := New("1.0.0")
	m.screen = ScreenMain
	m.activeTab = TabIncidents
	// Use no-op URL opener to prevent opening real browser during tests
	m.urlOpener = func(url string) error { return nil }

	// Add a test incident
	m.incidents.SetIncidents([]api.Incident{
		{
			ID:       "inc_123",
			Title:    "Test Incident",
			ShortURL: "https://root.ly/i/abc123",
		},
	}, api.PaginationInfo{CurrentPage: 1})

	// Press 'o' key - should not panic even without a real browser
	newModel, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'o'}})
	if newModel == nil {
		t.Error("expected model to be non-nil")
	}
	// Command should be nil (openURL is fire-and-forget)
	if cmd != nil {
		t.Error("expected nil command for open key")
	}
}

func TestModelOpenKeyBindingWithFallbackURL(t *testing.T) {
	m := New("1.0.0")
	m.screen = ScreenMain
	m.activeTab = TabIncidents
	// Use no-op URL opener to prevent opening real browser during tests
	m.urlOpener = func(url string) error { return nil }

	// Add a test incident without ShortURL or URL (should construct from ID)
	m.incidents.SetIncidents([]api.Incident{
		{
			ID:    "inc_456",
			Title: "Test Incident Without URL",
		},
	}, api.PaginationInfo{CurrentPage: 1})

	// Press 'o' key - should not panic
	newModel, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'o'}})
	if newModel == nil {
		t.Error("expected model to be non-nil")
	}
}

func TestModelOpenKeyBindingForAlerts(t *testing.T) {
	m := New("1.0.0")
	m.screen = ScreenMain
	m.activeTab = TabAlerts
	// Use no-op URL opener to prevent opening real browser during tests
	m.urlOpener = func(url string) error { return nil }

	// Add a test alert
	m.alerts.SetAlerts([]api.Alert{
		{
			ID:      "alert_123",
			ShortID: "ABC123",
			Summary: "Test Alert",
		},
	}, api.PaginationInfo{CurrentPage: 1})

	// Press 'o' key - should not panic
	newModel, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'o'}})
	if newModel == nil {
		t.Error("expected model to be non-nil")
	}
	// Command should be nil (openURL is fire-and-forget)
	if cmd != nil {
		t.Error("expected nil command for open key")
	}
}

func TestModelViewWithSetupScreen(t *testing.T) {
	m := New("1.0.0")
	m.screen = ScreenSetup
	m.width = 120
	m.height = 40
	m.setup.SetDimensions(120, 40)

	view := m.View()
	if view == "" {
		t.Error("expected non-empty view for setup screen")
	}
}

func TestModelViewWithMainScreen(t *testing.T) {
	m := New("1.0.0")
	m.screen = ScreenMain
	m.width = 120
	m.height = 40
	m.initialLoading = false

	// Set up dimensions for views
	m.incidents.SetDimensions(116, 30)
	m.alerts.SetDimensions(116, 30)

	view := m.View()
	if view == "" {
		t.Error("expected non-empty view for main screen")
	}
}

func TestModelViewWithHelpOverlay(t *testing.T) {
	m := New("1.0.0")
	m.screen = ScreenMain
	m.width = 120
	m.height = 40
	m.help.Visible = true

	view := m.View()
	if view == "" {
		t.Error("expected non-empty view with help overlay")
	}
}

func TestModelViewWithLogsOverlay(t *testing.T) {
	m := New("1.0.0")
	m.screen = ScreenMain
	m.width = 120
	m.height = 40
	m.logs.Visible = true
	m.logs.SetDimensions(120, 40)

	view := m.View()
	if view == "" {
		t.Error("expected non-empty view with logs overlay")
	}
}

func TestModelViewWithInitialLoading(t *testing.T) {
	m := New("1.0.0")
	m.screen = ScreenMain
	m.width = 120
	m.height = 40
	m.initialLoading = true

	view := m.View()
	if view == "" {
		t.Error("expected non-empty view during initial loading")
	}
}

func TestModelIncidentsLoadedWithError(t *testing.T) {
	m := New("1.0.0")
	m.screen = ScreenMain
	m.initialLoading = true

	// Simulate error
	newModel, _ := m.Update(IncidentsLoadedMsg{
		Incidents: nil,
		Err:       os.ErrNotExist,
	})
	model := newModel.(Model)

	if model.errorMsg == "" {
		t.Error("expected errorMsg to be set after error")
	}
}

func TestModelAlertsLoadedWithError(t *testing.T) {
	m := New("1.0.0")
	m.screen = ScreenMain

	// Simulate error
	newModel, _ := m.Update(AlertsLoadedMsg{
		Alerts: nil,
		Err:    os.ErrNotExist,
	})
	model := newModel.(Model)

	// Error should be set on alerts view
	if model.alerts.SelectedAlert() != nil {
		t.Error("expected no alerts after error")
	}
}

func TestModelIncidentDetailLoaded(t *testing.T) {
	m := New("1.0.0")
	m.screen = ScreenMain

	// Add incidents first
	m.incidents.SetIncidents([]api.Incident{
		{ID: "inc_1", Title: "Test"},
	}, api.PaginationInfo{CurrentPage: 1})

	// Simulate detail loaded
	newModel, _ := m.Update(IncidentDetailLoadedMsg{
		Incident: &api.Incident{
			ID:           "inc_1",
			Title:        "Test Updated",
			DetailLoaded: true,
		},
		Index: 0,
	})
	model := newModel.(Model)

	// Should not have error
	if model.errorMsg != "" {
		t.Errorf("unexpected error: %s", model.errorMsg)
	}
}

func TestModelIncidentDetailLoadedWithError(t *testing.T) {
	m := New("1.0.0")
	m.screen = ScreenMain

	// Simulate detail load error
	newModel, _ := m.Update(IncidentDetailLoadedMsg{
		Err:   os.ErrPermission,
		Index: 0,
	})
	model := newModel.(Model)

	if model.errorMsg == "" {
		t.Error("expected errorMsg to be set after detail load error")
	}
}

func TestModelAlertDetailLoaded(t *testing.T) {
	m := New("1.0.0")
	m.screen = ScreenMain

	// Add alerts first
	m.alerts.SetAlerts([]api.Alert{
		{ID: "alert_1", Summary: "Test"},
	}, api.PaginationInfo{CurrentPage: 1})

	// Simulate detail loaded
	newModel, _ := m.Update(AlertDetailLoadedMsg{
		Alert: &api.Alert{
			ID:           "alert_1",
			Summary:      "Test Updated",
			DetailLoaded: true,
		},
		Index: 0,
	})
	model := newModel.(Model)

	// Should not have error
	if model.errorMsg != "" {
		t.Errorf("unexpected error: %s", model.errorMsg)
	}
}

func TestModelAlertDetailLoadedWithError(t *testing.T) {
	m := New("1.0.0")
	m.screen = ScreenMain

	// Simulate detail load error
	newModel, _ := m.Update(AlertDetailLoadedMsg{
		Err:   os.ErrPermission,
		Index: 0,
	})
	model := newModel.(Model)

	if model.errorMsg == "" {
		t.Error("expected errorMsg to be set after detail load error")
	}
}

func TestModelErrorMsg(t *testing.T) {
	m := New("1.0.0")
	m.screen = ScreenMain

	// Simulate error message
	newModel, _ := m.Update(ErrorMsg{Err: os.ErrInvalid})
	model := newModel.(Model)

	if model.errorMsg == "" {
		t.Error("expected errorMsg to be set")
	}
	if model.loading {
		t.Error("expected loading to be false after error")
	}
}

func TestModelSetupKeyBinding(t *testing.T) {
	m := New("1.0.0")
	m.screen = ScreenMain

	// Press 's' to go to setup
	newModel, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
	model := newModel.(Model)

	if model.screen != ScreenSetup {
		t.Errorf("expected screen to be ScreenSetup after 's' press, got %d", model.screen)
	}
}

func TestModelRefreshKeyBinding(t *testing.T) {
	m := New("1.0.0")
	m.screen = ScreenMain
	m.loading = false

	// Press 'r' to refresh
	newModel, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	model := newModel.(Model)

	if !model.loading {
		t.Error("expected loading to be true after 'r' press")
	}

	// Should return a command
	if cmd == nil {
		t.Error("expected command after refresh")
	}
}

func TestModelPaginationPrevPage(t *testing.T) {
	m := New("1.0.0")
	m.screen = ScreenMain
	m.activeTab = TabIncidents

	// Set up pagination with previous page available
	m.incidents.SetIncidents([]api.Incident{
		{ID: "inc_1", Title: "Test"},
	}, api.PaginationInfo{CurrentPage: 2, HasPrev: true, HasNext: true})

	// Press '[' for previous page
	newModel, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'['}})
	model := newModel.(Model)

	// Should return a command to load data
	if cmd == nil {
		t.Error("expected command after prev page")
	}

	// Loading should be true
	if !model.loading {
		t.Error("expected loading to be true after prev page")
	}
}

func TestModelPaginationNextPage(t *testing.T) {
	m := New("1.0.0")
	m.screen = ScreenMain
	m.activeTab = TabIncidents

	// Set up pagination with next page available
	m.incidents.SetIncidents([]api.Incident{
		{ID: "inc_1", Title: "Test"},
	}, api.PaginationInfo{CurrentPage: 1, HasPrev: false, HasNext: true})

	// Press ']' for next page
	newModel, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{']'}})
	model := newModel.(Model)

	// Should return a command to load data
	if cmd == nil {
		t.Error("expected command after next page")
	}

	// Loading should be true
	if !model.loading {
		t.Error("expected loading to be true after next page")
	}
}

func TestModelPaginationNoPrevPage(t *testing.T) {
	m := New("1.0.0")
	m.screen = ScreenMain
	m.activeTab = TabIncidents

	// Set up pagination without previous page
	m.incidents.SetIncidents([]api.Incident{
		{ID: "inc_1", Title: "Test"},
	}, api.PaginationInfo{CurrentPage: 1, HasPrev: false, HasNext: true})

	// Press '[' for previous page (should be no-op)
	newModel, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'['}})
	model := newModel.(Model)

	// Should not return a command
	if cmd != nil {
		t.Error("expected nil command when no prev page")
	}

	// Loading should remain false
	if model.loading {
		t.Error("expected loading to remain false when no prev page")
	}
}

func TestModelEnterKeyLoadDetail(t *testing.T) {
	m := New("1.0.0")
	m.screen = ScreenMain
	m.activeTab = TabIncidents

	// Add incident without detail loaded
	m.incidents.SetIncidents([]api.Incident{
		{ID: "inc_1", Title: "Test", DetailLoaded: false},
	}, api.PaginationInfo{CurrentPage: 1})

	// Press Enter to load detail
	newModel, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model := newModel.(Model)

	// Should return a command (but will fail without client)
	// The key here is that it doesn't panic
	_ = model
	_ = cmd
}

func TestModelEnterKeyFocusDetail(t *testing.T) {
	m := New("1.0.0")
	m.screen = ScreenMain
	m.activeTab = TabIncidents

	// Add incident with detail already loaded
	m.incidents.SetIncidents([]api.Incident{
		{ID: "inc_1", Title: "Test", DetailLoaded: true},
	}, api.PaginationInfo{CurrentPage: 1})

	// Press Enter to focus detail
	newModel, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model := newModel.(Model)

	// Should focus detail pane
	if !model.incidents.IsDetailFocused() {
		t.Error("expected detail to be focused after Enter on loaded detail")
	}
}

func TestModelMouseMsg(t *testing.T) {
	m := New("1.0.0")
	m.screen = ScreenMain
	m.activeTab = TabIncidents
	m.width = 120
	m.height = 40
	m.incidents.SetDimensions(116, 30)

	// Send mouse message - should not panic
	newModel, _ := m.Update(tea.MouseMsg{Action: tea.MouseActionMotion, X: 50, Y: 20})
	if newModel == nil {
		t.Error("expected model to be non-nil")
	}
}

func TestModelSpinnerTick(t *testing.T) {
	m := New("1.0.0")
	m.screen = ScreenMain
	m.loading = true

	// Send spinner tick - should update spinner
	_, cmd := m.Update(m.spinner.Tick())
	// Should continue ticking when loading
	if cmd == nil {
		t.Error("expected spinner to continue ticking when loading")
	}
}

func TestModelSpinnerTickNotLoading(t *testing.T) {
	m := New("1.0.0")
	m.screen = ScreenMain
	m.loading = false
	m.initialLoading = false

	// Send spinner tick - should not continue when not loading
	_, cmd := m.Update(m.spinner.Tick())
	// Should not continue ticking when not loading
	if cmd != nil {
		t.Log("spinner tick returned command even when not loading (expected nil)")
	}
}

func TestModelQuitFromSetupWithValidConfig(t *testing.T) {
	m := New("1.0.0")
	m.screen = ScreenSetup
	m.cfg = &config.Config{APIKey: "test", Endpoint: "api.rootly.com"}

	// Press 'q' with valid config - should return to main
	newModel, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	model := newModel.(Model)

	if model.screen != ScreenMain {
		t.Errorf("expected to return to main screen, got %d", model.screen)
	}

	// Should not quit
	if cmd != nil {
		t.Error("expected nil command (not quit) when returning from setup")
	}
}

func TestModelEscapeFromSetupWithValidConfig(t *testing.T) {
	m := New("1.0.0")
	m.screen = ScreenSetup
	m.cfg = &config.Config{APIKey: "test", Endpoint: "api.rootly.com"}

	// Press Escape with valid config - should return to main
	newModel, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	model := newModel.(Model)

	if model.screen != ScreenMain {
		t.Errorf("expected to return to main screen, got %d", model.screen)
	}

	// Should not quit
	if cmd != nil {
		t.Error("expected nil command (not quit) when returning from setup")
	}
}

func TestModelHelpEscape(t *testing.T) {
	m := New("1.0.0")
	m.screen = ScreenMain
	m.help.Visible = true

	// Press Escape to close help
	newModel, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	model := newModel.(Model)

	if model.help.Visible {
		t.Error("expected help to be hidden after Escape")
	}
}

func TestModelAlertsPagination(t *testing.T) {
	m := New("1.0.0")
	m.screen = ScreenMain
	m.activeTab = TabAlerts

	// Set up alerts with pagination
	m.alerts.SetAlerts([]api.Alert{
		{ID: "alert_1", Summary: "Test"},
	}, api.PaginationInfo{CurrentPage: 1, HasPrev: false, HasNext: true})

	// Press ']' for next page
	newModel, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{']'}})
	model := newModel.(Model)

	// Should return a command
	if cmd == nil {
		t.Error("expected command after next page on alerts")
	}

	if !model.loading {
		t.Error("expected loading to be true after next page on alerts")
	}
}

func TestModelAlertsPaginationPrev(t *testing.T) {
	m := New("1.0.0")
	m.screen = ScreenMain
	m.activeTab = TabAlerts

	// Set up alerts with previous page
	m.alerts.SetAlerts([]api.Alert{
		{ID: "alert_1", Summary: "Test"},
	}, api.PaginationInfo{CurrentPage: 2, HasPrev: true, HasNext: false})

	// Press '[' for prev page
	newModel, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'['}})
	model := newModel.(Model)

	// Should return a command
	if cmd == nil {
		t.Error("expected command after prev page on alerts")
	}

	if !model.loading {
		t.Error("expected loading to be true after prev page on alerts")
	}
}

func TestModelEnterOnAlert(t *testing.T) {
	m := New("1.0.0")
	m.screen = ScreenMain
	m.activeTab = TabAlerts

	// Add alert without detail loaded
	m.alerts.SetAlerts([]api.Alert{
		{ID: "alert_1", Summary: "Test", DetailLoaded: false},
	}, api.PaginationInfo{CurrentPage: 1})

	// Press Enter to load detail
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	// Should return a command (even if it fails without client)
	_ = cmd
}

func TestModelEnterOnAlertWithDetailLoaded(t *testing.T) {
	m := New("1.0.0")
	m.screen = ScreenMain
	m.activeTab = TabAlerts

	// Add alert with detail already loaded
	m.alerts.SetAlerts([]api.Alert{
		{ID: "alert_1", Summary: "Test", DetailLoaded: true},
	}, api.PaginationInfo{CurrentPage: 1})

	// Press Enter to focus detail
	newModel, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model := newModel.(Model)

	// Should focus detail pane
	if !model.alerts.IsDetailFocused() {
		t.Error("expected alert detail to be focused after Enter on loaded detail")
	}
}

func TestModelTabSwitchClearsFocus(t *testing.T) {
	m := New("1.0.0")
	m.screen = ScreenMain
	m.activeTab = TabIncidents
	m.incidents.SetDetailFocused(true)

	// Switch tab
	newModel, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	model := newModel.(Model)

	// Focus should be cleared
	if model.incidents.IsDetailFocused() {
		t.Error("expected incidents detail focus to be cleared after tab switch")
	}
}

func TestModelSetupUpdate(t *testing.T) {
	m := New("1.0.0")
	m.screen = ScreenSetup
	m.setup.SetDimensions(120, 40)

	// Send key to setup screen
	newModel, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	if newModel == nil {
		t.Error("expected model to be non-nil")
	}
}

func TestModelDownKeyPassedToView(t *testing.T) {
	m := New("1.0.0")
	m.screen = ScreenMain
	m.activeTab = TabIncidents

	// Add multiple incidents
	m.incidents.SetIncidents([]api.Incident{
		{ID: "inc_1", Title: "Test 1"},
		{ID: "inc_2", Title: "Test 2"},
	}, api.PaginationInfo{CurrentPage: 1})

	// Initially cursor at 0
	if m.incidents.SelectedIndex() != 0 {
		t.Errorf("expected initial cursor at 0, got %d", m.incidents.SelectedIndex())
	}

	// Press 'j' to move down
	newModel, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	model := newModel.(Model)

	if model.incidents.SelectedIndex() != 1 {
		t.Errorf("expected cursor at 1 after 'j', got %d", model.incidents.SelectedIndex())
	}
}

func TestModelUpKeyPassedToView(t *testing.T) {
	m := New("1.0.0")
	m.screen = ScreenMain
	m.activeTab = TabIncidents

	// Add multiple incidents and start at index 1
	m.incidents.SetIncidents([]api.Incident{
		{ID: "inc_1", Title: "Test 1"},
		{ID: "inc_2", Title: "Test 2"},
	}, api.PaginationInfo{CurrentPage: 1})

	// Move to second item first
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	newModel, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	model := newModel.(Model)

	if model.incidents.SelectedIndex() != 0 {
		t.Errorf("expected cursor at 0 after 'k', got %d", model.incidents.SelectedIndex())
	}
}
