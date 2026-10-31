package mcp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func readJSON(t *testing.T, path string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]any
	if err := json.Unmarshal(data, &document); err != nil {
		t.Fatal(err)
	}
	return document
}

func TestStoreScopesWriteClaudeShapes(t *testing.T) {
	home, project := t.TempDir(), t.TempDir()
	store := &Store{UserPath: filepath.Join(home, ".rootly-cli", "mcp.json"), CWD: project}
	writeFile(t, filepath.Join(project, ".mcp.json"), `{"mcpServers":{"maestro":{"command":"maestro","args":["mcp"]}},"other":{"keep":true}}`)

	if err := store.Add(ScopeLocal, "fs", ServerConfig{Command: "npx", Args: []string{"-y", "server"}, Env: map[string]string{"TOKEN": "secret"}}); err != nil {
		t.Fatal(err)
	}
	if err := store.Add(ScopeUser, "braintrust", ServerConfig{Type: TransportHTTP, URL: "https://api.braintrust.dev/mcp"}); err != nil {
		t.Fatal(err)
	}
	if err := store.Add(ScopeProject, "dart", ServerConfig{Command: "fvm", Args: []string{"dart", "mcp-server"}}); err != nil {
		t.Fatal(err)
	}
	if err := store.Add(ScopeUser, "braintrust", ServerConfig{Type: TransportHTTP, URL: "https://x"}); err == nil {
		t.Fatal("duplicate name in the same scope should fail")
	}

	user := readJSON(t, store.UserPath)
	if user["mcpServers"].(map[string]any)["braintrust"].(map[string]any)["type"] != "http" {
		t.Fatalf("user scope: %v", user)
	}
	local := user["projects"].(map[string]any)[project].(map[string]any)["mcpServers"].(map[string]any)["fs"].(map[string]any)
	if local["command"] != "npx" || local["type"] != nil {
		t.Fatalf("local scope should be stdio without type: %v", local)
	}
	projectDocument := readJSON(t, filepath.Join(project, ".mcp.json"))
	if projectDocument["other"] == nil || projectDocument["mcpServers"].(map[string]any)["maestro"] == nil || projectDocument["mcpServers"].(map[string]any)["dart"] == nil {
		t.Fatalf("project write must preserve other keys and servers: %v", projectDocument)
	}
	if info, err := os.Stat(store.UserPath); err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("user config perms: %v %v", info, err)
	}

	all, err := store.All()
	if err != nil || len(all) != 4 {
		t.Fatalf("All() = %d servers, %v", len(all), err)
	}
	if err := store.Remove(ScopeProject, "maestro"); err != nil {
		t.Fatal(err)
	}
	if readJSON(t, filepath.Join(project, ".mcp.json"))["other"] == nil {
		t.Fatal("remove must preserve other keys")
	}
	if err := store.Remove(ScopeUser, "missing"); err == nil {
		t.Fatal("removing an unknown server should fail")
	}
}

func TestValidate(t *testing.T) {
	for _, config := range []ServerConfig{{}, {Type: "http"}, {Type: "ws", URL: "https://x"}, {Command: "x", URL: "https://x"}} {
		if config.Validate() == nil {
			t.Errorf("expected invalid: %+v", config)
		}
	}
	if err := (ServerConfig{Command: "npx"}).Validate(); err != nil {
		t.Error(err)
	}
}

func TestExpandEnv(t *testing.T) {
	lookup := func(name string) (string, bool) {
		value, ok := map[string]string{"TOKEN": "abc", "EMPTY": ""}[name]
		return value, ok
	}
	cases := map[string]string{
		"Bearer ${TOKEN}":          "Bearer abc",
		"${MISSING:-fallback}":     "fallback",
		"${EMPTY:-fallback}":       "fallback",
		"${MISSING}":               "",
		"$TOKEN stays":             "$TOKEN stays",
		"${TOKEN}/${MISSING:-x/y}": "abc/x/y",
	}
	for input, want := range cases {
		if got := ExpandEnv(input, lookup); got != want {
			t.Errorf("ExpandEnv(%q) = %q, want %q", input, got, want)
		}
	}
	config := ServerConfig{Command: "${TOKEN}", Args: []string{"--k=${TOKEN}"}, Env: map[string]string{"A": "${TOKEN}"}, URL: "https://${MISSING:-host}/mcp", Headers: map[string]string{"Authorization": "Bearer ${TOKEN}"}}
	expanded := config.Expanded(lookup)
	if expanded.Command != "abc" || expanded.Args[0] != "--k=abc" || expanded.Env["A"] != "abc" || expanded.URL != "https://host/mcp" || expanded.Headers["Authorization"] != "Bearer abc" {
		t.Fatalf("Expanded = %+v", expanded)
	}
	if config.Env["A"] != "${TOKEN}" {
		t.Fatal("Expanded must not mutate the original config")
	}
}

func TestMasking(t *testing.T) {
	config := ServerConfig{
		Command: "docker", Args: []string{"run", "--token=sk-live", "-e", "X"},
		Env: map[string]string{"SCALR_API_TOKEN": "secret"}, Headers: map[string]string{"Authorization": "Bearer secret"},
		URL: "https://user:pass@example.com/mcp?api_key=secret&x=1",
	}
	masked := config.Masked()
	text, _ := json.Marshal(masked)
	if strings.Contains(string(text), "secret") || strings.Contains(string(text), "pass@") || strings.Contains(string(text), "sk-live") {
		t.Fatalf("masked config leaks a secret: %s", text)
	}
	pairs := MaskedPairs(masked.Env, "=")
	if len(pairs) != 1 || pairs[0] != "SCALR_API_TOKEN="+Mask {
		t.Fatalf("pairs = %v", pairs)
	}
}

func TestClientToolNamer(t *testing.T) {
	namer := NewClientToolNamer()
	if got := namer.Name("My Files", "read-file"); got != "local_my_files_read_file" {
		t.Fatalf("got %q", got)
	}
	if got := namer.Name("my_files", "read file"); got != "local_my_files_read_file_2" {
		t.Fatalf("collision: got %q", got)
	}
	long := namer.Name(strings.Repeat("server", 10), strings.Repeat("tool", 10))
	if len(long) > len(ClientToolPrefix)+60 {
		t.Fatalf("name too long: %d", len(long))
	}
	if got := namer.Name("Ünïcode!!", "✓"); !strings.HasPrefix(got, ClientToolPrefix) {
		t.Fatalf("got %q", got)
	}
	for _, name := range []string{long, namer.Name("a", "b")} {
		for _, r := range strings.TrimPrefix(name, ClientToolPrefix) {
			if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '_' {
				t.Fatalf("invalid rune %q in %q", r, name)
			}
		}
	}
	again := namer.Name(strings.Repeat("server", 10), strings.Repeat("tool", 10))
	if again == long || len(again) > len(ClientToolPrefix)+60 {
		t.Fatalf("truncated collision: %q vs %q", again, long)
	}
}

func TestImportedSourcesRealShapes(t *testing.T) {
	home, project := t.TempDir(), t.TempDir()
	writeFile(t, filepath.Join(home, ".claude.json"), `{
  "numStartups": 3,
  "mcpServers": {"sentry": {"type": "http", "url": "https://mcp.sentry.dev/mcp"}},
  "projects": {"`+project+`": {"allowedTools": [], "mcpServers": {"local-db": {"type": "stdio", "command": "npx", "args": ["-y", "db"], "env": {"DB": "x"}}}}}
}`)
	writeFile(t, filepath.Join(home, ".codex", "config.toml"), `model = "gpt-5"
[mcp_servers.docs]
command = "npx"
args = ["-y", "docs-mcp"]
[mcp_servers.docs.env]
DOCS_TOKEN = "t"

[mcp_servers.linear]
url = "https://mcp.linear.app/mcp"
bearer_token_env_var = "LINEAR_TOKEN"
`)
	writeFile(t, filepath.Join(home, ".cursor", "mcp.json"), `{"mcpServers":{"cursor-user":{"command":"uvx","args":["x"]}}}`)
	writeFile(t, filepath.Join(project, ".cursor", "mcp.json"), `{"mcpServers":{"cursor-project":{"url":"https://example.com/sse","type":"sse"}}}`)
	writeFile(t, filepath.Join(project, ".vscode", "mcp.json"), `{"inputs":[],"servers":{"github":{"type":"http","url":"https://api.githubcopilot.com/mcp/"}}}`)
	desktop := filepath.Join(home, ".config", "Claude", "claude_desktop_config.json")
	if runtime.GOOS == "darwin" {
		desktop = filepath.Join(home, "Library", "Application Support", "Claude", "claude_desktop_config.json")
	}
	writeFile(t, desktop, `{"mcpServers":{"filesystem":{"command":"npx","args":["-y","@modelcontextprotocol/server-filesystem","/Users/me/Desktop"]}}}`)

	discovery := Discovery{Home: home, CWD: project, GOOS: runtime.GOOS}
	servers, problems := discovery.Imported()
	if len(problems) != 0 {
		t.Fatalf("problems: %+v", problems)
	}
	got := map[string]Server{}
	for _, server := range servers {
		got[server.Name] = server
		if server.Scope != ScopeImported || !server.ReadOnly() {
			t.Errorf("%s should be imported read-only", server.Name)
		}
	}
	for _, name := range []string{"sentry", "local-db", "docs", "linear", "cursor-user", "cursor-project", "github", "filesystem"} {
		if _, ok := got[name]; !ok {
			t.Errorf("missing imported server %s (got %v)", name, got)
		}
	}
	if got["linear"].Config.Transport() != TransportHTTP || got["linear"].Config.BearerTokenEnvVar != "LINEAR_TOKEN" {
		t.Errorf("codex url server: %+v", got["linear"].Config)
	}
	if got["docs"].Config.Env["DOCS_TOKEN"] != "t" || got["docs"].Config.Transport() != TransportStdio {
		t.Errorf("codex stdio server: %+v", got["docs"].Config)
	}
	if got["cursor-project"].Config.Transport() != TransportSSE || got["github"].Config.Transport() != TransportHTTP {
		t.Errorf("transports: %+v %+v", got["cursor-project"].Config, got["github"].Config)
	}

	writeFile(t, filepath.Join(home, ".cursor", "mcp.json"), `{not json`)
	if _, problems := discovery.Imported(); len(problems) != 1 {
		t.Fatalf("broken file should be reported once, got %+v", problems)
	}
}

func TestStateApprovalAndEnable(t *testing.T) {
	store := &StateStore{Path: filepath.Join(t.TempDir(), "mcp-state.json")}
	project := Server{Name: "maestro", Scope: ScopeProject, Path: "/repo/.mcp.json", Config: ServerConfig{Command: "maestro"}}
	imported := Server{Name: "sentry", Scope: ScopeImported, Path: "/home/.claude.json", Config: ServerConfig{Type: "http", URL: "https://x"}}
	user := Server{Name: "fs", Scope: ScopeUser, Config: ServerConfig{Command: "npx"}}

	state, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if !state.NeedsApproval(project) || state.NeedsApproval(user) {
		t.Fatal("only project servers need approval")
	}
	if state.IsEnabled(imported) || !state.IsEnabled(user) {
		t.Fatal("imported servers start off, others on")
	}
	if err := store.Update(func(state *State) { state.Approve(project); state.SetEnabled(imported, true) }); err != nil {
		t.Fatal(err)
	}
	state, _ = store.Load()
	if state.NeedsApproval(project) || !state.IsEnabled(imported) {
		t.Fatal("approval and enable should persist")
	}
	changed := project
	changed.Config.Args = []string{"--evil"}
	if !state.NeedsApproval(changed) {
		t.Fatal("a config change must require approval again")
	}
	if info, err := os.Stat(store.Path); err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("state perms: %v %v", info, err)
	}
}

func TestTruncateOutput(t *testing.T) {
	short := "ok"
	if TruncateOutput(short) != short {
		t.Fatal("short output changed")
	}
	long := strings.Repeat("é", MaxToolOutputBytes)
	got := TruncateOutput(long)
	if len(got) > MaxToolOutputBytes || !strings.Contains(got, "truncated") {
		t.Fatalf("len=%d", len(got))
	}
}

func TestTokenStorePerms(t *testing.T) {
	store := &TokenStore{Path: filepath.Join(t.TempDir(), "mcp-oauth.json")}
	if err := store.Put("https://api.braintrust.dev/mcp", StoredCredential{ClientID: "c", AccessToken: "a"}); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(store.Path); err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("token perms: %v %v", info, err)
	}
	if credential, ok := store.Get("https://api.braintrust.dev/mcp"); !ok || credential.AccessToken != "a" {
		t.Fatal("token not stored")
	}
	if removed, err := store.Delete("https://api.braintrust.dev/mcp"); err != nil || !removed {
		t.Fatal("token not deleted")
	}
}
