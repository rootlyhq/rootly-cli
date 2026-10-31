package agent

import (
	"context"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/rootlyhq/rootly-cli/internal/api"
	"github.com/rootlyhq/rootly-cli/internal/mcp"
)

type fakeMCPManager struct {
	fakeLocalTools
	views    []mcp.ServerView
	toggled  []string
	approved []string
	problems []string
	changes  chan struct{}
}

func (f *fakeMCPManager) Views() []mcp.ServerView  { return f.views }
func (f *fakeMCPManager) Changes() <-chan struct{} { return f.changes }
func (f *fakeMCPManager) Problems() []string       { return f.problems }
func (f *fakeMCPManager) Connecting() int          { return 0 }
func (f *fakeMCPManager) Enable(string) error      { return nil }
func (f *fakeMCPManager) Reconnect(string) error   { return nil }
func (f *fakeMCPManager) ToggleSession(id string) error {
	f.toggled = append(f.toggled, id)
	return nil
}
func (f *fakeMCPManager) Approve(id string) error                               { f.approved = append(f.approved, id); return nil }
func (f *fakeMCPManager) Login(context.Context, string, mcp.LoginOptions) error { return nil }

type fakeConnectorAPI struct {
	fakeTUIAPI
	pages []int
}

func (f *fakeConnectorAPI) ListAIConnectors(_ context.Context, page, _ int) (*api.AIConnectorPage, error) {
	f.pages = append(f.pages, page)
	count := 12
	if page == 1 {
		return &api.AIConnectorPage{Connectors: []api.AIConnector{{ID: "c1", Name: "Datadog", ProviderType: "datadog", AvailableToAgent: true, ToolCount: &count}}, NextPage: 2}, nil
	}
	return &api.AIConnectorPage{Connectors: []api.AIConnector{{ID: "c2", Name: "Sentry", ProviderType: "sentry"}}}, nil
}

func newMCPTestModel(t *testing.T, manager *fakeMCPManager, connectors *fakeConnectorAPI) *TUIModel {
	t.Helper()
	model := NewTUIModel(context.Background(), &fakeStreamer{}, TUIOptions{NoColor: true, API: connectors, MCP: manager})
	model.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	return model
}

func runCmd(model *TUIModel, cmd tea.Cmd) {
	if cmd == nil {
		return
	}
	if msg := cmd(); msg != nil {
		model.Update(msg)
	}
}

func testViews() []mcp.ServerView {
	tools := make([]mcp.ToolInfo, 1000)
	for index := range tools {
		tools[index] = mcp.ToolInfo{Name: "tool_" + strings.Repeat("x", index%5) + string(rune('a'+index%26)), ReadOnly: index%2 == 0}
	}
	tools[999] = mcp.ToolInfo{Name: "write_file", Description: "Write a file"}
	return []mcp.ServerView{
		{Server: mcp.Server{Name: "maestro", Scope: mcp.ScopeProject, Path: "/repo/.mcp.json", Config: mcp.ServerConfig{Command: "maestro", Args: []string{"mcp"}}}, Status: mcp.StatusPending},
		{Server: mcp.Server{Name: "fs", Scope: mcp.ScopeUser, Config: mcp.ServerConfig{Command: "npx", Env: map[string]string{"TOKEN": "secret-value"}}}, Status: mcp.StatusConnected, Tools: tools},
		{Server: mcp.Server{Name: "braintrust", Scope: mcp.ScopeUser, Config: mcp.ServerConfig{Type: "http", URL: "https://api.braintrust.dev/mcp", Headers: map[string]string{"Authorization": "Bearer hidden"}}}, Status: mcp.StatusNeedsAuth},
		{Server: mcp.Server{Name: "sentry", Scope: mcp.ScopeImported, SourceLabel: "Claude Code", Config: mcp.ServerConfig{Type: "http", URL: "https://mcp.sentry.dev/mcp"}}, Status: mcp.StatusOff},
	}
}

func TestMCPPageOpensGroupsAndEscRestoresChat(t *testing.T) {
	manager := &fakeMCPManager{views: testViews(), changes: make(chan struct{})}
	connectors := &fakeConnectorAPI{}
	model := newMCPTestModel(t, manager, connectors)
	model.composer.SetValue("draft question")
	before := model.View().Content

	for _, command := range []string{"/connectors", "/mcp"} {
		model.composer.SetValue(command)
		_, cmd := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
		runCmd(model, cmd)
		if !model.mcpPage.open {
			t.Fatalf("%s did not open the MCP page", command)
		}
		view := model.View().Content
		for _, want := range []string{"Manage MCP servers", "Rootly cloud", "Datadog", "✔ 12 tools", "Load more", "Project MCPs", "‖ pending approval", "User MCPs (~/.rootly-cli/mcp.json)", "✔ 1000 tools", "▲ needs authentication", "Imported (read-only)", "○ off", "Esc back to chat"} {
			if !strings.Contains(view, want) {
				t.Errorf("%s view missing %q:\n%s", command, want, view)
			}
		}
		if strings.Contains(view, "secret-value") || strings.Contains(view, "hidden") {
			t.Fatal("MCP page leaks a secret")
		}
		model.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
		if model.mcpPage.open {
			t.Fatal("Esc should close the MCP page")
		}
	}
	model.composer.SetValue("draft question")
	if after := model.View().Content; after != before {
		t.Fatalf("chat changed after Esc:\nbefore:\n%s\nafter:\n%s", before, after)
	}
	if len(connectors.pages) != 2 || connectors.pages[1] != 1 {
		t.Fatalf("each /mcp visit should refetch page 1, pages = %v", connectors.pages)
	}
}

func TestMCPPageLoadMoreDetailFilterAndActions(t *testing.T) {
	manager := &fakeMCPManager{views: testViews(), changes: make(chan struct{})}
	connectors := &fakeConnectorAPI{}
	model := newMCPTestModel(t, manager, connectors)
	runCmd(model, model.openMCPPage())

	press := func(keys ...tea.KeyPressMsg) {
		for _, key := range keys {
			_, cmd := model.Update(key)
			runCmd(model, cmd)
		}
	}
	down := tea.KeyPressMsg{Code: tea.KeyDown}
	enter := tea.KeyPressMsg{Code: tea.KeyEnter}
	press(down, enter)
	if len(connectors.pages) != 2 || !strings.Contains(model.View().Content, "Sentry") {
		t.Fatalf("Load more did not fetch page 2: %v", connectors.pages)
	}

	model.mcpPage.cursor = 0
	press(enter)
	if view := model.View().Content; !strings.Contains(view, "permissions come from Tool access on rootly.com") {
		t.Fatalf("cloud detail:\n%s", view)
	}
	press(tea.KeyPressMsg{Code: tea.KeyEscape})

	press(down, down)
	row, _ := model.selectedMCPRow()
	if row.view == nil || row.view.Server.Name != "maestro" {
		t.Fatalf("expected maestro row, got %+v", row)
	}
	press(tea.KeyPressMsg{Code: 'a', Text: "a"})
	if len(manager.approved) != 1 {
		t.Fatal("a should approve")
	}
	press(down, enter)
	view := model.View().Content
	if !strings.Contains(view, "Tools (1000)") || !strings.Contains(view, "[space] on/off for this session") || strings.Contains(view, "secret-value") {
		t.Fatalf("detail view:\n%s", view)
	}
	if lines := strings.Count(view, "\n"); lines > 40 {
		t.Fatalf("detail list not virtualized: %d lines", lines)
	}
	press(tea.KeyPressMsg{Code: '/', Text: "/"})
	for _, r := range "write" {
		press(tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	press(enter)
	if view := model.View().Content; !strings.Contains(view, "write_file") || !strings.Contains(view, "1 match") {
		t.Fatalf("filtered detail:\n%s", view)
	}
	press(tea.KeyPressMsg{Code: tea.KeySpace, Text: " "})
	if len(manager.toggled) != 1 || manager.toggled[0] != testViews()[1].Server.ID() {
		t.Fatalf("space should toggle fs, got %v", manager.toggled)
	}
}

func TestTUIClientToolWriteNeedsApproval(t *testing.T) {
	manager := &fakeMCPManager{changes: make(chan struct{})}
	manager.tools = map[string]mcp.ToolRef{"local_fs_write_file": {ServerID: "user:fs", ServerName: "fs", Tool: "write_file"}}
	model := newMCPTestModel(t, manager, &fakeConnectorAPI{})
	model.state = tuiStreaming
	model.turn = 1
	_, cmd := model.Update(streamMessage{turn: 1, event: Event{Type: EventClientToolCall, ClientToolCall: &ClientToolCall{CallID: "c1", Name: "local_fs_write_file", Arguments: map[string]any{"path": "/tmp/x"}}}})
	_ = cmd
	if model.localConfirm == nil || !strings.Contains(model.View().Content, "write_file") {
		t.Fatalf("expected a confirm card:\n%s", model.View().Content)
	}
	_, cmd = model.Update(tea.KeyPressMsg{Code: 'n', Text: "n"})
	if model.localConfirm != nil || cmd == nil {
		t.Fatal("n should decline and post a result")
	}
	if msg, ok := cmd().(localToolDoneMsg); !ok || msg.err != nil {
		t.Fatalf("decline result = %#v", msg)
	}
	if len(manager.calls) != 0 {
		t.Fatal("declined tool must not run")
	}
}
