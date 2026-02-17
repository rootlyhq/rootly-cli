package app

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"golang.design/x/clipboard"

	"github.com/rootlyhq/rootly-tui/internal/api"
	"github.com/rootlyhq/rootly-tui/internal/config"
	"github.com/rootlyhq/rootly-tui/internal/debug"
	"github.com/rootlyhq/rootly-tui/internal/i18n"
	"github.com/rootlyhq/rootly-tui/internal/styles"
	"github.com/rootlyhq/rootly-tui/internal/views"
)

type Screen int

const (
	ScreenSetup Screen = iota
	ScreenMain
)

type Tab int

const (
	TabIncidents Tab = iota
	TabAlerts
)

// URLOpener is a function type for opening URLs in a browser (injectable for testing)
type URLOpener func(url string) error

// defaultURLOpener is the production URL opener
var defaultURLOpener URLOpener = openURLInBrowser

type Model struct {
	// Core state
	version   string
	screen    Screen
	activeTab Tab
	keys      KeyMap
	width     int
	height    int

	// Config and API
	cfg       *config.Config
	apiClient *api.Client

	// Views
	setup     views.SetupModel
	incidents views.IncidentsModel
	alerts    views.AlertsModel
	help      views.HelpModel
	logs      views.LogsModel
	about     views.AboutModel
	spinner   spinner.Model

	// Loading state
	loading        bool
	initialLoading bool
	statusMsg      string
	errorMsg       string

	// URL opener (injectable for testing)
	urlOpener URLOpener
}

func New(version string) Model {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = styles.Spinner

	m := Model{
		version:   version,
		screen:    ScreenSetup,
		activeTab: TabIncidents,
		keys:      DefaultKeyMap(),
		setup:     views.NewSetupModel(),
		incidents: views.NewIncidentsModel(),
		alerts:    views.NewAlertsModel(),
		help:      views.NewHelpModel(),
		logs:      views.NewLogsModel(),
		about:     views.NewAboutModel(version),
		spinner:   s,
		urlOpener: defaultURLOpener,
	}

	// Check if config exists
	if config.Exists() {
		cfg, err := config.Load()
		if err == nil && cfg.IsValid() {
			m.cfg = cfg
			// Set language from config
			if cfg.Language != "" {
				i18n.SetLanguage(i18n.Language(cfg.Language))
			}
			// Set layout from config
			if cfg.Layout != "" {
				m.incidents.SetLayout(cfg.Layout)
				m.alerts.SetLayout(cfg.Layout)
			}
			// Create the API client once here
			client, err := api.NewClient(cfg)
			if err == nil {
				m.apiClient = client
				m.screen = ScreenMain
				m.initialLoading = true
			}
			// If client creation fails, fall through to setup screen
		}
	}

	return m
}

func (m Model) Init() tea.Cmd {
	if m.screen == ScreenMain {
		return tea.Batch(
			m.spinner.Tick,
			m.loadData(),
		)
	}
	return m.setup.Init()
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		// Handle quit/escape - if on setup screen with valid config, return to main instead of exiting
		if key.Matches(msg, m.keys.Quit) || (m.screen == ScreenSetup && msg.String() == "esc") {
			if m.screen == ScreenSetup && m.cfg != nil && m.cfg.IsValid() {
				// Return to main screen
				m.screen = ScreenMain
				return m, nil
			}
			return m, tea.Quit
		}

		// Handle logs overlay first
		if m.logs.Visible {
			if key.Matches(msg, m.keys.Logs) || msg.String() == "esc" {
				m.logs.Toggle()
				return m, nil
			}
			var cmd tea.Cmd
			m.logs, cmd = m.logs.Update(msg)
			return m, cmd
		}

		// Handle about overlay
		if m.about.Visible {
			if key.Matches(msg, m.keys.About) || msg.String() == "esc" {
				m.about.Toggle()
				return m, nil
			}
			return m, nil
		}

		// Handle help overlay
		if m.help.Visible {
			if key.Matches(msg, m.keys.Help) || msg.String() == "esc" {
				m.help.Toggle()
				return m, nil
			}
			return m, nil
		}

		// Handle sort menu
		if m.activeTab == TabIncidents && m.incidents.IsSortMenuVisible() {
			if m.incidents.HandleSortMenuKey(msg.String()) {
				// Sort field changed, reload incidents from API
				m.incidents.SetLoading(true)
				return m, m.loadIncidents()
			}
			return m, nil
		}

		// Handle setup screen
		if m.screen == ScreenSetup {
			var cmd tea.Cmd
			m.setup, cmd = m.setup.Update(msg)
			cmds = append(cmds, cmd)
			return m, tea.Batch(cmds...)
		}

		// Handle main screen navigation
		switch {
		case key.Matches(msg, m.keys.Help):
			m.help.Toggle()
			return m, nil

		case key.Matches(msg, m.keys.Logs):
			m.logs.Toggle()
			if m.logs.Visible {
				return m, m.logs.StartAutoRefresh()
			}
			return m, nil

		case key.Matches(msg, m.keys.About):
			m.about.Toggle()
			return m, nil

		case key.Matches(msg, m.keys.Setup):
			// Reset to setup screen with existing config
			m.screen = ScreenSetup
			m.setup = views.NewSetupModelWithConfig(m.cfg)
			m.setup.SetDimensions(m.width, m.height)
			return m, m.setup.Init()

		case key.Matches(msg, m.keys.Tab):
			// Clear focus when switching tabs
			m.incidents.SetDetailFocused(false)
			m.alerts.SetDetailFocused(false)
			if m.activeTab == TabIncidents {
				m.activeTab = TabAlerts
			} else {
				m.activeTab = TabIncidents
			}
			return m, nil

		case key.Matches(msg, m.keys.Refresh):
			m.loading = true
			m.statusMsg = i18n.T("common.refreshing")
			// Clear cache on manual refresh
			if m.apiClient != nil {
				m.apiClient.ClearCache()
			}
			return m, m.loadData()

		case key.Matches(msg, m.keys.PrevPage):
			if m.activeTab == TabIncidents && m.incidents.HasPrevPage() {
				m.incidents.PrevPage()
				m.incidents.SetLoading(true)
				m.loading = true // Keeps spinner ticking
				return m, tea.Batch(m.spinner.Tick, m.loadIncidents())
			} else if m.activeTab == TabAlerts && m.alerts.HasPrevPage() {
				m.alerts.PrevPage()
				m.alerts.SetLoading(true)
				m.loading = true
				return m, tea.Batch(m.spinner.Tick, m.loadAlerts())
			}
			return m, nil

		case key.Matches(msg, m.keys.NextPage):
			if m.activeTab == TabIncidents && m.incidents.HasNextPage() {
				m.incidents.NextPage()
				m.incidents.SetLoading(true)
				m.loading = true
				return m, tea.Batch(m.spinner.Tick, m.loadIncidents())
			} else if m.activeTab == TabAlerts && m.alerts.HasNextPage() {
				m.alerts.NextPage()
				m.alerts.SetLoading(true)
				m.loading = true
				return m, tea.Batch(m.spinner.Tick, m.loadAlerts())
			}
			return m, nil

		case key.Matches(msg, m.keys.Enter):
			// Fetch detailed data for selected item, or focus detail pane for scrolling if already loaded
			if m.activeTab == TabIncidents {
				inc := m.incidents.SelectedIncident()
				if inc != nil {
					if !inc.DetailLoaded {
						m.incidents.SetDetailLoading(inc.ID)
						return m, tea.Batch(m.spinner.Tick, m.loadIncidentDetail(inc.ID, inc.UpdatedAt, m.incidents.SelectedIndex()))
					}
					// Detail already loaded, focus the detail pane for scrolling
					m.incidents.SetDetailFocused(true)
				}
			} else {
				alert := m.alerts.SelectedAlert()
				if alert != nil {
					if !alert.DetailLoaded {
						m.alerts.SetDetailLoading(alert.ID)
						return m, tea.Batch(m.spinner.Tick, m.loadAlertDetail(alert.ID, alert.UpdatedAt, m.alerts.SelectedIndex()))
					}
					// Detail already loaded, focus the detail pane for scrolling
					m.alerts.SetDetailFocused(true)
				}
			}
			return m, nil

		case key.Matches(msg, m.keys.Open):
			// Open URL in browser
			var url string
			if m.activeTab == TabIncidents {
				inc := m.incidents.SelectedIncident()
				if inc != nil {
					// Prefer short URL, then URL, then construct from ID
					if inc.ShortURL != "" {
						url = inc.ShortURL
					} else if inc.URL != "" {
						url = inc.URL
					} else if inc.ID != "" {
						url = fmt.Sprintf("https://rootly.com/account/incidents/%s", inc.ID)
					}
				}
			} else {
				alert := m.alerts.SelectedAlert()
				if alert != nil {
					// Construct URL from short ID
					if alert.ShortID != "" {
						url = fmt.Sprintf("https://rootly.com/account/alerts/%s", alert.ShortID)
					}
				}
			}
			if url != "" && m.urlOpener != nil {
				_ = m.urlOpener(url)
			}
			return m, nil

		case key.Matches(msg, m.keys.Sort):
			// Toggle sort menu for incidents tab
			if m.activeTab == TabIncidents {
				m.incidents.ToggleSortMenu()
			}
			return m, nil

		case key.Matches(msg, m.keys.Copy):
			// Copy detail panel to clipboard
			var text string
			if m.activeTab == TabIncidents {
				text = m.incidents.GetDetailPlainText()
			} else {
				text = m.alerts.GetDetailPlainText()
			}
			if text != "" {
				if err := clipboard.Init(); err != nil {
					debug.Logger.Error("Failed to initialize clipboard", "error", err)
					m.statusMsg = i18n.T("logs.clipboard_unavailable")
				} else {
					clipboard.Write(clipboard.FmtText, []byte(text))
					m.statusMsg = i18n.T("logs.copied")
				}
			}
			return m, nil

		default:
			// Pass key events to active view
			if m.activeTab == TabIncidents {
				var cmd tea.Cmd
				m.incidents, cmd = m.incidents.Update(msg)
				cmds = append(cmds, cmd)
			} else {
				var cmd tea.Cmd
				m.alerts, cmd = m.alerts.Update(msg)
				cmds = append(cmds, cmd)
			}
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.incidents.SetDimensions(msg.Width-4, msg.Height-10)
		m.alerts.SetDimensions(msg.Width-4, msg.Height-10)
		m.logs.SetDimensions(msg.Width, msg.Height)
		return m, nil

	case tea.MouseMsg:
		// Forward mouse events to logs view when visible
		if m.logs.Visible {
			var cmd tea.Cmd
			m.logs, cmd = m.logs.Update(msg)
			cmds = append(cmds, cmd)
			return m, tea.Batch(cmds...)
		}
		// Forward mouse events to active view for viewport scrolling
		if m.screen == ScreenMain && !m.help.Visible {
			if m.activeTab == TabIncidents {
				var cmd tea.Cmd
				m.incidents, cmd = m.incidents.Update(msg)
				cmds = append(cmds, cmd)
			} else {
				var cmd tea.Cmd
				m.alerts, cmd = m.alerts.Update(msg)
				cmds = append(cmds, cmd)
			}
		}
		return m, tea.Batch(cmds...)

	case spinner.TickMsg:
		// Only continue spinner when actually loading
		if m.loading || m.initialLoading || m.incidents.IsDetailLoading() || m.alerts.IsDetailLoading() {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			cmds = append(cmds, cmd)
		}

	// Setup screen messages
	case views.APIKeyValidatedMsg:
		m.setup.HandleValidationResult(msg)
		m.setup.SetTesting(false)
		return m, nil

	case views.ConfigSavedMsg:
		if msg.Success {
			// Config saved, load it and switch to main screen
			cfg, err := config.Load()
			if err == nil && cfg.IsValid() {
				m.cfg = cfg
				// Update language from saved config
				if cfg.Language != "" {
					i18n.SetLanguage(i18n.Language(cfg.Language))
				}
				// Update layout from saved config
				if cfg.Layout != "" {
					m.incidents.SetLayout(cfg.Layout)
					m.alerts.SetLayout(cfg.Layout)
				}
				client, err := api.NewClient(cfg)
				if err == nil {
					m.apiClient = client
					m.screen = ScreenMain
					m.initialLoading = true
					return m, tea.Batch(m.spinner.Tick, m.loadData())
				}
			}
		}
		return m, nil

	case views.ConnectionSavedMsg:
		m.setup.HandleConnectionSaved(msg)
		if msg.Success {
			// Connection saved, load it and switch to main screen
			cfg, err := config.Load()
			if err == nil && cfg.IsValid() {
				m.cfg = cfg
				// Update language from saved config
				if cfg.Language != "" {
					i18n.SetLanguage(i18n.Language(cfg.Language))
				}
				// Update layout from saved config
				if cfg.Layout != "" {
					m.incidents.SetLayout(cfg.Layout)
					m.alerts.SetLayout(cfg.Layout)
				}
				client, err := api.NewClient(cfg)
				if err == nil {
					m.apiClient = client
					m.screen = ScreenMain
					m.initialLoading = true
					return m, tea.Batch(m.spinner.Tick, m.loadData())
				}
			}
		}
		return m, nil

	case views.PreferencesSavedMsg:
		m.setup.HandlePreferencesSaved(msg)
		if msg.Success {
			// Preferences saved, update settings but stay on setup screen
			cfg, err := config.Load()
			if err == nil {
				m.cfg = cfg
				// Update language from saved config
				if cfg.Language != "" {
					i18n.SetLanguage(i18n.Language(cfg.Language))
				}
				// Update layout from saved config
				if cfg.Layout != "" {
					m.incidents.SetLayout(cfg.Layout)
					m.alerts.SetLayout(cfg.Layout)
				}
			}
		}
		return m, nil

	// Logs refresh message
	case views.LogsRefreshMsg:
		if m.logs.Visible {
			var cmd tea.Cmd
			m.logs, cmd = m.logs.Update(msg)
			return m, cmd
		}
		return m, nil

	// Data loading messages
	case IncidentsLoadedMsg:
		m.loading = false
		m.initialLoading = false
		if msg.Err != nil {
			m.errorMsg = msg.Err.Error()
			m.incidents.SetError(msg.Err.Error())
		} else {
			m.incidents.SetIncidents(msg.Incidents, msg.Pagination)
			m.errorMsg = ""
			m.statusMsg = ""
		}
		return m, nil

	case AlertsLoadedMsg:
		if msg.Err != nil {
			m.alerts.SetError(msg.Err.Error())
		} else {
			m.alerts.SetAlerts(msg.Alerts, msg.Pagination)
		}
		return m, nil

	case IncidentDetailLoadedMsg:
		m.incidents.ClearDetailLoading()
		if msg.Err != nil {
			m.errorMsg = msg.Err.Error()
		} else if msg.Incident != nil {
			m.incidents.UpdateIncidentDetail(msg.Index, msg.Incident)
			m.errorMsg = ""
			// Auto-focus detail pane for scrolling after load completes
			m.incidents.SetDetailFocused(true)
		}
		return m, nil

	case AlertDetailLoadedMsg:
		m.alerts.ClearDetailLoading()
		if msg.Err != nil {
			m.errorMsg = msg.Err.Error()
		} else if msg.Alert != nil {
			m.alerts.UpdateAlertDetail(msg.Index, msg.Alert)
			m.errorMsg = ""
			// Auto-focus detail pane for scrolling after load completes
			m.alerts.SetDetailFocused(true)
		}
		return m, nil

	case ErrorMsg:
		m.errorMsg = msg.Err.Error()
		m.loading = false
		return m, nil
	}

	return m, tea.Batch(cmds...)
}

func (m Model) View() string {
	if m.width == 0 {
		return i18n.T("common.loading")
	}

	// Setup screen
	if m.screen == ScreenSetup {
		return m.setup.View()
	}

	// Main screen
	var b strings.Builder

	// Header with tabs
	header := m.renderHeader()
	b.WriteString(header)
	b.WriteString("\n\n")

	// Main content
	if m.initialLoading {
		b.WriteString(m.spinner.View() + " " + i18n.T("common.loading"))
	} else {
		// Pass spinner to views for loading state
		m.incidents.SetSpinner(m.spinner.View())
		m.alerts.SetSpinner(m.spinner.View())

		if m.activeTab == TabIncidents {
			b.WriteString(m.incidents.View())
		} else {
			b.WriteString(m.alerts.View())
		}
	}

	// Status bar
	b.WriteString("\n")
	b.WriteString(m.renderStatusBar())

	// Help bar
	b.WriteString("\n")
	hasSelection := false
	var currentPage, totalPages, totalCount int
	if m.activeTab == TabIncidents {
		hasSelection = m.incidents.SelectedIncident() != nil
		currentPage = m.incidents.CurrentPage()
		totalPages = m.incidents.TotalPages()
		totalCount = m.incidents.TotalCount()
	} else {
		hasSelection = m.alerts.SelectedAlert() != nil
		currentPage = m.alerts.CurrentPage()
		totalPages = m.alerts.TotalPages()
		totalCount = m.alerts.TotalCount()
	}
	isIncidentsTab := m.activeTab == TabIncidents
	b.WriteString(views.RenderHelpBar(m.width, hasSelection, m.loading, isIncidentsTab, currentPage, totalPages, totalCount))

	// Wrap content
	content := styles.App.Render(b.String())

	// Help overlay
	if m.help.Visible {
		helpDialog := m.help.View()
		content = lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, helpDialog)
	}

	// Logs overlay
	if m.logs.Visible {
		logsDialog := m.logs.View()
		content = lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, logsDialog)
	}

	// About overlay
	if m.about.Visible {
		aboutDialog := m.about.View()
		content = lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, aboutDialog)
	}

	// Sort menu overlay (incidents tab only)
	if m.activeTab == TabIncidents && m.incidents.IsSortMenuVisible() {
		sortMenu := m.incidents.RenderSortMenu()
		content = lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, sortMenu)
	}

	return content
}

func (m Model) renderHeader() string {
	title := styles.Title.Render(i18n.T("app.title"))

	// Tab indicators
	var incidentsTab, alertsTab string
	if m.activeTab == TabIncidents {
		incidentsTab = styles.TabActive.Render(i18n.T("incidents.title"))
		alertsTab = styles.TabInactive.Render(i18n.T("alerts.title"))
	} else {
		incidentsTab = styles.TabInactive.Render(i18n.T("incidents.title"))
		alertsTab = styles.TabActive.Render(i18n.T("alerts.title"))
	}
	tabs := incidentsTab + " " + alertsTab

	// Version
	version := styles.TextDim.Render("v" + m.version)

	// Calculate spacing
	leftPart := title + "  "
	leftWidth := lipgloss.Width(leftPart)
	tabsWidth := lipgloss.Width(tabs)
	rightWidth := lipgloss.Width(version)
	spacing := m.width - leftWidth - tabsWidth - rightWidth - 10

	if spacing < 1 {
		spacing = 1
	}

	return styles.Header.Width(m.width).Render(
		leftPart + strings.Repeat(" ", spacing/2) + tabs + strings.Repeat(" ", spacing/2) + version,
	)
}

func (m Model) renderStatusBar() string {
	if m.errorMsg != "" {
		return styles.Error.Render("Error: " + m.errorMsg)
	}
	// Don't show loading in status bar when views handle it (page loading)
	// Views show their own spinner in the content area
	if m.statusMsg != "" && !m.loading {
		return styles.StatusBar.Render(m.statusMsg)
	}
	return ""
}

func (m Model) loadData() tea.Cmd {
	return tea.Batch(
		m.loadIncidents(),
		m.loadAlerts(),
	)
}

func (m Model) loadIncidents() tea.Cmd {
	// Capture the client, page, and sort - it should already be initialized in New()
	client := m.apiClient
	page := m.incidents.CurrentPage()
	sort := m.incidents.GetSortParam()
	return func() tea.Msg {
		if client == nil {
			return IncidentsLoadedMsg{Err: fmt.Errorf("API client not initialized")}
		}

		ctx := context.Background()
		result, err := client.ListIncidents(ctx, page, sort)
		if err != nil {
			return IncidentsLoadedMsg{Err: err}
		}

		return IncidentsLoadedMsg{
			Incidents:  result.Incidents,
			Pagination: result.Pagination,
		}
	}
}

func (m Model) loadAlerts() tea.Cmd {
	// Capture the client and page - it should already be initialized in New()
	client := m.apiClient
	page := m.alerts.CurrentPage()
	return func() tea.Msg {
		if client == nil {
			return AlertsLoadedMsg{Err: fmt.Errorf("API client not initialized")}
		}

		ctx := context.Background()
		result, err := client.ListAlerts(ctx, page)
		if err != nil {
			return AlertsLoadedMsg{Err: err}
		}

		return AlertsLoadedMsg{
			Alerts:     result.Alerts,
			Pagination: result.Pagination,
		}
	}
}

func (m Model) loadIncidentDetail(id string, updatedAt time.Time, index int) tea.Cmd {
	client := m.apiClient
	return func() tea.Msg {
		if client == nil {
			return IncidentDetailLoadedMsg{Err: fmt.Errorf("API client not initialized")}
		}

		ctx := context.Background()
		incident, err := client.GetIncident(ctx, id, updatedAt)
		if err != nil {
			return IncidentDetailLoadedMsg{Err: err, Index: index}
		}

		return IncidentDetailLoadedMsg{
			Incident: incident,
			Index:    index,
		}
	}
}

func (m Model) loadAlertDetail(id string, updatedAt time.Time, index int) tea.Cmd {
	client := m.apiClient
	return func() tea.Msg {
		if client == nil {
			return AlertDetailLoadedMsg{Err: fmt.Errorf("API client not initialized")}
		}

		ctx := context.Background()
		alert, err := client.GetAlert(ctx, id, updatedAt)
		if err != nil {
			return AlertDetailLoadedMsg{Err: err, Index: index}
		}

		return AlertDetailLoadedMsg{
			Alert: alert,
			Index: index,
		}
	}
}

// Close cleans up resources (cache, connections) when the app exits
func (m Model) Close() error {
	if m.apiClient != nil {
		return m.apiClient.Close()
	}
	return nil
}

// openURLInBrowser opens the given URL in the default browser
func openURLInBrowser(url string) error {
	ctx := context.Background()
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.CommandContext(ctx, "open", url)
	case "linux":
		cmd = exec.CommandContext(ctx, "xdg-open", url)
	case "windows":
		cmd = exec.CommandContext(ctx, "rundll32", "url.dll,FileProtocolHandler", url)
	default:
		return fmt.Errorf("unsupported platform: %s", runtime.GOOS)
	}
	return cmd.Start()
}
