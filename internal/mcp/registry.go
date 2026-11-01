package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/rootlyhq/rootly-cli/internal/config"
)

const (
	RegistryURL        = "https://registry.modelcontextprotocol.io/v0/servers"
	indexFile          = "mcp-index.json"
	registryPageLimit  = 100
	registryMaxPages   = 200
	IndexMaxAge        = 7 * 24 * time.Hour
	officialMetaKey    = "io.modelcontextprotocol.registry/official"
	maxMarketNameBytes = 40
)

// MarketEntry is one installable server from the curated list or the official registry.
type MarketEntry struct {
	Name        string       `json:"name"`
	Title       string       `json:"title,omitempty"`
	Description string       `json:"description,omitempty"`
	Author      string       `json:"author,omitempty"`
	EnvHint     string       `json:"env_hint,omitempty"`
	Rootly      bool         `json:"rootly,omitempty"`
	Config      ServerConfig `json:"config"`
}

type marketIndex struct {
	IndexedAt time.Time     `json:"indexed_at"`
	Entries   []MarketEntry `json:"entries"`
}

func npxConfig(pkg string, args ...string) ServerConfig {
	return ServerConfig{Command: "npx", Args: append([]string{"-y", pkg}, args...)}
}

func uvxConfig(pkg string) ServerConfig { return ServerConfig{Command: "uvx", Args: []string{pkg}} }

// CuratedEntries lists vendors Rootly integrates with first, then common servers.
func CuratedEntries() []MarketEntry {
	return []MarketEntry{
		{Name: "datadog", Author: "datadog", Rootly: true, Description: "Datadog: monitors, metrics, logs (remote, OAuth).", Config: ServerConfig{Type: TransportHTTP, URL: "https://mcp.datadoghq.com/api/unstable/mcp-server/mcp"}},
		{Name: "sentry", Author: "sentry", Rootly: true, Description: "Sentry: issues and events (remote, OAuth).", Config: ServerConfig{Type: TransportHTTP, URL: "https://mcp.sentry.dev/mcp"}},
		{Name: "linear", Author: "linear", Rootly: true, Description: "Linear: issues and projects (remote, OAuth).", Config: ServerConfig{Type: TransportHTTP, URL: "https://mcp.linear.app/mcp"}},
		{Name: "notion", Author: "notion", Rootly: true, Description: "Notion: pages and databases (remote, OAuth).", Config: ServerConfig{Type: TransportHTTP, URL: "https://mcp.notion.com/mcp"}},
		{Name: "github", Author: "modelcontextprotocol", Description: "GitHub: repos, issues, PRs, code search.", EnvHint: "GITHUB_PERSONAL_ACCESS_TOKEN", Config: npxConfig("@modelcontextprotocol/server-github")},
		{Name: "gitlab", Author: "modelcontextprotocol", Description: "GitLab: projects, issues, MRs.", EnvHint: "GITLAB_PERSONAL_ACCESS_TOKEN", Config: npxConfig("@modelcontextprotocol/server-gitlab")},
		{Name: "filesystem", Author: "modelcontextprotocol", Description: "Local filesystem access, scoped to a path.", Config: npxConfig("@modelcontextprotocol/server-filesystem", "${PWD}")},
		{Name: "postgres", Author: "modelcontextprotocol", Description: "Query a PostgreSQL database (read-only).", EnvHint: "DATABASE_URL", Config: npxConfig("@modelcontextprotocol/server-postgres", "${DATABASE_URL}")},
		{Name: "slack", Author: "modelcontextprotocol", Description: "Slack: channels, messages, users.", EnvHint: "SLACK_BOT_TOKEN", Config: npxConfig("@modelcontextprotocol/server-slack")},
		{Name: "memory", Author: "modelcontextprotocol", Description: "Persistent knowledge-graph memory.", Config: npxConfig("@modelcontextprotocol/server-memory")},
		{Name: "fetch", Author: "modelcontextprotocol", Description: "Fetch web pages as markdown.", Config: uvxConfig("mcp-server-fetch")},
		{Name: "aws-api", Author: "awslabs", Description: "AWS: call AWS APIs with your local credentials.", EnvHint: "AWS_PROFILE", Config: uvxConfig("awslabs.aws-api-mcp-server@latest")},
		{Name: "aws-docs", Author: "awslabs", Description: "AWS documentation: search and read.", Config: uvxConfig("awslabs.aws-documentation-mcp-server@latest")},
		{Name: "aws-cloudwatch", Author: "awslabs", Description: "AWS CloudWatch: metrics, logs, alarms.", EnvHint: "AWS_PROFILE", Config: uvxConfig("awslabs.cloudwatch-mcp-server@latest")},
	}
}

type registryPage struct {
	Servers []struct {
		Server registryServer             `json:"server"`
		Meta   map[string]json.RawMessage `json:"_meta"`
	} `json:"servers"`
	Metadata struct {
		NextCursor string `json:"nextCursor"`
	} `json:"metadata"`
}

type registryServer struct {
	Name        string `json:"name"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Repository  struct {
		URL string `json:"url"`
	} `json:"repository"`
	Packages []struct {
		RegistryType string `json:"registryType"`
		Identifier   string `json:"identifier"`
		Transport    struct {
			Type string `json:"type"`
		} `json:"transport"`
	} `json:"packages"`
	Remotes []struct {
		Type string `json:"type"`
		URL  string `json:"url"`
	} `json:"remotes"`
}

var nonSlug = regexp.MustCompile(`[^a-z0-9]+`)

func marketName(registryName string) string {
	last := registryName[strings.LastIndex(registryName, "/")+1:]
	name := strings.Trim(nonSlug.ReplaceAllString(strings.ToLower(last), "-"), "-")
	if len(name) > maxMarketNameBytes {
		name = strings.Trim(name[:maxMarketNameBytes], "-")
	}
	return name
}

var githubOwner = regexp.MustCompile(`(?i)github\.com/([^/]+)`)

func registryAuthor(server registryServer) string {
	if match := githubOwner.FindStringSubmatch(server.Repository.URL); match != nil {
		return match[1]
	}
	parts := strings.Split(strings.SplitN(server.Name, "/", 2)[0], ".")
	if len(parts) > 2 && parts[0] == "io" && parts[1] == "github" {
		return parts[2]
	}
	if len(parts) > 1 {
		return parts[1]
	}
	return parts[0]
}

// installFromRegistry maps npm→npx -y, pypi→uvx, oci→docker run -i --rm, else the first remote URL.
func installFromRegistry(server registryServer) (ServerConfig, bool) {
	for _, pkg := range server.Packages {
		if pkg.Identifier == "" || (pkg.Transport.Type != "" && pkg.Transport.Type != "stdio") {
			continue
		}
		switch pkg.RegistryType {
		case "npm":
			return npxConfig(pkg.Identifier), true
		case "pypi":
			return uvxConfig(pkg.Identifier), true
		case "oci":
			return ServerConfig{Command: "docker", Args: []string{"run", "-i", "--rm", pkg.Identifier}}, true
		}
	}
	for _, remote := range server.Remotes {
		if remote.URL == "" {
			continue
		}
		if remote.Type == "sse" {
			return ServerConfig{Type: TransportSSE, URL: remote.URL}, true
		}
		return ServerConfig{Type: TransportHTTP, URL: remote.URL}, true
	}
	return ServerConfig{}, false
}

func isLatest(meta map[string]json.RawMessage) bool {
	raw, ok := meta[officialMetaKey]
	if !ok {
		return true
	}
	var official struct {
		IsLatest bool `json:"isLatest"`
	}
	return json.Unmarshal(raw, &official) == nil && official.IsLatest
}

// FetchRegistry pages through the official registry and keeps the latest version of each installable server.
func FetchRegistry(ctx context.Context, client *http.Client, baseURL string, progress func(entries, pages int)) ([]MarketEntry, error) {
	if client == nil {
		client = &http.Client{Timeout: 20 * time.Second}
	}
	var entries []MarketEntry
	seen := map[string]bool{}
	cursor := ""
	for page := 1; page <= registryMaxPages; page++ {
		next, err := fetchRegistryPage(ctx, client, baseURL, cursor)
		if err != nil {
			if len(entries) > 0 {
				return entries, nil
			}
			return nil, err
		}
		for _, item := range next.Servers {
			if item.Server.Name == "" || !isLatest(item.Meta) || seen[item.Server.Name] {
				continue
			}
			install, ok := installFromRegistry(item.Server)
			name := marketName(item.Server.Name)
			if !ok || name == "" || install.Validate() != nil {
				continue
			}
			seen[item.Server.Name] = true
			entries = append(entries, MarketEntry{Name: name, Title: item.Server.Title, Description: item.Server.Description, Author: registryAuthor(item.Server), Config: install})
		}
		if progress != nil {
			progress(len(entries), page)
		}
		if next.Metadata.NextCursor == "" {
			break
		}
		cursor = next.Metadata.NextCursor
	}
	return entries, nil
}

func fetchRegistryPage(ctx context.Context, client *http.Client, baseURL, cursor string) (*registryPage, error) {
	endpoint, err := url.Parse(baseURL)
	if err != nil {
		return nil, err
	}
	query := endpoint.Query()
	query.Set("limit", fmt.Sprint(registryPageLimit))
	if cursor != "" {
		query.Set("cursor", cursor)
	}
	endpoint.RawQuery = query.Encode()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), http.NoBody)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", "application/json")
	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("registry returned HTTP %d", response.StatusCode)
	}
	var page registryPage
	if err := json.NewDecoder(response.Body).Decode(&page); err != nil {
		return nil, fmt.Errorf("decode registry page: %w", err)
	}
	return &page, nil
}

func IndexPath() string { return filepath.Join(config.Dir(), indexFile) }

func SaveIndex(path string, entries []MarketEntry, now time.Time) error {
	data, err := json.Marshal(marketIndex{IndexedAt: now, Entries: entries})
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

func LoadIndex(path string) (entries []MarketEntry, indexedAt time.Time, err error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, time.Time{}, err
	}
	var index marketIndex
	if err := json.Unmarshal(data, &index); err != nil {
		return nil, time.Time{}, err
	}
	return index.Entries, index.IndexedAt, nil
}

// Catalog merges curated entries (first) with the registry index; the first entry per name wins.
func Catalog(indexed []MarketEntry) []MarketEntry {
	seen := map[string]bool{}
	var entries []MarketEntry
	for _, group := range [][]MarketEntry{CuratedEntries(), indexed} {
		for _, entry := range group {
			if entry.Name == "" || seen[entry.Name] {
				continue
			}
			seen[entry.Name] = true
			entries = append(entries, entry)
		}
	}
	return entries
}

func fuzzyScore(query, text string) int {
	text = strings.ToLower(text)
	if index := strings.Index(text, query); index >= 0 {
		return 100 - min(index, 99)
	}
	matched := 0
	for _, char := range text {
		if matched < len(query) && byte(char) == query[matched] {
			matched++
		}
	}
	if matched == len(query) {
		return 1
	}
	return -1
}

// SearchMarket ranks entries by name, then title and description; ties keep catalog order.
func SearchMarket(entries []MarketEntry, query string, limit int) []MarketEntry {
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return entries[:min(limit, len(entries))]
	}
	type scored struct {
		entry MarketEntry
		score int
	}
	var matches []scored
	for _, entry := range entries {
		score := max(fuzzyScore(query, entry.Name)*2, fuzzyScore(query, entry.Title+" "+entry.Description))
		if score >= 0 {
			matches = append(matches, scored{entry, score})
		}
	}
	sort.SliceStable(matches, func(i, j int) bool { return matches[i].score > matches[j].score })
	results := make([]MarketEntry, 0, min(limit, len(matches)))
	for _, match := range matches[:min(limit, len(matches))] {
		results = append(results, match.entry)
	}
	return results
}

// InstallLabel is the exact command or URL that adding the entry will run or contact.
func InstallLabel(entry MarketEntry) string {
	if entry.Config.URL != "" {
		return entry.Config.Type + " " + entry.Config.URL
	}
	return strings.TrimSpace(entry.Config.Command + " " + strings.Join(entry.Config.Args, " "))
}

// AddFromMarket writes the entry to USER scope switched off, so nothing runs until the user turns it on.
func AddFromMarket(store *Store, states *StateStore, entry MarketEntry) (Server, error) {
	if err := ValidateServerName(entry.Name); err != nil {
		return Server{}, err
	}
	if err := entry.Config.Validate(); err != nil {
		return Server{}, err
	}
	existing, err := store.All()
	if err != nil {
		return Server{}, err
	}
	for _, server := range existing {
		if server.Name == entry.Name {
			return Server{}, fmt.Errorf("an MCP server named %s already exists (%s scope)", entry.Name, server.Scope)
		}
	}
	server := Server{Name: entry.Name, Scope: ScopeUser, Path: store.UserPath, Config: entry.Config}
	if err := states.Update(func(state *State) { state.SetEnabled(server, false) }); err != nil {
		return Server{}, err
	}
	if err := store.Add(ScopeUser, entry.Name, entry.Config); err != nil {
		return Server{}, errors.Join(err, states.Update(func(state *State) { state.SetEnabled(server, true) }))
	}
	return server, nil
}

// LoadCatalog uses the cached index, refetching it when missing, stale or forced; on fetch failure it falls back to the cache or the curated list.
func LoadCatalog(ctx context.Context, client *http.Client, registryURL, indexPath string, refresh bool, progress func(entries, pages int)) ([]MarketEntry, error) {
	cached, indexedAt, cacheErr := LoadIndex(indexPath)
	if !refresh && cacheErr == nil && time.Since(indexedAt) < IndexMaxAge {
		return Catalog(cached), nil
	}
	fetched, err := FetchRegistry(ctx, client, registryURL, progress)
	if err != nil {
		return Catalog(cached), fmt.Errorf("could not refresh the MCP registry index: %w", err)
	}
	if err := SaveIndex(indexPath, fetched, time.Now()); err != nil {
		return Catalog(fetched), fmt.Errorf("could not cache the MCP registry index: %w", err)
	}
	return Catalog(fetched), nil
}

// AddCommand is the equivalent `rootly mcp add` invocation for an entry.
func AddCommand(entry MarketEntry) string {
	if entry.Config.URL != "" {
		return fmt.Sprintf("rootly mcp add -s user -t %s %s %s", entry.Config.Type, entry.Name, entry.Config.URL)
	}
	return strings.TrimSpace(fmt.Sprintf("rootly mcp add -s user %s -- %s", entry.Name, InstallLabel(entry)))
}
