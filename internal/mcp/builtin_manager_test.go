package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestBuiltinBashIsOptInAndStatePersists(t *testing.T) {
	manager, options := newBuiltinManager(t, func(string) (string, error) { return "", fmt.Errorf("not installed") })
	tools, omitted := manager.ClientTools()
	if containsClientTool(tools, "local_bash") {
		t.Fatal("bash must be off by default")
	}
	if len(omitted) != 0 {
		t.Fatalf("disabled built-ins must not be reported as omitted: %+v", omitted)
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
	if len(tools) > MaxClientTools || len(omitted) == 0 {
		t.Fatalf("combined client tool count = %d, omitted = %d", len(tools), len(omitted))
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
	if len(omitted) == 0 || len(advertised) >= MaxClientTools {
		t.Fatalf("large schemas were not limited: %d tools, %d omitted", len(advertised), len(omitted))
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

func TestClientToolsReportsOmittedToolReferences(t *testing.T) {
	manager, _ := newBuiltinManager(t, func(string) (string, error) { return "", fmt.Errorf("not installed") })
	manager.builtins = nil

	for _, serverName := range []string{"first", "second"} {
		tools := make([]*sdk.Tool, 35)
		for index := range tools {
			tools[index] = &sdk.Tool{
				Name:        fmt.Sprintf("tool_%02d", index),
				InputSchema: map[string]any{"type": "object"},
			}
		}
		addRuntimeTools(manager, serverName, tools)
	}

	advertised, omitted := manager.ClientTools()
	if len(advertised) != MaxClientTools {
		t.Fatalf("advertised %d tools, want %d", len(advertised), MaxClientTools)
	}
	if len(omitted) != 6 {
		t.Fatalf("omitted %d tools, want 6: %+v", len(omitted), omitted)
	}
	for index, ref := range omitted {
		wantName := fmt.Sprintf("tool_%02d", index+29)
		if ref.ServerName != "second" || ref.Tool != wantName {
			t.Errorf("omitted[%d] = %+v, want second · %s", index, ref, wantName)
		}
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

func TestManagerRemembersApprovalsAndReportsBadRules(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	rules := filepath.Join(home, ".rootly-cli", "rules")
	if err := os.MkdirAll(rules, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(rules, "broken.rules"), []byte(`prefix_rule(`), 0o600); err != nil {
		t.Fatal(err)
	}
	cwd := t.TempDir()
	options := ManagerOptions{
		CWD:       cwd,
		Store:     &Store{UserPath: filepath.Join(home, "mcp.json"), CWD: cwd},
		State:     &StateStore{Path: filepath.Join(home, "mcp-state.json")},
		Discovery: &Discovery{Home: home, CWD: cwd},
		LookPath:  func(string) (string, error) { return "/bin/true", nil },
	}
	manager := NewManager(options)
	defer manager.Close()
	if err := manager.Load(); err == nil || !strings.Contains(err.Error(), "broken.rules:1:13") {
		t.Fatalf("bad rules file not reported: %v", err)
	}
	if err := manager.SetBuiltinEnabled("local_bash", true); err != nil {
		t.Fatal(err)
	}
	manager.ClientTools()
	plan := map[string]any{"command": "terraform plan -out=tf.plan"}
	if !manager.NeedsApproval("local_bash", plan) {
		t.Fatal("terraform plan should ask before it is remembered")
	}
	if got := manager.ApprovalOptions("local_bash", plan); !got.Persist || strings.Join(got.Prefix, " ") != "terraform plan" {
		t.Fatalf("options = %+v", got)
	}
	if err := manager.RememberApproval("local_bash", plan, false); err != nil {
		t.Fatal(err)
	}
	if manager.NeedsApproval("local_bash", map[string]any{"command": "terraform plan"}) {
		t.Fatal("session approval not applied")
	}
	complexCommand := map[string]any{"command": "terraform plan > $HOME/plan"}
	if got := manager.ApprovalOptions("local_bash", complexCommand); got.Session || got.Persist {
		t.Fatalf("complex shell must be once-only, got %+v", got)
	}
	if err := manager.RememberApproval("local_bash", complexCommand, true); err == nil {
		t.Fatal("complex shell must not be saved")
	}
	if err := manager.RememberApproval("local_bash", map[string]any{"command": "npm test"}, true); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(rules, "broken.rules")); err != nil {
		t.Fatal(err)
	}
	fresh := NewManager(options)
	defer fresh.Close()
	if err := fresh.Load(); err != nil {
		t.Fatal(err)
	}
	if err := fresh.SetBuiltinEnabled("local_bash", true); err != nil {
		t.Fatal(err)
	}
	fresh.ClientTools()
	if fresh.NeedsApproval("local_bash", map[string]any{"command": "npm test -- --watch=false"}) {
		t.Fatal("saved rule not applied in a new process")
	}
	if !fresh.NeedsApproval("local_bash", plan) {
		t.Fatal("session approval leaked into a new process")
	}

	addRuntimeTools(manager, "deploys", []*sdk.Tool{{Name: "rollback", InputSchema: map[string]any{"type": "object"}}})
	all, _ := manager.ClientTools()
	name := ""
	for _, tool := range all {
		if strings.HasSuffix(tool.Name, "rollback") {
			name = tool.Name
		}
	}
	if got := manager.ApprovalOptions(name, nil); !got.Session || got.Persist {
		t.Fatalf("MCP tool options = %+v", got)
	}
	if err := manager.RememberApproval(name, nil, true); err == nil {
		t.Fatal("MCP tools cannot be saved as rules")
	}
	if err := manager.RememberApproval(name, nil, false); err != nil || manager.NeedsApproval(name, nil) {
		t.Fatalf("MCP session approval: %v", err)
	}
}

func TestSessionApprovalStaysWithItsServer(t *testing.T) {
	manager := NewManager(ManagerOptions{Store: NewStore(t.TempDir())})
	write := []*sdk.Tool{{Name: "write", InputSchema: map[string]any{"type": "object"}}}
	user := &runtimeServer{server: Server{Name: "docs", Scope: ScopeUser, Path: "user.json"}, status: StatusConnected, tools: write}
	project := &runtimeServer{server: Server{Name: "docs", Scope: ScopeProject, Path: "project.json"}, status: StatusConnected, tools: write}
	manager.mu.Lock()
	manager.servers = append(manager.servers, user, project)
	manager.byID[user.server.ID()] = user
	manager.byID[project.server.ID()] = project
	manager.mu.Unlock()
	nameFor := func(runtime *runtimeServer) string {
		tools, _ := manager.ClientTools()
		for _, tool := range tools {
			if ref, ok := manager.Lookup(tool.Name); ok && ref.ServerID == runtime.server.ID() && ref.Tool == "write" {
				return tool.Name
			}
		}
		t.Fatalf("no write tool for %s", runtime.server.ID())
		return ""
	}
	approved := nameFor(user)
	if err := manager.RememberApproval(approved, nil, false); err != nil || manager.NeedsApproval(approved, nil) {
		t.Fatalf("session approval for the user server: %v", err)
	}
	if other := nameFor(project); !manager.NeedsApproval(other, nil) {
		t.Fatal("the other docs server inherited the approval")
	}
	manager.mu.Lock()
	user.sessionOff = true
	manager.mu.Unlock()
	if other := nameFor(project); !manager.NeedsApproval(other, nil) {
		t.Fatalf("%s moved to the other server and kept the approval", other)
	}
	manager.mu.Lock()
	user.sessionOff = false
	manager.mu.Unlock()
	if manager.NeedsApproval(nameFor(user), nil) {
		t.Fatal("approval lost for the server it was given to")
	}
}
