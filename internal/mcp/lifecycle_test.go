package mcp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

const stdioHelperEnv = "ROOTLY_MCP_TEST_STDIO_SERVER"

// TestMain lets the test binary act as a real stdio MCP server so the
// manager lifecycle runs against an actual child process.
func TestMain(m *testing.M) {
	if os.Getenv(stdioHelperEnv) == "1" {
		server := sdk.NewServer(&sdk.Implementation{Name: "helper", Version: "1"}, nil)
		server.AddTool(&sdk.Tool{Name: "echo", Description: "Echo text", InputSchema: json.RawMessage(`{"type":"object","properties":{"text":{"type":"string"}}}`)},
			func(_ context.Context, request *sdk.CallToolRequest) (*sdk.CallToolResult, error) {
				var args struct{ Text string }
				_ = json.Unmarshal(request.Params.Arguments, &args)
				if args.Text == "fail" {
					return &sdk.CallToolResult{IsError: true, Content: []sdk.Content{&sdk.TextContent{Text: "tool failed"}}}, nil
				}
				return &sdk.CallToolResult{Content: []sdk.Content{&sdk.TextContent{Text: "echo: " + args.Text}}}, nil
			})
		server.AddTool(&sdk.Tool{Name: "structured", Description: "Structured only", InputSchema: json.RawMessage(`{"type":"object"}`)},
			func(context.Context, *sdk.CallToolRequest) (*sdk.CallToolResult, error) {
				return &sdk.CallToolResult{Content: []sdk.Content{}, StructuredContent: map[string]any{"ok": true}}, nil
			})
		_ = server.Run(context.Background(), &sdk.StdioTransport{})
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func stdioManager(t *testing.T, servers map[string]ServerConfig) *Manager {
	t.Helper()
	home := t.TempDir()
	cwd := t.TempDir()
	userPath := filepath.Join(home, ".rootly-cli", "mcp.json")
	writeFile(t, userPath, marshalTestJSON(t, map[string]any{"mcpServers": servers}))
	manager := NewManager(ManagerOptions{
		CWD:       cwd,
		Store:     &Store{UserPath: userPath, CWD: cwd},
		State:     &StateStore{Path: filepath.Join(home, ".rootly-cli", "mcp-state.json")},
		Tokens:    &TokenStore{Path: filepath.Join(home, ".rootly-cli", "mcp-tokens.json")},
		Discovery: &Discovery{Home: home, CWD: cwd},
		LookPath:  func(string) (string, error) { return "", os.ErrNotExist },
	})
	t.Cleanup(manager.Close)
	if err := manager.Load(); err != nil {
		t.Fatal(err)
	}
	return manager
}

func waitConnected(t *testing.T, manager *Manager) {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for manager.Connecting() > 0 {
		if time.Now().After(deadline) {
			t.Fatalf("servers still connecting: %+v", manager.Views())
		}
		select {
		case <-manager.Changes():
		case <-time.After(50 * time.Millisecond):
		}
	}
}

func viewByName(t *testing.T, manager *Manager, name string) ServerView {
	t.Helper()
	for _, view := range manager.Views() {
		if view.Server.Name == name {
			return view
		}
	}
	t.Fatalf("server %q missing from %+v", name, manager.Views())
	return ServerView{}
}

func toolNamed(t *testing.T, manager *Manager, suffix string) string {
	t.Helper()
	tools, _ := manager.ClientTools()
	for _, tool := range tools {
		if strings.HasSuffix(tool.Name, suffix) {
			return tool.Name
		}
	}
	t.Fatalf("no client tool ending in %q: %+v", suffix, tools)
	return ""
}

func TestManagerStdioLifecycle(t *testing.T) {
	manager := stdioManager(t, map[string]ServerConfig{
		"helper": {Command: os.Args[0], Env: map[string]string{stdioHelperEnv: "1"}},
		"broken": {Command: filepath.Join(t.TempDir(), "missing-binary")},
	})
	manager.Start()
	waitConnected(t, manager)

	helper := viewByName(t, manager, "helper")
	if helper.Status != StatusConnected || len(helper.Tools) != 2 {
		t.Fatalf("helper view = %+v", helper)
	}
	if broken := viewByName(t, manager, "broken"); broken.Status == StatusConnected || broken.Detail == "" {
		t.Fatalf("missing binary should fail with a reason: %+v", broken)
	}

	echo := toolNamed(t, manager, "echo")
	if result := manager.Call(context.Background(), echo, map[string]any{"text": "hi"}); result.IsError || result.Output != "echo: hi" {
		t.Fatalf("echo call = %+v", result)
	}
	if result := manager.Call(context.Background(), echo, map[string]any{"text": "fail"}); !result.IsError || result.Output != "tool failed" {
		t.Fatalf("tool error not surfaced: %+v", result)
	}
	if result := manager.Call(context.Background(), toolNamed(t, manager, "structured"), nil); result.IsError || result.Output != `{"ok":true}` {
		t.Fatalf("structured-only result = %+v", result)
	}
	if result := manager.Call(context.Background(), "nope_tool", nil); !result.IsError {
		t.Fatalf("unknown tool must error: %+v", result)
	}

	id := helper.Server.ID()
	if err := manager.ToggleSession(id); err != nil {
		t.Fatal(err)
	}
	if view := viewByName(t, manager, "helper"); !view.SessionOff {
		t.Fatalf("toggle should switch the server off for this session: %+v", view)
	}
	for _, tool := range func() []ClientTool { tools, _ := manager.ClientTools(); return tools }() {
		if strings.HasSuffix(tool.Name, "echo") {
			t.Fatal("tools of a session-off server must not be advertised")
		}
	}
	if result := manager.Call(context.Background(), echo, map[string]any{"text": "hi"}); !result.IsError {
		t.Fatalf("calling a session-off server must fail: %+v", result)
	}
	if err := manager.ToggleSession(id); err != nil {
		t.Fatal(err)
	}
	waitConnected(t, manager)
	if err := manager.Reconnect(id); err != nil {
		t.Fatal(err)
	}
	waitConnected(t, manager)
	if view := viewByName(t, manager, "helper"); view.Status != StatusConnected || view.SessionOff {
		t.Fatalf("reconnect should bring the server back: %+v", view)
	}
	if result := manager.Call(context.Background(), toolNamed(t, manager, "echo"), map[string]any{"text": "again"}); result.Output != "echo: again" {
		t.Fatalf("call after reconnect = %+v", result)
	}
	if err := manager.ToggleSession("unknown"); err == nil {
		t.Fatal("unknown server id must error")
	}
	if err := manager.Reconnect("unknown"); err == nil {
		t.Fatal("unknown server id must error")
	}

	manager.Close()
	if result := manager.Call(context.Background(), echo, map[string]any{"text": "closed"}); !result.IsError {
		t.Fatalf("calls after Close must fail: %+v", result)
	}
}

func TestMarketAddStaysOffUntilEnabledThenConnects(t *testing.T) {
	manager := stdioManager(t, map[string]ServerConfig{})
	manager.Start()
	server, err := manager.AddFromMarket(MarketEntry{Name: "helper", Config: ServerConfig{Command: os.Args[0], Env: map[string]string{stdioHelperEnv: "1"}}})
	if err != nil {
		t.Fatal(err)
	}
	if server.Scope != ScopeUser {
		t.Fatalf("marketplace adds must go to user scope, got %q", server.Scope)
	}
	view := viewByName(t, manager, "helper")
	if view.Status != StatusOff {
		t.Fatalf("marketplace add must start off, got %+v", view)
	}
	if err := manager.Reconnect(view.Server.ID()); err == nil || !strings.Contains(err.Error(), "is off") {
		t.Fatalf("reconnecting an off server = %v", err)
	}
	if _, err := manager.AddFromMarket(MarketEntry{Name: "helper", Config: ServerConfig{Command: "npx"}}); err == nil {
		t.Fatal("adding a duplicate name must fail")
	}
	if err := manager.Enable(view.Server.ID()); err != nil {
		t.Fatal(err)
	}
	waitConnected(t, manager)
	if view := viewByName(t, manager, "helper"); view.Status != StatusConnected || len(view.Tools) != 2 {
		t.Fatalf("enabled market server = %+v", view)
	}
	if err := manager.Enable("unknown"); err == nil {
		t.Fatal("unknown server id must error")
	}
	if err := manager.Login(context.Background(), "unknown", LoginOptions{}); err == nil {
		t.Fatal("login for an unknown server must error")
	}
}

func TestProbeStatusReportsUnauthenticatedStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()
	status, err := ProbeStatus(context.Background(), nil, ServerConfig{Type: TransportHTTP, URL: server.URL})
	if err != nil || status != http.StatusUnauthorized {
		t.Fatalf("ProbeStatus = %d, %v", status, err)
	}
	if _, err := ProbeStatus(context.Background(), server.Client(), ServerConfig{Type: TransportHTTP, URL: "http://127.0.0.1:1"}); err == nil {
		t.Fatal("unreachable server must error")
	}
}
