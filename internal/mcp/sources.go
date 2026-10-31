package mcp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"sort"

	toml "github.com/pelletier/go-toml/v2"
)

// ImportSource is a third-party MCP config rootly can read but never writes.
type ImportSource struct {
	Label string
	Path  string
	parse func([]byte, string) (map[string]ServerConfig, error)
}

type Discovery struct {
	Home string
	CWD  string
	GOOS string
}

func NewDiscovery(cwd string) Discovery {
	home, _ := os.UserHomeDir()
	return Discovery{Home: home, CWD: cwd, GOOS: runtime.GOOS}
}

func (d Discovery) Sources() []ImportSource {
	sources := []ImportSource{
		{Label: "Claude Code", Path: filepath.Join(d.Home, ".claude.json"), parse: parseClaudeCode},
		{Label: "Codex", Path: filepath.Join(d.Home, ".codex", "config.toml"), parse: parseCodexTOML},
		{Label: "Cursor", Path: filepath.Join(d.Home, ".cursor", "mcp.json"), parse: parseMCPServersKey},
		{Label: "Cursor (project)", Path: filepath.Join(d.CWD, ".cursor", "mcp.json"), parse: parseMCPServersKey},
		{Label: "VS Code (project)", Path: filepath.Join(d.CWD, ".vscode", "mcp.json"), parse: parseVSCode},
	}
	switch d.GOOS {
	case "darwin":
		sources = append(sources, ImportSource{Label: "Claude Desktop", Path: filepath.Join(d.Home, "Library", "Application Support", "Claude", "claude_desktop_config.json"), parse: parseMCPServersKey})
	case "linux":
		sources = append(sources, ImportSource{Label: "Claude Desktop", Path: filepath.Join(d.Home, ".config", "Claude", "claude_desktop_config.json"), parse: parseMCPServersKey})
	}
	return sources
}

type ImportError struct {
	Source ImportSource
	Err    error
}

// Imported lists servers from every readable third-party config. Missing
// files are skipped; malformed files are reported without failing the rest.
func (d Discovery) Imported() ([]Server, []ImportError) {
	var servers []Server
	var problems []ImportError
	for _, source := range d.Sources() {
		data, err := os.ReadFile(source.Path)
		if err != nil {
			if !os.IsNotExist(err) {
				problems = append(problems, ImportError{Source: source, Err: err})
			}
			continue
		}
		parsed, err := source.parse(data, d.CWD)
		if err != nil {
			problems = append(problems, ImportError{Source: source, Err: err})
			continue
		}
		servers = append(servers, sortedServers(parsed, ScopeImported, source.Path, source.Label)...)
	}
	return servers, problems
}

func parseMCPServersKey(data []byte, _ string) (map[string]ServerConfig, error) {
	document := jsonDocument{}
	if err := json.Unmarshal(data, &document); err != nil {
		return nil, err
	}
	return decodeServersLenient(document["mcpServers"]), nil
}

func parseClaudeCode(data []byte, cwd string) (map[string]ServerConfig, error) {
	var document struct {
		MCPServers json.RawMessage                       `json:"mcpServers"`
		Projects   map[string]map[string]json.RawMessage `json:"projects"`
	}
	if err := json.Unmarshal(data, &document); err != nil {
		return nil, err
	}
	servers := decodeServersLenient(document.MCPServers)
	for name, server := range decodeServersLenient(document.Projects[cwd]["mcpServers"]) {
		servers[name] = server
	}
	return servers, nil
}

func parseVSCode(data []byte, _ string) (map[string]ServerConfig, error) {
	document := jsonDocument{}
	if err := json.Unmarshal(data, &document); err != nil {
		return nil, err
	}
	return decodeServersLenient(document["servers"]), nil
}

type codexServer struct {
	Command           string            `toml:"command"`
	Args              []string          `toml:"args"`
	Env               map[string]string `toml:"env"`
	URL               string            `toml:"url"`
	BearerTokenEnvVar string            `toml:"bearer_token_env_var"`
	HTTPHeaders       map[string]string `toml:"http_headers"`
	EnvHTTPHeaders    map[string]string `toml:"env_http_headers"`
	Enabled           *bool             `toml:"enabled"`
}

func parseCodexTOML(data []byte, _ string) (map[string]ServerConfig, error) {
	var document struct {
		MCPServers map[string]codexServer `toml:"mcp_servers"`
	}
	if err := toml.Unmarshal(data, &document); err != nil {
		return nil, err
	}
	servers := make(map[string]ServerConfig, len(document.MCPServers))
	names := make([]string, 0, len(document.MCPServers))
	for name := range document.MCPServers {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		entry := document.MCPServers[name]
		if entry.Enabled != nil && !*entry.Enabled {
			continue
		}
		server := ServerConfig{Command: entry.Command, Args: entry.Args, Env: entry.Env}
		if entry.URL != "" {
			server = ServerConfig{Type: TransportHTTP, URL: entry.URL, BearerTokenEnvVar: entry.BearerTokenEnvVar}
			headers := map[string]string{}
			for key, value := range entry.HTTPHeaders {
				headers[key] = value
			}
			for key, variable := range entry.EnvHTTPHeaders {
				headers[key] = "${" + variable + "}"
			}
			if len(headers) > 0 {
				server.Headers = headers
			}
		}
		servers[name] = server
	}
	return servers, nil
}
