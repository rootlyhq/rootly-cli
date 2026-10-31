package agent

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/rootlyhq/rootly-cli/internal/api"
	"github.com/rootlyhq/rootly-cli/internal/mcp"
)

const cloudConnectorPageSize = 25

// MCPManager is the agent's handle on local MCP servers.
type MCPManager interface {
	LocalTools
	Views() []mcp.ServerView
	BuiltinTools() []mcp.BuiltinToolInfo
	SetBuiltinEnabled(string, bool) error
	Problems() []string
	Changes() <-chan struct{}
	Connecting() int
	ToggleSession(string) error
	Approve(string) error
	Enable(string) error
	Reconnect(string) error
	Login(context.Context, string, mcp.LoginOptions) error
}

type ConnectorLister interface {
	ListAIConnectors(context.Context, int, int) (*api.AIConnectorPage, error)
}

type mcpChangedMsg struct{}

type cloudConnectorsMsg struct {
	request int
	page    *api.AIConnectorPage
	err     error
}

type mcpLoginMsg struct {
	id     string
	url    string
	err    error
	done   bool
	events <-chan mcpLoginMsg
}

type mcpRowKind int

const (
	mcpRowHeader mcpRowKind = iota
	mcpRowServer
	mcpRowCloud
	mcpRowLoadMore
	mcpRowNote
)

type mcpRow struct {
	kind  mcpRowKind
	text  string
	view  *mcp.ServerView
	cloud *api.AIConnector
}

type mcpPage struct {
	open            bool
	cursor          int
	offset          int
	filtering       bool
	filter          string
	detail          string
	detailCursor    int
	detailOffset    int
	detailFiltering bool
	detailFilter    string
	cloud           []api.AIConnector
	cloudNext       int
	cloudLoading    bool
	cloudLoaded     bool
	cloudErr        string
	cloudRequest    int
	message         string
	loginURL        string
	views           []mcp.ServerView
}

func (m *TUIModel) watchMCP() tea.Cmd {
	if m.mcp == nil {
		return nil
	}
	changes := m.mcp.Changes()
	return func() tea.Msg {
		if _, ok := <-changes; !ok {
			return nil
		}
		return mcpChangedMsg{}
	}
}

func (m *TUIModel) openMCPPage() tea.Cmd {
	m.mcpPage.open = true
	m.mcpPage.cursor, m.mcpPage.offset = 0, 0
	m.mcpPage.filtering, m.mcpPage.filter = false, ""
	m.mcpPage.detail = ""
	m.mcpPage.message = ""
	m.refreshMCPViews()
	if !m.mcpPage.cloudLoading {
		m.mcpPage.cloud, m.mcpPage.cloudLoaded = nil, false
		return m.loadCloudConnectors(1)
	}
	return nil
}

func (m *TUIModel) refreshMCPViews() {
	if m.mcp == nil {
		m.mcpPage.views = nil
		return
	}
	m.mcpPage.views = m.mcp.Views()
	builtinTools := m.mcp.BuiltinTools()
	if len(builtinTools) == 0 {
		return
	}
	builtinView := mcp.ServerView{
		Server: mcp.Server{Name: "Built-in", Scope: mcp.ScopeBuiltin, Path: "builtin"},
		Status: mcp.StatusConnected,
		Tools:  make([]mcp.ToolInfo, 0, len(builtinTools)),
	}
	for _, tool := range builtinTools {
		builtinView.Tools = append(builtinView.Tools, mcp.ToolInfo(tool))
	}
	m.mcpPage.views = append([]mcp.ServerView{builtinView}, m.mcpPage.views...)
}

func (m *TUIModel) loadCloudConnectors(page int) tea.Cmd {
	lister, ok := m.api.(ConnectorLister)
	if !ok {
		m.mcpPage.cloudLoaded = true
		m.mcpPage.cloudErr = "unavailable"
		return nil
	}
	m.mcpPage.cloudLoading = true
	m.mcpPage.cloudRequest++
	request := m.mcpPage.cloudRequest
	ctx := m.ctx
	return func() tea.Msg {
		result, err := lister.ListAIConnectors(ctx, page, cloudConnectorPageSize)
		return cloudConnectorsMsg{request: request, page: result, err: err}
	}
}

func (m *TUIModel) handleCloudConnectors(msg cloudConnectorsMsg) {
	if msg.request != m.mcpPage.cloudRequest {
		return
	}
	m.mcpPage.cloudLoading = false
	m.mcpPage.cloudLoaded = true
	if msg.err != nil {
		m.mcpPage.cloudErr = sanitizeTerminalText(msg.err.Error())
		if msg.page != nil && (msg.page.StatusCode == 403 || msg.page.StatusCode == 404) {
			m.mcpPage.cloudErr = "not available for your account"
		}
		return
	}
	m.mcpPage.cloudErr = ""
	m.mcpPage.cloud = append(m.mcpPage.cloud, msg.page.Connectors...)
	m.mcpPage.cloudNext = msg.page.NextPage
}

func (m *TUIModel) mcpRows() []mcpRow {
	page := &m.mcpPage
	filter := strings.ToLower(page.filter)
	match := func(name string) bool { return filter == "" || strings.Contains(strings.ToLower(name), filter) }
	rows := []mcpRow{{kind: mcpRowHeader, text: "Rootly cloud"}}
	cloudRows := 0
	for index := range page.cloud {
		if match(page.cloud[index].Name) {
			rows = append(rows, mcpRow{kind: mcpRowCloud, cloud: &page.cloud[index]})
			cloudRows++
		}
	}
	switch {
	case page.cloudLoading:
		rows = append(rows, mcpRow{kind: mcpRowNote, text: "Loading…"})
	case page.cloudErr != "":
		rows = append(rows, mcpRow{kind: mcpRowNote, text: "Could not load cloud connectors: " + page.cloudErr})
	case page.cloudNext > 0:
		rows = append(rows, mcpRow{kind: mcpRowLoadMore, text: "Load more…"})
	case cloudRows == 0 && page.cloudLoaded:
		rows = append(rows, mcpRow{kind: mcpRowNote, text: "No cloud connectors"})
	}
	groups := []struct {
		title  string
		scopes []string
	}{
		{"Built-in", []string{mcp.ScopeBuiltin}},
		{"Project MCPs", []string{mcp.ScopeProject, mcp.ScopeLocal}},
		{"User MCPs (~/.rootly-cli/mcp.json)", []string{mcp.ScopeUser}},
		{"Imported (read-only)", []string{mcp.ScopeImported}},
	}
	for _, group := range groups {
		title := group.title
		if group.scopes[0] == mcp.ScopeProject {
			if dir, err := os.Getwd(); err == nil {
				title += " (" + displayHomePath(dir) + ")"
			}
		}
		groupRows := []mcpRow{}
		for index := range page.views {
			view := &page.views[index]
			if !containsString(group.scopes, view.Server.Scope) || !matchesMCPView(view, match) {
				continue
			}
			groupRows = append(groupRows, mcpRow{kind: mcpRowServer, view: view})
		}
		if len(groupRows) == 0 && filter != "" {
			continue
		}
		rows = append(rows, mcpRow{kind: mcpRowHeader, text: title})
		if len(groupRows) == 0 {
			hint := "None · rootly mcp add …"
			switch group.scopes[0] {
			case mcp.ScopeImported:
				hint = "None found in Claude Code, Codex, Cursor, VS Code or Claude Desktop"
			case mcp.ScopeBuiltin:
				hint = "No built-in tools available"
			}
			rows = append(rows, mcpRow{kind: mcpRowNote, text: hint})
		}
		rows = append(rows, groupRows...)
	}
	return rows
}

func matchesMCPView(view *mcp.ServerView, match func(string) bool) bool {
	if match(view.Server.Name) {
		return true
	}
	if view.Server.Scope != mcp.ScopeBuiltin {
		return false
	}
	for _, tool := range view.Tools {
		if match(tool.Name) {
			return true
		}
	}
	return false
}

func containsString(values []string, value string) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
}

func displayHomePath(path string) string {
	home, err := os.UserHomeDir()
	if err == nil && home != "" && (path == home || strings.HasPrefix(path, home+string(os.PathSeparator))) {
		return "~" + strings.TrimPrefix(path, home)
	}
	return path
}

func selectable(row mcpRow) bool {
	return row.kind == mcpRowServer || row.kind == mcpRowCloud || row.kind == mcpRowLoadMore
}

func (m *TUIModel) moveMCPCursor(rows []mcpRow, delta int) {
	if len(rows) == 0 {
		return
	}
	index := m.mcpPage.cursor
	for step := 0; step < len(rows); step++ {
		index += delta
		if index < 0 || index >= len(rows) {
			return
		}
		if selectable(rows[index]) {
			m.mcpPage.cursor = index
			return
		}
	}
}

func (m *TUIModel) clampMCPCursor(rows []mcpRow) {
	if m.mcpPage.cursor >= len(rows) {
		m.mcpPage.cursor = len(rows) - 1
	}
	if m.mcpPage.cursor < 0 {
		m.mcpPage.cursor = 0
	}
	if len(rows) > 0 && !selectable(rows[m.mcpPage.cursor]) {
		for index := m.mcpPage.cursor; index < len(rows); index++ {
			if selectable(rows[index]) {
				m.mcpPage.cursor = index
				return
			}
		}
		for index := m.mcpPage.cursor; index >= 0; index-- {
			if selectable(rows[index]) {
				m.mcpPage.cursor = index
				return
			}
		}
	}
}

func (m *TUIModel) selectedMCPRow() (mcpRow, bool) {
	rows := m.mcpRows()
	m.clampMCPCursor(rows)
	if len(rows) == 0 || !selectable(rows[m.mcpPage.cursor]) {
		return mcpRow{}, false
	}
	return rows[m.mcpPage.cursor], true
}

func (m *TUIModel) detailView() (*mcp.ServerView, *api.AIConnector) {
	id := m.mcpPage.detail
	if strings.HasPrefix(id, "cloud:") {
		for index := range m.mcpPage.cloud {
			if "cloud:"+m.mcpPage.cloud[index].ID == id {
				return nil, &m.mcpPage.cloud[index]
			}
		}
		return nil, nil
	}
	for index := range m.mcpPage.views {
		if m.mcpPage.views[index].Server.ID() == id {
			return &m.mcpPage.views[index], nil
		}
	}
	return nil, nil
}

func (m *TUIModel) handleMCPPageKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	page := &m.mcpPage
	key := msg.String()
	if page.detail != "" {
		return m.handleMCPDetailKey(msg)
	}
	if page.filtering {
		switch key {
		case "esc":
			page.filtering, page.filter = false, ""
		case "enter":
			page.filtering = false
		case "backspace":
			if page.filter != "" {
				page.filter = string([]rune(page.filter)[:len([]rune(page.filter))-1])
			}
		default:
			if msg.Text != "" {
				page.filter += msg.Text
			}
		}
		page.cursor, page.offset = 0, 0
		m.clampMCPCursor(m.mcpRows())
		return m, nil
	}
	rows := m.mcpRows()
	m.clampMCPCursor(rows)
	switch key {
	case "esc", "q", "ctrl+c":
		page.open = false
		return m, nil
	case "up", "k":
		m.moveMCPCursor(rows, -1)
	case "down", "j":
		m.moveMCPCursor(rows, 1)
	case "pgup":
		for range max(1, m.mcpListHeight()-1) {
			m.moveMCPCursor(rows, -1)
		}
	case "pgdown":
		for range max(1, m.mcpListHeight()-1) {
			m.moveMCPCursor(rows, 1)
		}
	case "home", "g":
		page.cursor = 0
		m.clampMCPCursor(rows)
	case "end", "G":
		page.cursor = len(rows) - 1
		m.clampMCPCursor(rows)
	case "/":
		page.filtering = true
	case "enter":
		row, ok := m.selectedMCPRow()
		if !ok {
			return m, nil
		}
		switch row.kind {
		case mcpRowLoadMore:
			cmd := m.loadCloudConnectors(page.cloudNext)
			return m, cmd
		case mcpRowCloud:
			page.detail = "cloud:" + row.cloud.ID
		case mcpRowServer:
			page.detail = row.view.Server.ID()
			if row.view.Status == mcp.StatusNeedsAuth {
				page.message = "Needs authentication: press l to log in"
			}
		}
		page.detailCursor, page.detailOffset, page.detailFilter, page.detailFiltering = 0, 0, "", false
	case "space", " ", "a", "l", "r":
		row, ok := m.selectedMCPRow()
		if ok && row.kind == mcpRowServer {
			if row.view.Server.Scope == mcp.ScopeBuiltin && (key == "space" || key == " ") {
				return m, nil
			}
			cmd := m.mcpAction(row.view, key)
			return m, cmd
		}
	}
	return m, nil
}

func (m *TUIModel) handleMCPDetailKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	page := &m.mcpPage
	key := msg.String()
	view, _ := m.detailView()
	if page.detailFiltering {
		switch key {
		case "esc":
			page.detailFiltering, page.detailFilter = false, ""
		case "enter":
			page.detailFiltering = false
		case "backspace":
			if page.detailFilter != "" {
				page.detailFilter = string([]rune(page.detailFilter)[:len([]rune(page.detailFilter))-1])
			}
		default:
			if msg.Text != "" {
				page.detailFilter += msg.Text
			}
		}
		page.detailCursor, page.detailOffset = 0, 0
		return m, nil
	}
	tools := m.detailTools(view)
	switch key {
	case "esc", "q", "backspace":
		page.detail, page.message = "", ""
	case "ctrl+c":
		page.detail, page.open = "", false
	case "up", "k":
		page.detailCursor = max(0, page.detailCursor-1)
	case "down", "j":
		page.detailCursor = min(max(0, len(tools)-1), page.detailCursor+1)
	case "pgup":
		page.detailCursor = max(0, page.detailCursor-m.mcpListHeight())
	case "pgdown":
		page.detailCursor = min(max(0, len(tools)-1), page.detailCursor+m.mcpListHeight())
	case "/":
		page.detailFiltering = true
	case "space", " ", "a", "l", "r":
		if view != nil {
			cmd := m.mcpAction(view, key)
			return m, cmd
		}
	}
	return m, nil
}

func (m *TUIModel) mcpAction(view *mcp.ServerView, key string) tea.Cmd {
	if m.mcp == nil {
		return nil
	}
	page := &m.mcpPage
	if view.Server.Scope == mcp.ScopeBuiltin {
		if key == "space" || key == " " {
			m.toggleBuiltinTool(view)
		}
		return nil
	}
	id := view.Server.ID()
	name := sanitizeTerminalText(view.Server.Name)
	var err error
	switch key {
	case "space", " ":
		if view.Status == mcp.StatusOff {
			err = m.mcp.Enable(id)
			page.message = "Enabled " + name
		} else {
			err = m.mcp.ToggleSession(id)
			if view.SessionOff {
				page.message = name + " is on for this session"
			} else {
				page.message = name + " is off for this session"
			}
		}
	case "a":
		err = m.mcp.Approve(id)
		page.message = "Approved " + name
	case "r":
		err = m.mcp.Reconnect(id)
		page.message = "Reconnecting " + name + "…"
	case "l":
		if view.Server.Config.Transport() == mcp.TransportStdio {
			page.message = name + " is a stdio server; there is nothing to log in to"
			return nil
		}
		page.message = "Opening your browser to log in to " + name + "…"
		return m.startMCPLogin(id)
	}
	if err != nil {
		page.message = sanitizeTerminalText(err.Error())
	}
	m.refreshMCPViews()
	return nil
}

func (m *TUIModel) toggleBuiltinTool(view *mcp.ServerView) {
	tools := m.detailTools(view)
	if len(tools) == 0 || m.mcpPage.detailCursor >= len(tools) {
		return
	}
	tool := tools[m.mcpPage.detailCursor]
	enabled := !tool.Enabled
	if err := m.mcp.SetBuiltinEnabled(tool.Name, enabled); err != nil {
		m.mcpPage.message = sanitizeTerminalText(err.Error())
	} else if enabled {
		m.mcpPage.message = "Enabled " + sanitizeTerminalText(tool.Name)
	} else {
		m.mcpPage.message = "Disabled " + sanitizeTerminalText(tool.Name)
	}
	m.refreshMCPViews()
}

func (m *TUIModel) startMCPLogin(id string) tea.Cmd {
	manager := m.mcp
	ctx := m.ctx
	events := make(chan mcpLoginMsg, 2)
	go func() {
		defer close(events)
		err := manager.Login(ctx, id, mcp.LoginOptions{OpenURL: func(url string) {
			_ = mcp.OpenBrowser(url)
			events <- mcpLoginMsg{id: id, url: url}
		}})
		events <- mcpLoginMsg{id: id, err: err, done: true}
	}()
	return waitMCPLogin(events)
}

func waitMCPLogin(events <-chan mcpLoginMsg) tea.Cmd {
	return func() tea.Msg {
		msg, ok := <-events
		if !ok {
			return nil
		}
		msg.events = events
		return msg
	}
}

func (m *TUIModel) handleMCPLogin(msg mcpLoginMsg) tea.Cmd {
	if !msg.done {
		m.mcpPage.loginURL = msg.url
		return waitMCPLogin(msg.events)
	}
	m.mcpPage.loginURL = ""
	if msg.err != nil {
		m.mcpPage.message = "Login failed: " + sanitizeTerminalText(msg.err.Error())
	} else {
		m.mcpPage.message = "Logged in. Reconnecting…"
	}
	m.refreshMCPViews()
	return nil
}

func (m *TUIModel) detailTools(view *mcp.ServerView) []mcp.ToolInfo {
	if view == nil {
		return nil
	}
	filter := strings.ToLower(m.mcpPage.detailFilter)
	if filter == "" {
		return view.Tools
	}
	tools := make([]mcp.ToolInfo, 0, len(view.Tools))
	for _, tool := range view.Tools {
		if strings.Contains(strings.ToLower(tool.Name), filter) || strings.Contains(strings.ToLower(tool.Description), filter) {
			tools = append(tools, tool)
		}
	}
	return tools
}

func (m *TUIModel) mcpListHeight() int {
	return max(3, m.height-6)
}

func mcpStatusLabel(view mcp.ServerView) string {
	if view.Server.Scope == mcp.ScopeBuiltin {
		enabled := 0
		for _, tool := range view.Tools {
			if tool.Enabled {
				enabled++
			}
		}
		return fmt.Sprintf("%d/%d enabled", enabled, len(view.Tools))
	}
	if view.SessionOff {
		return "○ off for this session"
	}
	switch view.Status {
	case mcp.StatusConnected:
		return "✔ " + pluralTools(len(view.Tools))
	case mcp.StatusError:
		return "✘ error"
	case mcp.StatusNeedsAuth:
		if view.Detail != "" && strings.Contains(view.Detail, "reconnect") {
			return "▲ reconnect"
		}
		return "▲ needs authentication"
	case mcp.StatusPending:
		return "‖ pending approval"
	case mcp.StatusConnecting:
		return "… connecting"
	default:
		return "○ off"
	}
}

func cloudStatusLabel(connector api.AIConnector) string {
	if !connector.AvailableToAgent {
		status := connector.Status
		if status == "" {
			status = "off"
		}
		return "○ " + sanitizeTerminalText(status)
	}
	if connector.ToolCount != nil {
		return "✔ " + pluralTools(*connector.ToolCount)
	}
	return "✔ available"
}

func pluralTools(count int) string {
	if count == 1 {
		return "1 tool"
	}
	return strconv.Itoa(count) + " tools"
}

func (m *TUIModel) mcpSummary() string {
	servers, tools := len(m.mcpPage.cloud), 0
	for _, connector := range m.mcpPage.cloud {
		if connector.AvailableToAgent && connector.ToolCount != nil {
			tools += *connector.ToolCount
		}
	}
	for _, view := range m.mcpPage.views {
		if view.Server.Scope == mcp.ScopeBuiltin {
			for _, tool := range view.Tools {
				if tool.Enabled {
					tools++
				}
			}
			continue
		}
		servers++
		if view.Status == mcp.StatusConnected && !view.SessionOff {
			tools += len(view.Tools)
		}
	}
	return fmt.Sprintf("%d servers · %d tools available to the agent", servers, tools)
}

func (m *TUIModel) mcpStyle(style lipgloss.Style, text string) string {
	if m.noColor {
		return text
	}
	return style.Render(text)
}

func (m *TUIModel) mcpPageView() string {
	if m.mcpPage.detail != "" {
		return m.mcpDetailView()
	}
	width := max(20, m.width-1)
	bold := lipgloss.NewStyle().Bold(true)
	lines := []string{m.mcpStyle(bold, "Manage MCP servers"), faint(m.noColor, m.mcpSummary()), ""}
	if m.mcp != nil {
		if problems := m.mcp.Problems(); len(problems) > 0 {
			lines[1] += faint(m.noColor, " · ✘ config: "+sanitizeTerminalText(strings.Join(problems, "; ")))
		}
	}
	if m.mcpPage.filtering || m.mcpPage.filter != "" {
		lines[2] = "Filter: " + sanitizeTerminalText(m.mcpPage.filter) + map[bool]string{true: "▌", false: ""}[m.mcpPage.filtering]
	}
	rows := m.mcpRows()
	m.clampMCPCursor(rows)
	height := m.mcpListHeight()
	page := &m.mcpPage
	if page.cursor < page.offset {
		page.offset = page.cursor
	}
	if page.cursor >= page.offset+height {
		page.offset = page.cursor - height + 1
	}
	page.offset = max(0, min(page.offset, max(0, len(rows)-height)))
	end := min(len(rows), page.offset+height)
	for index := page.offset; index < end; index++ {
		lines = append(lines, ansi.Truncate(m.renderMCPRow(rows[index], index == page.cursor), width, "…"))
	}
	for len(lines) < height+3 {
		lines = append(lines, "")
	}
	footer := "↑/↓ move · Enter details · / filter · Esc back to chat · rootly mcp add … to add one"
	if page.message != "" {
		footer = sanitizeTerminalText(page.message)
	}
	lines = append(lines, "", faint(m.noColor, ansi.Truncate(footer, width, "…")))
	return strings.Join(lines, "\n")
}

func (m *TUIModel) renderMCPRow(row mcpRow, selected bool) string {
	prefix := "  "
	if selected {
		prefix = "❯ "
	}
	switch row.kind {
	case mcpRowHeader:
		return m.mcpStyle(lipgloss.NewStyle().Bold(true), row.text)
	case mcpRowNote:
		return "  " + faint(m.noColor, row.text)
	case mcpRowLoadMore:
		return prefix + row.text
	case mcpRowCloud:
		name := sanitizeTerminalText(row.cloud.Name)
		return prefix + padRight(name, 28) + " " + padRight(cloudStatusLabel(*row.cloud), 24) + " " + faint(m.noColor, sanitizeTerminalText(row.cloud.ProviderType))
	case mcpRowServer:
		view := row.view
		name := sanitizeTerminalText(view.Server.Name)
		origin := sanitizeTerminalText(view.Server.Config.Masked().Origin())
		if view.Server.Scope == mcp.ScopeBuiltin {
			origin = "local tools"
		}
		if view.Server.Scope == mcp.ScopeImported {
			origin += " · " + view.Server.SourceLabel
		}
		if view.Server.Scope == mcp.ScopeLocal {
			origin += " · local"
		}
		return prefix + padRight(name, 28) + " " + padRight(mcpStatusLabel(*view), 24) + " " + faint(m.noColor, origin)
	}
	return ""
}

func padRight(text string, width int) string {
	text = ansi.Truncate(text, width, "…")
	if gap := width - ansi.StringWidth(text); gap > 0 {
		return text + strings.Repeat(" ", gap)
	}
	return text
}

func (m *TUIModel) mcpDetailView() string {
	width := max(20, m.width-1)
	bold := lipgloss.NewStyle().Bold(true)
	view, cloud := m.detailView()
	lines := []string{}
	switch {
	case cloud != nil:
		lines = append(lines,
			m.mcpStyle(bold, sanitizeTerminalText(cloud.Name)),
			"Provider: "+sanitizeTerminalText(cloud.ProviderType),
			"Status: "+cloudStatusLabel(*cloud),
			"",
			"Cloud connector: permissions come from Tool access on rootly.com",
		)
		if cloud.ManageURL != "" {
			lines = append(lines, "Manage: "+sanitizeTerminalText(cloud.ManageURL))
		}
		lines = append(lines, "", faint(m.noColor, "Esc back"))
		return strings.Join(lines, "\n")
	case view == nil:
		return "This server is no longer configured.\n\n" + faint(m.noColor, "Esc back")
	}
	if view.Server.Scope == mcp.ScopeBuiltin {
		return m.builtinDetailView(view, width, bold)
	}
	config := view.Server.Config.Masked()
	source := displayHomePath(view.Server.Path)
	if view.Server.Scope == mcp.ScopeImported {
		source = view.Server.SourceLabel + " · " + source + " (read-only)"
	}
	lines = append(lines,
		m.mcpStyle(bold, sanitizeTerminalText(view.Server.Name))+"  "+mcpStatusLabel(*view),
		faint(m.noColor, sanitizeTerminalText(config.Transport()+" · "+config.Origin())),
		faint(m.noColor, "Source: "+sanitizeTerminalText(source)),
	)
	if pairs := mcp.MaskedPairs(config.Env, "="); len(pairs) > 0 {
		lines = append(lines, faint(m.noColor, "Env: "+sanitizeTerminalText(strings.Join(pairs, ", "))))
	}
	if pairs := mcp.MaskedPairs(config.Headers, ": "); len(pairs) > 0 {
		lines = append(lines, faint(m.noColor, "Headers: "+sanitizeTerminalText(strings.Join(pairs, ", "))))
	}
	if view.Detail != "" {
		lines = append(lines, sanitizeTerminalText(view.Detail))
	}
	if m.mcpPage.loginURL != "" {
		lines = append(lines, "If your browser didn't open, visit: "+sanitizeTerminalText(m.mcpPage.loginURL))
	}
	lines = append(lines, "", "[space] on/off for this session   [a] approve   [l] log in   [r] reconnect", "")
	tools := m.detailTools(view)
	header := fmt.Sprintf("Tools (%d)", len(view.Tools))
	if m.mcpPage.detailFiltering || m.mcpPage.detailFilter != "" {
		header += fmt.Sprintf(" · filter: %s%s · %d match", sanitizeTerminalText(m.mcpPage.detailFilter), map[bool]string{true: "▌", false: ""}[m.mcpPage.detailFiltering], len(tools))
	}
	lines = append(lines, m.mcpStyle(bold, header))
	height := max(1, m.height-len(lines)-3)
	page := &m.mcpPage
	page.detailCursor = min(page.detailCursor, max(0, len(tools)-1))
	if page.detailCursor < page.detailOffset {
		page.detailOffset = page.detailCursor
	}
	if page.detailCursor >= page.detailOffset+height {
		page.detailOffset = page.detailCursor - height + 1
	}
	page.detailOffset = max(0, min(page.detailOffset, max(0, len(tools)-height)))
	end := min(len(tools), page.detailOffset+height)
	for index := page.detailOffset; index < end; index++ {
		tool := tools[index]
		prefix := "  "
		if index == page.detailCursor {
			prefix = "❯ "
		}
		mark := "write"
		if tool.ReadOnly {
			mark = "read-only"
		}
		description := strings.Join(strings.Fields(sanitizeTerminalText(mcp.MaskCommand(tool.Description))), " ")
		lines = append(lines, ansi.Truncate(prefix+padRight(sanitizeTerminalText(tool.Name), 32)+" "+padRight(mark, 10)+" "+faint(m.noColor, description), width, "…"))
	}
	if len(view.Tools) == 0 {
		lines = append(lines, faint(m.noColor, "  No tools yet"))
	}
	footer := "↑/↓ move · / filter tools · Esc back"
	if page.message != "" {
		footer = sanitizeTerminalText(page.message)
	}
	lines = append(lines, "", faint(m.noColor, ansi.Truncate(footer, width, "…")))
	return strings.Join(lines, "\n")
}

func (m *TUIModel) builtinDetailView(view *mcp.ServerView, width int, bold lipgloss.Style) string {
	lines := []string{
		m.mcpStyle(bold, "Built-in tools") + "  " + mcpStatusLabel(*view),
		faint(m.noColor, "Filesystem access is confined to the current working directory."),
		"",
		"[space] enable/disable selected tool   [bash] always requires approval",
	}
	tools := m.detailTools(view)
	header := fmt.Sprintf("Tools (%d)", len(view.Tools))
	if m.mcpPage.detailFiltering || m.mcpPage.detailFilter != "" {
		header += fmt.Sprintf(" · filter: %s%s · %d match", sanitizeTerminalText(m.mcpPage.detailFilter), map[bool]string{true: "▌", false: ""}[m.mcpPage.detailFiltering], len(tools))
	}
	lines = append(lines, m.mcpStyle(bold, header))
	height := max(1, m.height-len(lines)-3)
	page := &m.mcpPage
	page.detailCursor = min(page.detailCursor, max(0, len(tools)-1))
	if page.detailCursor < page.detailOffset {
		page.detailOffset = page.detailCursor
	}
	if page.detailCursor >= page.detailOffset+height {
		page.detailOffset = page.detailCursor - height + 1
	}
	page.detailOffset = max(0, min(page.detailOffset, max(0, len(tools)-height)))
	end := min(len(tools), page.detailOffset+height)
	for index := page.detailOffset; index < end; index++ {
		tool := tools[index]
		prefix := "  "
		if index == page.detailCursor {
			prefix = "❯ "
		}
		mark := "off"
		if tool.Enabled {
			mark = "enabled"
		}
		if tool.Name == "local_bash" {
			mark += " · approval"
		}
		description := strings.Join(strings.Fields(sanitizeTerminalText(mcp.MaskCommand(tool.Description))), " ")
		lines = append(lines, ansi.Truncate(prefix+padRight(sanitizeTerminalText(tool.Name), 32)+" "+padRight(mark, 16)+" "+faint(m.noColor, description), width, "…"))
	}
	if len(view.Tools) == 0 {
		lines = append(lines, faint(m.noColor, "  No built-in tools available"))
	}
	footer := "↑/↓ move · space toggle · / filter tools · Esc back"
	if page.message != "" {
		footer = sanitizeTerminalText(page.message)
	}
	lines = append(lines, "", faint(m.noColor, ansi.Truncate(footer, width, "…")))
	return strings.Join(lines, "\n")
}

// mcpStatusLine is the pinned line about local servers above the prompt.
func (m *TUIModel) mcpStatusLine() string {
	if m.mcp == nil {
		return ""
	}
	if connecting := m.mcp.Connecting(); connecting > 0 {
		noun := "servers"
		if connecting == 1 {
			noun = "server"
		}
		return faint(m.noColor, fmt.Sprintf("MCP · connecting %d %s…", connecting, noun))
	}
	attention := 0
	for _, view := range m.mcpPage.views {
		if view.Server.Scope != mcp.ScopeBuiltin && !view.SessionOff &&
			(view.Status == mcp.StatusError || view.Status == mcp.StatusNeedsAuth || view.Status == mcp.StatusPending) {
			attention++
		}
	}
	if attention == 0 {
		return ""
	}
	noun := "servers need"
	if attention == 1 {
		noun = "server needs"
	}
	return faint(m.noColor, fmt.Sprintf("MCP · %d %s attention · /mcp", attention, noun))
}

func (m *TUIModel) updateLocalMCP(message tea.Msg) (tea.Cmd, bool) {
	switch msg := message.(type) {
	case mcpChangedMsg:
		m.refreshMCPViews()
		return m.watchMCP(), true
	case cloudConnectorsMsg:
		m.handleCloudConnectors(msg)
		return nil, true
	case mcpLoginMsg:
		return m.handleMCPLogin(msg), true
	case localToolDoneMsg:
		if msg.err != nil && msg.turn == m.turn {
			cmd := m.handleLocalToolDone(msg)
			if cmd != nil {
				return tea.Batch(cmd, m.println(faint(m.noColor, sanitizeTerminalText(msg.err.Error())))), true
			}
			return m.println(faint(m.noColor, sanitizeTerminalText(msg.err.Error()))), true
		}
		return m.handleLocalToolDone(msg), true
	}
	return nil, false
}

func (m *TUIModel) handleLocalToolDone(msg localToolDoneMsg) tea.Cmd {
	if msg.turn != m.turn {
		return nil
	}
	if msg.approval {
		m.localApprovalRunning = false
	}
	if msg.readOnly && m.localReadOnlyRunning > 0 {
		m.localReadOnlyRunning--
	}
	return m.startNextLocalTool()
}

func (m *TUIModel) updateOtherMessage(message tea.Msg) (tea.Model, tea.Cmd) {
	if cmd, handled := m.updateLocalMCP(message); handled {
		return m, cmd
	}
	return m.updatePickerMessage(message)
}
