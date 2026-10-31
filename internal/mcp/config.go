package mcp

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/rootlyhq/rootly-cli/internal/config"
)

const (
	TransportStdio = "stdio"
	TransportHTTP  = "http"
	TransportSSE   = "sse"

	ScopeLocal    = "local"
	ScopeUser     = "user"
	ScopeProject  = "project"
	ScopeImported = "imported"

	userConfigFile    = "mcp.json"
	projectConfigFile = ".mcp.json"
)

var serverNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,63}$`)

type ServerConfig struct {
	Type              string            `json:"type,omitempty"`
	Command           string            `json:"command,omitempty"`
	Args              []string          `json:"args,omitempty"`
	Env               map[string]string `json:"env,omitempty"`
	URL               string            `json:"url,omitempty"`
	Headers           map[string]string `json:"headers,omitempty"`
	BearerTokenEnvVar string            `json:"bearer_token_env_var,omitempty"`
}

func (c ServerConfig) Transport() string {
	switch strings.ToLower(c.Type) {
	case TransportHTTP, "streamable-http", "streamable_http", "streamablehttp":
		return TransportHTTP
	case TransportSSE:
		return TransportSSE
	case "", TransportStdio:
		if c.Command == "" && c.URL != "" {
			return TransportHTTP
		}
		return TransportStdio
	default:
		return strings.ToLower(c.Type)
	}
}

func (c ServerConfig) Validate() error {
	switch c.Transport() {
	case TransportStdio:
		if strings.TrimSpace(c.Command) == "" {
			return errors.New("stdio server needs a command")
		}
	case TransportHTTP, TransportSSE:
		if strings.TrimSpace(c.URL) == "" {
			return fmt.Errorf("%s server needs a url", c.Transport())
		}
	default:
		return fmt.Errorf("unsupported transport %q (expected stdio, http or sse)", c.Type)
	}
	return nil
}

func (c ServerConfig) Hash() string {
	data, _ := json.Marshal(c)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

type Server struct {
	Name        string
	Config      ServerConfig
	Scope       string
	Path        string
	SourceLabel string
}

func (s Server) ID() string {
	return s.Scope + ":" + s.Path + ":" + s.Name
}

func (s Server) ReadOnly() bool {
	return s.Scope == ScopeImported
}

func ValidateServerName(name string) error {
	if !serverNamePattern.MatchString(name) {
		return fmt.Errorf("invalid server name %q: use letters, digits, '-', '_' or '.' (max 64)", name)
	}
	return nil
}

func UserConfigPath() string {
	return filepath.Join(config.Dir(), userConfigFile)
}

func ProjectConfigPath(cwd string) string {
	return filepath.Join(cwd, projectConfigFile)
}

type jsonDocument map[string]json.RawMessage

func readJSONDocument(path string) (jsonDocument, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return jsonDocument{}, nil
		}
		return nil, err
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		return jsonDocument{}, nil
	}
	document := jsonDocument{}
	if err := json.Unmarshal(data, &document); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return document, nil
}

func writeJSONDocument(path string, document jsonDocument, perm os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(document, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".*")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if info, statErr := os.Stat(path); statErr == nil {
		perm = info.Mode().Perm()
	}
	if err := os.Chmod(tmp.Name(), perm); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

func decodeServers(raw json.RawMessage) (map[string]ServerConfig, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return map[string]ServerConfig{}, nil
	}
	servers := map[string]ServerConfig{}
	if err := json.Unmarshal(raw, &servers); err != nil {
		return nil, err
	}
	return servers, nil
}

func decodeServersLenient(raw json.RawMessage) map[string]ServerConfig {
	if len(raw) == 0 {
		return map[string]ServerConfig{}
	}
	entries := map[string]json.RawMessage{}
	if json.Unmarshal(raw, &entries) != nil {
		return map[string]ServerConfig{}
	}
	servers := make(map[string]ServerConfig, len(entries))
	for name, entry := range entries {
		var server ServerConfig
		if json.Unmarshal(entry, &server) == nil {
			servers[name] = server
		}
	}
	return servers
}

func sortedServers(servers map[string]ServerConfig, scope, path, label string) []Server {
	names := make([]string, 0, len(servers))
	for name := range servers {
		names = append(names, name)
	}
	sort.Strings(names)
	result := make([]Server, 0, len(names))
	for _, name := range names {
		result = append(result, Server{Name: name, Config: servers[name], Scope: scope, Path: path, SourceLabel: label})
	}
	return result
}

// Store reads and writes the scopes rootly owns: user and local in
// ~/.rootly-cli/mcp.json, project in ./.mcp.json.
type Store struct {
	UserPath string
	CWD      string
}

func NewStore(cwd string) *Store {
	return &Store{UserPath: UserConfigPath(), CWD: cwd}
}

func (s *Store) ProjectPath() string {
	return ProjectConfigPath(s.CWD)
}

func (s *Store) Servers(scope string) ([]Server, error) {
	switch scope {
	case ScopeUser:
		document, err := readJSONDocument(s.UserPath)
		if err != nil {
			return nil, err
		}
		servers, err := decodeServers(document["mcpServers"])
		if err != nil {
			return nil, fmt.Errorf("parse %s mcpServers: %w", s.UserPath, err)
		}
		return sortedServers(servers, ScopeUser, s.UserPath, "rootly"), nil
	case ScopeLocal:
		document, err := readJSONDocument(s.UserPath)
		if err != nil {
			return nil, err
		}
		projects := map[string]jsonDocument{}
		if raw, ok := document["projects"]; ok {
			if err := json.Unmarshal(raw, &projects); err != nil {
				return nil, fmt.Errorf("parse %s projects: %w", s.UserPath, err)
			}
		}
		servers, err := decodeServers(projects[s.CWD]["mcpServers"])
		if err != nil {
			return nil, fmt.Errorf("parse %s projects[%q]: %w", s.UserPath, s.CWD, err)
		}
		return sortedServers(servers, ScopeLocal, s.UserPath, "rootly"), nil
	case ScopeProject:
		path := s.ProjectPath()
		document, err := readJSONDocument(path)
		if err != nil {
			return nil, err
		}
		servers, err := decodeServers(document["mcpServers"])
		if err != nil {
			return nil, fmt.Errorf("parse %s mcpServers: %w", path, err)
		}
		return sortedServers(servers, ScopeProject, path, "project"), nil
	default:
		return nil, fmt.Errorf("unknown scope %q (expected local, user or project)", scope)
	}
}

// All returns every writable-scope server, local first, then project, then user.
func (s *Store) All() ([]Server, error) {
	var all []Server
	var errs []error
	for _, scope := range []string{ScopeLocal, ScopeProject, ScopeUser} {
		servers, err := s.Servers(scope)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		all = append(all, servers...)
	}
	return all, errors.Join(errs...)
}

func (s *Store) Add(scope, name string, server ServerConfig) error {
	if err := ValidateServerName(name); err != nil {
		return err
	}
	if err := server.Validate(); err != nil {
		return err
	}
	return s.update(scope, func(servers map[string]ServerConfig) error {
		if _, exists := servers[name]; exists {
			return fmt.Errorf("MCP server %q already exists in %s config", name, scope)
		}
		servers[name] = server
		return nil
	})
}

func (s *Store) Remove(scope, name string) error {
	return s.update(scope, func(servers map[string]ServerConfig) error {
		if _, exists := servers[name]; !exists {
			return fmt.Errorf("no MCP server named %q in %s config", name, scope)
		}
		delete(servers, name)
		return nil
	})
}

func (s *Store) update(scope string, change func(map[string]ServerConfig) error) error {
	switch scope {
	case ScopeUser, ScopeProject:
		path := s.UserPath
		perm := os.FileMode(0o600)
		if scope == ScopeProject {
			path, perm = s.ProjectPath(), 0o644
		}
		document, err := readJSONDocument(path)
		if err != nil {
			return err
		}
		servers, err := decodeServers(document["mcpServers"])
		if err != nil {
			return fmt.Errorf("parse %s mcpServers: %w", path, err)
		}
		if err := change(servers); err != nil {
			return err
		}
		encoded, err := json.Marshal(servers)
		if err != nil {
			return err
		}
		document["mcpServers"] = encoded
		return writeJSONDocument(path, document, perm)
	case ScopeLocal:
		document, err := readJSONDocument(s.UserPath)
		if err != nil {
			return err
		}
		projects := map[string]jsonDocument{}
		if raw, ok := document["projects"]; ok {
			if err := json.Unmarshal(raw, &projects); err != nil {
				return fmt.Errorf("parse %s projects: %w", s.UserPath, err)
			}
		}
		project := projects[s.CWD]
		if project == nil {
			project = jsonDocument{}
		}
		servers, err := decodeServers(project["mcpServers"])
		if err != nil {
			return err
		}
		if err := change(servers); err != nil {
			return err
		}
		encoded, err := json.Marshal(servers)
		if err != nil {
			return err
		}
		project["mcpServers"] = encoded
		if len(servers) == 0 && len(project) == 1 {
			delete(projects, s.CWD)
		} else {
			projects[s.CWD] = project
		}
		encodedProjects, err := json.Marshal(projects)
		if err != nil {
			return err
		}
		document["projects"] = encodedProjects
		return writeJSONDocument(s.UserPath, document, 0o600)
	default:
		return fmt.Errorf("unknown scope %q (expected local, user or project)", scope)
	}
}
