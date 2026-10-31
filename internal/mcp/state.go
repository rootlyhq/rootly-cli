package mcp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"

	"github.com/rootlyhq/rootly-cli/internal/config"
)

const stateFile = "mcp-state.json"

type Approval struct {
	Project string `json:"project"`
	Server  string `json:"server"`
	Hash    string `json:"hash"`
}

// State is rootly-owned MCP state: which imported servers are enabled and
// which project .mcp.json servers were approved.
type State struct {
	EnabledImported []string   `json:"enabled_imported,omitempty"`
	Disabled        []string   `json:"disabled,omitempty"`
	Approvals       []Approval `json:"approvals,omitempty"`
}

type StateStore struct {
	Path string
	mu   sync.Mutex
}

func NewStateStore() *StateStore {
	return &StateStore{Path: filepath.Join(config.Dir(), stateFile)}
}

func (s *StateStore) Load() (State, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.load()
}

func (s *StateStore) load() (State, error) {
	var state State
	data, err := os.ReadFile(s.Path)
	if err != nil {
		if os.IsNotExist(err) {
			return state, nil
		}
		return state, err
	}
	if len(data) == 0 {
		return state, nil
	}
	err = json.Unmarshal(data, &state)
	return state, err
}

func (s *StateStore) Update(change func(*State)) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	state, err := s.load()
	if err != nil {
		return err
	}
	change(&state)
	document := jsonDocument{}
	encoded, err := json.Marshal(state)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(encoded, &document); err != nil {
		return err
	}
	return writeJSONDocument(s.Path, document, 0o600)
}

func (state State) IsApproved(server Server) bool {
	for _, approval := range state.Approvals {
		if approval.Project == server.Path && approval.Server == server.Name && approval.Hash == server.Config.Hash() {
			return true
		}
	}
	return false
}

func (state *State) Approve(server Server) {
	kept := state.Approvals[:0]
	for _, approval := range state.Approvals {
		if approval.Project != server.Path || approval.Server != server.Name {
			kept = append(kept, approval)
		}
	}
	state.Approvals = append(kept, Approval{Project: server.Path, Server: server.Name, Hash: server.Config.Hash()})
}

func (state State) IsEnabled(server Server) bool {
	if server.Scope == ScopeImported {
		return containsString(state.EnabledImported, server.ID())
	}
	return !containsString(state.Disabled, server.ID())
}

func (state *State) SetEnabled(server Server, enabled bool) {
	if server.Scope == ScopeImported {
		state.EnabledImported = setString(state.EnabledImported, server.ID(), enabled)
		return
	}
	state.Disabled = setString(state.Disabled, server.ID(), !enabled)
}

func containsString(values []string, value string) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
}

func setString(values []string, value string, present bool) []string {
	kept := make([]string, 0, len(values)+1)
	for _, candidate := range values {
		if candidate != value {
			kept = append(kept, candidate)
		}
	}
	if present {
		kept = append(kept, value)
	}
	return kept
}

// NeedsApproval reports whether a project .mcp.json server still needs the
// user's approval before it may start.
func (state State) NeedsApproval(server Server) bool {
	return server.Scope == ScopeProject && !state.IsApproved(server)
}
