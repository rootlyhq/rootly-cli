package i18n

// Message IDs for all translatable strings
// Organized by component using dot notation

const (
	// Common / Shared
	MsgError      = "common.error"
	MsgLoading    = "common.loading"
	MsgRefreshing = "common.refreshing"
	MsgPage       = "common.page"
	MsgOf         = "common.of"

	// App
	MsgAppTitle = "app.title"

	// Setup screen
	MsgWelcome           = "setup.welcome"
	MsgEnterCredentials  = "setup.enter_credentials"
	MsgAPIEndpoint       = "setup.api_endpoint"
	MsgAPIKey            = "setup.api_key"
	MsgTimezone          = "setup.timezone"
	MsgLanguage          = "setup.language"
	MsgUseArrowsToChange = "setup.use_arrows"
	MsgTestingConnection = "setup.testing_connection"
	MsgConnectionSuccess = "setup.connection_success"
	MsgTestConnection    = "setup.test_connection"
	MsgSaveAndContinue   = "setup.save_and_continue"
	MsgSetupHelp         = "setup.help"

	// Incidents view
	MsgIncidents         = "incidents.title"
	MsgLoadingPage       = "incidents.loading_page"
	MsgNoIncidents       = "incidents.none_found"
	MsgSelectIncident    = "incidents.select_prompt"
	MsgPressEnterDetails = "incidents.press_enter"
	MsgLoadingDetails    = "incidents.loading_details"

	// Incidents - Table columns
	MsgColSeverity = "incidents.col.severity"
	MsgColID       = "incidents.col.id"
	MsgColStatus   = "incidents.col.status"
	MsgColTitle    = "incidents.col.title"

	// Incidents - Detail fields
	MsgDescription     = "incidents.detail.description"
	MsgStatus          = "incidents.detail.status"
	MsgSeverity        = "incidents.detail.severity"
	MsgCreatedBy       = "incidents.detail.created_by"
	MsgServices        = "incidents.detail.services"
	MsgEnvironments    = "incidents.detail.environments"
	MsgTeams           = "incidents.detail.teams"
	MsgRoles           = "incidents.detail.roles"
	MsgCauses          = "incidents.detail.causes"
	MsgTypes           = "incidents.detail.types"
	MsgFunctionalities = "incidents.detail.functionalities"
	MsgLinks           = "incidents.detail.links"

	// Incidents - Timeline
	MsgTimeline     = "incidents.timeline.title"
	MsgCreated      = "incidents.timeline.created"
	MsgStarted      = "incidents.timeline.started"
	MsgDetected     = "incidents.timeline.detected"
	MsgAcknowledged = "incidents.timeline.acknowledged"
	MsgMitigated    = "incidents.timeline.mitigated"
	MsgResolved     = "incidents.timeline.resolved"

	// Incidents - Link types
	MsgRootly = "incidents.links.rootly"
	MsgSlack  = "incidents.links.slack"
	MsgJira   = "incidents.links.jira"

	// Alerts view
	MsgAlerts      = "alerts.title"
	MsgNoAlerts    = "alerts.none_found"
	MsgSelectAlert = "alerts.select_prompt"

	// Alerts - Detail fields
	MsgAlertSource       = "alerts.detail.source"
	MsgAlertCreated      = "alerts.detail.created"
	MsgEnded             = "alerts.detail.ended"
	MsgUrgency           = "alerts.detail.urgency"
	MsgAlertServices     = "alerts.detail.services"
	MsgAlertEnvironments = "alerts.detail.environments"
	MsgAlertTeams        = "alerts.detail.teams"
	MsgResponders        = "alerts.detail.responders"
	MsgLabels            = "alerts.detail.labels"
	MsgAlertLinks        = "alerts.detail.links"

	// Help overlay
	MsgKeyboardShortcuts = "help.title"
	MsgPressToClose      = "help.press_to_close"

	// Help - Sections
	MsgNavigation = "help.section.navigation"
	MsgActions    = "help.section.actions"
	MsgGeneral    = "help.section.general"

	// Help - Navigation keys
	MsgMoveDown     = "help.nav.move_down"
	MsgMoveUp       = "help.nav.move_up"
	MsgGoToFirst    = "help.nav.first"
	MsgGoToLast     = "help.nav.last"
	MsgPreviousPage = "help.nav.prev_page"
	MsgNextPage     = "help.nav.next_page"
	MsgSwitchTabs   = "help.nav.switch_tabs"

	// Help - Action keys
	MsgRefreshData = "help.action.refresh"
	MsgViewDetails = "help.action.details"
	MsgOpenURL     = "help.action.open_url"
	MsgViewLogs    = "help.action.logs"
	MsgOpenSetup   = "help.action.setup"
	MsgToggleHelp  = "help.action.help"
	MsgViewAbout   = "help.action.about"
	MsgQuit        = "help.action.quit"

	// Help bar (bottom status bar)
	MsgHelpBarNavigate = "helpbar.navigate"
	MsgHelpBarPage     = "helpbar.page"
	MsgHelpBarSwitch   = "helpbar.switch"
	MsgHelpBarRefresh  = "helpbar.refresh"
	MsgHelpBarOpen     = "helpbar.open"
	MsgHelpBarLogs     = "helpbar.logs"
	MsgHelpBarSetup    = "helpbar.setup"
	MsgHelpBarHelp     = "helpbar.help"
	MsgHelpBarAbout    = "helpbar.about"
	MsgHelpBarQuit     = "helpbar.quit"

	// Logs overlay
	MsgDebugLogs           = "logs.title"
	MsgNoLogsYet           = "logs.empty"
	MsgShowingLines        = "logs.showing_lines"
	MsgCopied              = "logs.copied"
	MsgClipboardUnavail    = "logs.clipboard_unavailable"
	MsgLogsHelp            = "logs.help"
	MsgLogsHelpNoClipboard = "logs.help_no_clipboard"
	MsgLogsMemory          = "logs.memory"
	MsgLogsFollowing       = "logs.following"
	MsgLogsLineCount       = "logs.line_count"
	MsgLogsScrollPercent   = "logs.scroll_percent"

	// About overlay
	MsgAboutTitle       = "about.title"
	MsgAboutDescription = "about.description"
	MsgAboutSystem      = "about.system"
	MsgAboutGoVersion   = "about.go_version"
	MsgAboutPlatform    = "about.platform"
	MsgAboutDocs        = "about.docs"
	MsgAboutClose       = "about.press_to_close"
)
