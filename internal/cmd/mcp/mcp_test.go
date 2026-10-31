package mcp

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/pflag"

	localmcp "github.com/rootlyhq/rootly-cli/internal/mcp"
)

type testEnv struct {
	home    string
	project string
}

func setupEnv(t *testing.T) testEnv {
	t.Helper()
	home, project := t.TempDir(), t.TempDir()
	restore := SetEnvironment(Environment{
		CWD: func() (string, error) { return project, nil },
		Store: func(cwd string) *localmcp.Store {
			return &localmcp.Store{UserPath: filepath.Join(home, ".rootly-cli", "mcp.json"), CWD: cwd}
		},
		State: func() *localmcp.StateStore {
			return &localmcp.StateStore{Path: filepath.Join(home, ".rootly-cli", "mcp-state.json")}
		},
		Tokens: func() *localmcp.TokenStore {
			return &localmcp.TokenStore{Path: filepath.Join(home, ".rootly-cli", "mcp-oauth.json")}
		},
		Discovery: func(cwd string) localmcp.Discovery { return localmcp.Discovery{Home: home, CWD: cwd, GOOS: "linux"} },
		IsTTY:     func() bool { return false },
		Stdin:     strings.NewReader(""),
		HTTP:      http.DefaultClient,
		OpenURL:   func(string) error { return nil },
	})
	t.Cleanup(restore)
	return testEnv{home: home, project: project}
}

func resetFlags(flags *pflag.FlagSet) {
	flags.VisitAll(func(flag *pflag.Flag) {
		if slice, ok := flag.Value.(pflag.SliceValue); ok {
			_ = slice.Replace(nil)
		} else {
			_ = flag.Value.Set(flag.DefValue)
		}
		flag.Changed = false
	})
}

func run(t *testing.T, args ...string) (string, error) {
	t.Helper()
	for _, command := range MCPCmd.Commands() {
		resetFlags(command.Flags())
	}
	var out bytes.Buffer
	MCPCmd.SetOut(&out)
	MCPCmd.SetErr(&out)
	MCPCmd.SetArgs(args)
	err := MCPCmd.Execute()
	return out.String(), err
}

func readServers(t *testing.T, path string) map[string]any {
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

func TestAddScopesAndMaskedOutput(t *testing.T) {
	env := setupEnv(t)
	if out, err := run(t, "add", "-e", "TOKEN=secret-value", "fs", "--", "npx", "-y", "@modelcontextprotocol/server-filesystem", "/tmp"); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	if out, err := run(t, "add", "-s", "project", "maestro", "--", "maestro", "mcp"); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	if out, err := run(t, "add", "-s", "user", "-t", "sse", "-H", "Authorization: Bearer hidden", "events", "https://example.com/sse"); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	if out, err := run(t, "add", "-s", "user", "--url", "https://mcp.linear.app/mcp", "--bearer-token-env-var", "LINEAR_TOKEN", "linear"); err != nil {
		t.Fatalf("%v: %s", err, out)
	}

	user := readServers(t, filepath.Join(env.home, ".rootly-cli", "mcp.json"))
	local := user["projects"].(map[string]any)[env.project].(map[string]any)["mcpServers"].(map[string]any)["fs"].(map[string]any)
	if local["command"] != "npx" || len(local["args"].([]any)) != 3 {
		t.Fatalf("local fs = %v", local)
	}
	events := user["mcpServers"].(map[string]any)["events"].(map[string]any)
	if events["type"] != "sse" || events["headers"].(map[string]any)["Authorization"] != "Bearer hidden" {
		t.Fatalf("events = %v", events)
	}
	linear := user["mcpServers"].(map[string]any)["linear"].(map[string]any)
	if linear["type"] != "http" || linear["bearer_token_env_var"] != "LINEAR_TOKEN" {
		t.Fatalf("linear = %v", linear)
	}
	if readServers(t, filepath.Join(env.project, ".mcp.json"))["mcpServers"].(map[string]any)["maestro"] == nil {
		t.Fatal("project server missing")
	}

	list, err := run(t, "list")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"fs", "maestro", "events", "linear", "pending approval"} {
		if !strings.Contains(list, want) {
			t.Errorf("list missing %q:\n%s", want, list)
		}
	}
	get, err := run(t, "get", "events")
	if err != nil {
		t.Fatal(err)
	}
	all := list + get
	if fsGet, err := run(t, "get", "fs"); err == nil {
		all += fsGet
	}
	if strings.Contains(all, "secret-value") || strings.Contains(all, "hidden") {
		t.Fatalf("output leaks a secret:\n%s", all)
	}
	if !strings.Contains(all, localmcp.Mask) {
		t.Fatalf("expected masked values:\n%s", all)
	}

	if _, err := run(t, "remove", "fs"); err != nil {
		t.Fatal(err)
	}
	if _, err := run(t, "get", "fs"); err == nil {
		t.Fatal("removed server still found")
	}
}

func TestAddRejectsBadInput(t *testing.T) {
	setupEnv(t)
	cases := [][]string{
		{"add", "-H", "X: y", "fs", "--", "npx"},
		{"add", "-t", "ws", "x", "https://x"},
		{"add", "-s", "galaxy", "x", "--", "npx"},
		{"add", "-e", "NOEQUALS", "x", "--", "npx"},
		{"add", "bad name!", "--", "npx"},
	}
	for _, args := range cases {
		if _, err := run(t, args...); err == nil {
			t.Errorf("%v should fail", args)
		}
	}
}

func TestAddHTTPNeedsAuthentication(t *testing.T) {
	setupEnv(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("WWW-Authenticate", `Bearer resource_metadata="https://example.com/.well-known/oauth-protected-resource"`)
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()
	out, err := run(t, "add", "--transport", "http", "braintrust", server.URL+"/mcp")
	if err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	if !strings.Contains(out, "needs authentication") || !strings.Contains(out, "rootly mcp login braintrust") {
		t.Fatalf("out = %s", out)
	}
}

func TestAddJSONAndImport(t *testing.T) {
	env := setupEnv(t)
	if out, err := run(t, "add-json", "-s", "user", "everything", `{"command":"npx","args":["-y","@modelcontextprotocol/server-everything"]}`); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	if _, err := run(t, "add-json", "broken", `{"type":"http"}`); err == nil {
		t.Fatal("invalid json config should fail")
	}
	if err := os.MkdirAll(filepath.Join(env.project, ".vscode"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(env.project, ".vscode", "mcp.json"), []byte(`{"servers":{"github":{"type":"http","url":"https://api.githubcopilot.com/mcp/"}}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := run(t, "import"); err == nil {
		t.Fatal("import without a selection and no TTY should fail")
	}
	out, err := run(t, "import", "github")
	if err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	servers := readServers(t, filepath.Join(env.home, ".rootly-cli", "mcp.json"))["mcpServers"].(map[string]any)
	if servers["github"] == nil || servers["everything"] == nil {
		t.Fatalf("servers = %v", servers)
	}
	if _, err := run(t, "remove", "-s", "imported", "github"); err == nil {
		t.Fatal("imported servers are read-only")
	}
}
