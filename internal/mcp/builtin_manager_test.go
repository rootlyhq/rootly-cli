package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestBuiltinBashIsOptInAndStatePersists(t *testing.T) {
	manager, options := newBuiltinManager(t, func(string) (string, error) { return "", fmt.Errorf("not installed") })
	tools, _ := manager.ClientTools()
	if containsClientTool(tools, "local_bash") {
		t.Fatal("bash must be off by default")
	}
	if result := manager.Call(context.Background(), "local_bash", map[string]any{"command": "printf unexpected"}); !result.IsError {
		t.Fatal("disabled bash unexpectedly ran")
	}
	if err := manager.SetBuiltinEnabled("local_bash", true); err != nil {
		t.Fatal(err)
	}
	tools, _ = manager.ClientTools()
	if !containsClientTool(tools, "local_bash") {
		t.Fatal("bash was not advertised after opt-in")
	}
	if !manager.NeedsApproval("local_bash", map[string]any{"command": "printf safe"}) {
		t.Fatal("bash must always require approval")
	}
	result := manager.Call(context.Background(), "local_bash", map[string]any{"command": "printf opt-in"})
	if result.IsError || !strings.Contains(result.Output, "opt-in") {
		t.Fatalf("opted-in bash did not run: %#v", result)
	}
	reloaded := NewManager(options)
	defer reloaded.Close()
	if err := reloaded.Load(); err != nil {
		t.Fatal(err)
	}
	if !containsBuiltin(reloaded.BuiltinTools(), "local_bash", true) {
		t.Fatal("bash enablement did not persist in MCP state")
	}
}

func TestCombinedClientToolLimitsAndBuiltinNameReservation(t *testing.T) {
	manager, _ := newBuiltinManager(t, func(string) (string, error) { return "/bin/true", nil })
	addRuntimeTools(manager, "sentry", []*sdk.Tool{
		{Name: "cli", InputSchema: map[string]any{"type": "object"}, Annotations: &sdk.ToolAnnotations{ReadOnlyHint: true}},
	})
	mcpTools := make([]*sdk.Tool, 59)
	for index := range mcpTools {
		mcpTools[index] = &sdk.Tool{
			Name:        fmt.Sprintf("tool_%02d", index),
			Description: "MCP tool",
			InputSchema: map[string]any{"type": "object"},
		}
	}
	addRuntimeTools(manager, "workspace", mcpTools)

	tools, omitted := manager.ClientTools()
	if len(tools) > MaxClientTools || omitted == 0 {
		t.Fatalf("combined client tool count = %d, omitted = %d", len(tools), omitted)
	}
	if !containsClientTool(tools, "local_sentry_cli") {
		t.Fatal("built-in sentry-cli name is missing")
	}
	if !containsClientTool(tools, "local_sentry_cli_2") {
		t.Fatal("MCP tool did not receive a collision-safe name")
	}
	if ref, ok := manager.Lookup("local_sentry_cli_2"); !ok || ref.ServerID == builtinServerID {
		t.Fatalf("collision-safe MCP reference is invalid: %+v %t", ref, ok)
	}
	if manager.NeedsApproval("local_sentry_cli_2", nil) {
		t.Fatal("read-only MCP approval behavior changed during name reservation")
	}
	for _, tool := range tools {
		if len(tool.InputSchema) > MaxSchemaBytes {
			t.Fatalf("%s schema exceeds limit: %d", tool.Name, len(tool.InputSchema))
		}
	}
	encoded, err := json.Marshal(tools)
	if err != nil {
		t.Fatal(err)
	}
	if len(encoded) > MaxClientToolsBytes {
		t.Fatalf("combined schema payload is %d bytes, limit is %d", len(encoded), MaxClientToolsBytes)
	}
}

func TestCombinedSchemaByteLimitWithMCPTools(t *testing.T) {
	manager, _ := newBuiltinManager(t, func(string) (string, error) { return "", fmt.Errorf("not installed") })
	tools := make([]*sdk.Tool, 40)
	for index := range tools {
		tools[index] = &sdk.Tool{
			Name:        fmt.Sprintf("large_%02d", index),
			Description: "large schema",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"value": map[string]any{"type": "string", "description": strings.Repeat("x", 10_000)},
				},
			},
		}
	}
	tools[0].InputSchema = map[string]any{
		"type": "object",
		"properties": map[string]any{
			"value": map[string]any{"type": "string", "description": strings.Repeat("x", MaxSchemaBytes+1)},
		},
	}
	addRuntimeTools(manager, "large", tools)

	advertised, omitted := manager.ClientTools()
	if omitted == 0 || len(advertised) >= MaxClientTools {
		t.Fatalf("large schemas were not limited: %d tools, %d omitted", len(advertised), omitted)
	}
	for _, tool := range advertised {
		if len(tool.InputSchema) > MaxSchemaBytes {
			t.Fatalf("%s schema exceeds limit: %d", tool.Name, len(tool.InputSchema))
		}
	}
	encoded, err := json.Marshal(advertised)
	if err != nil {
		t.Fatal(err)
	}
	if len(encoded) > MaxClientToolsBytes {
		t.Fatalf("combined schema payload is %d bytes, limit is %d", len(encoded), MaxClientToolsBytes)
	}
}

func newBuiltinManager(t *testing.T, lookPath func(string) (string, error)) (*Manager, ManagerOptions) {
	t.Helper()
	cwd, home := t.TempDir(), t.TempDir()
	options := ManagerOptions{
		CWD:       cwd,
		Store:     &Store{UserPath: filepath.Join(home, "mcp.json"), CWD: cwd},
		State:     &StateStore{Path: filepath.Join(home, "mcp-state.json")},
		Discovery: &Discovery{Home: home, CWD: cwd},
		LookPath:  lookPath,
	}
	manager := NewManager(options)
	if err := manager.Load(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(manager.Close)
	return manager, options
}

func addRuntimeTools(manager *Manager, name string, tools []*sdk.Tool) {
	server := Server{Name: name, Scope: ScopeUser, Path: filepath.Join("project", name+".mcp.json")}
	runtime := &runtimeServer{server: server, status: StatusConnected, tools: tools}
	manager.mu.Lock()
	manager.servers = append(manager.servers, runtime)
	manager.byID[server.ID()] = runtime
	manager.mu.Unlock()
}

func containsClientTool(tools []ClientTool, name string) bool {
	for _, tool := range tools {
		if tool.Name == name {
			return true
		}
	}
	return false
}

func containsBuiltin(tools []BuiltinToolInfo, name string, enabled bool) bool {
	for _, tool := range tools {
		if tool.Name == name {
			return tool.Enabled == enabled
		}
	}
	return false
}
