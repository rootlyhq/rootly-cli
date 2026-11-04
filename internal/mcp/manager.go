package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/rootlyhq/rootly-cli/internal/mcp/builtin"
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

const builtinServerID = "builtin"

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
	Enabled     bool
}

type BuiltinToolInfo struct {
	Name        string
	Description string
	ReadOnly    bool
	Enabled     bool
}

type ServerView struct {
	Server     Server
	Status     Status
	Detail     string
	Tools      []ToolInfo
	Prompts    []PromptInfo
	Resources  []ResourceInfo
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
	prompts    []PromptInfo
	resources  []ResourceInfo
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
	LookPath  builtin.LookPathFunc
}

// Manager owns local MCP connections for one agent run.
type Manager struct {
	options ManagerOptions
	ctx     context.Context
	cancel  context.CancelFunc

	mu             sync.Mutex
	servers        []*runtimeServer
	byID           map[string]*runtimeServer
	toolRefs       map[string]ToolRef
	builtins       *builtin.Registry
	builtinEnabled map[string]bool
	builtinErr     error
	sessionTools   map[string]bool
	problems       []string
	changes        chan struct{}
	closed         bool
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
	rulesDir := builtin.DefaultRulesDir()
	if options.Discovery.Home != "" {
		rulesDir = filepath.Join(options.Discovery.Home, ".rootly-cli", "rules")
	}
	builtins, builtinErr := builtin.NewRegistryWithPolicy(options.CWD, options.LookPath, builtin.LoadPolicy(rulesDir))
	ctx, cancel := context.WithCancel(context.Background())
	return &Manager{
		options:        options,
		ctx:            ctx,
		cancel:         cancel,
		byID:           map[string]*runtimeServer{},
		toolRefs:       map[string]ToolRef{},
		builtins:       builtins,
		builtinEnabled: map[string]bool{},
		builtinErr:     builtinErr,
		sessionTools:   map[string]bool{},
		changes:        make(chan struct{}, 1),
	}
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
	if m.builtinErr != nil {
		m.problems = append(m.problems, m.builtinErr.Error())
	}
	if m.builtins != nil {
		if err := m.builtins.Policy().LoadError(); err != nil {
			m.problems = append(m.problems, "Local tool rules (~/.rootly-cli/rules) skipped: "+err.Error())
		}
	}
	if stateErr != nil {
		m.problems = append(m.problems, stateErr.Error())
	}
	m.builtinEnabled = make(map[string]bool, len(state.BuiltinTools))
	for name, enabled := range state.BuiltinTools {
		m.builtinEnabled[name] = enabled
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
		if runtime == nil || runtime.server.Config.Hash() != server.Config.Hash() || runtime.server.Project != server.Project {
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
		session, tools, status, detail, probe := m.connect(ctx, server)
		var prompts []PromptInfo
		var resources []ResourceInfo
		if session != nil {
			discoverCtx, cancelDiscover := context.WithTimeout(ctx, server.Config.CallTimeout(connectTimeout))
			prompts, resources = discoverExtras(discoverCtx, session)
			cancelDiscover()
		}
		m.mu.Lock()
		defer m.mu.Unlock()
		if runtime.generation != generation || m.closed {
			if session != nil {
				_ = session.Close()
			}
			return
		}
		runtime.session, runtime.tools, runtime.status, runtime.detail = session, tools, status, detail
		runtime.prompts, runtime.resources = prompts, resources
		if session != nil {
			m.watchAuth(runtime, generation, probe)
		}
		m.notify()
	}()
	m.notify()
}

// watchAuth must run with m.mu held; a 401 seen while discovering prompts and resources marks the server at once.
func (m *Manager) watchAuth(runtime *runtimeServer, generation int, probe *authProbe) {
	if probe.watch(func() { m.loseAuth(runtime, generation) }) {
		runtime.close()
		runtime.status, runtime.detail = StatusNeedsAuth, "needs authentication, Enter to log in"
	}
}

func (m *Manager) loseAuth(runtime *runtimeServer, generation int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if runtime.generation != generation || runtime.session == nil {
		return
	}
	runtime.close()
	runtime.status, runtime.detail = StatusNeedsAuth, "needs authentication, Enter to log in"
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
	r.prompts = nil
	r.resources = nil
}

func (m *Manager) connect(ctx context.Context, server Server) (session *sdk.ClientSession, tools []*sdk.Tool, status Status, detail string, probe *authProbe) {
	config := server.Config.Expanded(m.options.Lookup)
	connectCtx, cancel := context.WithTimeout(ctx, config.CallTimeout(connectTimeout))
	defer cancel()
	transport, stderr, probe, err := m.transport(ctx, server, config)
	if err != nil {
		return nil, nil, StatusError, err.Error(), nil
	}
	client := sdk.NewClient(&sdk.Implementation{Name: "rootly-cli", Version: m.options.Version}, nil)
	session, err = client.Connect(connectCtx, transport, nil)
	if err != nil {
		return nil, nil, failureStatus(err, probe), failureDetail(err, stderr, probe), nil
	}
	for tool, err := range session.Tools(connectCtx, nil) {
		if err != nil {
			_ = session.Close()
			return nil, nil, failureStatus(err, probe), failureDetail(err, stderr, probe), nil
		}
		tools = append(tools, tool)
	}
	sort.SliceStable(tools, func(i, j int) bool { return tools[i].Name < tools[j].Name })
	return session, tools, StatusConnected, "", probe
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
		bearer, refresh := m.bearerFuncs(config)
		client := newServerHTTPClientWithRefresh(config, bearer, refresh, probe)
		if config.Transport() == TransportSSE {
			return &sdk.SSEClientTransport{Endpoint: config.URL, HTTPClient: client}, nil, probe, nil
		}
		return &sdk.StreamableClientTransport{Endpoint: config.URL, HTTPClient: client, DisableStandaloneSSE: true}, nil, probe, nil
	default:
		return nil, nil, nil, fmt.Errorf("unsupported transport %q", config.Type)
	}
}

func (m *Manager) bearerFuncs(config ServerConfig) (bearer, refresh func(context.Context) string) {
	if config.BearerTokenEnvVar != "" {
		return func(context.Context) string {
			value, _ := m.options.Lookup(config.BearerTokenEnvVar)
			return value
		}, nil
	}
	tokens := m.options.Tokens
	serverURL := config.URL
	var source *persistingTokenSource
	var mu sync.Mutex
	bearer = func(ctx context.Context) string {
		mu.Lock()
		defer mu.Unlock()
		if source == nil || !source.base.token().Valid() {
			credential, ok := tokens.Get(serverURL)
			if !ok {
				return ""
			}
			source = tokens.tokenSourceWithContext(ctx, serverURL, credential, credential.oauthConfig(), credential.token(), nil).(*persistingTokenSource)
		}
		token, err := source.Token()
		if err != nil {
			return ""
		}
		return token.AccessToken
	}
	refresh = func(ctx context.Context) string {
		mu.Lock()
		defer mu.Unlock()
		credential, ok := tokens.Get(serverURL)
		if !ok || credential.RefreshToken == "" {
			return ""
		}
		expired := credential.token()
		expired.Expiry = time.Now().Add(-time.Minute)
		source = nil
		refreshing := &persistingTokenSource{store: tokens, serverURL: serverURL, base: credential, source: credential.oauthConfig().TokenSource(ctx, expired), last: expired.AccessToken}
		token, err := refreshing.Token()
		if err != nil || token.AccessToken == "" || token.AccessToken == credential.AccessToken {
			return ""
		}
		return token.AccessToken
	}
	return bearer, refresh
}

type authProbe struct {
	mu           sync.Mutex
	unauthorized bool
	onExpired    func()
}

func (p *authProbe) mark() {
	p.mu.Lock()
	p.unauthorized = true
	onExpired := p.onExpired
	p.mu.Unlock()
	if onExpired != nil {
		onExpired()
	}
}

func (p *authProbe) watch(onExpired func()) (alreadyUnauthorized bool) {
	if p == nil {
		return false
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.onExpired = onExpired
	return p.unauthorized
}

func (p *authProbe) seen() bool {
	if p == nil {
		return false
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.unauthorized
}

// newServerHTTPClient sends the configured headers and bearer token only to
// the server's own origin and refuses redirects to any other origin.
func newServerHTTPClient(config ServerConfig, bearer func(context.Context) string, probe *authProbe) *http.Client {
	return newServerHTTPClientWithRefresh(config, bearer, nil, probe)
}

func newServerHTTPClientWithRefresh(config ServerConfig, bearer, refresh func(context.Context) string, probe *authProbe) *http.Client {
	return &http.Client{
		Transport: &headerTransport{
			base:    http.DefaultTransport,
			origin:  originOf(config.URL),
			headers: config.Headers,
			bearer:  bearer,
			refresh: refresh,
			probe:   probe,
		},
		CheckRedirect: rejectCrossOriginRedirect,
	}
}

func originOf(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" {
		return ""
	}
	scheme, port := strings.ToLower(parsed.Scheme), parsed.Port()
	if port == "" {
		port = map[string]string{"http": "80", "https": "443"}[scheme]
	}
	return scheme + "://" + strings.ToLower(parsed.Hostname()) + ":" + port
}

func rejectCrossOriginRedirect(request *http.Request, via []*http.Request) error {
	if len(via) >= 10 {
		return errors.New("stopped after 10 redirects")
	}
	if len(via) > 0 && originOf(request.URL.String()) != originOf(via[0].URL.String()) {
		return fmt.Errorf("refusing cross-origin redirect to %s", request.URL.Host)
	}
	return nil
}

type headerTransport struct {
	base    http.RoundTripper
	origin  string
	headers map[string]string
	bearer  func(context.Context) string
	refresh func(context.Context) string
	probe   *authProbe
}

func (t *headerTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	if t.origin == "" || originOf(request.URL.String()) != t.origin {
		return t.base.RoundTrip(request)
	}
	request = request.Clone(request.Context())
	for key, value := range t.headers {
		request.Header.Set(key, value)
	}
	usedBearer := false
	if request.Header.Get("Authorization") == "" && t.bearer != nil {
		if token := t.bearer(request.Context()); token != "" {
			request.Header.Set("Authorization", "Bearer "+token)
			usedBearer = true
		}
	}
	response, err := t.base.RoundTrip(request)
	if err == nil && response.StatusCode == http.StatusUnauthorized && usedBearer {
		response, err = t.retryWithRefreshedToken(request, response)
	}
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
		view := ServerView{Server: runtime.server, Status: runtime.status, Detail: runtime.detail, SessionOff: runtime.sessionOff, Prompts: runtime.prompts, Resources: runtime.resources}
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
		if !runtime.server.NeedsProjectTrust() {
			return errors.New("only project-controlled servers need approval")
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
func (m *Manager) ClientTools() (advertised []ClientTool, omitted []ToolRef) {
	m.mu.Lock()
	defer m.mu.Unlock()
	namer := NewClientToolNamer()
	refs := map[string]ToolRef{}
	var tools []ClientTool
	total := 2
	if m.builtins != nil {
		builtinTools := m.builtins.Tools()
		for _, tool := range builtinTools {
			namer.Reserve(tool.Name())
		}
		for _, tool := range builtinTools {
			if !m.builtinEnabledLocked(tool.Name()) {
				continue
			}
			schema := tool.Schema()
			if len(schema) > MaxSchemaBytes || len(tools) >= MaxClientTools {
				omitted = append(omitted, ToolRef{
					ServerID: builtinServerID, ServerName: "Built-in",
					Tool: strings.TrimPrefix(tool.Name(), ClientToolPrefix),
				})
				continue
			}
			candidate := ClientTool{
				Name:        tool.Name(),
				Description: truncateRunes(tool.Description(), maxDescriptionLength),
				InputSchema: schema,
				ReadOnly:    tool.AdvertisedReadOnly(),
			}
			encoded, err := json.Marshal(candidate)
			if err != nil || total+len(encoded)+1 > MaxClientToolsBytes {
				omitted = append(omitted, ToolRef{
					ServerID: builtinServerID, ServerName: "Built-in",
					Tool: strings.TrimPrefix(tool.Name(), ClientToolPrefix),
				})
				continue
			}
			total += len(encoded) + 1
			tools = append(tools, candidate)
			refs[candidate.Name] = ToolRef{
				ServerID:   builtinServerID,
				ServerName: "Built-in",
				Tool:       strings.TrimPrefix(candidate.Name, ClientToolPrefix),
				ReadOnly:   candidate.ReadOnly,
			}
		}
	}
	for _, runtime := range m.servers {
		if runtime.status != StatusConnected || runtime.sessionOff {
			continue
		}
		for _, tool := range runtime.tools {
			schema, ok := clientSchema(tool.InputSchema)
			if !ok || len(tools) >= MaxClientTools {
				omitted = append(omitted, ToolRef{
					ServerID: runtime.server.ID(), ServerName: runtime.server.Name, Tool: tool.Name,
					ReadOnly: readOnly(tool),
				})
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
				omitted = append(omitted, ToolRef{
					ServerID: runtime.server.ID(), ServerName: runtime.server.Name, Tool: tool.Name,
					ReadOnly: readOnly(tool),
				})
				continue
			}
			total += len(encoded) + 1
			tools = append(tools, candidate)
			refs[candidate.Name] = ToolRef{ServerID: runtime.server.ID(), ServerName: runtime.server.Name, Tool: tool.Name, ReadOnly: candidate.ReadOnly}
		}
	}
	m.toolRefs = refs
	return tools, omitted
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

func (m *Manager) NeedsApproval(name string, args map[string]any) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	ref, ok := m.toolRefs[name]
	if !ok {
		return true
	}
	if ref.ServerID != builtinServerID {
		return !ref.ReadOnly && !m.sessionTools[sessionToolKey(ref)]
	}
	tool, ok := m.builtins.Lookup(name)
	return !ok || !m.builtinEnabledLocked(name) || tool.NeedsApproval(args)
}

// sessionToolKey identifies an MCP tool by its server and original name:
// generated client names are rebuilt each turn and can move between servers.
func sessionToolKey(ref ToolRef) string {
	return ref.ServerID + "\x00" + ref.Tool
}

// ReadOnlyCall reports whether a call reads only on its own merits (not
// through a remembered approval or allow rule), so it may run alongside
// other reads.
func (m *Manager) ReadOnlyCall(name string, args map[string]any) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	ref, ok := m.toolRefs[name]
	if !ok {
		return false
	}
	if ref.ServerID != builtinServerID {
		return ref.ReadOnly
	}
	tool, ok := m.builtins.Lookup(name)
	return ok && m.builtinEnabledLocked(name) && tool.Approval(args).ReadOnly
}

// ApprovalOptions are the ways a call that needs approval may be remembered:
// for the session (MCP tools by name, commands by Prefix) or, for commands,
// as an allow rule in ~/.rootly-cli/rules/default.rules.
type ApprovalOptions struct {
	Session bool
	Persist bool
	Prefix  []string
}

func (m *Manager) ApprovalOptions(name string, args map[string]any) ApprovalOptions {
	m.mu.Lock()
	defer m.mu.Unlock()
	ref, ok := m.toolRefs[name]
	if !ok {
		return ApprovalOptions{}
	}
	if ref.ServerID != builtinServerID {
		return ApprovalOptions{Session: !ref.ReadOnly}
	}
	tool, ok := m.builtins.Lookup(name)
	if !ok || !m.builtinEnabledLocked(name) {
		return ApprovalOptions{}
	}
	approval := tool.Approval(args)
	if !approval.Needs || len(approval.Prefix) == 0 {
		return ApprovalOptions{}
	}
	return ApprovalOptions{Session: true, Persist: true, Prefix: approval.Prefix}
}

// RememberApproval allows this kind of call for the session or, with
// persist, saves an allow rule for its command prefix.
func (m *Manager) RememberApproval(name string, args map[string]any, persist bool) error {
	options := m.ApprovalOptions(name, args)
	switch {
	case len(options.Prefix) > 0 && persist:
		return m.builtins.Policy().SaveAllow(options.Prefix)
	case len(options.Prefix) > 0:
		m.builtins.Policy().AllowForSession(options.Prefix)
		return nil
	case options.Session && !persist:
		m.mu.Lock()
		defer m.mu.Unlock()
		ref, ok := m.toolRefs[name]
		if !ok {
			return errors.New("unknown local tool " + name)
		}
		m.sessionTools[sessionToolKey(ref)] = true
		return nil
	}
	return errors.New("this call can only be approved once")
}

func (m *Manager) BuiltinTools() []BuiltinToolInfo {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.builtins == nil {
		return nil
	}
	tools := m.builtins.Tools()
	result := make([]BuiltinToolInfo, 0, len(tools))
	for _, tool := range tools {
		result = append(result, BuiltinToolInfo{
			Name:        tool.Name(),
			Description: tool.Description(),
			ReadOnly:    tool.AdvertisedReadOnly(),
			Enabled:     m.builtinEnabledLocked(tool.Name()),
		})
	}
	return result
}

func (m *Manager) SetBuiltinEnabled(name string, enabled bool) error {
	if m.builtins == nil {
		return errors.New("built-in tools are unavailable")
	}
	if _, ok := m.builtins.Lookup(name); !ok {
		return fmt.Errorf("unknown built-in tool %q", name)
	}
	if err := m.options.State.Update(func(state *State) { state.SetBuiltinEnabled(name, enabled) }); err != nil {
		return err
	}
	m.mu.Lock()
	m.builtinEnabled[name] = enabled
	m.mu.Unlock()
	m.notify()
	return nil
}

func (m *Manager) builtinEnabledLocked(name string) bool {
	if enabled, ok := m.builtinEnabled[name]; ok {
		return enabled
	}
	return name != "local_bash"
}

// Call runs a client tool by its generated name.
func (m *Manager) Call(ctx context.Context, name string, arguments map[string]any) CallResult {
	m.mu.Lock()
	ref, ok := m.toolRefs[name]
	if ok && ref.ServerID == builtinServerID {
		tool, found := m.builtins.Lookup(name)
		enabled := m.builtinEnabledLocked(name)
		m.mu.Unlock()
		if !found {
			return CallResult{Output: "Unknown built-in tool " + name, IsError: true}
		}
		if !enabled {
			return CallResult{Output: "Built-in tool " + name + " is off", IsError: true}
		}
		result := tool.Run(ctx, arguments)
		return CallResult{Output: TruncateOutput(result.Output), IsError: result.IsError}
	}
	var session *sdk.ClientSession
	timeout := time.Duration(0)
	if ok {
		if runtime := m.byID[ref.ServerID]; runtime != nil && !runtime.sessionOff {
			session = runtime.session
			timeout = runtime.server.Config.CallTimeout(0)
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
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}
	result, err := session.CallTool(ctx, &sdk.CallToolParams{Name: ref.Tool, Arguments: arguments})
	if err != nil && timeout > 0 && errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return CallResult{Output: fmt.Sprintf("Local tool timed out after %s", timeout), IsError: true}
	}
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

func (t *headerTransport) retryWithRefreshedToken(request *http.Request, response *http.Response) (*http.Response, error) {
	if t.refresh == nil || (request.Body != nil && request.Body != http.NoBody && request.GetBody == nil) {
		return response, nil
	}
	token := t.refresh(request.Context())
	if token == "" {
		return response, nil
	}
	retry := request.Clone(request.Context())
	if request.GetBody != nil {
		body, err := request.GetBody()
		if err != nil {
			return response, nil
		}
		retry.Body = body
	}
	retry.Header.Set("Authorization", "Bearer "+token)
	_, _ = io.Copy(io.Discard, response.Body)
	_ = response.Body.Close()
	return t.base.RoundTrip(retry)
}

// AddFromMarket installs a marketplace entry in USER scope, switched off until the user turns it on.
func (m *Manager) AddFromMarket(entry MarketEntry) (Server, error) {
	imported, _ := m.options.Discovery.Imported()
	for _, server := range imported {
		if server.Name == entry.Name {
			return Server{}, fmt.Errorf("an MCP server named %s is already imported from %s", entry.Name, server.Path)
		}
	}
	server, err := AddFromMarket(m.options.Store, m.options.State, entry)
	if err != nil {
		return Server{}, err
	}
	_ = m.Load()
	return server, nil
}
