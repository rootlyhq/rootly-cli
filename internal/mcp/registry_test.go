package mcp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const registryPage1 = `{"servers":[
 {"server":{"name":"io.github.acme/pager-mcp","description":"Pager","packages":[{"registryType":"npm","identifier":"@acme/pager-mcp","transport":{"type":"stdio"}}]},"_meta":{"io.modelcontextprotocol.registry/official":{"isLatest":false}}},
 {"server":{"name":"io.github.acme/pager-mcp","description":"Pager latest","packages":[{"registryType":"npm","identifier":"@acme/pager-mcp","transport":{"type":"stdio"}}]},"_meta":{"io.modelcontextprotocol.registry/official":{"isLatest":true}}},
 {"server":{"name":"com.example/py-tool","description":"Py","packages":[{"registryType":"pypi","identifier":"py-tool","transport":{"type":"stdio"}}]},"_meta":{"io.modelcontextprotocol.registry/official":{"isLatest":true}}}
],"metadata":{"nextCursor":"c2"}}`

const registryPage2 = `{"servers":[
 {"server":{"name":"io.github.box/oci-tool","repository":{"url":"https://github.com/boxco/oci"},"packages":[{"registryType":"oci","identifier":"ghcr.io/box/oci:1","transport":{"type":"stdio"}}]},"_meta":{"io.modelcontextprotocol.registry/official":{"isLatest":true}}},
 {"server":{"name":"dev.remote/events","remotes":[{"type":"sse","url":"https://events.example/sse"}]},"_meta":{"io.modelcontextprotocol.registry/official":{"isLatest":true}}},
 {"server":{"name":"dev.remote/api","remotes":[{"type":"streamable-http","url":"https://api.example/mcp"}]},"_meta":{"io.modelcontextprotocol.registry/official":{"isLatest":true}}},
 {"server":{"name":"dev.empty/nothing"},"_meta":{"io.modelcontextprotocol.registry/official":{"isLatest":true}}}
],"metadata":{}}`

func TestFetchRegistryPagesAndMapsInstalls(t *testing.T) {
	var queries []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		queries = append(queries, r.URL.RawQuery)
		if r.URL.Query().Get("cursor") == "c2" {
			_, _ = w.Write([]byte(registryPage2))
			return
		}
		_, _ = w.Write([]byte(registryPage1))
	}))
	defer server.Close()
	entries, err := FetchRegistry(context.Background(), server.Client(), server.URL+"/v0/servers", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(queries) != 2 || queries[0] != "limit=100" || queries[1] != "cursor=c2&limit=100" {
		t.Fatalf("queries = %v", queries)
	}
	got := map[string]string{}
	for _, entry := range entries {
		got[entry.Name] = InstallLabel(entry)
	}
	want := map[string]string{
		"pager-mcp": "npx -y @acme/pager-mcp",
		"py-tool":   "uvx py-tool",
		"oci-tool":  "docker run -i --rm ghcr.io/box/oci:1",
		"events":    "sse https://events.example/sse",
		"api":       "http https://api.example/mcp",
	}
	if len(got) != len(want) {
		t.Fatalf("entries = %v", got)
	}
	for name, label := range want {
		if got[name] != label {
			t.Fatalf("%s = %q, want %q", name, got[name], label)
		}
	}
	for _, entry := range entries {
		if entry.Name == "pager-mcp" && entry.Description != "Pager latest" {
			t.Fatalf("kept a non-latest version: %+v", entry)
		}
		if entry.Name == "oci-tool" && entry.Author != "boxco" {
			t.Fatalf("author = %q", entry.Author)
		}
	}
}

func TestCatalogPutsRootlyVendorsFirstAndSearchRanks(t *testing.T) {
	catalog := Catalog([]MarketEntry{{Name: "sentry", Description: "registry duplicate"}, {Name: "pager-mcp", Description: "Paging tool", Config: npxConfig("@acme/pager-mcp")}})
	var first []string
	for _, entry := range catalog[:4] {
		first = append(first, entry.Name)
	}
	if strings.Join(first, ",") != "datadog,sentry,linear,notion" {
		t.Fatalf("first = %v", first)
	}
	for _, entry := range catalog {
		if entry.Description == "registry duplicate" {
			t.Fatal("curated entry should win over the registry duplicate")
		}
	}
	if results := SearchMarket(catalog, "pager", 5); len(results) == 0 || results[0].Name != "pager-mcp" {
		t.Fatalf("search = %+v", results)
	}
	if results := SearchMarket(catalog, "", 3); len(results) != 3 || results[0].Name != "datadog" {
		t.Fatalf("empty search = %+v", results)
	}
	if results := SearchMarket(catalog, "zzzzqqq", 5); len(results) != 0 {
		t.Fatalf("no-match search = %+v", results)
	}
}

func TestIndexCacheRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mcp-index.json")
	now := time.Now().UTC().Truncate(time.Second)
	if err := SaveIndex(path, []MarketEntry{{Name: "x", Config: uvxConfig("x")}}, now); err != nil {
		t.Fatal(err)
	}
	entries, indexedAt, err := LoadIndex(path)
	if err != nil || len(entries) != 1 || !indexedAt.Equal(now) || entries[0].Config.Command != "uvx" {
		t.Fatalf("entries=%v at=%v err=%v", entries, indexedAt, err)
	}
}

func TestAddFromMarketWritesUserScopeSwitchedOff(t *testing.T) {
	home := t.TempDir()
	cwd := t.TempDir()
	store := &Store{UserPath: filepath.Join(home, "mcp.json"), CWD: cwd}
	states := &StateStore{Path: filepath.Join(home, "mcp-state.json")}
	entry := MarketEntry{Name: "pager-mcp", Config: npxConfig("@acme/pager-mcp")}
	server, err := AddFromMarket(store, states, entry)
	if err != nil {
		t.Fatal(err)
	}
	users, _ := store.Servers(ScopeUser)
	if len(users) != 1 || users[0].Name != "pager-mcp" || users[0].ID() != server.ID() {
		t.Fatalf("user servers = %+v", users)
	}
	if project, _ := store.Servers(ScopeProject); len(project) != 0 {
		t.Fatalf("marketplace add leaked into project scope: %+v", project)
	}
	state, _ := states.Load()
	if state.IsEnabled(users[0]) {
		t.Fatal("marketplace server must stay off until the user turns it on")
	}
	if _, err := AddFromMarket(store, states, entry); err == nil {
		t.Fatal("duplicate name should fail")
	}
}
