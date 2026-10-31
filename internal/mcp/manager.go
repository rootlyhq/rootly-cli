package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

type Status string

const (
	StatusOff        Status = "off"
	StatusPending    Status = "pending"
	StatusConnecting Status = "connecting"
	StatusConnected  Status = "connected"
	StatusNeedsAuth  Status = "auth"
	StatusError      Status = "error"
)

const (
	MaxClientTools       = 64
	MaxSchemaBytes       = 16 * 1024
	MaxClientToolsBytes  = 256 * 1024
	MaxToolOutputBytes   = 100 * 1024
	maxDescriptionLength = 1024
	connectTimeout       = 45 * time.Second
)

type ToolInfo struct {
	Name        string
	Description string
	ReadOnly    bool
}

type ServerView struct {
	Server     Server
	Status     Status
	Detail     string
	Tools      []ToolInfo
	SessionOff bool
}

type ClientTool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"input_schema"`
	ReadOnly    bool            `json:"read_only"`
}

type ToolRef struct {
	ServerID   string
	ServerName string
	Tool       string
	ReadOnly   bool
}

type CallResult struct {
	Output  string
	IsError bool
}

type runtimeServer struct {
	server     Server
	status     Status
	detail     string
	tools      []*sdk.Tool
	session    *sdk.ClientSession
	sessionOff bool
	generation int
	cancel     context.CancelFunc
}

type ManagerOptions struct {
	CWD       string
	Store     *Store
	State     *StateStore
	Tokens    *TokenStore
	Discovery *Discovery
	Version   string
	Lookup    func(string) (string, bool)
}

// Manager owns local MCP connections for one agent run.
type Manager struct {
	options ManagerOptions
	ctx     context.Context
	cancel  context.CancelFunc

	mu       sync.Mutex
	servers  []*runtimeServer
	byID     map[string]*runtimeServer
	toolRefs map[string]ToolRef
	problems []string
	changes  chan struct{}
	closed   bool
}

func NewManager(options ManagerOptions) *Manager {
	if options.Store == nil {
		options.Store = NewStore(options.CWD)
	}
	if options.State == nil {
		options.State = NewStateStore()
	}
	if options.Tokens == nil {
		options.Tokens = NewTokenStore()
	}
	if options.Discovery == nil {
		discovery := NewDiscovery(options.CWD)
		options.Discovery = &discovery
	}
	if options.Lookup == nil {
		options.Lookup = os.LookupEnv
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &Manager{options: options, ctx: ctx, cancel: cancel, byID: map[string]*runtimeServer{}, toolRefs: map[string]ToolRef{}, changes: make(chan struct{}, 1)}
}

// Changes signals (coalesced) whenever a server's status or tools change.
func (m *Manager) Changes() <-chan struct{} {
	return m.changes
}

func (m *Manager) notify() {
	select {
	case m.changes <- struct{}{}:
	default:
	}
}

// Load reads every config source; servers shadowed by a higher-precedence
// scope (local > project > user) are dropped like Claude Code does.
func (m *Manager) Load() error {
	configured, err := m.options.Store.All()
	imported, importProblems := m.options.Discovery.Imported()
	state, stateErr := m.options.State.Load()
	m.mu.Lock()
	defer m.mu.Unlock()
	m.problems = nil
	if err != nil {
		m.problems = append(m.problems, err.Error())
	}
	if stateErr != nil {
		m.problems = append(m.problems, stateErr.Error())
	}
	for _, problem := range importProblems {
		m.problems = append(m.problems, fmt.Sprintf("%s (%s): %v", problem.Source.Label, problem.Source.Path, problem.Err))
	}
	seen := map[string]bool{}
	var servers []Server
	for _, server := range configured {
		if seen[server.Name] {
			continue
		}
		seen[server.Name] = true
		servers = append(servers, server)
	}
	servers = append(servers, imported...)
	previous := m.byID
	m.servers = nil
	m.byID = map[string]*runtimeServer{}
	for _, server := range servers {
		runtime := previous[server.ID()]
		if runtime == nil || runtime.server.Config.Hash() != server.Config.Hash() {
			if runtime != nil {
				runtime.close()
			}
			runtime = &runtimeServer{server: server}
			runtime.status = initialStatus(state, server)
		} else if status := initialStatus(state, server); status == StatusOff || status == StatusPending {
			runtime.close()
			runtime.status, runtime.detail = status, ""
		}
		m.servers = append(m.servers, runtime)
		m.byID[server.ID()] = runtime
	}
	for id, runtime := range previous {
		if m.byID[id] == nil {
			runtime.close()
		}
	}
	m.notify()
	if len(m.problems) > 0 {
		return errors.New(strings.Join(m.problems, "; "))
	}
	return nil
}

func initialStatus(state State, server Server) Status {
	switch {
	case !state.IsEnabled(server):
		return StatusOff
	case state.NeedsApproval(server):
		return StatusPending
	default:
		return StatusConnecting
	}
}

func (m *Manager) Problems() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]string(nil), m.problems...)
}

// Start connects every enabled, approved server in the background.
func (m *Manager) Start() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, runtime := range m.servers {
		if runtime.status == StatusConnecting && runtime.session == nil && !runtime.sessionOff {
			m.connectLocked(runtime)
		}
	}
}

func (m *Manager) connectLocked(runtime *runtimeServer) {
	if m.closed {
		return
	}
	runtime.close()
	runtime.generation++
	generation := runtime.generation
	runtime.status = StatusConnecting
	runtime.detail = ""
	ctx, cancel := context.WithCancel(m.ctx)
	runtime.cancel = cancel
	server := runtime.server
	go func() {
		session, tools, status, detail := m.connect(ctx, server)
		m.mu.Lock()
		defer m.mu.Unlock()
		if runtime.generation != generation || m.closed {
			if session != nil {
				_ = session.Close()
			}
			return
		}
		runtime.session, runtime.tools, runtime.status, runtime.detail = session, tools, status, detail
		m.notify()
	}()
	m.notify()
}

func (r *runtimeServer) close() {
	r.generation++
	if r.cancel != nil {
		r.cancel()
		r.cancel = nil
	}
	if r.session != nil {
		session := r.session
		r.session = nil
		go func() { _ = session.Close() }()
	}
	r.tools = nil
}

func (m *Manager) connect(ctx context.Context, server Server) (*sdk.ClientSession, []*sdk.Tool, Status, string) {
	config := server.Config.Expanded(m.options.Lookup)
	connectCtx, cancel := context.WithTimeout(ctx, connectTimeout)
	defer cancel()
	transport, stderr, unauthorized, err := m.transport(ctx, server, config)
	if err != nil {
		return nil, nil, StatusError, err.Error()
	}
	client := sdk.NewClient(&sdk.Implementation{Name: "rootly-cli", Version: m.options.Version}, nil)
	session, err := client.Connect(connectCtx, transport, nil)
	if err != nil {
		return nil, nil, failureStatus(err, unauthorized), failureDetail(err, stderr, unauthorized)
	}
	var tools []*sdk.Tool
	for tool, err := range session.Tools(connectCtx, nil) {
		if err != nil {
			_ = session.Close()
			return nil, nil, failureStatus(err, unauthorized), failureDetail(err, stderr, unauthorized)
		}
		tools = append(tools, tool)
	}
	sort.SliceStable(tools, func(i, j int) bool { return tools[i].Name < tools[j].Name })
	return session, tools, StatusConnected, ""
}

func failureStatus(err error, unauthorized *authProbe) Status {
	if errors.Is(err, ErrNeedsAuth) || unauthorized.seen() {
		return StatusNeedsAuth
	}
	return StatusError
}

func failureDetail(err error, stderr *tailBuffer, unauthorized *authProbe) string {
	if errors.Is(err, ErrNeedsAuth) || unauthorized.seen() {
		return "needs authentication, Enter to log in"
	}
	if errors.Is(err, exec.ErrNotFound) {
		return "exited: command not found"
	}
	message := err.Error()
	if stderr != nil {
		if line := stderr.lastLine(); line != "" {
			message = line
		}
	}
	return oneLine(message)
}

func oneLine(value string) string {
	value = strings.Join(strings.Fields(value), " ")
	if len(value) > 160 {
		value = value[:157] + "..."
	}
	return value
}

func (m *Manager) transport(ctx context.Context, server Server, config ServerConfig) (sdk.Transport, *tailBuffer, *authProbe, error) {
	switch config.Transport() {
	case TransportStdio:
		if _, err := exec.LookPath(config.Command); err != nil {
			return nil, nil, nil, fmt.Errorf("exited: command not found")
		}
		command := exec.CommandContext(ctx, config.Command, config.Args...)
		command.Env = os.Environ()
		for _, key := range sortedKeys(config.Env) {
			command.Env = append(command.Env, key+"="+config.Env[key])
		}
		if server.Scope == ScopeProject || server.Scope == ScopeLocal {
			command.Dir = m.options.CWD
		}
		stderr := &tailBuffer{}
		command.Stderr = stderr
		return &sdk.CommandTransport{Command: command, TerminateDuration: 2 * time.Second}, stderr, nil, nil
	case TransportHTTP, TransportSSE:
		probe := &authProbe{}
		client := &http.Client{Transport: &headerTransport{
			base:    http.DefaultTransport,
			headers: config.Headers,
			bearer:  m.bearer(config),
			probe:   probe,
		}}
		if config.Transport() == TransportSSE {
			return &sdk.SSEClientTransport{Endpoint: config.URL, HTTPClient: client}, nil, probe, nil
		}
		return &sdk.StreamableClientTransport{Endpoint: config.URL, HTTPClient: client, DisableStandaloneSSE: true}, nil, probe, nil
	default:
		return nil, nil, nil, fmt.Errorf("unsupported transport %q", config.Type)
	}
}

func (m *Manager) bearer(config ServerConfig) func() string {
	if config.BearerTokenEnvVar != "" {
		return func() string {
			value, _ := m.options.Lookup(config.BearerTokenEnvVar)
			return value
		}
	}
	tokens := m.options.Tokens
	url := config.URL
	var source *persistingTokenSource
	var mu sync.Mutex
	return func() string {
		mu.Lock()
		defer mu.Unlock()
		if source == nil {
			credential, ok := tokens.Get(url)
			if !ok {
				return ""
			}
			source = tokens.tokenSource(url, credential, credential.oauthConfig(), credential.token(), nil).(*persistingTokenSource)
		}
		token, err := source.Token()
		if err != nil {
			return ""
		}
		return token.AccessToken
	}
}

type authProbe struct {
	mu           sync.Mutex
	unauthorized bool
}

func (p *authProbe) mark() {
	p.mu.Lock()
	p.unauthorized = true
	p.mu.Unlock()
}

func (p *authProbe) seen() bool {
	if p == nil {
		return false
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.unauthorized
}

type headerTransport struct {
	base    http.RoundTripper
	headers map[string]string
	bearer  func() string
	probe   *authProbe
}

func (t *headerTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	request = request.Clone(request.Context())
	for key, value := range t.headers {
		request.Header.Set(key, value)
	}
	if request.Header.Get("Authorization") == "" && t.bearer != nil {
		if token := t.bearer(); token != "" {
			request.Header.Set("Authorization", "Bearer "+token)
		}
	}
	response, err := t.base.RoundTrip(request)
	if err == nil && response.StatusCode == http.StatusUnauthorized && t.probe != nil {
		t.probe.mark()
	}
	return response, err
}

type tailBuffer struct {
	mu   sync.Mutex
	data []byte
}

func (b *tailBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.data = append(b.data, p...)
	if len(b.data) > 4096 {
		b.data = b.data[len(b.data)-4096:]
	}
	return len(p), nil
}

func (b *tailBuffer) lastLine() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	lines := bytes.Split(bytes.TrimSpace(b.data), []byte("\n"))
	if len(lines) == 0 {
		return ""
	}
	return strings.TrimSpace(string(lines[len(lines)-1]))
}

func (m *Manager) Views() []ServerView {
	m.mu.Lock()
	defer m.mu.Unlock()
	views := make([]ServerView, 0, len(m.servers))
	for _, runtime := range m.servers {
		view := ServerView{Server: runtime.server, Status: runtime.status, Detail: runtime.detail, SessionOff: runtime.sessionOff}
		view.Tools = make([]ToolInfo, 0, len(runtime.tools))
		for _, tool := range runtime.tools {
			view.Tools = append(view.Tools, ToolInfo{Name: tool.Name, Description: tool.Description, ReadOnly: readOnly(tool)})
		}
		views = append(views, view)
	}
	return views
}

// Connecting returns how many servers are still starting.
func (m *Manager) Connecting() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	count := 0
	for _, runtime := range m.servers {
		if runtime.status == StatusConnecting && !runtime.sessionOff {
			count++
		}
	}
	return count
}

func (m *Manager) withServer(id string, change func(*runtimeServer) error) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	runtime := m.byID[id]
	if runtime == nil {
		return fmt.Errorf("unknown MCP server")
	}
	err := change(runtime)
	m.notify()
	return err
}

// ToggleSession turns a server on or off for this agent run only.
func (m *Manager) ToggleSession(id string) error {
	return m.withServer(id, func(runtime *runtimeServer) error {
		runtime.sessionOff = !runtime.sessionOff
		if !runtime.sessionOff && runtime.session == nil && (runtime.status == StatusConnecting || runtime.status == StatusError) {
			m.connectLocked(runtime)
		}
		return nil
	})
}

// Approve records approval of a project server and starts it.
func (m *Manager) Approve(id string) error {
	return m.withServer(id, func(runtime *runtimeServer) error {
		if runtime.server.Scope != ScopeProject {
			return errors.New("only project (.mcp.json) servers need approval")
		}
		if err := m.options.State.Update(func(state *State) { state.Approve(runtime.server) }); err != nil {
			return err
		}
		if runtime.status == StatusPending {
			m.connectLocked(runtime)
		}
		return nil
	})
}

// Enable persistently enables a server (imported servers start off).
func (m *Manager) Enable(id string) error {
	return m.withServer(id, func(runtime *runtimeServer) error {
		if err := m.options.State.Update(func(state *State) { state.SetEnabled(runtime.server, true) }); err != nil {
			return err
		}
		state, err := m.options.State.Load()
		if err != nil {
			return err
		}
		if state.NeedsApproval(runtime.server) {
			runtime.status = StatusPending
			return nil
		}
		m.connectLocked(runtime)
		return nil
	})
}

func (m *Manager) Reconnect(id string) error {
	return m.withServer(id, func(runtime *runtimeServer) error {
		if runtime.status == StatusOff || runtime.status == StatusPending {
			return fmt.Errorf("%s is %s", runtime.server.Name, map[Status]string{StatusOff: "off", StatusPending: "pending approval"}[runtime.status])
		}
		runtime.sessionOff = false
		m.connectLocked(runtime)
		return nil
	})
}

// Login runs OAuth for an HTTP server, then reconnects it.
func (m *Manager) Login(ctx context.Context, id string, options LoginOptions) error {
	m.mu.Lock()
	runtime := m.byID[id]
	m.mu.Unlock()
	if runtime == nil {
		return fmt.Errorf("unknown MCP server")
	}
	config := runtime.server.Config.Expanded(m.options.Lookup)
	if err := m.options.Tokens.Login(ctx, config, options); err != nil {
		return err
	}
	m.mu.Lock()
	inactive := runtime.status == StatusOff || runtime.status == StatusPending
	m.mu.Unlock()
	if inactive {
		m.notify()
		return nil
	}
	return m.Reconnect(id)
}

// ClientTools returns the tools to advertise for the next turn and
// remembers how each generated name maps back to its server and tool.
func (m *Manager) ClientTools() (advertised []ClientTool, omitted int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	namer := NewClientToolNamer()
	refs := map[string]ToolRef{}
	var tools []ClientTool
	skipped := 0
	total := 2
	for _, runtime := range m.servers {
		if runtime.status != StatusConnected || runtime.sessionOff {
			continue
		}
		for _, tool := range runtime.tools {
			schema, ok := clientSchema(tool.InputSchema)
			if !ok || len(tools) >= MaxClientTools {
				skipped++
				continue
			}
			candidate := ClientTool{
				Name:        namer.Name(runtime.server.Name, tool.Name),
				Description: truncateRunes(firstNonEmpty(tool.Description, tool.Title, tool.Name), maxDescriptionLength),
				InputSchema: schema,
				ReadOnly:    readOnly(tool),
			}
			encoded, _ := json.Marshal(candidate)
			if total+len(encoded)+1 > MaxClientToolsBytes {
				skipped++
				continue
			}
			total += len(encoded) + 1
			tools = append(tools, candidate)
			refs[candidate.Name] = ToolRef{ServerID: runtime.server.ID(), ServerName: runtime.server.Name, Tool: tool.Name, ReadOnly: candidate.ReadOnly}
		}
	}
	m.toolRefs = refs
	return tools, skipped
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func truncateRunes(value string, limit int) string {
	if utf8.RuneCountInString(value) <= limit {
		return value
	}
	runes := []rune(value)
	return string(runes[:limit-1]) + "…"
}

func readOnly(tool *sdk.Tool) bool {
	return tool.Annotations != nil && tool.Annotations.ReadOnlyHint
}

func clientSchema(schema any) (json.RawMessage, bool) {
	if schema == nil {
		return json.RawMessage(`{"type":"object","properties":{}}`), true
	}
	encoded, err := json.Marshal(schema)
	if err != nil || len(encoded) > MaxSchemaBytes {
		return nil, false
	}
	var object map[string]any
	if json.Unmarshal(encoded, &object) != nil || object == nil {
		return nil, false
	}
	if kind, ok := object["type"]; ok && kind != "object" {
		return nil, false
	}
	if _, ok := object["type"]; !ok {
		object["type"] = "object"
		encoded, _ = json.Marshal(object)
	}
	return encoded, true
}

func (m *Manager) Lookup(name string) (ToolRef, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	ref, ok := m.toolRefs[name]
	return ref, ok
}

// Call runs a client tool by its generated name.
func (m *Manager) Call(ctx context.Context, name string, arguments map[string]any) CallResult {
	m.mu.Lock()
	ref, ok := m.toolRefs[name]
	var session *sdk.ClientSession
	if ok {
		if runtime := m.byID[ref.ServerID]; runtime != nil && !runtime.sessionOff {
			session = runtime.session
		}
	}
	m.mu.Unlock()
	if !ok {
		return CallResult{Output: "Unknown local tool " + name, IsError: true}
	}
	if session == nil {
		return CallResult{Output: "Local MCP server " + ref.ServerName + " is not connected", IsError: true}
	}
	if arguments == nil {
		arguments = map[string]any{}
	}
	result, err := session.CallTool(ctx, &sdk.CallToolParams{Name: ref.Tool, Arguments: arguments})
	if err != nil {
		return CallResult{Output: TruncateOutput("Local tool failed: " + err.Error()), IsError: true}
	}
	return CallResult{Output: TruncateOutput(renderResult(result)), IsError: result.IsError}
}

func renderResult(result *sdk.CallToolResult) string {
	var parts []string
	for _, content := range result.Content {
		if text, ok := content.(*sdk.TextContent); ok {
			parts = append(parts, text.Text)
			continue
		}
		encoded, err := json.Marshal(content)
		if err == nil {
			parts = append(parts, string(encoded))
		}
	}
	if len(parts) == 0 && result.StructuredContent != nil {
		if encoded, err := json.Marshal(result.StructuredContent); err == nil {
			parts = append(parts, string(encoded))
		}
	}
	return strings.Join(parts, "\n")
}

const truncationMarker = "\n[output truncated to 100 KB]"

// TruncateOutput caps output at 100 KB on a UTF-8 boundary.
func TruncateOutput(output string) string {
	if len(output) <= MaxToolOutputBytes {
		return output
	}
	cut := MaxToolOutputBytes - len(truncationMarker)
	for cut > 0 && !utf8.RuneStart(output[cut]) {
		cut--
	}
	return output[:cut] + truncationMarker
}

// Close stops every server (stdio processes are terminated).
func (m *Manager) Close() {
	m.mu.Lock()
	m.closed = true
	sessions := make([]*sdk.ClientSession, 0, len(m.servers))
	for _, runtime := range m.servers {
		if runtime.session != nil {
			sessions = append(sessions, runtime.session)
			runtime.session = nil
		}
	}
	m.mu.Unlock()
	var wg sync.WaitGroup
	for _, session := range sessions {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = session.Close()
		}()
	}
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
	}
	m.cancel()
}
