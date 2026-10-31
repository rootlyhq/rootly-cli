package mcp

import (
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
