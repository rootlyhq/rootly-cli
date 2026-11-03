package mcp

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	localmcp "github.com/rootlyhq/rootly-cli/internal/mcp"
)

func discoveredServers(names ...string) []localmcp.Server {
	servers := make([]localmcp.Server, len(names))
	for index, name := range names {
		servers[index] = localmcp.Server{Name: name, Config: localmcp.ServerConfig{Command: "npx"}, SourceLabel: "Claude Code"}
	}
	return servers
}

func TestParseSelection(t *testing.T) {
	discovered := discoveredServers("a", "b", "c")
	cases := map[string][]string{
		"":        nil,
		"all":     {"a", "b", "c"},
		"ALL":     {"a", "b", "c"},
		"*":       {"a", "b", "c"},
		"1,3":     {"a", "c"},
		"3 1 3":   {"c", "a"},
		" 2 , 2 ": {"b"},
	}
	for input, want := range cases {
		selected, err := parseSelection(strings.TrimSpace(input), discovered)
		if err != nil {
			t.Fatalf("parseSelection(%q): %v", input, err)
		}
		var got []string
		for _, server := range selected {
			got = append(got, server.Name)
		}
		if strings.Join(got, ",") != strings.Join(want, ",") {
			t.Errorf("parseSelection(%q) = %v, want %v", input, got, want)
		}
	}
	for _, input := range []string{"0", "4", "x", "1,-2"} {
		if _, err := parseSelection(input, discovered); err == nil || !strings.Contains(err.Error(), "between 1 and 3") {
			t.Errorf("parseSelection(%q) error = %v", input, err)
		}
	}
}

func TestSelectImportsByNameAndWithoutTerminal(t *testing.T) {
	discovered := discoveredServers("github", "sentry", "linear")
	importAll = false
	selected, err := selectImports(importCmd, discovered, []string{"linear", "github"})
	if err != nil || len(selected) != 2 || selected[0].Name != "github" || selected[1].Name != "linear" {
		t.Fatalf("named selection = %+v, %v", selected, err)
	}
	if _, err := selectImports(importCmd, discovered, []string{"zeta", "github", "alpha"}); err == nil ||
		err.Error() != "not found in other tools' configs: alpha, zeta" {
		t.Fatalf("missing names error = %v", err)
	}
	importAll = true
	t.Cleanup(func() { importAll = false })
	if selected, err := selectImports(importCmd, discovered, nil); err != nil || len(selected) != 3 {
		t.Fatalf("--all = %+v, %v", selected, err)
	}
	importAll = false
	if _, err := selectImports(importCmd, discovered, nil); err == nil || !strings.Contains(err.Error(), "--all when not running in a terminal") {
		t.Fatalf("non-terminal picker error = %v", err)
	}
}

func TestDescribeStateShowsScopeAndActivation(t *testing.T) {
	setupEnv(t)
	state, err := env.State().Load()
	if err != nil {
		t.Fatal(err)
	}
	tokens := env.Tokens()
	imported := localmcp.Server{Name: "gh", Scope: localmcp.ScopeImported, SourceLabel: "Claude Code", Config: localmcp.ServerConfig{Command: "npx"}}
	if got := describeState(state, tokens, imported); !strings.Contains(got, "imported from Claude Code, read-only") || !strings.Contains(got, "off") {
		t.Errorf("imported server state = %q", got)
	}
	remote := localmcp.Server{Name: "remote", Scope: localmcp.ScopeUser, Config: localmcp.ServerConfig{Type: localmcp.TransportHTTP, URL: "https://mcp.example.com"}}
	got := describeState(state, tokens, remote)
	if got != "user" {
		t.Errorf("remote user server state = %q", got)
	}
}

func TestAddRejectsConflictingTransportFlags(t *testing.T) {
	setupEnv(t)
	cases := []struct {
		args []string
		want string
	}{
		{[]string{"add", "x", "--url", "https://mcp.example.com", "https://other.example.com"}, "--url takes the URL"},
		{[]string{"add", "x", "-H", "Authorization: Bearer y", "--", "npx", "server"}, "only apply to http and sse"},
		{[]string{"add", "x", "--transport", "sse", "a", "b"}, "sse servers take exactly one URL"},
		{[]string{"add", "x", "--transport", "http", "ftp://mcp.example.com"}, `invalid URL "ftp://mcp.example.com"`},
		{[]string{"add", "x", "-e", "TOKEN=1", "https://mcp.example.com"}, "-e/--env only applies to stdio"},
		{[]string{"add", "x", "--transport", "grpc", "y"}, `invalid transport "grpc"`},
		{[]string{"add", "x", "-e", "NOVALUE", "--", "npx"}, "KEY=value"},
	}
	for _, tc := range cases {
		if _, err := run(t, tc.args...); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("rootly mcp %v error = %v, want %q", tc.args, err, tc.want)
		}
	}
}

func TestAddHTTPWithCredentialsSkipsAuthenticationHint(t *testing.T) {
	setupEnv(t)
	out, err := run(t, "add", "x", "--url", "https://mcp.example.com", "-H", "Authorization: Bearer secret")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "needs authentication") || strings.Contains(out, "secret") {
		t.Fatalf("output = %q", out)
	}
	out, err = run(t, "add", "y", "--url", "https://mcp.example.com", "--bearer-token-env-var", "MCP_TOKEN")
	if err != nil || strings.Contains(out, "needs authentication") {
		t.Fatalf("bearer env var add = %q, %v", out, err)
	}
	if got := maskedValue("Authorization: Bearer secret", ":"); got != "Authorization:"+localmcp.Mask {
		t.Errorf("maskedValue = %q", got)
	}
	if got := maskedValue("no-separator", ":"); got != "no-separator" {
		t.Errorf("maskedValue without separator = %q", got)
	}
}

func TestCommandsFailWhenWorkingDirIsGone(t *testing.T) {
	setupEnv(t)
	previous := env.CWD
	env.CWD = func() (string, error) { return "", errors.New("cwd removed") }
	t.Cleanup(func() { env.CWD = previous })
	if _, err := run(t, "add", "x", "--", "npx", "server"); err == nil || !strings.Contains(err.Error(), "cwd removed") {
		t.Fatalf("add with no cwd = %v", err)
	}
}

func TestDefaultEnvironmentUsesRealStores(t *testing.T) {
	defaults := DefaultEnvironment()
	if defaults.CWD == nil || defaults.Store == nil || defaults.State == nil || defaults.Tokens == nil || defaults.HTTP == nil || defaults.IsTTY == nil {
		t.Fatalf("default environment has nil members: %+v", defaults)
	}
	if defaults.HTTP.Timeout != 5*time.Second {
		t.Errorf("default HTTP timeout = %v", defaults.HTTP.Timeout)
	}
	home, _ := os.UserHomeDir()
	if got := displayPath(filepath.Join(home, ".rootly-cli", "mcp.json")); got != filepath.Join("~", ".rootly-cli", "mcp.json") {
		t.Errorf("displayPath(home file) = %q", got)
	}
	if got := displayPath("/srv/project/.mcp.json"); got != "/srv/project/.mcp.json" {
		t.Errorf("displayPath(outside home) = %q", got)
	}
}
