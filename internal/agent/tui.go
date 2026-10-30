package agent

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/list"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"
	"charm.land/glamour/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/colorprofile"
	"github.com/google/uuid"
	"github.com/oapi-codegen/nullable"
	openapi_types "github.com/oapi-codegen/runtime/types"
	rootly "github.com/rootlyhq/rootly-go"

	"github.com/rootlyhq/rootly-cli/internal/api"
)

type tuiState int

const (
	tuiIdle tuiState = iota
	tuiStreaming
	tuiConfirming
)

type streamMessage struct {
	event Event
	err   error
	done  bool
	turn  int
}

type TUIOptions struct {
	NoColor      bool
	Version      string
	API          TUIAPI
	SessionID    *uuid.UUID
	SessionTitle string
	APIHost      string
	CurrentUser  *rootly.CurrentUserResponse
	Incident     *api.Incident
	Alert        *api.Alert
}

type TUIAPI interface {
	GetCurrentUserWithResponse(context.Context) (*rootly.GetCurrentUserResponse, error)
	ListAiChatSessionsWithResponse(context.Context, *rootly.ListAiChatSessionsParams) (*rootly.ListAiChatSessionsResponse, error)
	ListAiChatSessionMessagesWithResponse(context.Context, openapi_types.UUID, *rootly.ListAiChatSessionMessagesParams) (*rootly.ListAiChatSessionMessagesResponse, error)
	UpdateAiChatSessionWithApplicationVndAPIPlusJSONBodyWithResponse(context.Context, rootly.ID, rootly.UpdateAiChatSessionApplicationVndAPIPlusJSONRequestBody) (*rootly.UpdateAiChatSessionResponse, error)
	DeleteAiChatSessionWithResponse(context.Context, rootly.ID) (*rootly.DeleteAiChatSessionResponse, error)
	GetIncidentByID(context.Context, string) (*api.Incident, error)
	GetAlertByID(context.Context, string) (*api.Alert, error)
	APIHost() string
}

type transcriptTurn struct {
	user      string
	assistant string
}

type historyLoadedMsg struct {
	request      int
	messages     []rootly.AiChatSessionMessage
	err          error
	title        string
	sessionTitle string
}

type sessionPageMsg struct {
	request  int
	sessions []rootly.AiChatSession
	before   string
	hasMore  bool
	err      error
}

type sessionActionMsg struct {
	request      int
	action       string
	message      string
	sessionTitle *string
	err          error
}

type bindingResolvedMsg struct {
	request  int
	incident *api.Incident
	alert    *api.Alert
	err      error
}

type TUIModel struct {
	streamer            Streamer
	state               tuiState
	composer            textarea.Model
	spinner             spinner.Model
	width               int
	height              int
	status              string
	answer              strings.Builder
	sessionID           *uuid.UUID
	pending             *rootly.AiChatStreamConfirmationRequired
	ctx                 context.Context
	cancel              context.CancelFunc
	stream              chan streamMessage
	startedAt           time.Time
	doneStatus          string
	idleInterrupt       bool
	currentParams       *rootly.StreamAiChatParams
	turn                int
	noColor             bool
	renderer            *glamour.TermRenderer
	rendererWidth       int
	renderedSource      string
	rendered            string
	renderCount         int
	resumeTextSeparator bool
	api                 TUIAPI
	apiHost             string
	currentUser         *rootly.CurrentUserResponse
	userLoaded          bool
	sessionTitle        string
	incident            *api.Incident
	alert               *api.Alert
	incidentUsed        bool
	alertUsed           bool
	resumeLinePending   bool
	historyRequest      int
	promptHistory       []string
	historyIndex        int
	historyDraft        string
	currentPrompt       string
	turnStartedAt       time.Time
	turnVerbIndex       int
	verbIndex           func(int) int
	now                 func() time.Time
	version             string
	colorProfile        colorprofile.Profile
	colorProfileKnown   bool
	welcomeStarted      bool
	welcomePending      bool
	welcomeAnimating    bool
	welcomePrinted      bool
	welcomeFrame        int
	pendingPrints       []string
	quitting            bool
	lastAnswer          string
	transcript          []transcriptTurn
	activities          activityTracker
	activityCommands    []tea.Cmd
	statusRequest       int
	sessionPicker       list.Model
	pickerOpen          bool
	pickerBefore        string
	pickerHasMore       bool
	pickerRequest       int
	pickerLoading       bool
	deleteConfirming    bool
	actionRequest       int
	commandMenu         list.Model
	commandMenuOpen     bool
}

func NewTUIModel(ctx context.Context, streamer Streamer, options TUIOptions) *TUIModel {
	composer := textarea.New()
	composer.DynamicHeight = true
	composer.MinHeight = 1
	composer.MaxHeight = 8
	composer.Placeholder = "Ask Rootly..."
	composer.ShowLineNumbers = false
	composer.SetHeight(1)
	composer.SetPromptFunc(2, func(info textarea.PromptInfo) string {
		if info.LineNumber == 0 {
			return "❯ "
		}
		return "  "
	})
	composer.KeyMap.InsertNewline = key.NewBinding(key.WithKeys("enter", "shift+enter", "alt+enter"))
	_ = composer.Focus()

	spin := spinner.New()
	spin.Spinner = spinner.Dot

	var currentUser *rootly.CurrentUserResponse
	if options.CurrentUser != nil {
		user := *options.CurrentUser
		currentUser = &user
	}
	sessionTitle := sanitizeTerminalText(options.SessionTitle)

	return &TUIModel{
		streamer:          streamer,
		composer:          composer,
		spinner:           spin,
		ctx:               ctx,
		width:             80,
		height:            24,
		noColor:           options.NoColor,
		version:           options.Version,
		verbIndex:         rand.IntN,
		now:               time.Now,
		api:               options.API,
		apiHost:           options.APIHost,
		currentUser:       currentUser,
		userLoaded:        options.CurrentUser != nil,
		sessionID:         options.SessionID,
		sessionTitle:      sessionTitle,
		resumeLinePending: options.SessionID != nil && sessionTitle == "",
		incident:          options.Incident,
		alert:             options.Alert,
		historyIndex:      -1,
	}
}

func (m *TUIModel) Init() tea.Cmd {
	m.welcomeStarted = true
	m.welcomePending = !m.noColor
	m.welcomeAnimating = false
	m.welcomePrinted = false
	m.welcomeFrame = 0
	m.pendingPrints = nil
	m.quitting = false
	cmds := make([]tea.Cmd, 0, 5)
	if m.noColor || (m.colorProfileKnown && m.colorProfile < colorprofile.ANSI256) {
		cmds = append(cmds, m.printWelcomeBanner())
	} else if m.colorProfileKnown {
		m.welcomePending = false
		m.welcomeAnimating = true
	}
	cmds = append(cmds, m.composer.Focus())
	if m.sessionID != nil {
		if !m.resumeLinePending {
			cmds = append(cmds, m.println(m.resumedLine()))
		}
		cmds = append(cmds, m.loadHistory(*m.sessionID, ""))
	}
	if header := m.bindingHeader(); header != "" {
		cmds = append(cmds, m.println(faint(m.noColor, header)))
	}
	if m.welcomeAnimating {
		cmds = append(cmds, m.welcomeBannerTick())
	}
	return tea.Sequence(cmds...)
}

func (m *TUIModel) resumedLine() string {
	title := sanitizeTerminalText(m.sessionTitle)
	if title == "" && m.sessionID != nil {
		title = shortSessionID(m.sessionID.String())
	}
	return faint(m.noColor, fmt.Sprintf("Resumed %q", title))
}

func (m *TUIModel) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := message.(type) {
	case tea.ColorProfileMsg:
		m.colorProfile = msg.Profile
		m.colorProfileKnown = true
		if !m.welcomePending {
			return m, nil
		}
		if !m.noColor && msg.Profile >= colorprofile.ANSI256 {
			m.welcomePending = false
			m.welcomeAnimating = true
			m.welcomeFrame = 0
			cmd := m.welcomeBannerTick()
			return m, cmd
		}
		cmd := m.printWelcomeBanner()
		return m, cmd
	case welcomeBannerTickMsg:
		if !m.welcomeAnimating {
			return m, nil
		}
		m.welcomeFrame++
		if m.welcomeFrame >= welcomeSettleFrame {
			cmd := m.printWelcomeBanner()
			return m, cmd
		}
		cmd := m.welcomeBannerTick()
		return m, cmd
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.composer.SetWidth(max(20, msg.Width-2))
		m.composer.MaxHeight = max(3, msg.Height/3)
		if m.pickerOpen {
			m.sessionPicker.SetSize(msg.Width, min(12, max(1, msg.Height/2)))
		}
		if m.commandMenuOpen {
			m.commandMenu.SetSize(max(20, msg.Width), min(6, len(m.commandMenu.Items())))
		}
		return m, nil
	case spinner.TickMsg:
		if m.state == tuiStreaming {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			return m, tea.Batch(cmd, m.spinner.Tick)
		}
	case streamMessage:
		return m.updateStream(msg)
	case historyLoadedMsg:
		if msg.request != m.historyRequest {
			return m, nil
		}
		var resumedCmd tea.Cmd
		if m.resumeLinePending {
			if msg.sessionTitle != "" {
				m.sessionTitle = sanitizeTerminalText(msg.sessionTitle)
			}
			m.resumeLinePending = false
			resumedCmd = m.println(m.resumedLine())
		}
		if msg.err != nil {
			errorCmd := m.println(faint(m.noColor, "Could not load session history: "+sanitizeTerminalText(msg.err.Error())))
			if resumedCmd != nil {
				return m, tea.Sequence(resumedCmd, errorCmd)
			}
			return m, errorCmd
		}
		cmd := m.printHistory(msg.title, msg.messages)
		if resumedCmd != nil {
			return m, tea.Sequence(resumedCmd, cmd)
		}
		return m, cmd
	case statusLoadedMsg:
		if msg.request != m.statusRequest {
			return m, nil
		}
		m.userLoaded = true
		if msg.response != nil && msg.response.ApplicationVndAPIJSON200 != nil {
			m.currentUser = msg.response.ApplicationVndAPIJSON200
		}
		cmd := m.println(m.statusLines())
		return m, cmd
	case sessionPageMsg:
		cmd := m.handleSessionPage(msg)
		return m, cmd
	case sessionActionMsg:
		if msg.request != m.actionRequest {
			return m, nil
		}
		cmd := m.handleSessionAction(msg)
		return m, cmd
	case bindingResolvedMsg:
		if msg.request != m.actionRequest {
			return m, nil
		}
		if msg.err != nil {
			cmd := m.println(sanitizeTerminalText(msg.err.Error()))
			return m, cmd
		}
		m.sessionID = nil
		m.answer.Reset()
		m.transcript = nil
		m.lastAnswer = ""
		m.sessionTitle = ""
		m.incident, m.alert = msg.incident, msg.alert
		m.incidentUsed, m.alertUsed = false, false
		m.status = ""
		cmd := m.println(faint(m.noColor, m.bindingHeader()))
		return m, cmd
	case tea.KeyPressMsg:
		return m.updateKey(msg)
	default:
		if m.pickerOpen {
			var cmd tea.Cmd
			m.sessionPicker, cmd = m.sessionPicker.Update(message)
			return m, cmd
		}
	}
	return m, nil
}

func (m *TUIModel) updateStream(message streamMessage) (tea.Model, tea.Cmd) {
	if message.turn != m.turn {
		return m, nil
	}
	if m.state != tuiStreaming {
		return m, nil
	}
	if message.done {
		return m.finishTurn(message.err)
	}
	if message.err != nil {
		m.status = sanitizeTerminalText(message.err.Error())
		cmd := m.waitForStream()
		return m, cmd
	}
	m.handleEvent(message.event)
	if message.event.Type == rootly.AiChatStreamEventError && message.event.Error != nil {
		return m.finishTurn(errors.New(sanitizeTerminalText(message.event.Error.Message)))
	}
	if len(m.activityCommands) > 0 {
		commands := append([]tea.Cmd(nil), m.activityCommands...)
		m.activityCommands = nil
		commands = append(commands, tea.Batch(m.waitForStream(), m.spinner.Tick))
		return m, tea.Sequence(commands...)
	}
	cmd := m.waitForStream()
	return m, cmd
}

func (m *TUIModel) handleEvent(event Event) {
	for _, line := range m.activities.Handle(event) {
		m.activityCommands = append(m.activityCommands, m.println(faint(m.noColor, sanitizeTerminalText(line))))
	}
	switch event.Type {
	case rootly.AiChatStreamEventSessionID:
		if event.SessionID != nil {
			if id, err := uuid.Parse(event.SessionID.SessionID); err == nil {
				m.sessionID = &id
			}
		}
	case rootly.AiChatStreamEventText:
		if event.Text != nil {
			if m.resumeTextSeparator {
				if m.answer.Len() > 0 && !strings.HasSuffix(m.answer.String(), "\n") {
					m.answer.WriteString("\n\n")
				}
				m.resumeTextSeparator = false
			}
			m.answer.WriteString(sanitizeTerminalText(event.Text.Content))
		}
	case rootly.AiChatStreamEventStatus:
		if event.Status != nil {
			m.status = sanitizeTerminalText(event.Status.Status)
		}
	case rootly.AiChatStreamEventTaskUpdate:
		if event.TaskUpdate != nil {
			m.status = fmt.Sprintf("%s (%s)", sanitizeTerminalText(event.TaskUpdate.Title), sanitizeTerminalText(event.TaskUpdate.Status))
		}
	case rootly.AiChatStreamEventSkillLoaded:
		if event.SkillLoaded != nil {
			skillName := sanitizeTerminalText(event.SkillLoaded.Skill.Name)
			m.activityCommands = append(m.activityCommands, m.println(faint(m.noColor, "• Loaded skill "+skillName)))
		}
	case rootly.AiChatStreamEventConfirmationRequired:
		m.pending = event.ConfirmationRequired
	case rootly.AiChatStreamEventError:
		if event.Error != nil {
			m.status = sanitizeTerminalText(event.Error.Message)
		}
	case rootly.AiChatStreamEventDone:
		if event.Done != nil {
			m.doneStatus = sanitizeTerminalText(event.Done.Status)
		}
	}
}

func (m *TUIModel) finishTurn(err error) (tea.Model, tea.Cmd) {
	if m.cancel != nil {
		m.cancel()
	}
	m.cancel = nil
	m.stream = nil
	m.state = tuiIdle
	if err != nil {
		for _, line := range m.activities.Finish(false) {
			m.activityCommands = append(m.activityCommands, m.println(faint(m.noColor, line)))
		}
	} else if m.doneStatus == "confirmation_required" {
		m.activities.dropConfirmation()
		for _, line := range m.activities.Finish(true) {
			m.activityCommands = append(m.activityCommands, m.println(faint(m.noColor, line)))
		}
	} else if m.doneStatus != "" {
		for _, line := range m.activities.Finish(m.doneStatus == "complete") {
			m.activityCommands = append(m.activityCommands, m.println(faint(m.noColor, line)))
		}
	}
	commands := append([]tea.Cmd(nil), m.activityCommands...)
	m.activityCommands = nil
	if err != nil {
		m.status = ""
		m.pending = nil
		m.doneStatus = ""
		cmd := m.scrollback(sanitizeTerminalText(err.Error()), lipgloss.NewStyle().Foreground(lipgloss.Color("1")))
		m.activities.Reset()
		return m, sequenceTUICommands(append(commands, cmd)...)
	}
	if m.doneStatus == "error" {
		m.status = ""
		m.pending = nil
		m.doneStatus = ""
		cmd := m.scrollback("AI chat turn failed", lipgloss.NewStyle().Foreground(lipgloss.Color("1")))
		m.activities.Reset()
		return m, sequenceTUICommands(append(commands, cmd)...)
	}
	if m.doneStatus == "confirmation_required" && m.pending != nil {
		if expired(m.pending) {
			m.pending = nil
			m.status = ""
			m.doneStatus = ""
			cmd := m.scrollback("Confirmation expired; no action was taken.", lipgloss.NewStyle())
			m.activities.Reset()
			return m, sequenceTUICommands(append(commands, cmd)...)
		}
		m.state = tuiConfirming
		return m, sequenceTUICommands(commands...)
	}
	if m.doneStatus == "cancelled" {
		m.status = ""
		m.pending = nil
		m.doneStatus = ""
		cmd := m.scrollback("Action cancelled.", lipgloss.NewStyle())
		m.activities.Reset()
		return m, sequenceTUICommands(append(commands, cmd)...)
	}
	if m.doneStatus == "expired" {
		m.status = ""
		m.pending = nil
		m.doneStatus = ""
		cmd := m.scrollback("Confirmation expired; no action was taken.", lipgloss.NewStyle())
		m.activities.Reset()
		return m, sequenceTUICommands(append(commands, cmd)...)
	}

	rawAnswer := m.answer.String()
	answer := strings.TrimSpace(rawAnswer)
	if answer != "" {
		m.lastAnswer = rawAnswer
		m.transcript = append(m.transcript, transcriptTurn{user: m.currentPrompt, assistant: rawAnswer})
		commands = append(commands,
			m.println("\n"+m.renderAssistantAnswer(rawAnswer)+"\n"+m.turnFooter()),
			tea.RequestWindowSize,
		)
	}
	if m.doneStatus != "" && m.doneStatus != "complete" && m.doneStatus != "confirmed" {
		m.status = "turn halted: " + sanitizeTerminalText(m.doneStatus)
	} else {
		m.status = ""
	}
	m.pending = nil
	m.doneStatus = ""
	m.turnStartedAt = time.Time{}
	m.currentPrompt = ""
	m.activities.Reset()
	return m, sequenceTUICommands(commands...)
}

func sequenceTUICommands(commands ...tea.Cmd) tea.Cmd {
	sequence := make([]tea.Cmd, 0, len(commands))
	for _, command := range commands {
		if command != nil {
			sequence = append(sequence, command)
		}
	}
	if len(sequence) == 0 {
		return nil
	}
	return tea.Sequence(sequence...)
}

func (m *TUIModel) scrollback(message string, style lipgloss.Style) tea.Cmd {
	lines := make([]string, 0, 4)
	rawAnswer := m.answer.String()
	if strings.TrimSpace(rawAnswer) != "" {
		m.lastAnswer = rawAnswer
		lines = append(lines, "", m.renderAssistantAnswer(rawAnswer))
	}
	if m.noColor {
		lines = append(lines, message)
	} else {
		lines = append(lines, style.Render(message))
	}
	if !m.turnStartedAt.IsZero() && m.sessionID != nil {
		lines = append(lines, m.turnFooter())
	}
	m.answer.Reset()
	m.turnStartedAt = time.Time{}
	m.currentPrompt = ""
	return m.println(strings.Join(lines, "\n"))
}

func (m *TUIModel) updateKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if m.commandMenuOpen {
		switch msg.String() {
		case "esc":
			m.commandMenuOpen = false
			return m, nil
		case "up", "down":
			var cmd tea.Cmd
			m.commandMenu, cmd = m.commandMenu.Update(msg)
			return m, cmd
		case "tab":
			if item, ok := m.commandMenu.SelectedItem().(slashCommandItem); ok {
				m.composer.SetValue(item.command + " ")
			}
			m.commandMenuOpen = false
			return m, nil
		case "enter":
			typed := strings.TrimSpace(m.composer.Value())
			if item, ok := m.commandMenu.SelectedItem().(slashCommandItem); ok &&
				len(typed) < len(item.command) && strings.HasPrefix(strings.ToLower(item.command), strings.ToLower(typed)) {
				m.composer.SetValue(item.command)
			}
			m.commandMenuOpen = false
			cmd := m.submit()
			return m, cmd
		}
	}
	if m.pickerOpen {
		return m.handlePickerKey(msg)
	}
	if m.deleteConfirming {
		switch strings.ToLower(msg.String()) {
		case "y":
			m.deleteConfirming = false
			cmd := m.deleteSession()
			return m, cmd
		case "n", "esc":
			m.deleteConfirming = false
			return m, nil
		}
		return m, nil
	}
	if msg.String() == "ctrl+c" {
		if m.state == tuiStreaming {
			cmd := m.scrollback("Stopped locally. The server may still finish this turn and save it to the session.", lipgloss.NewStyle().Faint(true))
			if m.cancel != nil {
				m.cancel()
			}
			m.state = tuiIdle
			m.cancel = nil
			m.stream = nil
			m.status = ""
			return m, cmd
		}
		if m.idleInterrupt {
			cmd := m.quitCommand()
			return m, cmd
		}
		m.idleInterrupt = true
		m.composer.Reset()
		m.status = "press Ctrl+C again to exit"
		return m, nil
	}
	if m.idleInterrupt {
		m.idleInterrupt = false
		m.status = ""
	}

	if m.state == tuiConfirming {
		switch strings.ToLower(msg.String()) {
		case "y":
			cmd := m.handleConfirmationDecision(rootly.StreamAiChatParamsDecisionConfirm)
			return m, cmd
		case "n", "esc":
			cmd := m.handleConfirmationDecision(rootly.StreamAiChatParamsDecisionCancel)
			return m, cmd
		}
		return m, nil
	}

	if msg.String() == "ctrl+d" && strings.TrimSpace(m.composer.Value()) == "" {
		cmd := m.quitCommand()
		return m, cmd
	}
	if m.state == tuiStreaming {
		return m, nil
	}
	if m.historyKey(msg) {
		return m, nil
	}

	if msg.Code == tea.KeyEnter && !msg.Mod.Contains(tea.ModAlt) && !msg.Mod.Contains(tea.ModShift) {
		cmd := m.submit()
		return m, cmd
	}
	if msg.Code == tea.KeyEnter && (msg.Mod.Contains(tea.ModAlt) || msg.Mod.Contains(tea.ModShift)) {
		m.idleInterrupt = false
		var cmd tea.Cmd
		m.composer, cmd = m.composer.Update(msg)
		m.refreshCommandMenu()
		return m, cmd
	}

	if msg.String() == "esc" {
		m.idleInterrupt = false
	}
	m.historyIndex = -1
	var cmd tea.Cmd
	m.composer, cmd = m.composer.Update(msg)
	m.refreshCommandMenu()
	return m, cmd
}

func (m *TUIModel) handleConfirmationDecision(decision rootly.StreamAiChatParamsDecision) tea.Cmd {
	if expired(m.pending) {
		m.pending = nil
		m.state = tuiIdle
		m.status = ""
		m.doneStatus = ""
		return m.scrollback("Confirmation expired; no action was taken.", lipgloss.NewStyle())
	}
	return m.resume(decision)
}

func (m *TUIModel) submit() tea.Cmd {
	welcomeCmd := m.printWelcomeBanner()
	message := strings.TrimSpace(m.composer.Value())
	m.composer.Reset()
	m.idleInterrupt = false
	m.status = ""
	if message == "" {
		return welcomeCmd
	}
	if isExitAlias(message) {
		return sequenceTUICommands(welcomeCmd, m.quitCommand())
	}
	if strings.HasPrefix(message, "/") {
		m.commandMenuOpen = false
		return sequenceTUICommands(welcomeCmd, m.runSlashCommand(message))
	}

	m.currentPrompt = message
	m.promptHistory = append(m.promptHistory, message)
	m.historyIndex = -1
	m.actionRequest++
	m.startNewTurn()
	m.activities.Reset()
	params := &rootly.StreamAiChatParams{Message: &message, SessionID: m.sessionID}
	if m.incident != nil && !m.incidentUsed {
		if id, err := uuid.Parse(m.incident.ID); err == nil {
			params.IncidentID = &id
		}
		m.incidentUsed = true
	}
	if m.alert != nil && !m.alertUsed {
		if id, err := uuid.Parse(m.alert.ID); err == nil {
			params.AlertID = &id
		}
		m.alertUsed = true
	}
	cmd := m.startStream(params)
	return sequenceTUICommands(welcomeCmd, m.println(renderPromptEcho(message, m.width, m.noColor)), cmd)
}

func (m *TUIModel) runSlashCommand(command string) tea.Cmd {
	parts := strings.Fields(strings.TrimSpace(command))
	if len(parts) == 0 {
		return nil
	}
	switch parts[0] {
	case "/exit", "/quit":
		return m.quitCommand()
	case "/new", "/clear":
		m.startNewSession()
		m.status = "Started a new session."
	case "/help":
		m.status = m.helpView()
	case "/status":
		return m.statusCommand()
	case "/resume":
		return m.openSessionPicker()
	case "/rename":
		title := strings.TrimSpace(strings.Join(parts[1:], " "))
		body := rootly.UpdateAiChatSession{}
		message := "Renamed to generated title."
		if title == "" {
			body.Title = nullable.NewNullNullable[string]()
		} else {
			body.Title = nullable.NewNullableWithValue(title)
			message = "Renamed to " + title + "."
		}
		return m.sessionAction("rename", message, body, &title)
	case "/pin", "/unpin":
		pinned := parts[0] == "/pin"
		body := rootly.UpdateAiChatSession{Pinned: &pinned}
		action := "Unpinned"
		if pinned {
			action = "Pinned"
		}
		if m.sessionID == nil {
			return m.println("No session yet: send a message first.")
		}
		return m.sessionAction(parts[0], action+" session "+shortSessionID(m.sessionID.String())+".", body, nil)
	case "/archive":
		archived := true
		if m.sessionID == nil {
			return m.println("No session yet: send a message first.")
		}
		return m.sessionAction("archive", "Archived session "+shortSessionID(m.sessionID.String())+".", rootly.UpdateAiChatSession{Archived: &archived}, nil)
	case "/delete":
		if m.sessionID == nil {
			return m.println("No session yet: send a message first.")
		}
		m.deleteConfirming = true
		return nil
	case "/copy":
		if m.sessionID == nil {
			return m.println("No session yet: send a message first.")
		}
		if m.lastAnswer == "" {
			return m.println("Nothing to copy yet.")
		}
		return tea.Batch(tea.SetClipboard(m.lastAnswer), m.println(faint(m.noColor, "Copied last answer.")))
	case "/export":
		return m.exportTranscript(strings.Join(parts[1:], " "))
	case "/incident", "/alert":
		if len(parts) < 2 {
			return m.println(fmt.Sprintf("Usage: %s <id>", parts[0]))
		}
		return m.resolveBinding(strings.TrimPrefix(parts[0], "/"), parts[1])
	default:
		return m.println(fmt.Sprintf("Unknown command %s. Type /help.", parts[0]))
	}
	return nil
}

func (m *TUIModel) startStream(params *rootly.StreamAiChatParams) tea.Cmd {
	m.answer.Reset()
	return m.beginStream(params)
}

func (m *TUIModel) beginStream(params *rootly.StreamAiChatParams) tea.Cmd {
	m.turn++
	turn := m.turn
	m.state = tuiStreaming
	m.resumeTextSeparator = params.CheckpointID != nil
	m.pending = nil
	m.doneStatus = ""
	if m.turnStartedAt.IsZero() {
		m.startNewTurn()
	}
	m.startedAt = m.turnStartedAt
	m.currentParams = params
	m.stream = make(chan streamMessage, 16)
	ctx, cancel := context.WithCancel(m.ctx)
	m.cancel = cancel
	stream := m.stream

	go func() {
		defer close(stream)
		err := StreamTurn(ctx, m.streamer, params, func(event Event) bool {
			select {
			case stream <- streamMessage{event: event, turn: turn}:
				return true
			case <-ctx.Done():
				return false
			}
		})
		select {
		case stream <- streamMessage{err: err, done: true, turn: turn}:
		case <-ctx.Done():
		}
	}()

	return tea.Batch(m.waitForStream(), m.spinner.Tick)
}

func (m *TUIModel) resume(decision rootly.StreamAiChatParamsDecision) tea.Cmd {
	if m.pending == nil || m.sessionID == nil {
		m.state = tuiIdle
		m.status = "confirmation is missing its session or checkpoint"
		return nil
	}
	checkpointID, err := uuid.Parse(m.pending.CheckpointID)
	if err != nil {
		m.state = tuiIdle
		m.status = "confirmation checkpoint is invalid"
		return nil
	}
	params := &rootly.StreamAiChatParams{
		SessionID:    m.sessionID,
		CheckpointID: &checkpointID,
		Decision:     &decision,
	}
	m.pending = nil
	return m.beginStream(params)
}

func (m *TUIModel) waitForStream() tea.Cmd {
	stream := m.stream
	turn := m.turn
	return func() tea.Msg {
		message, ok := <-stream
		if !ok {
			return streamMessage{err: fmt.Errorf("stream closed"), done: true, turn: turn}
		}
		return message
	}
}

func (m *TUIModel) View() tea.View {
	if m.quitting {
		return tea.NewView("")
	}
	var sections []string
	if m.welcomeAnimating {
		sections = append(sections, m.welcomeBannerFrame())
	}
	switch m.state {
	case tuiStreaming:
		markdown := m.answer.String()
		if !m.noColor {
			markdown = trimIncompleteTrailingMarkdownLink(markdown)
		}
		if rendered := m.renderMarkdown(markdown, m.width-2); rendered != "" {
			sections = append(sections, rendered)
		}
		elapsed := m.now().Sub(m.startedAt).Truncate(time.Second)
		status := m.status
		if status == "" {
			status = statusVerbs[m.turnVerbIndex].Active + "…"
		}
		sections = append(sections, fmt.Sprintf("%s %s · %s", m.spinner.View(), status, elapsed))
	case tuiConfirming:
		if m.pending != nil {
			sections = append(sections, renderConfirmationCard(m.pending, m.width, m.noColor, m.now()))
		}
	default:
		if header := m.bindingHeader(); header != "" {
			sections = append(sections, faint(m.noColor, header))
		}
		if m.status != "" {
			sections = append(sections, m.status)
		}
	}
	if m.deleteConfirming {
		sections = append(sections, "Delete this session? This can't be undone.\ny delete · n/Esc cancel")
	} else if m.pickerOpen {
		sections = append(sections, m.sessionPicker.View())
	} else if m.commandMenuOpen {
		sections = append(sections, m.commandMenu.View())
	}
	if m.state != tuiConfirming && !m.deleteConfirming {
		sections = append(sections,
			faint(m.noColor, strings.Repeat("─", max(1, m.width))),
			m.composer.View(),
			faint(m.noColor, strings.Repeat("─", max(1, m.width))),
			faint(m.noColor, "/ for commands · exit to quit"),
		)
	}
	body := strings.Join(sections, "\n")
	view := tea.NewView(body)
	return view
}

func (m *TUIModel) renderMarkdown(markdown string, width int) string {
	markdown = sanitizeTerminalText(markdown)
	if m.noColor {
		return markdown
	}
	if strings.TrimSpace(markdown) == "" {
		return ""
	}
	if width < 20 {
		width = 20
	}
	if m.renderer == nil || m.rendererWidth != width {
		renderer, err := glamour.NewTermRenderer(glamour.WithEnvironmentConfig(), glamour.WithWordWrap(width))
		if err != nil {
			return markdown
		}
		m.renderer = renderer
		m.rendererWidth = width
		m.renderedSource = ""
	}
	if m.renderedSource != markdown {
		source, links := preprocessMarkdownLinksWithURLs(markdown, m.apiHost)
		rendered, err := m.renderer.Render(source)
		if err != nil {
			return markdown
		}
		m.rendered = strings.TrimSpace(restoreMarkdownLinks(rendered, links))
		m.renderedSource = markdown
		m.renderCount++
	}
	return m.rendered
}
