package views

import (
	"context"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/rootlyhq/rootly-tui/internal/api"
	"github.com/rootlyhq/rootly-tui/internal/config"
	"github.com/rootlyhq/rootly-tui/internal/i18n"
	"github.com/rootlyhq/rootly-tui/internal/styles"
)

// Panel identifiers
type Panel int

const (
	PanelConnection Panel = iota
	PanelConfig
)

// Connection panel fields
type ConnectionField int

const (
	ConnFieldEndpoint ConnectionField = iota
	ConnFieldAPIKey
	ConnFieldButtons
)

// Config panel fields
type ConfigField int

const (
	ConfigFieldTimezone ConfigField = iota
	ConfigFieldLanguage
	ConfigFieldLayout
	ConfigFieldButton
)

const (
	testResultSuccess = "success"
	testResultError   = "error"
)

// SetupField is kept for backward compatibility with tests
type SetupField int

const (
	FieldEndpoint SetupField = iota
	FieldAPIKey
	FieldTimezone
	FieldLanguage
	FieldLayout
	FieldButtons
)

type SetupModel struct {
	// Connection panel
	endpoint   textinput.Model
	apiKey     textinput.Model
	connFocus  ConnectionField
	connButton int // 0 = Test, 1 = Save
	testing    bool
	testResult string
	testError  string
	connSaved  bool
	connSaving bool

	// Config panel
	timezones     []string
	timezoneIndex int
	languages     []string
	languageIndex int
	layouts       []string
	layoutIndex   int
	configFocus   ConfigField
	configSaved   bool
	configSaving  bool

	// Shared
	activePanel Panel
	spinner     spinner.Model
	width       int
	height      int

	// Track original values to detect changes
	originalTimezoneIndex int
	originalLanguageIndex int
	originalLayoutIndex   int
}

type APIKeyValidatedMsg struct {
	Valid bool
	Error string
}

type ConfigSavedMsg struct {
	Success bool
	Error   string
}

type ConnectionSavedMsg struct {
	Success bool
	Error   string
}

type PreferencesSavedMsg struct {
	Success bool
	Error   string
}

func NewSetupModel() SetupModel {
	return NewSetupModelWithConfig(nil)
}

// NewSetupModelWithConfig creates a setup model pre-populated with existing config values
func NewSetupModelWithConfig(cfg *config.Config) SetupModel {
	endpointInput := textinput.New()
	endpointInput.Placeholder = "api.rootly.com"
	endpointInput.Focus()
	endpointInput.Width = 40

	apiKeyInput := textinput.New()
	apiKeyInput.Placeholder = "Enter your API key"
	apiKeyInput.EchoMode = textinput.EchoPassword
	apiKeyInput.EchoCharacter = '*'
	apiKeyInput.Width = 40

	timezones := config.ListTimezones()
	languages := i18n.ListLanguages()
	layouts := []string{config.LayoutHorizontal, config.LayoutVertical}

	tzIndex := 0
	langIndex := 0
	layoutIndex := 0

	if cfg != nil && cfg.IsValid() {
		endpointInput.SetValue(cfg.Endpoint)
		apiKeyInput.SetValue(cfg.APIKey)

		for i, tz := range timezones {
			if tz == cfg.Timezone {
				tzIndex = i
				break
			}
		}

		langIndex = i18n.LanguageIndex(cfg.Language)

		for i, layout := range layouts {
			if layout == cfg.Layout {
				layoutIndex = i
				break
			}
		}
	} else {
		endpointInput.SetValue(config.DefaultEndpoint)

		detectedTZ := config.DetectTimezone()
		for i, tz := range timezones {
			if tz == detectedTZ {
				tzIndex = i
				break
			}
		}

		detectedLang := i18n.DetectLanguage()
		langIndex = i18n.LanguageIndex(string(detectedLang))
	}

	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = styles.Spinner

	return SetupModel{
		endpoint:              endpointInput,
		apiKey:                apiKeyInput,
		connFocus:             ConnFieldEndpoint,
		connButton:            0,
		timezones:             timezones,
		timezoneIndex:         tzIndex,
		languages:             languages,
		languageIndex:         langIndex,
		layouts:               layouts,
		layoutIndex:           layoutIndex,
		configFocus:           ConfigFieldTimezone,
		activePanel:           PanelConnection,
		spinner:               s,
		originalTimezoneIndex: tzIndex,
		originalLanguageIndex: langIndex,
		originalLayoutIndex:   layoutIndex,
	}
}

func (m SetupModel) Init() tea.Cmd {
	return textinput.Blink
}

func (m SetupModel) Update(msg tea.Msg) (SetupModel, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		if m.testing || m.connSaving || m.configSaving {
			return m, nil
		}
		return m.handleKeyMsg(msg)

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		cmds = append(cmds, cmd)

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	}

	// Update text inputs
	var cmd tea.Cmd
	if m.activePanel == PanelConnection {
		switch m.connFocus {
		case ConnFieldEndpoint:
			m.endpoint, cmd = m.endpoint.Update(msg)
			cmds = append(cmds, cmd)
		case ConnFieldAPIKey:
			m.apiKey, cmd = m.apiKey.Update(msg)
			cmds = append(cmds, cmd)
		}
	}

	return m, tea.Batch(cmds...)
}

func (m SetupModel) handleKeyMsg(msg tea.KeyMsg) (SetupModel, tea.Cmd) {
	switch msg.String() {
	case "tab":
		return m.handleKeyTab(), nil
	case "down", "j":
		return m.handleKeyDown(), nil
	case "up", "k":
		return m.handleKeyUp(), nil
	case "left", "h":
		return m.handleKeyLeft(), nil
	case "right", "l":
		return m.handleKeyRight(), nil
	case "enter":
		return m.handleKeyEnter()
	}
	return m, nil
}

func (m SetupModel) handleKeyTab() SetupModel {
	if m.activePanel == PanelConnection {
		m.activePanel = PanelConfig
		m.endpoint.Blur()
		m.apiKey.Blur()
	} else {
		m.activePanel = PanelConnection
		m.updateConnectionFocus()
	}
	return m
}

func (m SetupModel) handleKeyDown() SetupModel {
	if m.activePanel == PanelConnection {
		m.connFocus++
		if m.connFocus > ConnFieldButtons {
			m.connFocus = ConnFieldEndpoint
		}
		m.updateConnectionFocus()
	} else {
		m.configFocus++
		if m.configFocus > ConfigFieldButton {
			m.configFocus = ConfigFieldTimezone
		}
	}
	return m
}

func (m SetupModel) handleKeyUp() SetupModel {
	if m.activePanel == PanelConnection {
		m.connFocus--
		if m.connFocus < ConnFieldEndpoint {
			m.connFocus = ConnFieldButtons
		}
		m.updateConnectionFocus()
	} else {
		m.configFocus--
		if m.configFocus < ConfigFieldTimezone {
			m.configFocus = ConfigFieldButton
		}
	}
	return m
}

func (m SetupModel) handleKeyLeft() SetupModel {
	if m.activePanel == PanelConnection {
		if m.connFocus == ConnFieldButtons && m.connButton > 0 {
			m.connButton--
		}
	} else {
		m.handleConfigLeft()
	}
	return m
}

func (m *SetupModel) handleConfigLeft() {
	switch m.configFocus {
	case ConfigFieldTimezone:
		if m.timezoneIndex > 0 {
			m.timezoneIndex--
		}
	case ConfigFieldLanguage:
		if m.languageIndex > 0 {
			m.languageIndex--
			i18n.SetLanguage(i18n.Language(m.languages[m.languageIndex]))
		}
	case ConfigFieldLayout:
		if m.layoutIndex > 0 {
			m.layoutIndex--
		}
	}
}

func (m SetupModel) handleKeyRight() SetupModel {
	if m.activePanel == PanelConnection {
		if m.connFocus == ConnFieldButtons && m.connButton < 1 {
			m.connButton++
		}
	} else {
		m.handleConfigRight()
	}
	return m
}

func (m *SetupModel) handleConfigRight() {
	switch m.configFocus {
	case ConfigFieldTimezone:
		if m.timezoneIndex < len(m.timezones)-1 {
			m.timezoneIndex++
		}
	case ConfigFieldLanguage:
		if m.languageIndex < len(m.languages)-1 {
			m.languageIndex++
			i18n.SetLanguage(i18n.Language(m.languages[m.languageIndex]))
		}
	case ConfigFieldLayout:
		if m.layoutIndex < len(m.layouts)-1 {
			m.layoutIndex++
		}
	}
}

func (m SetupModel) handleKeyEnter() (SetupModel, tea.Cmd) {
	if m.activePanel == PanelConnection {
		return m.handleConnectionEnter()
	}
	return m.handleConfigEnter()
}

func (m SetupModel) handleConnectionEnter() (SetupModel, tea.Cmd) {
	if m.connFocus == ConnFieldButtons {
		if m.connButton == 0 {
			// Test connection
			m.testing = true
			m.testResult = ""
			m.testError = ""
			m.connSaved = false
			return m, tea.Batch(m.spinner.Tick, m.doTestConnection())
		}
		// Save connection
		if m.testResult == testResultSuccess {
			m.connSaving = true
			return m, m.doSaveConnection()
		}
		return m, nil
	}
	// Move to next field on enter
	m.connFocus++
	if m.connFocus > ConnFieldButtons {
		m.connFocus = ConnFieldButtons
	}
	m.updateConnectionFocus()
	return m, nil
}

func (m SetupModel) handleConfigEnter() (SetupModel, tea.Cmd) {
	if m.configFocus == ConfigFieldButton {
		// Save config
		m.configSaving = true
		return m, m.doSavePreferences()
	}
	// Move to next field on enter
	m.configFocus++
	if m.configFocus > ConfigFieldButton {
		m.configFocus = ConfigFieldButton
	}
	return m, nil
}

func (m *SetupModel) updateConnectionFocus() {
	m.endpoint.Blur()
	m.apiKey.Blur()

	switch m.connFocus {
	case ConnFieldEndpoint:
		m.endpoint.Focus()
	case ConnFieldAPIKey:
		m.apiKey.Focus()
	}
}

func (m SetupModel) doTestConnection() tea.Cmd {
	return func() tea.Msg {
		cfg := &config.Config{
			Endpoint: m.endpoint.Value(),
			APIKey:   m.apiKey.Value(),
		}

		client, err := api.NewClient(cfg)
		if err != nil {
			return APIKeyValidatedMsg{Valid: false, Error: err.Error()}
		}

		ctx := context.Background()
		if err := client.ValidateAPIKey(ctx); err != nil {
			return APIKeyValidatedMsg{Valid: false, Error: err.Error()}
		}

		return APIKeyValidatedMsg{Valid: true}
	}
}

func (m SetupModel) doSaveConnection() tea.Cmd {
	timezone := ""
	if m.timezoneIndex >= 0 && m.timezoneIndex < len(m.timezones) {
		timezone = m.timezones[m.timezoneIndex]
	}
	language := string(i18n.DefaultLanguage)
	if m.languageIndex >= 0 && m.languageIndex < len(m.languages) {
		language = m.languages[m.languageIndex]
	}
	layout := config.DefaultLayout
	if m.layoutIndex >= 0 && m.layoutIndex < len(m.layouts) {
		layout = m.layouts[m.layoutIndex]
	}

	return func() tea.Msg {
		cfg := &config.Config{
			Endpoint: m.endpoint.Value(),
			APIKey:   m.apiKey.Value(),
			Timezone: timezone,
			Language: language,
			Layout:   layout,
		}

		if err := config.Save(cfg); err != nil {
			return ConnectionSavedMsg{Success: false, Error: err.Error()}
		}

		return ConnectionSavedMsg{Success: true}
	}
}

func (m SetupModel) doSavePreferences() tea.Cmd {
	timezone := ""
	if m.timezoneIndex >= 0 && m.timezoneIndex < len(m.timezones) {
		timezone = m.timezones[m.timezoneIndex]
	}
	language := string(i18n.DefaultLanguage)
	if m.languageIndex >= 0 && m.languageIndex < len(m.languages) {
		language = m.languages[m.languageIndex]
	}
	layout := config.DefaultLayout
	if m.layoutIndex >= 0 && m.layoutIndex < len(m.layouts) {
		layout = m.layouts[m.layoutIndex]
	}

	return func() tea.Msg {
		// Load existing config to preserve connection settings
		existingCfg, err := config.Load()
		if err != nil {
			// If no existing config, use current values
			existingCfg = &config.Config{
				Endpoint: m.endpoint.Value(),
				APIKey:   m.apiKey.Value(),
			}
		}

		cfg := &config.Config{
			Endpoint: existingCfg.Endpoint,
			APIKey:   existingCfg.APIKey,
			Timezone: timezone,
			Language: language,
			Layout:   layout,
		}

		if err := config.Save(cfg); err != nil {
			return PreferencesSavedMsg{Success: false, Error: err.Error()}
		}

		return PreferencesSavedMsg{Success: true}
	}
}

func (m *SetupModel) HandleValidationResult(msg APIKeyValidatedMsg) {
	m.testing = false
	if msg.Valid {
		m.testResult = testResultSuccess
		m.testError = ""
	} else {
		m.testResult = testResultError
		m.testError = msg.Error
	}
}

func (m *SetupModel) HandleConnectionSaved(msg ConnectionSavedMsg) {
	m.connSaving = false
	m.connSaved = msg.Success
}

func (m *SetupModel) HandlePreferencesSaved(msg PreferencesSavedMsg) {
	m.configSaving = false
	m.configSaved = msg.Success
	if msg.Success {
		// Update original values after save
		m.originalTimezoneIndex = m.timezoneIndex
		m.originalLanguageIndex = m.languageIndex
		m.originalLayoutIndex = m.layoutIndex
	}
}

func (m SetupModel) IsTesting() bool {
	return m.testing
}

func (m *SetupModel) SetTesting(testing bool) {
	m.testing = testing
}

func (m *SetupModel) SetDimensions(width, height int) {
	m.width = width
	m.height = height
}

func (m SetupModel) IsConnectionSaved() bool {
	return m.connSaved
}

func (m SetupModel) IsConfigSaved() bool {
	return m.configSaved
}

func (m SetupModel) View() string {
	// Panel styles - width accommodates input fields (40) + padding (4) + border (2) + extra space
	panelWidth := 52
	activeBorder := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(styles.ColorPurple).
		Padding(1, 2).
		Width(panelWidth)

	inactiveBorder := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(styles.ColorBorder).
		Padding(1, 2).
		Width(panelWidth)

	// Connection panel
	connPanel := m.renderConnectionPanel()
	if m.activePanel == PanelConnection {
		connPanel = activeBorder.Render(connPanel)
	} else {
		connPanel = inactiveBorder.Render(connPanel)
	}

	// Config panel
	configPanel := m.renderConfigPanel()
	if m.activePanel == PanelConfig {
		configPanel = activeBorder.Render(configPanel)
	} else {
		configPanel = inactiveBorder.Render(configPanel)
	}

	// Join panels horizontally
	panels := lipgloss.JoinHorizontal(lipgloss.Top, connPanel, "  ", configPanel)

	// Help text
	help := styles.HelpBar.Render(i18n.T("setup.help_panels"))

	// Combine all
	content := lipgloss.JoinVertical(lipgloss.Left, panels, "", help)

	// Center on screen
	if m.width > 0 && m.height > 0 {
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, content)
	}

	return content
}

func (m SetupModel) renderConnectionPanel() string {
	var b strings.Builder

	// Panel title
	title := styles.DialogTitle.Render(i18n.T("setup.connection_title"))
	b.WriteString(title)
	b.WriteString("\n\n")

	// Endpoint field
	endpointLabel := styles.InputLabel.Render(i18n.T("setup.api_endpoint"))
	b.WriteString(endpointLabel)
	b.WriteString("\n")
	if m.activePanel == PanelConnection && m.connFocus == ConnFieldEndpoint {
		b.WriteString(styles.InputFieldFocused.Render(m.endpoint.View()))
	} else {
		b.WriteString(styles.InputField.Render(m.endpoint.View()))
	}
	b.WriteString("\n\n")

	// API Key field
	apiKeyLabel := styles.InputLabel.Render(i18n.T("setup.api_key"))
	b.WriteString(apiKeyLabel)
	b.WriteString("\n")
	if m.activePanel == PanelConnection && m.connFocus == ConnFieldAPIKey {
		b.WriteString(styles.InputFieldFocused.Render(m.apiKey.View()))
	} else {
		b.WriteString(styles.InputField.Render(m.apiKey.View()))
	}
	b.WriteString("\n\n")

	// Test result
	if m.testing {
		b.WriteString(m.spinner.View() + " " + i18n.T("setup.testing_connection"))
		b.WriteString("\n\n")
	} else if m.testResult == testResultSuccess {
		b.WriteString(styles.SuccessMsg.Render(i18n.T("setup.connection_success")))
		b.WriteString("\n\n")
	} else if m.testResult == testResultError {
		errMsg := i18n.T("common.error") + ": " + m.testError
		// Truncate error if too long
		if len(errMsg) > 40 {
			errMsg = errMsg[:37] + "..."
		}
		b.WriteString(styles.Error.Render(errMsg))
		b.WriteString("\n\n")
	} else {
		b.WriteString("\n\n")
	}

	// Saved message
	if m.connSaved {
		b.WriteString(styles.SuccessMsg.Render(i18n.T("setup.connection_saved")))
		b.WriteString("\n\n")
	} else if m.connSaving {
		b.WriteString(m.spinner.View() + " " + i18n.T("common.saving"))
		b.WriteString("\n\n")
	} else {
		b.WriteString("\n\n")
	}

	// Buttons
	var testBtn, saveBtn string
	if m.activePanel == PanelConnection && m.connFocus == ConnFieldButtons && m.connButton == 0 {
		testBtn = styles.ButtonFocused.Render(i18n.T("setup.test"))
	} else {
		testBtn = styles.Button.Render(i18n.T("setup.test"))
	}

	if m.activePanel == PanelConnection && m.connFocus == ConnFieldButtons && m.connButton == 1 {
		if m.testResult == testResultSuccess {
			saveBtn = styles.ButtonFocused.Render(i18n.T("setup.save"))
		} else {
			saveBtn = styles.ButtonDisabled.Render(i18n.T("setup.save"))
		}
	} else {
		if m.testResult == testResultSuccess {
			saveBtn = styles.Button.Render(i18n.T("setup.save"))
		} else {
			saveBtn = styles.ButtonDisabled.Render(i18n.T("setup.save"))
		}
	}

	b.WriteString(testBtn + " " + saveBtn)

	return b.String()
}

func (m SetupModel) renderConfigPanel() string {
	var b strings.Builder

	// Panel title
	title := styles.DialogTitle.Render(i18n.T("setup.preferences_title"))
	b.WriteString(title)
	b.WriteString("\n\n")

	// Timezone selector
	timezoneLabel := styles.InputLabel.Render(i18n.T("setup.timezone"))
	b.WriteString(timezoneLabel)
	b.WriteString("\n")
	selectedTZ := "UTC"
	if m.timezoneIndex >= 0 && m.timezoneIndex < len(m.timezones) {
		selectedTZ = m.timezones[m.timezoneIndex]
	}
	tzDisplay := fmt.Sprintf("◀ %s ▶", selectedTZ)
	if m.activePanel == PanelConfig && m.configFocus == ConfigFieldTimezone {
		b.WriteString(styles.InputFieldFocused.Render(tzDisplay))
	} else {
		b.WriteString(styles.InputField.Render(tzDisplay))
	}
	b.WriteString("\n\n")

	// Language selector
	languageLabel := styles.InputLabel.Render(i18n.T("setup.language"))
	b.WriteString(languageLabel)
	b.WriteString("\n")
	selectedLang := i18n.LanguageName(string(i18n.DefaultLanguage))
	if m.languageIndex >= 0 && m.languageIndex < len(m.languages) {
		selectedLang = i18n.LanguageName(m.languages[m.languageIndex])
	}
	langDisplay := fmt.Sprintf("◀ %s ▶", selectedLang)
	if m.activePanel == PanelConfig && m.configFocus == ConfigFieldLanguage {
		b.WriteString(styles.InputFieldFocused.Render(langDisplay))
	} else {
		b.WriteString(styles.InputField.Render(langDisplay))
	}
	b.WriteString("\n\n")

	// Layout selector
	layoutLabel := styles.InputLabel.Render(i18n.T("setup.layout"))
	b.WriteString(layoutLabel)
	b.WriteString("\n")
	selectedLayout := layoutDisplayName(config.DefaultLayout)
	if m.layoutIndex >= 0 && m.layoutIndex < len(m.layouts) {
		selectedLayout = layoutDisplayName(m.layouts[m.layoutIndex])
	}
	layoutDisplay := fmt.Sprintf("◀ %s ▶", selectedLayout)
	if m.activePanel == PanelConfig && m.configFocus == ConfigFieldLayout {
		b.WriteString(styles.InputFieldFocused.Render(layoutDisplay))
	} else {
		b.WriteString(styles.InputField.Render(layoutDisplay))
	}
	b.WriteString("\n\n")

	// Spacer to match connection panel height (test result area equivalent)
	b.WriteString("\n\n")

	// Saved message
	if m.configSaved {
		b.WriteString(styles.SuccessMsg.Render(i18n.T("setup.preferences_saved")))
		b.WriteString("\n\n")
	} else if m.configSaving {
		b.WriteString(m.spinner.View() + " " + i18n.T("common.saving"))
		b.WriteString("\n\n")
	} else {
		b.WriteString("\n\n")
	}

	// Save button
	var saveBtn string
	if m.activePanel == PanelConfig && m.configFocus == ConfigFieldButton {
		saveBtn = styles.ButtonFocused.Render(i18n.T("setup.save_preferences"))
	} else {
		saveBtn = styles.Button.Render(i18n.T("setup.save_preferences"))
	}
	b.WriteString(saveBtn)

	return b.String()
}

// layoutDisplayName returns a human-readable name for a layout value
func layoutDisplayName(layout string) string {
	switch layout {
	case config.LayoutHorizontal:
		return i18n.T("setup.layout_horizontal")
	case config.LayoutVertical:
		return i18n.T("setup.layout_vertical")
	default:
		return layout
	}
}

// Backward compatibility methods for tests
func (m SetupModel) FocusIndex() SetupField {
	// Map new structure to old SetupField for tests
	if m.activePanel == PanelConnection {
		switch m.connFocus {
		case ConnFieldEndpoint:
			return FieldEndpoint
		case ConnFieldAPIKey:
			return FieldAPIKey
		case ConnFieldButtons:
			return FieldButtons
		}
	} else {
		switch m.configFocus {
		case ConfigFieldTimezone:
			return FieldTimezone
		case ConfigFieldLanguage:
			return FieldLanguage
		case ConfigFieldLayout:
			return FieldLayout
		case ConfigFieldButton:
			return FieldButtons
		}
	}
	return FieldEndpoint
}

func (m SetupModel) ButtonIndex() int {
	if m.activePanel == PanelConnection {
		return m.connButton
	}
	return 0
}

func (m SetupModel) TimezoneIndex() int {
	return m.timezoneIndex
}

func (m SetupModel) LanguageIndex() int {
	return m.languageIndex
}

func (m SetupModel) LayoutIndex() int {
	return m.layoutIndex
}

func (m SetupModel) ActivePanel() Panel {
	return m.activePanel
}
