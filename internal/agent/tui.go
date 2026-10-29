package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"
	"charm.land/glamour/v2"
	"charm.land/lipgloss/v2"
	"github.com/google/uuid"
	rootly "github.com/rootlyhq/rootly-go"
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
}

type TUIModel struct {
	streamer      Streamer
	state         tuiState
	composer      textarea.Model
	spinner       spinner.Model
	width         int
	height        int
	status        string
	answer        strings.Builder
	sessionID     *uuid.UUID
	pending       *rootly.AiChatStreamConfirmationRequired
	ctx           context.Context
	cancel        context.CancelFunc
	stream        chan streamMessage
	startedAt     time.Time
	doneStatus    string
	idleInterrupt bool
	currentParams *rootly.StreamAiChatParams
}

func NewTUIModel(ctx context.Context, streamer Streamer) *TUIModel {
	composer := textarea.New()
	composer.Prompt = "> "
	composer.Placeholder = "Ask Rootly..."
	composer.ShowLineNumbers = false
	composer.SetHeight(3)
	composer.KeyMap.InsertNewline = key.NewBinding(key.WithKeys("enter", "shift+enter", "alt+enter"))
	_ = composer.Focus()

	spin := spinner.New()
	spin.Spinner = spinner.Dot

	return &TUIModel{
		streamer: streamer,
		composer: composer,
		spinner:  spin,
		ctx:      ctx,
		width:    80,
		height:   24,
	}
}

func (m *TUIModel) Init() tea.Cmd {
	return m.composer.Focus()
}

func (m *TUIModel) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := message.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.composer.SetWidth(max(20, msg.Width-2))
		m.composer.SetHeight(max(3, min(6, msg.Height/4)))
		return m, nil
	case spinner.TickMsg:
		if m.state == tuiStreaming {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			return m, tea.Batch(cmd, m.spinner.Tick)
		}
	case streamMessage:
		return m.updateStream(msg)
	case tea.KeyPressMsg:
		return m.updateKey(msg)
	}
	return m, nil
}

func (m *TUIModel) updateStream(message streamMessage) (tea.Model, tea.Cmd) {
	if m.state != tuiStreaming {
		return m, nil
	}
	if message.done {
		return m.finishTurn(message.err)
	}
	if message.err != nil {
		m.status = message.err.Error()
		cmd := m.waitForStream()
		return m, cmd
	}
	m.handleEvent(message.event)
	cmd := m.waitForStream()
	return m, cmd
}

func (m *TUIModel) handleEvent(event Event) {
	switch event.Type {
	case rootly.AiChatStreamEventSessionID:
		if event.SessionID != nil {
			if id, err := uuid.Parse(event.SessionID.SessionID); err == nil {
				m.sessionID = &id
			}
		}
	case rootly.AiChatStreamEventText:
		if event.Text != nil {
			m.answer.WriteString(event.Text.Content)
		}
	case rootly.AiChatStreamEventStatus:
		if event.Status != nil {
			m.status = event.Status.Status
		}
	case rootly.AiChatStreamEventTaskUpdate:
		if event.TaskUpdate != nil {
			m.status = fmt.Sprintf("%s (%s)", event.TaskUpdate.Title, event.TaskUpdate.Status)
		}
	case rootly.AiChatStreamEventConfirmationRequired:
		m.pending = event.ConfirmationRequired
	case rootly.AiChatStreamEventError:
		if event.Error != nil {
			m.status = event.Error.Message
		}
	case rootly.AiChatStreamEventDone:
		if event.Done != nil {
			m.doneStatus = event.Done.Status
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
		m.status = err.Error()
		return m, nil
	}
	if m.doneStatus == "error" {
		if m.status == "" {
			m.status = "AI chat turn failed"
		}
		return m, nil
	}
	if m.doneStatus == "confirmation_required" && m.pending != nil {
		if expired(m.pending) {
			m.pending = nil
			m.status = "confirmation expired; no action was taken"
			return m, tea.Println(m.status)
		}
		m.state = tuiConfirming
		return m, nil
	}

	cmds := make([]tea.Cmd, 0, 1)
	if answer := strings.TrimSpace(m.answer.String()); answer != "" {
		cmds = append(cmds, tea.Printf("Assistant:\n%s", renderMarkdown(answer, m.width)))
	}
	if m.doneStatus != "" && m.doneStatus != "complete" {
		m.status = "turn halted: " + m.doneStatus
	} else {
		m.status = ""
	}
	m.pending = nil
	m.doneStatus = ""
	return m, tea.Batch(cmds...)
}

func (m *TUIModel) updateKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if msg.String() == "ctrl+c" {
		if m.state == tuiStreaming {
			if m.cancel != nil {
				m.cancel()
			}
			m.state = tuiIdle
			m.cancel = nil
			m.stream = nil
			m.status = "cancelled"
			return m, nil
		}
		if m.idleInterrupt {
			return m, tea.Quit
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
			cmd := m.resume(rootly.StreamAiChatParamsDecisionConfirm)
			return m, cmd
		case "n", "esc":
			cmd := m.resume(rootly.StreamAiChatParamsDecisionCancel)
			return m, cmd
		}
		return m, nil
	}

	if msg.String() == "ctrl+d" && strings.TrimSpace(m.composer.Value()) == "" {
		return m, tea.Quit
	}
	if m.state == tuiStreaming {
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
		return m, cmd
	}

	if msg.String() == "esc" {
		m.idleInterrupt = false
	}
	var cmd tea.Cmd
	m.composer, cmd = m.composer.Update(msg)
	return m, cmd
}

func (m *TUIModel) submit() tea.Cmd {
	message := strings.TrimSpace(m.composer.Value())
	m.composer.Reset()
	m.idleInterrupt = false
	m.status = ""
	if message == "" {
		return nil
	}
	if strings.HasPrefix(message, "/") {
		return m.runSlashCommand(message)
	}

	params := &rootly.StreamAiChatParams{Message: &message, SessionID: m.sessionID}
	cmd := m.startStream(params)
	return tea.Batch(tea.Printf("You: %s", message), cmd)
}

func (m *TUIModel) runSlashCommand(command string) tea.Cmd {
	switch strings.TrimSpace(command) {
	case "/exit", "/quit":
		return tea.Quit
	case "/new":
		m.sessionID = nil
		m.answer.Reset()
		m.status = "Started a new session."
	case "/help":
		m.status = "/exit or /quit: exit   /new: start a new session   /help: show this help"
	default:
		m.status = "Unknown command. Use /help for available commands."
	}
	return nil
}

func (m *TUIModel) startStream(params *rootly.StreamAiChatParams) tea.Cmd {
	m.answer.Reset()
	return m.beginStream(params)
}

func (m *TUIModel) beginStream(params *rootly.StreamAiChatParams) tea.Cmd {
	m.state = tuiStreaming
	m.pending = nil
	m.doneStatus = ""
	m.startedAt = time.Now()
	m.currentParams = params
	m.stream = make(chan streamMessage, 16)
	ctx, cancel := context.WithCancel(m.ctx)
	m.cancel = cancel
	stream := m.stream

	go func() {
		err := StreamTurn(ctx, m.streamer, params, func(event Event) bool {
			select {
			case stream <- streamMessage{event: event}:
				return true
			case <-ctx.Done():
				return false
			}
		})
		select {
		case stream <- streamMessage{err: err, done: true}:
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
	return func() tea.Msg {
		message, ok := <-stream
		if !ok {
			return streamMessage{err: fmt.Errorf("stream closed"), done: true}
		}
		return message
	}
}

func (m *TUIModel) View() tea.View {
	var content strings.Builder
	switch m.state {
	case tuiStreaming:
		if rendered := renderMarkdown(m.answer.String(), m.width-2); rendered != "" {
			content.WriteString(rendered)
			content.WriteString("\n")
		}
		elapsed := time.Since(m.startedAt).Truncate(time.Second)
		status := m.status
		if status == "" {
			status = "Thinking"
		}
		fmt.Fprintf(&content, "%s %s · %s\n", m.spinner.View(), status, elapsed)
	case tuiConfirming:
		if m.pending != nil {
			fmt.Fprintf(&content, "%s\n", lipgloss.NewStyle().Bold(true).Render(m.pending.Question))
			if len(m.pending.Preview) > 0 {
				var preview bytes.Buffer
				if err := json.Indent(&preview, m.pending.Preview, "", "  "); err == nil {
					fmt.Fprintf(&content, "%s\n", indent(preview.String(), "  "))
				}
			}
			content.WriteString("Approve? (y/n)\n")
		}
	default:
		if m.status != "" {
			content.WriteString(m.status)
			content.WriteString("\n")
		}
	}
	content.WriteString(m.composer.View())
	view := tea.NewView(content.String())
	return view
}

func renderMarkdown(markdown string, width int) string {
	if strings.TrimSpace(markdown) == "" {
		return ""
	}
	if width < 20 {
		width = 20
	}
	renderer, err := glamour.NewTermRenderer(glamour.WithEnvironmentConfig(), glamour.WithWordWrap(width))
	if err != nil {
		return markdown
	}
	rendered, err := renderer.Render(markdown)
	if err != nil {
		return markdown
	}
	return strings.TrimSpace(rendered)
}
