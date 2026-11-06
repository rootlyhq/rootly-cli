package mcp

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSearchShowsExactInstallAndUserScopeAddLine(t *testing.T) {
	test := setupEnv(t)
	hits := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits++
		_, _ = w.Write([]byte(`{"servers":[{"server":{"name":"io.github.acme/pager-mcp","description":"Pager tool","packages":[{"registryType":"npm","identifier":"@acme/pager-mcp","transport":{"type":"stdio"}}]},"_meta":{"io.modelcontextprotocol.registry/official":{"isLatest":true}}}],"metadata":{}}`))
	}))
	defer server.Close()
	previousURL, previousPath := registryURL, indexPath
	cache := filepath.Join(test.home, ".rootly-cli", "mcp-index.json")
	registryURL, indexPath = server.URL, func() string { return cache }
	t.Cleanup(func() { registryURL, indexPath = previousURL, previousPath })

	out, err := run(t, "search", "pager")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"pager-mcp (acme)", "runs: npx -y @acme/pager-mcp", "add:  rootly mcp add -s user pager-mcp -- npx -y @acme/pager-mcp"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in:\n%s", want, out)
		}
	}
	if _, err := os.Stat(cache); err != nil {
		t.Fatalf("index not cached: %v", err)
	}
	if _, err := run(t, "search", "sentry"); err != nil || hits != 1 {
		t.Fatalf("second search should use the cache: hits=%d err=%v", hits, err)
	}
	out, _ = run(t, "search", "sentry")
	if !strings.Contains(out, "runs: http https://mcp.sentry.dev/mcp") {
		t.Fatalf("curated entry missing:\n%s", out)
	}
	if _, err := os.Stat(filepath.Join(test.home, ".rootly-cli", "mcp.json")); !os.IsNotExist(err) {
		t.Fatal("search must not install anything")
	}
}

func TestSearchFallsBackToCuratedWhenRegistryFails(t *testing.T) {
	test := setupEnv(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusBadGateway) }))
	defer server.Close()
	previousURL, previousPath := registryURL, indexPath
	registryURL, indexPath = server.URL, func() string { return filepath.Join(test.home, "idx.json") }
	t.Cleanup(func() { registryURL, indexPath = previousURL, previousPath })
	out, err := run(t, "search", "datadog")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "warning: could not refresh the MCP registry index") || !strings.Contains(out, "datadog (datadog)") {
		t.Fatalf("out:\n%s", out)
	}
}

func TestSearchStripsTerminalControls(t *testing.T) {
	if got := oneLine("evil\x1b]0;pwned\x07 \x1b[2Jtext\u202e\r\nmore"); got != "evil text more" {
		t.Fatalf("oneLine = %q", got)
	}
}

func TestSearchKeepsLongAddCommandsWhole(t *testing.T) {
	args := strings.Repeat("--flag-value ", 20)
	if got := plainText("rootly mcp add -s user x -- npx " + args); !strings.HasSuffix(got, args) {
		t.Fatalf("add line truncated: %q", got)
	}
}
