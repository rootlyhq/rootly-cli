package mcp

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestOriginMasksSecretArgs(t *testing.T) {
	origin := ServerConfig{Command: "gh-mcp", Args: []string{"--token=sk-live", "--api-key", "abc", "serve"}}.Origin()
	if strings.Contains(origin, "sk-live") || strings.Contains(origin, "abc") || !strings.Contains(origin, "serve") {
		t.Fatalf("origin leaks secrets: %q", origin)
	}
}

func TestMaskToolArgumentsRedactsCommandAndCLISecrets(t *testing.T) {
	values := map[string]any{
		"command": "export AWS_SECRET_ACCESS_KEY=aws-secret && gh --token 'cli-token'",
		"args":    []any{"--api-key", "api-secret", "AWS_ACCESS_KEY_ID=access-secret"},
	}
	encoded, err := json.Marshal(MaskToolArguments(values))
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"aws-secret", "cli-token", "api-secret", "access-secret"} {
		if strings.Contains(string(encoded), secret) {
			t.Errorf("masked arguments leaked %q: %s", secret, encoded)
		}
	}
}

func TestMaskCommandMasksShellCodeInSecretValues(t *testing.T) {
	for _, test := range []struct {
		command string
		code    string
	}{
		{command: `export TOKEN="$(rm x)"`, code: "rm x"},
		{command: "export TOKEN=\"`rm x`\"", code: "rm x"},
		{command: `export TOKEN="${rm x}"`, code: "rm x"},
		{command: `export TOKEN="<(rm x)"`, code: "rm x"},
		{command: `export TOKEN=">(rm x)"`, code: "rm x"},
	} {
		masked := MaskCommand(test.command)
		if strings.Contains(masked, test.code) || !strings.Contains(masked, Mask) {
			t.Errorf("config command did not mask executable code: %q", masked)
		}
	}
}

func TestMaskApprovalCommandKeepsExecutableCodeVisible(t *testing.T) {
	for _, command := range []string{
		`export TOKEN="$(rm -rf ./data)"`,
		"export TOKEN=\"`rm x`\"",
		`export TOKEN="${rm x}"`,
		`export TOKEN="<(rm x)"`,
		`export TOKEN=">(rm x)"`,
	} {
		if got := MaskApprovalCommand(command); !strings.Contains(got, "rm ") {
			t.Errorf("approval command hid executable code: %q", got)
		}
	}
}

func TestMaskingConfigArgumentsMasksShellSyntax(t *testing.T) {
	args := MaskCommandArgs([]string{"--password", "a$(b"})
	if strings.Contains(strings.Join(args, " "), "a$(b") || args[1] != Mask {
		t.Fatalf("argv secret was not masked: %#v", args)
	}
	toolArguments := MaskToolArguments(map[string]any{"password": "x$(y)"})
	if toolArguments["password"] != Mask {
		t.Fatalf("tool argument secret was not masked: %#v", toolArguments)
	}
	command := MaskCommand(`export TOKEN="$(rm -rf ./data)"`)
	if strings.Contains(command, "rm -rf ./data") || !strings.Contains(command, Mask) {
		t.Fatalf("command description did not mask executable code: %q", command)
	}
}

func TestMaskApprovalArgumentsPreservesShellCodeAtAnyDepth(t *testing.T) {
	arguments := MaskApprovalArguments(map[string]any{
		"password": "x${y}",
		"args":     []any{"--password", "a$(b)", "TOKEN=$(rm x)"},
		"nested":   map[string]any{"token": ">(rm x)"},
	})
	if arguments["password"] != "x${y}" {
		t.Fatalf("approval arguments masked executable code in secret-named value: %#v", arguments)
	}
	args := arguments["args"].([]string)
	if args[1] != "a$(b)" || args[2] != "TOKEN=$(rm x)" {
		t.Fatalf("approval arguments masked executable code in argv: %#v", args)
	}
	nested := arguments["nested"].(map[string]any)
	if nested["token"] != ">(rm x)" {
		t.Fatalf("approval arguments masked executable code in nested value: %#v", nested)
	}
}

func TestServerHTTPClientKeepsCredentialsOnOrigin(t *testing.T) {
	var leaked []string
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		leaked = append(leaked, r.Header.Get("X-Api-Key")+"|"+r.Header.Get("Authorization"))
		w.WriteHeader(http.StatusOK)
	}))
	defer other.Close()
	var seen string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/redirect" {
			http.Redirect(w, r, other.URL+"/steal", http.StatusFound)
			return
		}
		seen = r.Header.Get("X-Api-Key") + "|" + r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := newServerHTTPClient(ServerConfig{Type: TransportHTTP, URL: server.URL + "/mcp", Headers: map[string]string{"X-Api-Key": "k"}}, func() string { return "tok" }, nil)
	get := func(target string) error {
		request, _ := http.NewRequestWithContext(t.Context(), http.MethodGet, target, http.NoBody)
		response, err := client.Do(request)
		if err == nil {
			_ = response.Body.Close()
		}
		return err
	}
	if err := get(server.URL + "/mcp"); err != nil || seen != "k|Bearer tok" {
		t.Fatalf("same origin should get credentials, err=%v seen=%q", err, seen)
	}
	if err := get(server.URL + "/redirect"); err == nil || !strings.Contains(err.Error(), "cross-origin redirect") {
		t.Fatalf("cross-origin redirect should be refused, err=%v", err)
	}
	if err := get(other.URL + "/direct"); err != nil {
		t.Fatal(err)
	}
	if len(leaked) != 1 || leaked[0] != "|" {
		t.Fatalf("credentials reached another origin: %v", leaked)
	}
}

func TestImportedProjectFilesNeedApprovalPerHash(t *testing.T) {
	home, project := t.TempDir(), t.TempDir()
	writeFile(t, filepath.Join(home, ".claude.json"), `{"mcpServers":{"global":{"command":"g"}},"projects":{"`+project+`":{"mcpServers":{"repo-db":{"command":"db"}}}}}`)
	writeFile(t, filepath.Join(project, ".cursor", "mcp.json"), `{"mcpServers":{"cursor-tool":{"command":"run-me"}}}`)
	writeFile(t, filepath.Join(project, ".vscode", "mcp.json"), `{"servers":{"vs-tool":{"command":"vs"}}}`)
	store := &StateStore{Path: filepath.Join(home, "state.json")}

	servers, problems := Discovery{Home: home, CWD: project}.Imported()
	if len(problems) != 0 {
		t.Fatal(problems)
	}
	state, _ := store.Load()
	byName := map[string]Server{}
	for _, server := range servers {
		byName[server.Name] = server
	}
	if state.NeedsApproval(byName["global"]) {
		t.Fatal("user-level Claude server is not project-controlled")
	}
	for _, name := range []string{"repo-db", "cursor-tool", "vs-tool"} {
		if !state.NeedsApproval(byName[name]) {
			t.Fatalf("%s comes from the project and must need approval", name)
		}
	}
	if err := store.Update(func(state *State) { state.Approve(byName["cursor-tool"]) }); err != nil {
		t.Fatal(err)
	}
	state, _ = store.Load()
	if state.NeedsApproval(byName["cursor-tool"]) {
		t.Fatal("approved server should start")
	}

	writeFile(t, filepath.Join(project, ".cursor", "mcp.json"), `{"mcpServers":{"cursor-tool":{"command":"evil"}}}`)
	servers, _ = Discovery{Home: home, CWD: project}.Imported()
	for _, server := range servers {
		if server.Name == "cursor-tool" && !state.NeedsApproval(server) {
			t.Fatal("a changed command must be approved again")
		}
	}
}

func TestOriginOfNormalizesDefaultPorts(t *testing.T) {
	if originOf("https://MCP.example.com:443/a") != originOf("https://mcp.example.com/b") {
		t.Fatal("explicit default port should be the same origin")
	}
	if originOf("http://h:80") != originOf("http://h") || originOf("https://h:8443") == originOf("https://h") {
		t.Fatal("port normalization is wrong")
	}
}

func TestLoadDoesNotReuseGlobalTrustForProjectServer(t *testing.T) {
	home, project := t.TempDir(), t.TempDir()
	claude := filepath.Join(home, ".claude.json")
	writeFile(t, claude, `{"mcpServers":{"foo":{"command":"foo-not-installed"}}}`)
	store := &StateStore{Path: filepath.Join(home, ".rootly-cli", "mcp-state.json")}
	manager := NewManager(ManagerOptions{CWD: project, Store: &Store{UserPath: filepath.Join(home, ".rootly-cli", "mcp.json"), CWD: project}, State: store, Discovery: &Discovery{Home: home, CWD: project}})
	defer manager.Close()
	_ = manager.Load()
	if err := manager.Enable(manager.Views()[0].Server.ID()); err != nil {
		t.Fatal(err)
	}
	if status := manager.Views()[0].Status; status == StatusPending || status == StatusOff {
		t.Fatalf("enabled global server should start, got %s", status)
	}

	writeFile(t, claude, `{"projects":{"`+project+`":{"mcpServers":{"foo":{"command":"foo-not-installed"}}}}}`)
	_ = manager.Load()
	view := manager.Views()[0]
	if view.Server.Project == "" || view.Status != StatusPending {
		t.Fatalf("same-named project server must need approval, got project=%q status=%s", view.Server.Project, view.Status)
	}
}
