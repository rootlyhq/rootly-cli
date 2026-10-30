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
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/glamour/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/colorprofile"
	"github.com/charmbracelet/x/ansi"
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

type transcriptBlock struct {
	text      string
	gapBefore bool
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
	streamer             Streamer
	state                tuiState
	composer             textarea.Model
	spinner              spinner.Model
	width                int
	height               int
	windowSizeReceived   bool
	viewport             viewport.Model
	viewportContent      string
	newOutputAvailable   bool
	status               string
	answer               strings.Builder
	committedOffset      int
	sessionID            *uuid.UUID
	pending              *rootly.AiChatStreamConfirmationRequired
	ctx                  context.Context
	cancel               context.CancelFunc
	stream               chan streamMessage
	startedAt            time.Time
	doneStatus           string
	doneReceived         bool
	idleInterrupt        bool
	currentParams        *rootly.StreamAiChatParams
	turn                 int
	noColor              bool
	renderer             *glamour.TermRenderer
	rendererWidth        int
	renderedSource       string
	rendered             string
	renderCount          int
	resumeTextSeparator  bool
	api                  TUIAPI
	apiHost              string
	currentUser          *rootly.CurrentUserResponse
	userLoaded           bool
	sessionTitle         string
	incident             *api.Incident
	alert                *api.Alert
	incidentUsed         bool
	alertUsed            bool
	resumedSession       bool
	historyRequest       int
	promptHistory        []string
	historyIndex         int
	historyDraft         string
	currentPrompt        string
	turnStartedAt        time.Time
	turnVerbIndex        int
	verbIndex            func(int) int
	now                  func() time.Time
	version              string
	colorProfile         colorprofile.Profile
	colorProfileKnown    bool
	welcomePending       bool
	welcomeAnimating     bool
	welcomeFrame         int
	quitting             bool
	lastAnswer           string
	transcript           []transcriptBlock
	transcriptCache      string
	transcriptCacheWidth int
	transcriptCacheDirty bool
	conversation         []transcriptTurn
	activities           activityTracker
	statusRequest        int
	sessionPicker        list.Model
	pickerOpen           bool
	pickerBefore         string
	pickerHasMore        bool
	pickerRequest        int
	pickerLoading        bool
	deleteConfirming     bool
	actionRequest        int
	commandMenu          list.Model
	commandMenuOpen      bool
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

	model := &TUIModel{
		streamer:       streamer,
		composer:       composer,
		spinner:        spin,
		ctx:            ctx,
		width:          80,
		height:         24,
		noColor:        options.NoColor,
		version:        options.Version,
		verbIndex:      rand.IntN,
		now:            time.Now,
		api:            options.API,
		apiHost:        options.APIHost,
		currentUser:    currentUser,
		userLoaded:     options.CurrentUser != nil,
		sessionID:      options.SessionID,
		sessionTitle:   normalizeSessionTitle(sessionTitle),
		resumedSession: options.SessionID != nil,
		incident:       options.Incident,
		alert:          options.Alert,
		historyIndex:   -1,
	}
	model.viewport = viewport.New()
	model.viewport.SoftWrap = true
	model.viewport.FillHeight = true
	model.viewport.KeyMap = viewport.KeyMap{}
	model.resetTranscript()
	return model
}

func (m *TUIModel) Init() tea.Cmd {
	m.welcomePending = !m.noColor && !m.colorProfileKnown
	m.welcomeAnimating = !m.noColor && m.colorProfileKnown && m.colorProfile >= colorprofile.ANSI256
	m.welcomeFrame = 0
	m.quitting = false
	cmds := make([]tea.Cmd, 0, 5)
	cmds = append(cmds, m.composer.Focus())
	if m.sessionID != nil {
		cmds = append(cmds, m.loadHistory(*m.sessionID))
	}
	if m.welcomeAnimating {
		cmds = append(cmds, m.welcomeBannerTick())
	}
	return tea.Sequence(cmds...)
}

func (m *TUIModel) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := message.(type) {
	case tea.ColorProfileMsg:
		m.colorProfile = msg.Profile
		m.colorProfileKnown = true
		if !m.welcomePending {
			return m, nil
		}
		m.welcomePending = false
		if !m.noColor && msg.Profile >= colorprofile.ANSI256 {
			m.welcomeAnimating = true
			m.welcomeFrame = 0
			cmd := m.welcomeBannerTick()
			return m, cmd
		}
		return m, nil
	case welcomeBannerTickMsg:
		if !m.welcomeAnimating {
			return m, nil
		}
		m.welcomeFrame++
		if m.welcomeFrame >= welcomeSettleFrame {
			m.welcomeAnimating = false
			return m, nil
		}
		cmd := m.welcomeBannerTick()
		return m, cmd
	case tea.WindowSizeMsg:
		m.windowSizeReceived = true
		m.width = msg.Width
		m.height = msg.Height
		m.composer.SetWidth(max(1, msg.Width-2))
		m.composer.MaxHeight = max(3, msg.Height/3)
		if m.pickerOpen {
			m.sessionPicker.SetSize(msg.Width, min(12, max(1, msg.Height/2)))
		}
		if m.commandMenuOpen {
			m.commandMenu.SetSize(max(20, msg.Width), min(6, len(m.commandMenu.Items())))
		}
		return m, nil
	case tea.MouseWheelMsg:
		if !m.pickerOpen && !m.commandMenuOpen && !m.deleteConfirming {
			m.viewport, _ = m.viewport.Update(msg)
			m.updateNewOutputIndicator()
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
		if title := normalizeSessionTitle(msg.sessionTitle); title != "" {
			m.sessionTitle = title
		}
		if msg.err != nil {
			cmd := m.println(faint(m.noColor, "Could not load session history: "+sanitizeTerminalText(msg.err.Error())))
			return m, cmd
		}
		m.printHistory(msg.messages)
		return m, nil
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
		m.resetAnswer()
		m.resetTranscript()
		m.resumedSession = false
		m.lastAnswer = ""
		m.sessionTitle = ""
		m.incident, m.alert = msg.incident, msg.alert
		m.incidentUsed, m.alertUsed = false, false
		m.status = ""
		m.println(faint(m.noColor, m.bindingHeader()))
		return m, nil
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
	if message.event.Type == rootly.AiChatStreamEventText {
		m.commitCompleteAnswerBlocks()
	}
	cmd := m.waitForStream()
	return m, cmd
}

func (m *TUIModel) handleEvent(event Event) {
	for _, line := range m.activities.Handle(event) {
		m.println(faint(m.noColor, sanitizeTerminalText(line)))
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
			m.println(faint(m.noColor, "• Loaded skill "+skillName))
		}
	case rootly.AiChatStreamEventConfirmationRequired:
		m.pending = event.ConfirmationRequired
	case rootly.AiChatStreamEventError:
		if event.Error != nil {
			m.status = sanitizeTerminalText(event.Error.Message)
		}
	case rootly.AiChatStreamEventDone:
		m.doneReceived = true
		if event.Done != nil {
			m.doneStatus = sanitizeTerminalText(event.Done.Status)
		}
	}
}

func (m *TUIModel) resetAnswer() {
	m.answer.Reset()
	m.committedOffset = 0
}

func (m *TUIModel) uncommittedAnswer() string {
	answer := m.answer.String()
	offset := min(max(m.committedOffset, 0), len(answer))
	return answer[offset:]
}

func (m *TUIModel) commitCompleteAnswerBlocks() tea.Cmd {
	answer := m.answer.String()
	boundary := lastTopLevelBlockBoundary(answer, m.committedOffset)
	if boundary <= m.committedOffset {
		return nil
	}
	chunk := answer[m.committedOffset:boundary]
	if strings.TrimSpace(chunk) == "" {
		return nil
	}
	gapBefore := m.committedOffset > 0
	rendered := m.renderAssistantAnswerPart(chunk, m.committedOffset == 0)
	m.committedOffset = boundary
	return m.printlnWithGap(rendered, gapBefore)
}

func lastTopLevelBlockBoundary(answer string, committed int) int {
	boundary := committed
	inFence := false
	var fenceMarker rune
	fenceLength := 0
	for lineStart := 0; lineStart < len(answer); {
		lineEnd := strings.IndexByte(answer[lineStart:], '\n')
		if lineEnd < 0 {
			break
		}
		lineEnd += lineStart
		line := answer[lineStart:lineEnd]
		if marker, length, closing, ok := markdownFence(line); ok {
			if !inFence {
				inFence = true
				fenceMarker, fenceLength = marker, length
			} else if closing && marker == fenceMarker && length >= fenceLength {
				inFence = false
			}
		}
		if !inFence && lineEnd+1 < len(answer) && answer[lineEnd+1] == '\n' && lineEnd+2 > committed {
			boundary = lineEnd + 2
		}
		lineStart = lineEnd + 1
	}
	return boundary
}

func markdownFence(line string) (marker rune, length int, closing, ok bool) {
	indent := len(line) - len(strings.TrimLeft(line, " "))
	if indent > 3 {
		return 0, 0, false, false
	}
	line = line[indent:]
	if line == "" || (line[0] != '`' && line[0] != '~') {
		return 0, 0, false, false
	}
	marker = rune(line[0])
	length = 0
	for length < len(line) && rune(line[length]) == marker {
		length++
	}
	if length < 3 {
		return 0, 0, false, false
	}
	closing = strings.TrimSpace(line[length:]) == ""
	return marker, length, closing, true
}

func (m *TUIModel) finishTurn(err error) (tea.Model, tea.Cmd) {
	if err == nil && !m.doneReceived {
		err = errors.New("AI chat stream ended before the turn finished")
	}
	m.doneReceived = false
	if m.cancel != nil {
		m.cancel()
	}
	m.cancel = nil
	m.stream = nil
	m.state = tuiIdle
	if err != nil {
		for _, line := range m.activities.Finish(false) {
			m.println(faint(m.noColor, line))
		}
	} else if m.doneStatus == "confirmation_required" {
		m.activities.dropConfirmation()
		for _, line := range m.activities.Finish(true) {
			m.println(faint(m.noColor, line))
		}
	} else if m.doneStatus != "" {
		for _, line := range m.activities.Finish(isSuccessfulDoneStatus(m.doneStatus)) {
			m.println(faint(m.noColor, line))
		}
	}
	var commands []tea.Cmd
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
		if rawAnswer := m.answer.String(); strings.TrimSpace(rawAnswer) != "" {
			m.lastAnswer = rawAnswer
			if remainder := m.uncommittedAnswer(); strings.TrimSpace(remainder) != "" {
				rendered := m.renderAssistantAnswerPart(remainder, m.committedOffset == 0)
				commands = append(commands, m.printlnWithGap(rendered, m.committedOffset > 0))
			}
			m.committedOffset = len(rawAnswer)
		}
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
		m.conversation = append(m.conversation, transcriptTurn{user: m.currentPrompt, assistant: rawAnswer})
		remainder := m.uncommittedAnswer()
		footer := "\n" + m.turnFooter()
		if strings.TrimSpace(remainder) != "" {
			rendered := m.renderAssistantAnswerPart(remainder, m.committedOffset == 0)
			commands = append(commands, m.printlnWithGap(rendered+footer, m.committedOffset > 0))
		} else {
			commands = append(commands, m.println(footer))
		}
		m.committedOffset = len(rawAnswer)
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
		if remainder := m.uncommittedAnswer(); strings.TrimSpace(remainder) != "" {
			rendered := m.renderAssistantAnswerPart(remainder, m.committedOffset == 0)
			m.appendTranscriptBlockWithGap(rendered, m.committedOffset > 0)
		}
	}
	if m.noColor {
		lines = append(lines, message)
	} else {
		lines = append(lines, style.Render(message))
	}
	if !m.turnStartedAt.IsZero() && m.sessionID != nil {
		lines = append(lines, m.turnFooter())
	}
	m.resetAnswer()
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
		case "pgup", "pgdown", "home", "end", "ctrl+home", "ctrl+end":
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
	if m.scrollViewportKey(msg) {
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
	m.settleWelcomeAnimation()
	message := strings.TrimSpace(m.composer.Value())
	m.composer.Reset()
	m.idleInterrupt = false
	m.status = ""
	if message == "" {
		return nil
	}
	if isExitAlias(message) {
		return m.quitCommand()
	}
	if strings.HasPrefix(message, "/") {
		m.commandMenuOpen = false
		return m.runSlashCommand(message)
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
	return sequenceTUICommands(m.println(renderPromptEcho(message, m.width, m.noColor)), cmd)
}

func (m *TUIModel) scrollViewportKey(msg tea.KeyPressMsg) bool {
	switch msg.String() {
	case "pgup":
		m.viewport.PageUp()
	case "pgdown":
		m.viewport.PageDown()
	case "ctrl+home":
		m.viewport.GotoTop()
	case "ctrl+end":
		m.viewport.GotoBottom()
	case "home":
		if m.composer.Value() != "" {
			return false
		}
		m.viewport.GotoTop()
	case "end":
		if m.composer.Value() != "" {
			return false
		}
		m.viewport.GotoBottom()
	default:
		return false
	}
	m.updateNewOutputIndicator()
	return true
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
	m.resetAnswer()
	return m.beginStream(params)
}

func (m *TUIModel) beginStream(params *rootly.StreamAiChatParams) tea.Cmd {
	m.turn++
	turn := m.turn
	m.state = tuiStreaming
	m.resumeTextSeparator = params.CheckpointID != nil
	m.pending = nil
	m.doneStatus = ""
	m.doneReceived = false
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
	content := m.viewContent()
	view := tea.NewView(content)
	view.AltScreen = true
	view.MouseMode = tea.MouseModeCellMotion
	return view
}

func (m *TUIModel) viewContent() string {
	if m.quitting {
		return ""
	}
	if !m.windowSizeReceived || m.height <= 0 || m.width <= 0 {
		return m.footerContent()
	}

	overlay := m.overlayContent()
	footerRows := lipgloss.Height(m.footerContent())
	overlayRows := min(lipgloss.Height(overlay), max(0, m.height-footerRows))
	bodyHeight := max(0, m.height-footerRows-overlayRows)

	wasAtBottom := m.viewport.AtBottom()
	widthChanged := m.viewport.Width() != m.width
	heightChanged := m.viewport.Height() != bodyHeight
	if widthChanged {
		m.viewport.SetWidth(m.width)
	}
	if heightChanged {
		m.viewport.SetHeight(bodyHeight)
	}
	body := m.transcriptContent()
	if body != m.viewportContent {
		m.viewport.SetContent(body)
		m.viewportContent = body
		if wasAtBottom {
			m.viewport.GotoBottom()
			m.newOutputAvailable = false
		} else {
			m.newOutputAvailable = true
		}
	} else if wasAtBottom && (widthChanged || heightChanged) {
		m.viewport.GotoBottom()
	}
	m.updateNewOutputIndicator()
	footer := m.footerContent()

	sections := make([]string, 0, 3)
	if bodyHeight > 0 {
		sections = append(sections, m.viewport.View())
	}
	if overlayRows > 0 {
		sections = append(sections, clipViewRows(overlay, overlayRows, max(1, m.width-1)))
	}
	sections = append(sections, footer)
	return strings.Join(sections, "\n")
}

func (m *TUIModel) transcriptContent() string {
	m.rebuildTranscriptCache()
	header := strings.TrimSuffix(m.welcomeBannerFrame(), "\n")
	if !m.welcomeAnimating {
		header = strings.TrimSuffix(m.welcomeBanner(), "\n")
	}
	live := m.liveSection()
	liveGapBefore := m.committedOffset > 0 && strings.TrimSpace(m.liveAnswerRemainder()) != ""
	return joinHeaderAndConversation(header, m.transcriptCache, live, liveGapBefore)
}

func (m *TUIModel) rebuildTranscriptCache() {
	width := max(1, m.width-1)
	if !m.transcriptCacheDirty && m.transcriptCacheWidth == width {
		return
	}
	blocks := make([]transcriptBlock, 0, max(0, len(m.transcript)-1))
	for _, block := range m.transcript[1:] {
		blocks = append(blocks, transcriptBlock{
			text:      ansi.Wrap(block.text, width, ""),
			gapBefore: block.gapBefore,
		})
	}
	m.transcriptCache = joinTranscriptBlocks(blocks)
	m.transcriptCacheWidth = width
	m.transcriptCacheDirty = false
}

func joinTranscriptBlocks(blocks []transcriptBlock) string {
	var joined strings.Builder
	hasBlock := false
	for _, block := range blocks {
		text := strings.Trim(block.text, "\n")
		if text == "" {
			continue
		}
		if hasBlock {
			if block.gapBefore || strings.HasPrefix(strings.TrimSpace(ansi.Strip(text)), "❯ ") {
				joined.WriteString("\n\n")
			} else {
				joined.WriteByte('\n')
			}
		}
		joined.WriteString(text)
		hasBlock = true
	}
	return joined.String()
}

func joinHeaderAndConversation(header, committed, live string, liveGapBefore bool) string {
	header = strings.TrimRight(header, "\n")
	committed = strings.Trim(committed, "\n")
	live = strings.Trim(live, "\n")

	var joined strings.Builder
	joined.WriteString(header)
	if committed != "" {
		if header != "" {
			joined.WriteString("\n\n")
		}
		joined.WriteString(committed)
	}
	if live != "" {
		if committed == "" && header != "" {
			joined.WriteString("\n\n")
		} else if committed != "" || header != "" {
			if committed != "" && liveGapBefore {
				joined.WriteString("\n\n")
			} else {
				joined.WriteByte('\n')
			}
		}
		joined.WriteString(live)
	}
	return joined.String()
}

func (m *TUIModel) liveSection() string {
	var sections []string
	switch m.state {
	case tuiStreaming:
		markdown := m.liveAnswerRemainder()
		if strings.TrimSpace(markdown) != "" {
			sections = append(sections, m.renderAssistantAnswerPart(markdown, m.committedOffset == 0))
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
		if m.status != "" {
			sections = append(sections, m.status)
		}
	}
	return strings.Join(sections, "\n")
}

func (m *TUIModel) liveAnswerRemainder() string {
	markdown := m.uncommittedAnswer()
	if !m.noColor {
		markdown = trimIncompleteTrailingMarkdownLink(markdown)
	}
	return markdown
}

func (m *TUIModel) overlayContent() string {
	if m.deleteConfirming {
		return "Delete this session? This can't be undone.\ny delete · n/Esc cancel"
	} else if m.pickerOpen {
		return m.sessionPicker.View()
	} else if m.commandMenuOpen {
		return m.commandMenu.View()
	}
	return ""
}

func (m *TUIModel) footerContent() string {
	rule := faint(m.noColor, strings.Repeat("─", max(1, m.width-1)))
	hint := "/ for commands · exit to quit"
	if m.newOutputAvailable {
		hint += " · " + faint(m.noColor, "↓ new output · ctrl+end to jump")
	}
	hint = ansi.Truncate(hint, max(1, m.width-1), "")
	return strings.Join([]string{rule, m.composer.View(), rule, faint(m.noColor, hint)}, "\n")
}

func (m *TUIModel) updateNewOutputIndicator() {
	if m.viewport.AtBottom() {
		m.newOutputAvailable = false
	}
}

func clipViewRows(text string, rows, width int) string {
	if rows <= 0 {
		return ""
	}
	lines := strings.Split(ansi.Wrap(text, max(1, width), ""), "\n")
	if len(lines) > rows {
		lines = lines[:rows]
	}
	return strings.Join(lines, "\n")
}

func (m *TUIModel) appendTranscriptBlock(text string) {
	m.appendTranscriptBlockWithGap(text, false)
}

func (m *TUIModel) appendTranscriptBlockWithGap(text string, gapBefore bool) {
	m.transcript = append(m.transcript, transcriptBlock{text: text, gapBefore: gapBefore})
	m.transcriptCacheDirty = true
}

func (m *TUIModel) resetTranscript() {
	m.transcript = []transcriptBlock{{text: ""}}
	m.transcriptCache = ""
	m.transcriptCacheWidth = 0
	m.transcriptCacheDirty = true
	m.conversation = nil
}

func (m *TUIModel) println(text string) tea.Cmd {
	m.appendTranscriptBlock(text)
	return nil
}

func (m *TUIModel) printlnWithGap(text string, gapBefore bool) tea.Cmd {
	m.appendTranscriptBlockWithGap(text, gapBefore)
	return nil
}

func (m *TUIModel) Transcript() string {
	m.rebuildTranscriptCache()
	header := strings.TrimSuffix(m.welcomeBanner(), "\n")
	live := ""
	liveGapBefore := false
	if remainder := m.uncommittedAnswer(); strings.TrimSpace(remainder) != "" {
		live = m.renderAssistantAnswerPart(remainder, m.committedOffset == 0)
		liveGapBefore = m.committedOffset > 0
	}
	return joinHeaderAndConversation(header, m.transcriptCache, live, liveGapBefore) + "\n"
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
