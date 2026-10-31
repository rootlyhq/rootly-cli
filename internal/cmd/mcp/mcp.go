// Package mcp implements `rootly mcp`, which manages local MCP servers for
// `rootly agent` using the same file shapes as Claude Code.
package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"

	localmcp "github.com/rootlyhq/rootly-cli/internal/mcp"
)

var MCPCmd = &cobra.Command{
	Use:         "mcp",
	Short:       "Manage local MCP servers for rootly agent",
	Hidden:      true,
	Annotations: map[string]string{"skipAuth": "true"},
	Args:        cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		return cmd.Help()
	},
}

// Environment is swappable in tests.
type Environment struct {
	CWD       func() (string, error)
	Store     func(cwd string) *localmcp.Store
	State     func() *localmcp.StateStore
	Tokens    func() *localmcp.TokenStore
	Discovery func(cwd string) localmcp.Discovery
	IsTTY     func() bool
	Stdin     io.Reader
	HTTP      *http.Client
	OpenURL   func(string) error
}

var env = DefaultEnvironment()

func DefaultEnvironment() Environment {
	return Environment{
		CWD:       os.Getwd,
		Store:     localmcp.NewStore,
		State:     localmcp.NewStateStore,
		Tokens:    localmcp.NewTokenStore,
		Discovery: localmcp.NewDiscovery,
		IsTTY:     func() bool { return isatty.IsTerminal(os.Stdin.Fd()) },
		Stdin:     os.Stdin,
		HTTP:      &http.Client{Timeout: 5 * time.Second},
		OpenURL:   localmcp.OpenBrowser,
	}
}

// SetEnvironment replaces the environment and returns a restore func.
func SetEnvironment(next Environment) func() {
	previous := env
	env = next
	return func() { env = previous }
}

func cwd() (string, error) {
	dir, err := env.CWD()
	if err != nil {
		return "", err
	}
	return filepath.Abs(dir)
}

func displayPath(path string) string {
	home, err := os.UserHomeDir()
	if err == nil && home != "" && strings.HasPrefix(path, home+string(os.PathSeparator)) {
		return "~" + strings.TrimPrefix(path, home)
	}
	return path
}

func scopeTarget(store *localmcp.Store, scope string) string {
	switch scope {
	case localmcp.ScopeProject:
		return "project config (" + displayPath(store.ProjectPath()) + ")"
	case localmcp.ScopeLocal:
		return "local config (" + displayPath(store.UserPath) + " [project: " + store.CWD + "])"
	default:
		return "user config (" + displayPath(store.UserPath) + ")"
	}
}

func validScope(scope string) error {
	switch scope {
	case localmcp.ScopeLocal, localmcp.ScopeUser, localmcp.ScopeProject:
		return nil
	}
	return fmt.Errorf("invalid scope %q: expected local, user or project", scope)
}

var (
	addTransport string
	addScope     string
	addEnv       []string
	addHeaders   []string
	addURL       string
	addBearerEnv string
	addJSONScope string
	removeScope  string
	importAll    bool
)

var addCmd = &cobra.Command{
	Use:   "add [flags] <name> <commandOrUrl> [args...]",
	Short: "Add an MCP server (same syntax as `claude mcp add`)",
	Example: `  rootly mcp add --transport http braintrust https://api.braintrust.dev/mcp
  rootly mcp add --transport http sentry https://mcp.sentry.dev/mcp -H "Authorization: Bearer ${SENTRY_TOKEN}"
  rootly mcp add scalr -e SCALR_API_URL=https://rootly.scalr.io -- docker run --rm -i -e SCALR_API_TOKEN scalr/mcp-server
  rootly mcp add -s project maestro -- maestro mcp
  rootly mcp add linear --url https://mcp.linear.app/mcp --bearer-token-env-var LINEAR_TOKEN`,
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		server, err := buildAddConfig(args)
		if err != nil {
			return err
		}
		return addServer(cmd, args[0], addScope, server)
	},
}

func buildAddConfig(args []string) (localmcp.ServerConfig, error) {
	transport := strings.ToLower(addTransport)
	positional := args[1:]
	url := addURL
	if url == "" && transport == "" && len(positional) == 1 && looksLikeURL(positional[0]) {
		transport = localmcp.TransportHTTP
	}
	if url != "" {
		if len(positional) > 0 {
			return localmcp.ServerConfig{}, errors.New("--url takes the URL; don't also pass a command or URL argument")
		}
		if transport == "" {
			transport = localmcp.TransportHTTP
		}
	}
	if transport == "" {
		transport = localmcp.TransportStdio
	}
	envs, err := parsePairs(addEnv, "=", "-e/--env", "KEY=value")
	if err != nil {
		return localmcp.ServerConfig{}, err
	}
	headers, err := parsePairs(addHeaders, ":", "-H/--header", `"Name: value"`)
	if err != nil {
		return localmcp.ServerConfig{}, err
	}
	switch transport {
	case localmcp.TransportStdio:
		if len(positional) == 0 {
			return localmcp.ServerConfig{}, errors.New("missing command: rootly mcp add <name> -- <command> [args...]")
		}
		if len(headers) > 0 || addBearerEnv != "" {
			return localmcp.ServerConfig{}, errors.New("-H/--header and --bearer-token-env-var only apply to http and sse servers")
		}
		return localmcp.ServerConfig{Command: positional[0], Args: positional[1:], Env: envs}, nil
	case localmcp.TransportHTTP, localmcp.TransportSSE:
		if url == "" {
			if len(positional) != 1 {
				return localmcp.ServerConfig{}, fmt.Errorf("%s servers take exactly one URL: rootly mcp add --transport %s <name> <url>", transport, transport)
			}
			url = positional[0]
		}
		if !looksLikeURL(url) {
			return localmcp.ServerConfig{}, fmt.Errorf("invalid URL %q: expected http:// or https://", url)
		}
		if len(envs) > 0 {
			return localmcp.ServerConfig{}, errors.New("-e/--env only applies to stdio servers")
		}
		return localmcp.ServerConfig{Type: transport, URL: url, Headers: headers, BearerTokenEnvVar: addBearerEnv}, nil
	default:
		return localmcp.ServerConfig{}, fmt.Errorf("invalid transport %q: expected stdio, http or sse", addTransport)
	}
}

func looksLikeURL(value string) bool {
	lower := strings.ToLower(value)
	return strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://")
}

func parsePairs(values []string, separator, flag, shape string) (map[string]string, error) {
	if len(values) == 0 {
		return nil, nil
	}
	pairs := make(map[string]string, len(values))
	for _, value := range values {
		key, rest, ok := strings.Cut(value, separator)
		key = strings.TrimSpace(key)
		if !ok || key == "" {
			return nil, fmt.Errorf("invalid %s value %q: expected %s", flag, maskedValue(value, separator), shape)
		}
		if separator == ":" {
			rest = strings.TrimSpace(rest)
		}
		pairs[key] = rest
	}
	return pairs, nil
}

func maskedValue(value, separator string) string {
	if key, _, ok := strings.Cut(value, separator); ok {
		return key + separator + localmcp.Mask
	}
	return value
}

func addServer(cmd *cobra.Command, name, scope string, server localmcp.ServerConfig) error {
	if err := validScope(scope); err != nil {
		return err
	}
	dir, err := cwd()
	if err != nil {
		return err
	}
	store := env.Store(dir)
	if err := store.Add(scope, name, server); err != nil {
		return err
	}
	out := cmd.OutOrStdout()
	kind := strings.ToUpper(server.Transport())
	if server.Transport() == localmcp.TransportStdio {
		kind = "stdio"
	}
	_, _ = fmt.Fprintf(out, "Added %s MCP server %s to %s\n", kind, name, scopeTarget(store, scope))
	if scope == localmcp.ScopeProject {
		_, _ = fmt.Fprintln(out, "  project servers need approval before they start: open /mcp in rootly agent")
	}
	if server.Transport() != localmcp.TransportStdio && needsAuthentication(cmd.Context(), server) {
		_, _ = fmt.Fprintf(out, "  needs authentication: run `rootly mcp login %s` or /mcp\n", name)
	}
	return nil
}

func needsAuthentication(ctx context.Context, server localmcp.ServerConfig) bool {
	if ctx == nil {
		ctx = context.Background()
	}
	if server.BearerTokenEnvVar != "" {
		return false
	}
	for key := range server.Headers {
		if strings.EqualFold(key, "Authorization") {
			return false
		}
	}
	if _, ok := env.Tokens().Get(server.URL); ok {
		return false
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	status, err := localmcp.ProbeStatus(ctx, env.HTTP, server.Expanded(nil))
	return err == nil && status == http.StatusUnauthorized
}

var addJSONCmd = &cobra.Command{
	Use:     "add-json <name> <json>",
	Short:   "Add an MCP server from a JSON config",
	Example: `  rootly mcp add-json dart '{"command":"fvm","args":["dart","mcp-server"]}'`,
	Args:    cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		var server localmcp.ServerConfig
		decoder := json.NewDecoder(strings.NewReader(args[1]))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&server); err != nil {
			return fmt.Errorf("invalid MCP server JSON: %w", err)
		}
		return addServer(cmd, args[0], addJSONScope, server)
	},
}

type resolved struct {
	server localmcp.Server
	store  *localmcp.Store
}

func allServers() ([]localmcp.Server, *localmcp.Store, error) {
	dir, err := cwd()
	if err != nil {
		return nil, nil, err
	}
	store := env.Store(dir)
	servers, err := store.All()
	imported, _ := env.Discovery(dir).Imported()
	return append(servers, imported...), store, err
}

func findServer(name, scope string) (resolved, error) {
	servers, store, err := allServers()
	if err != nil && len(servers) == 0 {
		return resolved{}, err
	}
	var matches []localmcp.Server
	for _, server := range servers {
		if server.Name == name && (scope == "" || server.Scope == scope) {
			matches = append(matches, server)
		}
	}
	if len(matches) == 0 {
		if scope != "" {
			return resolved{}, fmt.Errorf("no MCP server named %q in %s config", name, scope)
		}
		return resolved{}, fmt.Errorf("no MCP server named %q (see `rootly mcp list`)", name)
	}
	return resolved{server: matches[0], store: store}, nil
}

var listCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"ls"},
	Short:   "List configured and imported MCP servers",
	Args:    cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		servers, _, err := allServers()
		if err != nil && len(servers) == 0 {
			return err
		}
		out := cmd.OutOrStdout()
		if len(servers) == 0 {
			_, _ = fmt.Fprintln(out, "No MCP servers configured. Add one with `rootly mcp add`, or copy existing ones with `rootly mcp import`.")
			return nil
		}
		state, _ := env.State().Load()
		tokens := env.Tokens()
		for _, server := range servers {
			_, _ = fmt.Fprintf(out, "%s: %s (%s) - %s\n", server.Name, server.Config.Origin(), server.Config.Transport(), describeState(state, tokens, server))
		}
		return err
	},
}

func describeState(state localmcp.State, tokens *localmcp.TokenStore, server localmcp.Server) string {
	scope := server.Scope
	if server.Scope == localmcp.ScopeImported {
		scope = "imported from " + server.SourceLabel + ", read-only"
	}
	parts := []string{scope}
	switch {
	case !state.IsEnabled(server):
		parts = append(parts, "off")
	case state.NeedsApproval(server):
		parts = append(parts, "pending approval")
	}
	if server.Config.Transport() != localmcp.TransportStdio {
		if _, ok := tokens.Get(server.Config.URL); ok {
			parts = append(parts, "logged in")
		}
	}
	return strings.Join(parts, ", ")
}

var getCmd = &cobra.Command{
	Use:   "get <name>",
	Short: "Show an MCP server's config (secrets masked)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		found, err := findServer(args[0], "")
		if err != nil {
			return err
		}
		server := found.server
		config := server.Config.Masked()
		state, _ := env.State().Load()
		out := cmd.OutOrStdout()
		_, _ = fmt.Fprintf(out, "%s:\n", server.Name)
		_, _ = fmt.Fprintf(out, "  Scope: %s\n", describeState(state, env.Tokens(), server))
		_, _ = fmt.Fprintf(out, "  Source: %s\n", displayPath(server.Path))
		_, _ = fmt.Fprintf(out, "  Type: %s\n", config.Transport())
		if config.Transport() == localmcp.TransportStdio {
			_, _ = fmt.Fprintf(out, "  Command: %s\n", config.Command)
			_, _ = fmt.Fprintf(out, "  Args: %s\n", strings.Join(config.Args, " "))
			if len(config.Env) > 0 {
				_, _ = fmt.Fprintf(out, "  Environment: %s\n", strings.Join(localmcp.MaskedPairs(config.Env, "="), ", "))
			}
		} else {
			_, _ = fmt.Fprintf(out, "  URL: %s\n", config.URL)
			if len(config.Headers) > 0 {
				_, _ = fmt.Fprintf(out, "  Headers: %s\n", strings.Join(localmcp.MaskedPairs(config.Headers, ": "), ", "))
			}
			if config.BearerTokenEnvVar != "" {
				_, _ = fmt.Fprintf(out, "  Bearer token env var: %s\n", config.BearerTokenEnvVar)
			}
		}
		if server.Scope != localmcp.ScopeImported {
			_, _ = fmt.Fprintf(out, "\nTo remove this server, run: rootly mcp remove %s -s %s\n", server.Name, server.Scope)
		}
		return nil
	},
}

var removeCmd = &cobra.Command{
	Use:     "remove <name>",
	Aliases: []string{"rm"},
	Short:   "Remove an MCP server",
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		dir, err := cwd()
		if err != nil {
			return err
		}
		store := env.Store(dir)
		scopes := []string{removeScope}
		if removeScope == "" {
			scopes = nil
			for _, scope := range []string{localmcp.ScopeLocal, localmcp.ScopeProject, localmcp.ScopeUser} {
				servers, _ := store.Servers(scope)
				for _, server := range servers {
					if server.Name == args[0] {
						scopes = append(scopes, scope)
					}
				}
			}
			switch len(scopes) {
			case 0:
				if found, findErr := findServer(args[0], localmcp.ScopeImported); findErr == nil {
					return fmt.Errorf("%s is imported from %s (read-only); edit %s instead", args[0], found.server.SourceLabel, displayPath(found.server.Path))
				}
				return fmt.Errorf("no MCP server named %q", args[0])
			case 1:
			default:
				return fmt.Errorf("MCP server %q exists in several scopes (%s); pass -s/--scope", args[0], strings.Join(scopes, ", "))
			}
		} else if err := validScope(removeScope); err != nil {
			return err
		}
		if err := store.Remove(scopes[0], args[0]); err != nil {
			return err
		}
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Removed MCP server %s from %s\n", args[0], scopeTarget(store, scopes[0]))
		return nil
	},
}

var loginCmd = &cobra.Command{
	Use:   "login <name>",
	Short: "Log in to an HTTP MCP server with OAuth",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		found, err := findServer(args[0], "")
		if err != nil {
			return err
		}
		config := found.server.Config.Expanded(nil)
		if config.Transport() == localmcp.TransportStdio {
			return fmt.Errorf("%s is a stdio server; OAuth login only applies to http and sse servers", args[0])
		}
		errOut := cmd.ErrOrStderr()
		ctx, cancel := context.WithTimeout(cmd.Context(), 5*time.Minute)
		defer cancel()
		err = env.Tokens().Login(ctx, config, localmcp.LoginOptions{OpenURL: func(url string) {
			_, _ = fmt.Fprintf(errOut, "Opening your browser to authorize %s.\nIf it doesn't open, visit:\n  %s\n", args[0], url)
			_ = env.OpenURL(url)
		}})
		if err != nil {
			return fmt.Errorf("login to %s failed: %w", args[0], err)
		}
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Logged in to %s. Tokens saved in ~/.rootly-cli (0600).\n", args[0])
		return nil
	},
}

var logoutCmd = &cobra.Command{
	Use:   "logout <name>",
	Short: "Forget the OAuth tokens of an HTTP MCP server",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		found, err := findServer(args[0], "")
		if err != nil {
			return err
		}
		removed, err := env.Tokens().Delete(found.server.Config.Expanded(nil).URL)
		if err != nil {
			return err
		}
		if !removed {
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Not logged in to %s.\n", args[0])
			return nil
		}
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Logged out of %s.\n", args[0])
		return nil
	},
}

var importCmd = &cobra.Command{
	Use:   "import [names...]",
	Short: "Copy MCP servers from Claude Code, Codex, Cursor, VS Code or Claude Desktop into your user config",
	Long: `Copy MCP servers from Claude Code (~/.claude.json), Codex (~/.codex/config.toml),
Cursor (~/.cursor/mcp.json, ./.cursor/mcp.json), VS Code (./.vscode/mcp.json) and
Claude Desktop into ~/.rootly-cli/mcp.json. Without names it asks which ones to copy.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		dir, err := cwd()
		if err != nil {
			return err
		}
		store := env.Store(dir)
		discovered, problems := env.Discovery(dir).Imported()
		errOut := cmd.ErrOrStderr()
		for _, problem := range problems {
			_, _ = fmt.Fprintf(errOut, "warning: could not read %s (%s): %v\n", problem.Source.Label, displayPath(problem.Source.Path), problem.Err)
		}
		if len(discovered) == 0 {
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), "No MCP servers found in Claude Code, Codex, Cursor, VS Code or Claude Desktop configs.")
			return nil
		}
		selected, err := selectImports(cmd, discovered, args)
		if err != nil {
			return err
		}
		existing, _ := store.Servers(localmcp.ScopeUser)
		taken := map[string]bool{}
		for _, server := range existing {
			taken[server.Name] = true
		}
		copied := 0
		for _, server := range selected {
			if taken[server.Name] {
				_, _ = fmt.Fprintf(errOut, "skipped %s: already in your user config\n", server.Name)
				continue
			}
			if err := store.Add(localmcp.ScopeUser, server.Name, server.Config); err != nil {
				_, _ = fmt.Fprintf(errOut, "skipped %s: %v\n", server.Name, err)
				continue
			}
			taken[server.Name] = true
			copied++
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Imported %s from %s\n", server.Name, server.SourceLabel)
		}
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Copied %d server(s) to %s\n", copied, scopeTarget(store, localmcp.ScopeUser))
		return nil
	},
}

func selectImports(cmd *cobra.Command, discovered []localmcp.Server, names []string) ([]localmcp.Server, error) {
	if importAll {
		return discovered, nil
	}
	if len(names) > 0 {
		wanted := map[string]bool{}
		for _, name := range names {
			wanted[name] = true
		}
		var selected []localmcp.Server
		for _, server := range discovered {
			if wanted[server.Name] {
				selected = append(selected, server)
				delete(wanted, server.Name)
			}
		}
		if len(wanted) > 0 {
			missing := make([]string, 0, len(wanted))
			for name := range wanted {
				missing = append(missing, name)
			}
			sort.Strings(missing)
			return nil, fmt.Errorf("not found in other tools' configs: %s", strings.Join(missing, ", "))
		}
		return selected, nil
	}
	if !env.IsTTY() {
		return nil, errors.New("pass server names or --all when not running in a terminal")
	}
	out := cmd.ErrOrStderr()
	_, _ = fmt.Fprintln(out, "MCP servers found in other tools:")
	for index, server := range discovered {
		_, _ = fmt.Fprintf(out, "  %2d. %-24s %s · from %s\n", index+1, server.Name, server.Config.Origin(), server.SourceLabel)
	}
	_, _ = fmt.Fprint(out, "Copy which into rootly? (e.g. 1,3 · all · Enter for none): ")
	reader := bufio.NewReader(env.Stdin)
	line, _ := reader.ReadString('\n')
	return parseSelection(strings.TrimSpace(line), discovered)
}

func parseSelection(input string, discovered []localmcp.Server) ([]localmcp.Server, error) {
	if input == "" {
		return nil, nil
	}
	if strings.EqualFold(input, "all") || input == "*" {
		return discovered, nil
	}
	var selected []localmcp.Server
	seen := map[int]bool{}
	for _, field := range strings.FieldsFunc(input, func(r rune) bool { return r == ',' || r == ' ' }) {
		number, err := strconv.Atoi(field)
		if err != nil || number < 1 || number > len(discovered) {
			return nil, fmt.Errorf("invalid selection %q: use numbers between 1 and %d", field, len(discovered))
		}
		if !seen[number] {
			seen[number] = true
			selected = append(selected, discovered[number-1])
		}
	}
	return selected, nil
}

func init() {
	addCmd.Flags().StringVarP(&addTransport, "transport", "t", "", "Transport: stdio, http or sse (default stdio)")
	addCmd.Flags().StringVarP(&addScope, "scope", "s", localmcp.ScopeLocal, "Config scope: local, user or project")
	addCmd.Flags().StringArrayVarP(&addEnv, "env", "e", nil, "Environment variable for stdio servers (KEY=value, repeatable)")
	addCmd.Flags().StringArrayVarP(&addHeaders, "header", "H", nil, `HTTP header ("Name: value", repeatable)`)
	addCmd.Flags().StringVar(&addURL, "url", "", "Server URL (Codex syntax; implies --transport http)")
	addCmd.Flags().StringVar(&addBearerEnv, "bearer-token-env-var", "", "Env var holding a bearer token for http servers (Codex syntax)")
	addJSONCmd.Flags().StringVarP(&addJSONScope, "scope", "s", localmcp.ScopeLocal, "Config scope: local, user or project")
	removeCmd.Flags().StringVarP(&removeScope, "scope", "s", "", "Config scope to remove from: local, user or project")
	importCmd.Flags().BoolVar(&importAll, "all", false, "Copy every discovered server")

	MCPCmd.AddCommand(addCmd, addJSONCmd, listCmd, getCmd, removeCmd, loginCmd, logoutCmd, importCmd)
}
