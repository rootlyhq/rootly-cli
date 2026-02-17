package views

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// Note: TestMain in help_test.go sets i18n.LangEnglish for all tests in this package

func TestNewSetupModel(t *testing.T) {
	m := NewSetupModel()

	// Default should be on connection panel, endpoint field
	if m.ActivePanel() != PanelConnection {
		t.Errorf("expected active panel to be connection, got %v", m.ActivePanel())
	}

	if m.FocusIndex() != FieldEndpoint {
		t.Errorf("expected focus on endpoint, got %v", m.FocusIndex())
	}

	if m.ButtonIndex() != 0 {
		t.Errorf("expected button index 0, got %d", m.ButtonIndex())
	}

	if m.IsTesting() {
		t.Error("expected testing to be false")
	}
}

func TestSetupModelInit(t *testing.T) {
	m := NewSetupModel()
	cmd := m.Init()

	if cmd == nil {
		t.Error("expected non-nil command from Init")
	}
}

func TestSetupModelPanelSwitch(t *testing.T) {
	m := NewSetupModel()

	// Initially on connection panel
	if m.ActivePanel() != PanelConnection {
		t.Error("expected initial panel to be connection")
	}

	// Tab switches to config panel
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	if m.ActivePanel() != PanelConfig {
		t.Error("expected config panel after tab")
	}

	// Tab switches back to connection panel
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	if m.ActivePanel() != PanelConnection {
		t.Error("expected connection panel after second tab")
	}
}

func TestSetupModelConnectionPanelNavigation(t *testing.T) {
	m := NewSetupModel()

	// Down moves to API key
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	if m.FocusIndex() != FieldAPIKey {
		t.Errorf("expected focus on API key after down, got %v", m.FocusIndex())
	}

	// Down moves to buttons
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	if m.FocusIndex() != FieldButtons {
		t.Errorf("expected focus on buttons after down, got %v", m.FocusIndex())
	}

	// Down wraps to endpoint
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	if m.FocusIndex() != FieldEndpoint {
		t.Errorf("expected focus on endpoint after wrap, got %v", m.FocusIndex())
	}

	// Up goes to buttons
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyUp})
	if m.FocusIndex() != FieldButtons {
		t.Errorf("expected focus on buttons after up, got %v", m.FocusIndex())
	}
}

func TestSetupModelConfigPanelNavigation(t *testing.T) {
	m := NewSetupModel()

	// Switch to config panel
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})

	// Initially on timezone
	if m.FocusIndex() != FieldTimezone {
		t.Errorf("expected focus on timezone, got %v", m.FocusIndex())
	}

	// Down moves to language
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	if m.FocusIndex() != FieldLanguage {
		t.Errorf("expected focus on language after down, got %v", m.FocusIndex())
	}

	// Down moves to layout
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	if m.FocusIndex() != FieldLayout {
		t.Errorf("expected focus on layout after down, got %v", m.FocusIndex())
	}

	// Down moves to button
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	if m.FocusIndex() != FieldButtons {
		t.Errorf("expected focus on button after down, got %v", m.FocusIndex())
	}

	// Down wraps to timezone
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	if m.FocusIndex() != FieldTimezone {
		t.Errorf("expected focus on timezone after wrap, got %v", m.FocusIndex())
	}
}

func TestSetupModelConnectionButtonNavigation(t *testing.T) {
	m := NewSetupModel()

	// Navigate to buttons (down twice)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})

	// Should be at buttons with index 0 (Test button)
	if m.FocusIndex() != FieldButtons {
		t.Errorf("expected focus on buttons, got %v", m.FocusIndex())
	}
	if m.ButtonIndex() != 0 {
		t.Errorf("expected button index 0, got %d", m.ButtonIndex())
	}

	// Right moves to save button
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRight})
	if m.ButtonIndex() != 1 {
		t.Errorf("expected button index 1 after right, got %d", m.ButtonIndex())
	}

	// Right at end stays at end
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRight})
	if m.ButtonIndex() != 1 {
		t.Errorf("expected button index to stay 1, got %d", m.ButtonIndex())
	}

	// Left moves back
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyLeft})
	if m.ButtonIndex() != 0 {
		t.Errorf("expected button index 0 after left, got %d", m.ButtonIndex())
	}
}

func TestSetupModelEnterMovesToNextInConnectionPanel(t *testing.T) {
	m := NewSetupModel()

	// Enter on endpoint moves to API key
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m.FocusIndex() != FieldAPIKey {
		t.Errorf("expected focus on API key after enter, got %v", m.FocusIndex())
	}

	// Enter on API key moves to buttons
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m.FocusIndex() != FieldButtons {
		t.Errorf("expected focus on buttons after enter, got %v", m.FocusIndex())
	}
}

func TestSetupModelEnterMovesToNextInConfigPanel(t *testing.T) {
	m := NewSetupModel()

	// Switch to config panel
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})

	// Enter on timezone moves to language
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m.FocusIndex() != FieldLanguage {
		t.Errorf("expected focus on language after enter, got %v", m.FocusIndex())
	}

	// Enter on language moves to layout
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m.FocusIndex() != FieldLayout {
		t.Errorf("expected focus on layout after enter, got %v", m.FocusIndex())
	}

	// Enter on layout moves to button
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m.FocusIndex() != FieldButtons {
		t.Errorf("expected focus on button after enter, got %v", m.FocusIndex())
	}
}

func TestSetupModelUpdateIgnoredWhileTesting(t *testing.T) {
	m := NewSetupModel()
	m.SetTesting(true)

	// Keys should be ignored while testing
	initialPanel := m.ActivePanel()
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	if m.ActivePanel() != initialPanel {
		t.Error("expected panel to stay same while testing")
	}
}

func TestSetupModelUpdateWindowSize(t *testing.T) {
	m := NewSetupModel()

	m, _ = m.Update(tea.WindowSizeMsg{Width: 100, Height: 50})

	// Window size should be stored (test via View rendering)
	view := m.View()
	if view == "" {
		t.Error("expected non-empty view after window size")
	}
}

func TestSetupModelHandleValidationResult(t *testing.T) {
	m := NewSetupModel()
	m.SetTesting(true)

	// Success case
	m.HandleValidationResult(APIKeyValidatedMsg{Valid: true})

	if m.IsTesting() {
		t.Error("expected testing to be false after validation")
	}

	// Error case
	m.SetTesting(true)
	m.HandleValidationResult(APIKeyValidatedMsg{Valid: false, Error: "invalid key"})

	if m.IsTesting() {
		t.Error("expected testing to be false after validation")
	}
}

func TestSetupModelIsTesting(t *testing.T) {
	m := NewSetupModel()

	if m.IsTesting() {
		t.Error("expected IsTesting to be false initially")
	}

	m.SetTesting(true)
	if !m.IsTesting() {
		t.Error("expected IsTesting to be true")
	}
}

func TestSetupModelSetTesting(t *testing.T) {
	m := NewSetupModel()

	m.SetTesting(true)
	if !m.IsTesting() {
		t.Error("expected testing to be true")
	}

	m.SetTesting(false)
	if m.IsTesting() {
		t.Error("expected testing to be false")
	}
}

func TestSetupModelView(t *testing.T) {
	m := NewSetupModel()
	m.SetDimensions(150, 50)

	view := m.View()

	// Should contain key elements for two-panel layout
	if !strings.Contains(view, "Connection") {
		t.Error("expected view to contain Connection panel title")
	}
	if !strings.Contains(view, "Preferences") {
		t.Error("expected view to contain Preferences panel title")
	}
	if !strings.Contains(view, "API Endpoint") {
		t.Error("expected view to contain API Endpoint label")
	}
	if !strings.Contains(view, "API Key") {
		t.Error("expected view to contain API Key label")
	}
	if !strings.Contains(view, "Timezone") {
		t.Error("expected view to contain Timezone label")
	}
	if !strings.Contains(view, "Language") {
		t.Error("expected view to contain Language label")
	}
	if !strings.Contains(view, "Layout") {
		t.Error("expected view to contain Layout label")
	}
}

func TestSetupModelViewWithTestResult(t *testing.T) {
	m := NewSetupModel()
	m.SetDimensions(150, 50)

	// Success state - set via HandleValidationResult
	m.HandleValidationResult(APIKeyValidatedMsg{Valid: true})
	view := m.View()
	if !strings.Contains(view, "successful") {
		t.Error("expected view to contain success message")
	}

	// Error state
	m.HandleValidationResult(APIKeyValidatedMsg{Valid: false, Error: "connection failed"})
	view = m.View()
	if !strings.Contains(view, "Error") {
		t.Error("expected view to contain error message")
	}
}

func TestSetupModelViewWhileTesting(t *testing.T) {
	m := NewSetupModel()
	m.SetDimensions(150, 50)
	m.SetTesting(true)

	view := m.View()
	if !strings.Contains(view, "Testing") {
		t.Error("expected view to contain testing message")
	}
}

func TestSetupModelViewWithoutDimensions(t *testing.T) {
	m := NewSetupModel()
	// Don't set dimensions

	view := m.View()

	// Should still render without centering
	if view == "" {
		t.Error("expected non-empty view")
	}
}

func TestSetupModelSetDimensions(t *testing.T) {
	m := NewSetupModel()

	m.SetDimensions(120, 60)

	// Verify via view rendering (dimensions are used for centering)
	view := m.View()
	if view == "" {
		t.Error("expected non-empty view after setting dimensions")
	}
}

func TestSetupModelTimezoneNavigation(t *testing.T) {
	m := NewSetupModel()

	// Switch to config panel
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})

	// Should be on timezone field
	if m.FocusIndex() != FieldTimezone {
		t.Errorf("expected focus on timezone, got %v", m.FocusIndex())
	}

	// Save initial index
	initialIndex := m.TimezoneIndex()

	// Move right (if there are more timezones)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRight})
	if m.TimezoneIndex() != initialIndex+1 && initialIndex < 100 {
		t.Errorf("expected timezone index to increase after right")
	}

	// Move left
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyLeft})
	if m.TimezoneIndex() != initialIndex {
		t.Errorf("expected timezone index %d after left, got %d", initialIndex, m.TimezoneIndex())
	}
}

func TestSetupModelLanguageNavigation(t *testing.T) {
	m := NewSetupModel()

	// Switch to config panel and navigate to language
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})

	// Should be on language field
	if m.FocusIndex() != FieldLanguage {
		t.Errorf("expected focus on language, got %v", m.FocusIndex())
	}

	// Start at first language
	initialIndex := m.LanguageIndex()

	// Move right
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRight})
	if m.LanguageIndex() != initialIndex+1 {
		t.Errorf("expected language index to increase after right")
	}

	// Move left
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyLeft})
	if m.LanguageIndex() != initialIndex {
		t.Errorf("expected language index %d after left, got %d", initialIndex, m.LanguageIndex())
	}
}

func TestSetupModelLayoutNavigation(t *testing.T) {
	m := NewSetupModel()

	// Switch to config panel and navigate to layout
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})

	// Should be on layout field
	if m.FocusIndex() != FieldLayout {
		t.Errorf("expected focus on layout, got %v", m.FocusIndex())
	}

	// Start at first layout (horizontal)
	if m.LayoutIndex() != 0 {
		t.Errorf("expected initial layout index 0, got %d", m.LayoutIndex())
	}

	// Move right
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRight})
	if m.LayoutIndex() != 1 {
		t.Errorf("expected layout index 1 after right, got %d", m.LayoutIndex())
	}

	// Move left
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyLeft})
	if m.LayoutIndex() != 0 {
		t.Errorf("expected layout index 0 after left, got %d", m.LayoutIndex())
	}
}

func TestSetupModelEnterOnTestButton(t *testing.T) {
	m := NewSetupModel()

	// Navigate to buttons
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})

	// Should be at buttons with test button focused
	if m.FocusIndex() != FieldButtons || m.ButtonIndex() != 0 {
		t.Fatal("expected to be at test button")
	}

	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if !m.IsTesting() {
		t.Error("expected testing to be true after pressing enter on test button")
	}
	if cmd == nil {
		t.Error("expected non-nil command")
	}
}

func TestSetupModelEnterOnSaveButtonWithoutSuccess(t *testing.T) {
	m := NewSetupModel()

	// Navigate to save button
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRight})

	// Should be at save button
	if m.ButtonIndex() != 1 {
		t.Fatal("expected to be at save button")
	}

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	// Should not save without successful test
	if cmd != nil {
		t.Error("expected nil command when test not successful")
	}
}

func TestSetupModelEnterOnSaveButtonWithSuccess(t *testing.T) {
	m := NewSetupModel()

	// Set test result to success
	m.HandleValidationResult(APIKeyValidatedMsg{Valid: true})

	// Navigate to save button
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRight})

	// Should be at save button
	if m.ButtonIndex() != 1 {
		t.Fatal("expected to be at save button")
	}

	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if cmd == nil {
		t.Error("expected non-nil command when saving with successful test")
	}
}

func TestSetupModelConnectionSaved(t *testing.T) {
	m := NewSetupModel()

	m.HandleConnectionSaved(ConnectionSavedMsg{Success: true})

	if !m.IsConnectionSaved() {
		t.Error("expected connection to be saved")
	}
}

func TestSetupModelPreferencesSaved(t *testing.T) {
	m := NewSetupModel()

	m.HandlePreferencesSaved(PreferencesSavedMsg{Success: true})

	if !m.IsConfigSaved() {
		t.Error("expected config/preferences to be saved")
	}
}

func TestSetupModelJKNavigation(t *testing.T) {
	m := NewSetupModel()

	// j moves down
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	if m.FocusIndex() != FieldAPIKey {
		t.Errorf("expected focus on API key after 'j', got %v", m.FocusIndex())
	}

	// k moves up
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	if m.FocusIndex() != FieldEndpoint {
		t.Errorf("expected focus on endpoint after 'k', got %v", m.FocusIndex())
	}
}

func TestSetupModelHLNavigation(t *testing.T) {
	m := NewSetupModel()

	// Switch to config panel for selector fields
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})

	initialTZ := m.TimezoneIndex()

	// 'l' moves right (increases index)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	if m.TimezoneIndex() != initialTZ+1 {
		t.Errorf("expected timezone index to increase after 'l'")
	}

	// 'h' moves left (decreases index)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})
	if m.TimezoneIndex() != initialTZ {
		t.Errorf("expected timezone index to decrease after 'h'")
	}
}
