package agent

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/google/uuid"
	rootly "github.com/rootlyhq/rootly-go"

	"github.com/rootlyhq/rootly-cli/internal/api"
)

type statusLoadedMsg struct {
	request  int
	response *rootly.GetCurrentUserResponse
}

type skillPickerPageMsg struct {
	request    int
	page       int
	skills     []rootly.AiSkill
	nextPage   int
	statusCode int
	err        error
}

type skillInvocationResolvedMsg struct {
	request    int
	token      string
	prompt     string
	skill      rootly.AiSkill
	found      bool
	statusCode int
	err        error
}

type skillSuggestionTickMsg struct {
	request int
	prefix  string
}

type skillSuggestionsLoadedMsg struct {
	request    int
	prefix     string
	skills     []rootly.AiSkill
	statusCode int
	err        error
}

type sessionPickerItem struct {
	session  rootly.AiChatSession
	loadMore bool
}

type skillPickerItem struct {
	skill    rootly.AiSkill
	loadMore bool
}

type slashCommandItem struct {
	command     string
	description string
	skill       bool
}

func (i slashCommandItem) FilterValue() string { return i.command }
func (i slashCommandItem) Title() string       { return i.command }
func (i slashCommandItem) Description() string { return i.description }

type slashCommandDelegate struct{}

func (slashCommandDelegate) Render(writer io.Writer, model list.Model, index int, item list.Item) {
	command, ok := item.(slashCommandItem)
	if !ok {
		return
	}
	prefix := "  "
	style := lipgloss.NewStyle().Foreground(lipgloss.Color("7"))
	if index == model.Index() {
		prefix = "› "
		style = lipgloss.NewStyle().Foreground(lipgloss.Color("6"))
	}
	_, _ = fmt.Fprintf(writer, "%s%s  %s", prefix, command.command, style.Render(command.description))
}

func (slashCommandDelegate) Height() int                         { return 1 }
func (slashCommandDelegate) Spacing() int                        { return 0 }
func (slashCommandDelegate) Update(tea.Msg, *list.Model) tea.Cmd { return nil }

func (i sessionPickerItem) FilterValue() string {
	if i.loadMore {
		return "Load more"
	}
	title := sanitizeTerminalText(i.session.Title)
	if strings.TrimSpace(title) == "" {
		return "Untitled"
	}
	return title
}

func (i sessionPickerItem) Title() string {
	if i.loadMore {
		return "Load more…"
	}
	title := sanitizeTerminalText(i.session.Title)
	if strings.TrimSpace(title) == "" {
		return "Untitled"
	}
	return title
}

func (i sessionPickerItem) Description() string {
	if i.loadMore {
		return "Fetch the next page"
	}
	description := "last active " + relativeAge(i.session.LastActiveAt)
	if i.session.Pinned != nil && *i.session.Pinned {
		description += " · pinned"
	}
	return description
}

func (i skillPickerItem) FilterValue() string {
	if i.loadMore {
		return "Load more"
	}
	return strings.Join([]string{
		sanitizeTerminalText(skillString(i.skill.Slug)),
		sanitizeTerminalText(skillString(i.skill.Name)),
		sanitizeTerminalText(strings.ReplaceAll(i.skill.Description.GetOrEmpty(), "\n", " ")),
	}, " ")
}

func (i skillPickerItem) Title() string {
	if i.loadMore {
		return "Load more"
	}
	marker := "○"
	if skillEnabled(i.skill) {
		marker = "✔"
	}
	return sanitizeTerminalText(marker + " /" + skillString(i.skill.Slug))
}

func (i skillPickerItem) Description() string {
	if i.loadMore {
		return "Fetch the next page"
	}
	parts := make([]string, 0, 3)
	if name := sanitizeTerminalText(strings.ReplaceAll(skillString(i.skill.Name), "\n", " ")); name != "" {
		parts = append(parts, name)
	}
	if version, err := i.skill.CurrentVersion.Get(); err == nil {
		parts = append(parts, fmt.Sprintf("v%d", version))
	}
	if invocable := skillInvocableLabels(i.skill.InvocableBy); invocable != "" {
		parts = append(parts, invocable)
	}
	description := strings.Join(parts, " · ")
	if text := strings.ReplaceAll(i.skill.Description.GetOrEmpty(), "\n", " "); text != "" {
		if description != "" {
			description += " · "
		}
		description += text
	}
	return sanitizeTerminalText(description)
}

var skillInvocationTokenPattern = regexp.MustCompile(`^/[a-z0-9]+(?:-[a-z0-9]+)*$`)

var builtinSkillSlugs = map[string]struct{}{
	"exit": {}, "quit": {}, "new": {}, "clear": {}, "resume": {}, "sessions": {},
	"rename": {}, "pin": {}, "unpin": {}, "archive": {}, "delete": {}, "copy": {},
	"export": {}, "incident": {}, "alert": {}, "status": {}, "help": {}, "skills": {},
}

func isSkillInvocationToken(token string) bool {
	return skillInvocationTokenPattern.MatchString(token)
}

func isBuiltinSlashCommand(command string) bool {
	_, ok := builtinSkillSlugs[strings.TrimPrefix(command, "/")]
	return ok
}

func skillEnabled(skill rootly.AiSkill) bool {
	return skill.Enabled != nil && *skill.Enabled
}

func skillString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func skillInvocableByUser(skill rootly.AiSkill) bool {
	for _, invocableBy := range skill.InvocableBy {
		if invocableBy == rootly.AiSkillInvocableBy("user") {
			return true
		}
	}
	return false
}

func skillInvocableLabels(values []rootly.AiSkillInvocableBy) string {
	labels := make([]string, 0, len(values))
	for _, value := range values {
		switch value {
		case rootly.AiSkillInvocableBy("user"):
			labels = append(labels, "you")
		case rootly.AiSkillInvocableBy("model"):
			labels = append(labels, "model")
		case rootly.AiSkillInvocableBy("automation"):
			labels = append(labels, "automations")
		}
	}
	return strings.Join(labels, " + ")
}

func relativeAge(value time.Time) string {
	elapsed := time.Since(value)
	switch {
	case elapsed < time.Minute:
		return "just now"
	case elapsed < time.Hour:
		return fmt.Sprintf("%dm ago", int(elapsed.Minutes()))
	case elapsed < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(elapsed.Hours()))
	case elapsed < 30*24*time.Hour:
		return fmt.Sprintf("%dd ago", int(elapsed.Hours()/24))
	default:
		return value.Format("2006-01-02")
	}
}

func (m *TUIModel) historyKey(msg tea.KeyPressMsg) bool {
	if msg.Code != tea.KeyUp && msg.Code != tea.KeyDown {
		return false
	}
	value := m.composer.Value()
	browsing := m.historyIndex >= 0
	if msg.Code == tea.KeyUp {
		if value != "" && (!browsing || m.composer.Line() != 0) {
			return false
		}
		if !browsing {
			m.historyDraft = value
			m.historyIndex = len(m.promptHistory)
		}
		if m.historyIndex > 0 {
			m.historyIndex--
			m.composer.SetValue(m.promptHistory[m.historyIndex])
		}
		return true
	}
	if !browsing || (value != "" && m.composer.Line() != m.composer.LineCount()-1) {
		return false
	}
	if m.historyIndex < len(m.promptHistory)-1 {
		m.historyIndex++
		m.composer.SetValue(m.promptHistory[m.historyIndex])
	} else {
		m.historyIndex = len(m.promptHistory)
		m.composer.SetValue(m.historyDraft)
	}
	return true
}

func (m *TUIModel) quitCommand() tea.Cmd {
	m.settleWelcomeAnimation()
	m.quitting = true
	if m.sessionID != nil {
		m.println(faint(m.noColor, "Resume with: rootly agent --resume "+m.sessionID.String()))
	}
	return tea.Quit
}

func (m *TUIModel) settleWelcomeAnimation() {
	m.welcomePending = false
	m.welcomeAnimating = false
}

func isExitAlias(message string) bool {
	message = strings.TrimSpace(message)
	return !strings.ContainsAny(message, "\r\n") && (strings.EqualFold(message, "exit") || strings.EqualFold(message, "quit"))
}

func renderPromptEcho(prompt string, noColor bool) string {
	lines := strings.Split(prompt, "\n")
	for index, line := range lines {
		prefix := "  "
		if index == 0 {
			prefix = "❯ "
			if !noColor {
				prefix = lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Faint(true).Render(prefix)
			}
		}
		lines[index] = prefix + line
	}
	return strings.Join(lines, "\n")
}

func (m *TUIModel) renderAssistantAnswer(markdown string) string {
	return m.renderAssistantAnswerPart(markdown, true)
}

func (m *TUIModel) renderAssistantAnswerPart(markdown string, first bool) string {
	lines := strings.Split(m.renderMarkdown(markdown, max(20, m.width-3)), "\n")
	start, end := 0, len(lines)
	for start < end && strings.TrimSpace(ansi.Strip(lines[start])) == "" {
		start++
	}
	for end > start && strings.TrimSpace(ansi.Strip(lines[end-1])) == "" {
		end--
	}
	lines = lines[start:end]
	if len(lines) == 0 {
		return ""
	}
	for index, line := range lines {
		line = trimLeadingANSIWhitespace(line)
		if index == 0 && first {
			lines[index] = "● " + line
		} else {
			lines[index] = "  " + line
		}
	}
	return ansi.Wrap(strings.Join(lines, "\n"), max(1, m.width-1), " ")
}

func trimLeadingANSIWhitespace(line string) string {
	var output strings.Builder
	started := false
	for index := 0; index < len(line); {
		if line[index] == '\x1b' && index+1 < len(line) {
			end := index + 2
			switch line[index+1] {
			case '[':
				for end < len(line) && (line[end] < '@' || line[end] > '~') {
					end++
				}
				if end < len(line) {
					end++
				}
			case ']':
				for end < len(line) && line[end] != '\a' {
					end++
				}
				if end < len(line) {
					end++
				}
			}
			output.WriteString(line[index:end])
			index = end
			continue
		}
		if !started && (line[index] == ' ' || line[index] == '\t') {
			index++
			continue
		}
		started = true
		output.WriteByte(line[index])
		index++
	}
	return output.String()
}

func (m *TUIModel) turnFooter() string {
	now := m.now()
	duration := now.Sub(m.turnStartedAt).Round(time.Second)
	verb := statusVerbs[m.turnVerbIndex].Done
	return faint(m.noColor, fmt.Sprintf("✻ %s for %s · done %s", verb, formatTurnDuration(duration), now.Format("3:04 PM")))
}

func (m *TUIModel) startNewTurn() {
	m.turnStartedAt = m.now()
	m.turnVerbIndex = m.verbIndex(len(statusVerbs))
}

func formatTurnDuration(duration time.Duration) string {
	if duration < 0 {
		duration = 0
	}
	if duration < time.Minute {
		return fmt.Sprintf("%ds", int(duration/time.Second))
	}
	return fmt.Sprintf("%dm %ds", int(duration/time.Minute), int(duration/time.Second)%60)
}

func shortSessionID(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}

func faint(noColor bool, text string) string {
	if noColor {
		return text
	}
	return lipgloss.NewStyle().Faint(true).Render(text)
}

func (m *TUIModel) bindingHeader() string {
	if m.incident != nil {
		id := m.incident.SequentialID
		if id == "" {
			id = shortSessionID(m.incident.ID)
		}
		return sanitizeTerminalText(fmt.Sprintf("Chatting about %s: %s", id, m.incident.Title))
	}
	if m.alert != nil {
		id := m.alert.ShortID
		if id == "" {
			id = shortSessionID(m.alert.ID)
		}
		return sanitizeTerminalText(fmt.Sprintf("Chatting about %s: %s", id, m.alert.Summary))
	}
	return ""
}

func (m *TUIModel) helpView() string {
	model := help.New()
	groups := [][]key.Binding{{
		key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "send")),
		key.NewBinding(key.WithKeys("shift+enter", "alt+enter"), key.WithHelp("shift/alt+enter", "new line")),
		key.NewBinding(key.WithKeys("up", "down"), key.WithHelp("↑/↓", "history")),
	}}
	return model.FullHelpView(groups) + "\n" +
		"/exit, /quit, exit, quit: exit\n" +
		"/new, /clear: start a new session\n" +
		"/resume, /sessions: choose a session\n" +
		"/skills [query]: browse skills\n" +
		"/rename [title]: rename or reset title\n" +
		"/pin, /unpin: change pinned state\n" +
		"/archive: archive this session\n" +
		"/delete: delete this session\n" +
		"/copy: copy last answer\n" +
		"/export [file]: write transcript\n" +
		"/incident <id>, /alert <id>: bind a resource\n" +
		"/status: show session and API info\n" +
		"PgUp/PgDn/wheel scroll · Ctrl+Home/Ctrl+End scroll · Home/End scroll when composer is empty\n" +
		"Hold Option (macOS) / Shift to select text\n" +
		"/help: show this help"
}

func (m *TUIModel) statusCommand() tea.Cmd {
	if m.userLoaded || m.api == nil {
		return m.println(m.statusLines())
	}
	m.statusRequest++
	request := m.statusRequest
	apiClient := m.api
	ctx := m.ctx
	return func() tea.Msg {
		response, _ := apiClient.GetCurrentUserWithResponse(ctx)
		return statusLoadedMsg{request: request, response: response}
	}
}

func (m *TUIModel) statusLines() string {
	session := "new session"
	if m.sessionID != nil {
		session = m.sessionID.String()
	}
	user := "unknown"
	if m.currentUser != nil {
		attributes := m.currentUser.Data.Attributes
		name := currentUserName(attributes)
		user = sanitizeTerminalText(strings.TrimSpace(name + " (" + attributes.Email + ")"))
	}
	host := m.apiHost
	if host == "" && m.api != nil {
		host = m.api.APIHost()
	}
	lines := []string{"session " + session}
	if title := sanitizeTerminalText(m.sessionTitle); title != "" {
		lines = append(lines, "title "+title)
	}
	lines = append(lines, "user "+user, "API "+sanitizeTerminalText(host))
	if header := m.bindingHeader(); header != "" {
		lines = append(lines, "binding "+strings.TrimPrefix(header, "Chatting about "))
	}
	for index := range lines {
		lines[index] = faint(m.noColor, lines[index])
	}
	return strings.Join(lines, "\n")
}

func (m *TUIModel) loadHistory(sessionID uuid.UUID) tea.Cmd {
	m.historyRequest++
	request := m.historyRequest
	apiClient := m.api
	ctx := m.ctx
	return func() tea.Msg {
		if apiClient == nil {
			return historyLoadedMsg{request: request, err: errors.New("history API unavailable")}
		}
		messages, sessionTitle, err := loadSessionHistory(ctx, apiClient, sessionID)
		return historyLoadedMsg{request: request, messages: messages, err: err, sessionTitle: sessionTitle}
	}
}

func (m *TUIModel) printHistory(messages []rootly.AiChatSessionMessage) {
	for _, message := range messages {
		content := sanitizeTerminalText(message.Content)
		switch message.Role {
		case rootly.AiChatSessionMessageRole("user"):
			m.println(renderPromptEcho(content, m.noColor))
			m.conversation = append(m.conversation, transcriptTurn{user: content})
		case rootly.AiChatSessionMessageRole("assistant"):
			m.appendAssistantTranscriptBlock(content, true, false)
			if len(m.conversation) > 0 && m.conversation[len(m.conversation)-1].assistant == "" {
				m.conversation[len(m.conversation)-1].assistant = content
			} else {
				m.conversation = append(m.conversation, transcriptTurn{assistant: content})
			}
		}
	}
}

func (m *TUIModel) openSessionPicker() tea.Cmd {
	m.pickerOpen = true
	m.pickerLoading = true
	m.actionRequest++
	m.pickerBefore = ""
	m.pickerHasMore = false
	delegate := list.NewDefaultDelegate()
	m.sessionPicker = list.New(nil, delegate, max(1, m.width), max(1, m.height))
	m.sessionPicker.Title = "Sessions"
	m.sessionPicker.SetShowTitle(true)
	m.sessionPicker.SetShowHelp(true)
	m.sessionPicker.SetShowStatusBar(true)
	m.sessionPicker.SetShowPagination(true)
	m.sessionPicker.SetFilteringEnabled(true)
	m.sessionPicker.SetSize(max(1, m.width), max(1, m.height))
	return m.loadSessionPage("")
}

func (m *TUIModel) loadSessionPage(before string) tea.Cmd {
	if m.api == nil {
		m.pickerOpen = false
		return m.println("session API unavailable")
	}
	m.pickerRequest++
	request := m.pickerRequest
	apiClient := m.api
	ctx := m.ctx
	return func() tea.Msg {
		limit, archived := 20, false
		params := &rootly.ListAiChatSessionsParams{Limit: &limit, Archived: &archived}
		if before != "" {
			params.Before = &before
		}
		response, err := apiClient.ListAiChatSessionsWithResponse(ctx, params)
		if err != nil {
			return sessionPageMsg{request: request, err: err}
		}
		if response == nil {
			return sessionPageMsg{request: request, err: errors.New("empty session list response")}
		}
		if response.ApplicationVndAPIJSON200 == nil {
			return sessionPageMsg{request: request, err: StatusResponseError(response.StatusCode(), response.Body)}
		}
		data := response.ApplicationVndAPIJSON200
		return sessionPageMsg{
			request: request, sessions: data.Sessions, hasMore: data.Meta.Pagination.HasMore,
			before: data.Meta.Pagination.NextBefore.GetOrEmpty(),
		}
	}
}

func (m *TUIModel) handleSessionPage(message sessionPageMsg) tea.Cmd {
	if message.request != m.pickerRequest || !m.pickerOpen {
		return nil
	}
	m.pickerLoading = false
	if message.err != nil {
		m.pickerOpen = false
		return m.println(sanitizeTerminalText(message.err.Error()))
	}
	m.pickerBefore = message.before
	m.pickerHasMore = message.hasMore && message.before != ""
	items := make([]list.Item, 0, len(message.sessions)+1)
	for _, session := range message.sessions {
		items = append(items, sessionPickerItem{session: session})
	}
	if m.pickerHasMore {
		items = append(items, sessionPickerItem{loadMore: true})
	}
	if len(m.sessionPicker.Items()) == 0 {
		m.sessionPicker.SetItems(items)
	} else {
		for index := len(m.sessionPicker.Items()) - 1; index >= 0; index-- {
			if item, ok := m.sessionPicker.Items()[index].(sessionPickerItem); ok && item.loadMore {
				m.sessionPicker.RemoveItem(index)
				break
			}
		}
		for _, item := range items {
			m.sessionPicker.InsertItem(len(m.sessionPicker.Items()), item)
		}
	}
	return nil
}

func (m *TUIModel) selectPickerItem(item sessionPickerItem) tea.Cmd {
	if item.loadMore {
		m.pickerLoading = true
		return m.loadSessionPage(m.pickerBefore)
	}
	id, err := uuid.Parse(item.session.ID.String())
	if err != nil {
		m.pickerOpen = false
		return m.println("invalid session UUID")
	}
	m.pickerOpen = false
	m.pickerLoading = false
	m.actionRequest++
	m.sessionID = &id
	m.sessionTitle = normalizeSessionTitle(item.Title())
	m.resumedSession = true
	m.resetAnswer()
	m.lastAnswer = ""
	m.resetTranscript()
	m.incident, m.alert = nil, nil
	m.incidentUsed, m.alertUsed = false, false
	return tea.Batch(m.loadHistory(id), m.restartWelcomeAnimation())
}

func (m *TUIModel) handlePickerKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if msg.String() == "esc" {
		if m.sessionPicker.FilterState() != list.Unfiltered {
			var cmd tea.Cmd
			m.sessionPicker, cmd = m.sessionPicker.Update(msg)
			return m, cmd
		}
		m.pickerOpen = false
		m.pickerLoading = false
		m.pickerRequest++
		return m, nil
	}
	if msg.String() == "enter" {
		selected, ok := m.sessionPicker.SelectedItem().(sessionPickerItem)
		if !ok {
			return m, nil
		}
		cmd := m.selectPickerItem(selected)
		return m, cmd
	}
	var cmd tea.Cmd
	m.sessionPicker, cmd = m.sessionPicker.Update(msg)
	return m, cmd
}

const aiSkillsDisabledMessage = "AI Skills isn't enabled for this organization."

func (m *TUIModel) openSkillPicker(query string) tea.Cmd {
	m.skillPickerOpen = true
	m.skillPickerLoading = true
	m.skillPickerNextPage = 0
	m.skillPickerQuery = query
	delegate := list.NewDefaultDelegate()
	m.skillPicker = list.New(nil, delegate, max(1, m.width), max(1, m.height))
	m.skillPicker.Title = "AI Skills"
	m.skillPicker.SetShowTitle(true)
	m.skillPicker.SetShowHelp(true)
	m.skillPicker.SetShowStatusBar(true)
	m.skillPicker.SetShowPagination(true)
	m.skillPicker.SetFilteringEnabled(true)
	m.skillPicker.SetSize(max(1, m.width), max(1, m.height))
	return m.loadSkillPickerPage(1)
}

func (m *TUIModel) loadSkillPickerPage(page int) tea.Cmd {
	if m.api == nil {
		m.skillPickerOpen = false
		m.skillPickerLoading = false
		return m.println("AI Skills API unavailable.")
	}
	m.skillPickerRequest++
	request, apiClient, ctx, query := m.skillPickerRequest, m.api, m.ctx, m.skillPickerQuery
	sortBy := rootly.ListAISkillsParamsSortName
	params := &rootly.ListAISkillsParams{Sort: &sortBy}
	if query != "" {
		params.FilterSearch = &query
	}
	return func() tea.Msg {
		response, err := apiClient.ListAISkillsWithResponse(ctx, params, aiSkillsPageRequestEditor(page, 25))
		if err != nil {
			return skillPickerPageMsg{request: request, page: page, err: err}
		}
		if response == nil {
			return skillPickerPageMsg{request: request, page: page, err: errors.New("empty AI skills response")}
		}
		if response.ApplicationVndAPIJSON200 == nil {
			return skillPickerPageMsg{
				request: request, page: page, statusCode: response.StatusCode(),
				err: aiSkillsResponseError(response.StatusCode(), response.Body),
			}
		}
		data := response.ApplicationVndAPIJSON200
		skills := make([]rootly.AiSkill, 0, len(data.Data))
		for _, item := range data.Data {
			skills = append(skills, item.Attributes)
		}
		return skillPickerPageMsg{
			request: request, page: page, skills: skills,
			nextPage: aiSkillsNextPage(data.Meta),
		}
	}
}

func (m *TUIModel) handleSkillPickerPage(message skillPickerPageMsg) tea.Cmd {
	if message.request != m.skillPickerRequest || !m.skillPickerOpen {
		return nil
	}
	m.skillPickerLoading = false
	if message.err != nil {
		m.skillPickerOpen = false
		if message.statusCode == http.StatusForbidden || message.statusCode == http.StatusNotFound {
			m.skillsUnavailable = true
			m.skillPickerRequest++
			return m.println(aiSkillsDisabledMessage)
		}
		return m.println(sanitizeTerminalText(message.err.Error()))
	}

	m.skillPickerNextPage = message.nextPage
	items := make([]list.Item, 0, len(message.skills)+1)
	for _, skill := range message.skills {
		slug := skillString(skill.Slug)
		if slug == "" || !isSkillInvocationToken("/"+slug) || isBuiltinSlashCommand("/"+slug) {
			continue
		}
		items = append(items, skillPickerItem{skill: skill})
	}
	if m.skillPickerNextPage > 0 {
		items = append(items, skillPickerItem{loadMore: true})
	}
	if message.page == 1 {
		m.skillPicker.SetItems(items)
		return nil
	}
	for index := len(m.skillPicker.Items()) - 1; index >= 0; index-- {
		if item, ok := m.skillPicker.Items()[index].(skillPickerItem); ok && item.loadMore {
			m.skillPicker.RemoveItem(index)
			break
		}
	}
	for _, item := range items {
		m.skillPicker.InsertItem(len(m.skillPicker.Items()), item)
	}
	return nil
}

func (m *TUIModel) selectSkillPickerItem(item skillPickerItem) tea.Cmd {
	if item.loadMore {
		m.skillPickerLoading = true
		return m.loadSkillPickerPage(m.skillPickerNextPage)
	}
	m.skillPickerOpen = false
	m.skillPickerLoading = false
	m.skillPickerRequest++
	slug := skillString(item.skill.Slug)
	token := "/" + slug
	if !skillEnabled(item.skill) {
		return m.println(token + " is disabled.")
	}
	if !skillInvocableByUser(item.skill) {
		return m.println(token + " can only be loaded by the model, not by name.")
	}
	draft := strings.TrimLeft(m.composer.Value(), " \t\r\n")
	value := token + " "
	if draft != "" {
		value += draft
	}
	m.composer.SetValue(value)
	m.composer.CursorEnd()
	return m.composer.Focus()
}

func (m *TUIModel) handleSkillPickerKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if msg.String() == "esc" {
		if m.skillPicker.FilterState() != list.Unfiltered {
			var cmd tea.Cmd
			m.skillPicker, cmd = m.skillPicker.Update(msg)
			return m, cmd
		}
		m.skillPickerOpen = false
		m.skillPickerLoading = false
		m.skillPickerRequest++
		return m, nil
	}
	if msg.String() == "enter" {
		selected, ok := m.skillPicker.SelectedItem().(skillPickerItem)
		if !ok {
			return m, nil
		}
		cmd := m.selectSkillPickerItem(selected)
		return m, cmd
	}
	var cmd tea.Cmd
	m.skillPicker, cmd = m.skillPicker.Update(msg)
	return m, cmd
}

func (m *TUIModel) lookupSkillForPrompt(token, prompt string) tea.Cmd {
	if m.api == nil {
		return m.println(fmt.Sprintf("Unknown command %s. Type /help.", token))
	}
	m.skillInvocationReq++
	request, apiClient, ctx := m.skillInvocationReq, m.api, m.ctx
	slug := strings.TrimPrefix(token, "/")
	sortBy := rootly.ListAISkillsParamsSortName
	params := &rootly.ListAISkillsParams{FilterSlug: &slug, Sort: &sortBy}
	return func() tea.Msg {
		response, err := apiClient.ListAISkillsWithResponse(ctx, params, aiSkillsPageRequestEditor(1, 25))
		if err != nil {
			return skillInvocationResolvedMsg{request: request, token: token, prompt: prompt, err: err}
		}
		if response == nil {
			return skillInvocationResolvedMsg{request: request, token: token, prompt: prompt, err: errors.New("empty AI skills response")}
		}
		if response.ApplicationVndAPIJSON200 == nil {
			return skillInvocationResolvedMsg{
				request: request, token: token, prompt: prompt, statusCode: response.StatusCode(),
				err: aiSkillsResponseError(response.StatusCode(), response.Body),
			}
		}
		for _, item := range response.ApplicationVndAPIJSON200.Data {
			if skillString(item.Attributes.Slug) == slug {
				return skillInvocationResolvedMsg{
					request: request, token: token, prompt: prompt, skill: item.Attributes, found: true,
				}
			}
		}
		return skillInvocationResolvedMsg{request: request, token: token, prompt: prompt}
	}
}

func (m *TUIModel) handleSkillInvocation(message skillInvocationResolvedMsg) tea.Cmd {
	restorePrompt := func() {
		if m.composer.Value() == "" {
			m.composer.SetValue(message.prompt)
			m.composer.CursorEnd()
		}
	}
	if message.statusCode == http.StatusForbidden || message.statusCode == http.StatusNotFound {
		m.skillsUnavailable = true
		restorePrompt()
		return m.println(fmt.Sprintf("Unknown command %s. Type /help.", message.token))
	}
	if message.err != nil {
		restorePrompt()
		return m.println(fmt.Sprintf("Couldn't check skill %s: %s", message.token, sanitizeTerminalText(message.err.Error())))
	}
	if !message.found {
		restorePrompt()
		return m.println(fmt.Sprintf("Unknown command %s. Type /help.", message.token))
	}
	if !skillEnabled(message.skill) {
		restorePrompt()
		return m.println(message.token + " is disabled.")
	}
	if !skillInvocableByUser(message.skill) {
		restorePrompt()
		return m.println(message.token + " can only be loaded by the model, not by name.")
	}
	return m.submitPrompt(message.prompt)
}

func aiSkillsPageRequestEditor(page, size int) rootly.RequestEditorFn {
	return func(_ context.Context, request *http.Request) error {
		if request.URL == nil {
			return errors.New("AI skills request has no URL")
		}
		query := request.URL.Query()
		query.Set("page[number]", strconv.Itoa(page))
		query.Set("page[size]", strconv.Itoa(size))
		request.URL.RawQuery = query.Encode()
		return nil
	}
}

func aiSkillsNextPage(meta map[string]interface{}) int {
	switch value := meta["next_page"].(type) {
	case float64:
		return int(value)
	case int:
		return value
	case int64:
		return int(value)
	case string:
		page, _ := strconv.Atoi(value)
		return page
	default:
		return 0
	}
}

func aiSkillsResponseError(status int, body []byte) error {
	if status == http.StatusForbidden || status == http.StatusNotFound {
		return fmt.Errorf("%s", aiSkillsDisabledMessage)
	}
	return StatusResponseError(status, body)
}

func (m *TUIModel) loadSkillSuggestions(prefix string, request int) tea.Cmd {
	if m.api == nil || m.skillsUnavailable {
		return nil
	}
	apiClient, ctx := m.api, m.ctx
	enabled := true
	invocableBy := "user"
	sortBy := rootly.ListAISkillsParamsSortName
	params := &rootly.ListAISkillsParams{
		FilterEnabled: &enabled, FilterInvocableBy: &invocableBy, Sort: &sortBy,
	}
	params.FilterSearch = &prefix
	return func() tea.Msg {
		response, err := apiClient.ListAISkillsWithResponse(ctx, params, aiSkillsPageRequestEditor(1, 6))
		if err != nil {
			return skillSuggestionsLoadedMsg{request: request, prefix: prefix, err: err}
		}
		if response == nil {
			return skillSuggestionsLoadedMsg{request: request, prefix: prefix, err: errors.New("empty AI skills response")}
		}
		if response.ApplicationVndAPIJSON200 == nil {
			return skillSuggestionsLoadedMsg{
				request: request, prefix: prefix, statusCode: response.StatusCode(),
				err: aiSkillsResponseError(response.StatusCode(), response.Body),
			}
		}
		skills := make([]rootly.AiSkill, 0, len(response.ApplicationVndAPIJSON200.Data))
		for _, item := range response.ApplicationVndAPIJSON200.Data {
			skills = append(skills, item.Attributes)
		}
		return skillSuggestionsLoadedMsg{request: request, prefix: prefix, skills: skills}
	}
}

func (m *TUIModel) refreshCommandMenu() tea.Cmd {
	m.skillSuggestionReq++
	request := m.skillSuggestionReq
	rawValue := m.composer.Value()
	value := strings.TrimSpace(rawValue)
	if rawValue != value || value == "" || !strings.HasPrefix(value, "/") || strings.ContainsAny(value, " \t\r\n") {
		m.commandMenuOpen = false
		return nil
	}
	commands := []slashCommandItem{
		{command: "/exit", description: "exit chat"}, {command: "/quit", description: "exit chat"},
		{command: "/new", description: "new session"}, {command: "/clear", description: "new session"},
		{command: "/resume", description: "choose a session"}, {command: "/sessions", description: "browse sessions"},
		{command: "/skills", description: "browse skills"}, {command: "/rename", description: "rename session"},
		{command: "/pin", description: "pin session"}, {command: "/unpin", description: "unpin session"},
		{command: "/archive", description: "archive session"}, {command: "/delete", description: "delete session"},
		{command: "/copy", description: "copy last answer"}, {command: "/export", description: "export transcript"},
		{command: "/incident", description: "bind incident"}, {command: "/alert", description: "bind alert"},
		{command: "/status", description: "show session info"}, {command: "/help", description: "show help"},
	}
	items := make([]list.Item, 0, 6)
	for _, command := range commands {
		if strings.HasPrefix(strings.ToLower(command.command), strings.ToLower(value)) {
			items = append(items, command)
			if len(items) == 6 {
				break
			}
		}
	}
	prefix := strings.TrimPrefix(strings.ToLower(value), "/")
	if skills, cached := m.skillSuggestionCache[prefix]; cached {
		for _, skill := range skills {
			slug := skillString(skill.Slug)
			if slug == "" || !skillEnabled(skill) || !skillInvocableByUser(skill) ||
				isBuiltinSlashCommand("/"+slug) || !strings.HasPrefix("/"+strings.ToLower(slug), strings.ToLower(value)) {
				continue
			}
			items = append(items, slashCommandItem{
				command: "/" + slug, description: sanitizeTerminalText(strings.ReplaceAll(skillString(skill.Name), "\n", " ")), skill: true,
			})
		}
	}
	if len(items) == 0 {
		m.commandMenuOpen = false
	} else {
		m.commandMenu = list.New(items, slashCommandDelegate{}, max(20, m.width), min(6, len(items)))
		m.commandMenu.SetShowTitle(false)
		m.commandMenu.SetShowFilter(false)
		m.commandMenu.SetShowStatusBar(false)
		m.commandMenu.SetShowPagination(false)
		m.commandMenu.SetShowHelp(false)
		m.commandMenu.SetFilteringEnabled(false)
		m.commandMenuOpen = true
	}
	if m.api == nil || m.skillsUnavailable {
		return nil
	}
	if _, cached := m.skillSuggestionCache[prefix]; cached {
		return nil
	}
	return tea.Tick(150*time.Millisecond, func(time.Time) tea.Msg {
		return skillSuggestionTickMsg{request: request, prefix: prefix}
	})
}

func (m *TUIModel) sessionAction(action, message string, body rootly.UpdateAiChatSessionApplicationVndAPIPlusJSONRequestBody, sessionTitle *string) tea.Cmd {
	if m.sessionID == nil {
		return m.println("No session yet: send a message first.")
	}
	if m.api == nil {
		return m.println("session API unavailable")
	}
	m.actionRequest++
	request, id, apiClient, ctx := m.actionRequest, *m.sessionID, m.api, m.ctx
	return func() tea.Msg {
		response, err := apiClient.UpdateAiChatSessionWithApplicationVndAPIPlusJSONBodyWithResponse(ctx, rootly.ID(id.String()), body)
		if err == nil && response == nil {
			err = errors.New("empty session update response")
		} else if err == nil && (response.StatusCode() < 200 || response.StatusCode() >= 300) {
			err = sessionActionStatusError(response.StatusCode(), response.Body)
		}
		return sessionActionMsg{request: request, action: action, message: message, sessionTitle: sessionTitle, err: err}
	}
}

func (m *TUIModel) deleteSession() tea.Cmd {
	if m.sessionID == nil {
		return m.println("No session yet: send a message first.")
	}
	if m.api == nil {
		return m.println("session API unavailable")
	}
	m.actionRequest++
	request, id, apiClient, ctx := m.actionRequest, *m.sessionID, m.api, m.ctx
	return func() tea.Msg {
		response, err := apiClient.DeleteAiChatSessionWithResponse(ctx, rootly.ID(id.String()))
		if err == nil && response == nil {
			err = errors.New("empty session delete response")
		} else if err == nil && (response.StatusCode() < 200 || response.StatusCode() >= 300) {
			err = sessionActionStatusError(response.StatusCode(), response.Body)
		}
		return sessionActionMsg{request: request, action: "delete", message: "Deleted session " + shortSessionID(id.String()) + ".", err: err}
	}
}

var errSessionActionNotFound = errors.New("session action not found")

const sessionActionNotFoundMessage = "Session not found. Incident and alert chats can't be renamed, pinned, archived or deleted from the CLI yet."

func sessionActionStatusError(status int, body []byte) error {
	if status == http.StatusNotFound {
		return errSessionActionNotFound
	}
	return StatusResponseError(status, body)
}

func (m *TUIModel) handleSessionAction(message sessionActionMsg) tea.Cmd {
	if message.err != nil {
		if errors.Is(message.err, errSessionActionNotFound) {
			return m.println(sessionActionNotFoundMessage)
		}
		return m.println(sanitizeTerminalText(message.err.Error()))
	}
	if message.action == "rename" {
		m.sessionTitle = ""
		if message.sessionTitle != nil {
			m.sessionTitle = sanitizeTerminalText(*message.sessionTitle)
		}
	}
	text := sanitizeTerminalText(message.message)
	if message.action == "archive" || message.action == "delete" {
		m.startNewSession()
		m.println(faint(m.noColor, text))
		return m.restartWelcomeAnimation()
	}
	return m.println(faint(m.noColor, text))
}

func (m *TUIModel) startNewSession() {
	m.actionRequest++
	m.pickerRequest++
	m.sessionID = nil
	m.sessionTitle = ""
	m.resetAnswer()
	m.resetTranscript()
	m.resumedSession = false
	m.lastAnswer = ""
	m.currentPrompt = ""
	m.incident = nil
	m.alert = nil
	m.incidentUsed = false
	m.alertUsed = false
}

func (m *TUIModel) exportTranscript(filename string) tea.Cmd {
	if m.sessionID == nil {
		return m.println("No session yet: send a message first.")
	}
	if len(m.conversation) == 0 {
		return m.println("Nothing to export yet.")
	}
	if strings.TrimSpace(filename) == "" {
		filename = fmt.Sprintf("rootly-chat-%s-%s.md", shortSessionID(m.sessionID.String()), time.Now().Format("20060102-150405"))
	}
	path, err := filepath.Abs(filename)
	if err == nil {
		var file *os.File
		file, err = os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if err == nil {
			_, err = file.WriteString(m.transcriptMarkdown())
			closeErr := file.Close()
			if err == nil {
				err = closeErr
			}
		}
	}
	if err != nil {
		return m.println("Export failed: " + sanitizeTerminalText(err.Error()))
	}
	return m.println(faint(m.noColor, "Exported "+path))
}

func (m *TUIModel) transcriptMarkdown() string {
	var output strings.Builder
	for _, turn := range m.conversation {
		if turn.user != "" {
			fmt.Fprintf(&output, "## You\n\n%s\n\n", turn.user)
		}
		if turn.assistant != "" {
			fmt.Fprintf(&output, "## Rootly\n\n%s\n\n", turn.assistant)
		}
	}
	return strings.TrimRight(output.String(), "\n") + "\n"
}

func (m *TUIModel) resolveBinding(kind, value string) tea.Cmd {
	if m.api == nil {
		return m.println("resource API unavailable")
	}
	m.actionRequest++
	request, apiClient, ctx := m.actionRequest, m.api, m.ctx
	return func() tea.Msg {
		if kind == "incident" {
			incident, err := apiClient.GetIncidentByID(ctx, api.NormalizeIncidentID(value))
			return bindingResolvedMsg{request: request, incident: incident, err: err}
		}
		alert, err := apiClient.GetAlertByID(ctx, value)
		return bindingResolvedMsg{request: request, alert: alert, err: err}
	}
}

func normalizeSessionTitle(title string) string {
	title = strings.TrimSpace(sanitizeTerminalText(title))
	if title == "" || title == "Untitled" {
		return ""
	}
	return title
}

func loadSessionHistory(ctx context.Context, apiClient TUIAPI, sessionID uuid.UUID) ([]rootly.AiChatSessionMessage, string, error) {
	page, size := 1, 100
	first, err := apiClient.ListAiChatSessionMessagesWithResponse(ctx, sessionID, &rootly.ListAiChatSessionMessagesParams{PageNumber: &page, PageSize: &size})
	if err != nil {
		return nil, "", err
	}
	if first == nil {
		return nil, "", errors.New("empty session history response")
	}
	if first.ApplicationVndAPIJSON200 == nil {
		return nil, "", StatusResponseError(first.StatusCode(), first.Body)
	}
	sessionTitle := normalizeSessionTitle(first.ApplicationVndAPIJSON200.Title)
	messages := append([]rootly.AiChatSessionMessage(nil), first.ApplicationVndAPIJSON200.Messages...)
	pages := historyPageCount(first.ApplicationVndAPIJSON200.Meta)
	if pages > 1 {
		last, lastSize := pages, 100
		response, err := apiClient.ListAiChatSessionMessagesWithResponse(ctx, sessionID, &rootly.ListAiChatSessionMessagesParams{PageNumber: &last, PageSize: &lastSize})
		if err != nil {
			return nil, "", err
		}
		if response == nil {
			return nil, "", errors.New("empty session history response")
		}
		if response.ApplicationVndAPIJSON200 == nil {
			return nil, "", StatusResponseError(response.StatusCode(), response.Body)
		}
		seen := make(map[string]bool, len(messages))
		for _, message := range messages {
			seen[message.ID.String()] = true
		}
		for _, message := range response.ApplicationVndAPIJSON200.Messages {
			if !seen[message.ID.String()] {
				messages = append(messages, message)
			}
		}
	}
	sort.Slice(messages, func(i, j int) bool { return messages[i].CreatedAt.Before(messages[j].CreatedAt) })
	filtered := messages[:0]
	for _, message := range messages {
		if message.Role == rootly.AiChatSessionMessageRole("user") || message.Role == rootly.AiChatSessionMessageRole("assistant") {
			filtered = append(filtered, message)
		}
	}
	if len(filtered) > 6 {
		filtered = filtered[len(filtered)-6:]
	}
	return filtered, sessionTitle, nil
}

func historyPageCount(meta map[string]interface{}) int {
	pagination, ok := meta["pagination"].(map[string]interface{})
	if !ok {
		return 1
	}
	for _, key := range []string{"pages", "last"} {
		switch value := pagination[key].(type) {
		case float64:
			if int(value) > 1 {
				return int(value)
			}
		case int:
			if value > 1 {
				return value
			}
		}
	}
	return 1
}

func StatusResponseError(status int, body []byte) error {
	return sanitizeTerminalError(describeError(&rootly.AiChatStreamStatusError{StatusCode: status, Body: body}))
}
