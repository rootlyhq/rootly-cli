package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/oauthex"
	"golang.org/x/oauth2"

	"github.com/rootlyhq/rootly-cli/internal/config"
)

const tokenFile = "mcp-oauth.json"

// ErrNeedsAuth means an HTTP server asked for OAuth and no usable token is stored.
var ErrNeedsAuth = errors.New("needs authentication")

type StoredCredential struct {
	ClientID     string    `json:"client_id"`
	ClientSecret string    `json:"client_secret,omitempty"`
	AuthURL      string    `json:"auth_url"`
	TokenURL     string    `json:"token_url"`
	AuthStyle    int       `json:"auth_style,omitempty"`
	Scopes       []string  `json:"scopes,omitempty"`
	RedirectURL  string    `json:"redirect_url,omitempty"`
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token,omitempty"`
	TokenType    string    `json:"token_type,omitempty"`
	Expiry       time.Time `json:"expiry,omitempty"`
}

func (c StoredCredential) oauthConfig() *oauth2.Config {
	return &oauth2.Config{
		ClientID:     c.ClientID,
		ClientSecret: c.ClientSecret,
		Endpoint:     oauth2.Endpoint{AuthURL: c.AuthURL, TokenURL: c.TokenURL, AuthStyle: oauth2.AuthStyle(c.AuthStyle)},
		Scopes:       c.Scopes,
		RedirectURL:  c.RedirectURL,
	}
}

func (c StoredCredential) token() *oauth2.Token {
	return &oauth2.Token{AccessToken: c.AccessToken, RefreshToken: c.RefreshToken, TokenType: c.TokenType, Expiry: c.Expiry}
}

// TokenStore keeps MCP OAuth credentials keyed by server URL in a 0600 file.
type TokenStore struct {
	Path string
	mu   sync.Mutex
}

func NewTokenStore() *TokenStore {
	return &TokenStore{Path: filepath.Join(config.Dir(), tokenFile)}
}

func (s *TokenStore) readAll() (map[string]StoredCredential, error) {
	credentials := map[string]StoredCredential{}
	data, err := os.ReadFile(s.Path)
	if err != nil {
		if os.IsNotExist(err) {
			return credentials, nil
		}
		return nil, err
	}
	if len(data) == 0 {
		return credentials, nil
	}
	if err := json.Unmarshal(data, &credentials); err != nil {
		return nil, fmt.Errorf("parse %s: %w", s.Path, err)
	}
	return credentials, nil
}

func (s *TokenStore) writeAll(credentials map[string]StoredCredential) error {
	document := jsonDocument{}
	for key, credential := range credentials {
		encoded, err := json.Marshal(credential)
		if err != nil {
			return err
		}
		document[key] = encoded
	}
	if err := writeJSONDocument(s.Path, document, 0o600); err != nil {
		return err
	}
	return os.Chmod(s.Path, 0o600)
}

func (s *TokenStore) Get(serverURL string) (StoredCredential, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	credentials, err := s.readAll()
	if err != nil {
		return StoredCredential{}, false
	}
	credential, ok := credentials[tokenKey(serverURL)]
	return credential, ok && credential.AccessToken != ""
}

func (s *TokenStore) Put(serverURL string, credential StoredCredential) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	credentials, err := s.readAll()
	if err != nil {
		return err
	}
	credentials[tokenKey(serverURL)] = credential
	return s.writeAll(credentials)
}

func (s *TokenStore) Delete(serverURL string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	credentials, err := s.readAll()
	if err != nil {
		return false, err
	}
	key := tokenKey(serverURL)
	if _, ok := credentials[key]; !ok {
		return false, nil
	}
	delete(credentials, key)
	return true, s.writeAll(credentials)
}

func tokenKey(serverURL string) string {
	return strings.TrimRight(strings.TrimSpace(serverURL), "/")
}

type persistingTokenSource struct {
	store     *TokenStore
	serverURL string
	base      StoredCredential
	source    oauth2.TokenSource
	mu        sync.Mutex
	last      string
}

func (p *persistingTokenSource) Token() (*oauth2.Token, error) {
	token, err := p.source.Token()
	if err != nil {
		return nil, err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if token.AccessToken != p.last {
		p.last = token.AccessToken
		credential := p.base
		credential.AccessToken = token.AccessToken
		credential.TokenType = token.TokenType
		credential.Expiry = token.Expiry
		if token.RefreshToken != "" {
			credential.RefreshToken = token.RefreshToken
		}
		p.base = credential
		_ = p.store.Put(p.serverURL, credential)
	}
	return token, nil
}

func (s *TokenStore) tokenSource(serverURL string, credential StoredCredential, cfg *oauth2.Config, token *oauth2.Token, client *http.Client) oauth2.TokenSource {
	ctx := context.Background()
	if client != nil {
		ctx = context.WithValue(ctx, oauth2.HTTPClient, client)
	}
	return &persistingTokenSource{store: s, serverURL: serverURL, base: credential, source: cfg.TokenSource(ctx, token), last: token.AccessToken}
}

type LoginOptions struct {
	// OpenURL shows the authorization URL to the user (browser + print).
	OpenURL func(string)
	Client  *http.Client
}

// Login runs the MCP authorization flow (protected-resource discovery,
// dynamic client registration, PKCE, loopback redirect) and stores the token.
func (s *TokenStore) Login(ctx context.Context, server ServerConfig, options LoginOptions) error {
	if server.Transport() == TransportStdio {
		return errors.New("OAuth login only applies to http and sse servers")
	}
	serverURL := server.URL
	listener, err := (&net.ListenConfig{}).Listen(ctx, "tcp", "127.0.0.1:0")
	if err != nil {
		return fmt.Errorf("start OAuth callback listener: %w", err)
	}
	defer func() { _ = listener.Close() }()
	redirectURL := fmt.Sprintf("http://127.0.0.1:%d/callback", listener.Addr().(*net.TCPAddr).Port)

	results := make(chan *auth.AuthorizationResult, 1)
	failures := make(chan error, 1)
	httpServer := &http.Server{ReadHeaderTimeout: 10 * time.Second, Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/callback" {
			http.NotFound(w, r)
			return
		}
		query := r.URL.Query()
		if problem := query.Get("error"); problem != "" {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = fmt.Fprintf(w, "<html><body><h2>Authorization failed</h2><p>%s</p></body></html>", html.EscapeString(problem))
			select {
			case failures <- fmt.Errorf("authorization failed: %s", problem):
			default:
			}
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = io.WriteString(w, "<html><body><h2>Rootly CLI is connected.</h2><p>You can close this tab.</p></body></html>")
		select {
		case results <- &auth.AuthorizationResult{Code: query.Get("code"), State: query.Get("state"), Iss: query.Get("iss")}:
		default:
		}
	})}
	go func() { _ = httpServer.Serve(listener) }()
	defer func() { _ = httpServer.Close() }()

	client := options.Client
	if client == nil {
		client = http.DefaultClient
	}
	var saveErr error
	handler, err := auth.NewAuthorizationCodeHandler(&auth.AuthorizationCodeHandlerConfig{
		DynamicClientRegistrationConfig: &auth.DynamicClientRegistrationConfig{Metadata: &oauthex.ClientRegistrationMetadata{
			ClientName:              "Rootly CLI",
			RedirectURIs:            []string{redirectURL},
			GrantTypes:              []string{"authorization_code", "refresh_token"},
			ResponseTypes:           []string{"code"},
			TokenEndpointAuthMethod: "none",
		}},
		RedirectURL:         redirectURL,
		RequestRefreshToken: true,
		Client:              client,
		AuthorizationCodeFetcher: func(ctx context.Context, args *auth.AuthorizationArgs) (*auth.AuthorizationResult, error) {
			if options.OpenURL != nil {
				options.OpenURL(args.URL)
			}
			select {
			case result := <-results:
				return result, nil
			case err := <-failures:
				return nil, err
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		},
		NewTokenSource: func(_ context.Context, cfg *oauth2.Config, token *oauth2.Token) (oauth2.TokenSource, error) {
			credential := StoredCredential{
				ClientID: cfg.ClientID, ClientSecret: cfg.ClientSecret,
				AuthURL: cfg.Endpoint.AuthURL, TokenURL: cfg.Endpoint.TokenURL, AuthStyle: int(cfg.Endpoint.AuthStyle),
				Scopes: cfg.Scopes, RedirectURL: cfg.RedirectURL,
				AccessToken: token.AccessToken, RefreshToken: token.RefreshToken, TokenType: token.TokenType, Expiry: token.Expiry,
			}
			saveErr = s.Put(serverURL, credential)
			return s.tokenSource(serverURL, credential, cfg, token, client), nil
		},
	})
	if err != nil {
		return err
	}

	request, response, err := probe(ctx, client, server)
	if err != nil {
		return err
	}
	if response.StatusCode != http.StatusUnauthorized && response.StatusCode != http.StatusForbidden {
		_ = response.Body.Close()
		return fmt.Errorf("%s did not ask for authorization (HTTP %d); no login needed", serverURL, response.StatusCode)
	}
	if err := handler.Authorize(ctx, request, response); err != nil {
		return err
	}
	return saveErr
}

func probe(ctx context.Context, client *http.Client, server ServerConfig) (*http.Request, *http.Response, error) {
	method, body := http.MethodPost, `{"jsonrpc":"2.0","id":0,"method":"ping"}`
	if server.Transport() == TransportSSE {
		method, body = http.MethodGet, ""
	}
	request, err := http.NewRequestWithContext(ctx, method, server.URL, strings.NewReader(body))
	if err != nil {
		return nil, nil, err
	}
	request.Header.Set("Accept", "application/json, text/event-stream")
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	for key, value := range server.Headers {
		request.Header.Set(key, value)
	}
	response, err := client.Do(request)
	if err != nil {
		return nil, nil, err
	}
	return request, response, nil
}

func OpenBrowser(url string) error {
	switch runtime.GOOS {
	case "darwin":
		return exec.CommandContext(context.Background(), "open", url).Start()
	case "windows":
		return exec.CommandContext(context.Background(), "rundll32", "url.dll,FileProtocolHandler", url).Start()
	default:
		return exec.CommandContext(context.Background(), "xdg-open", url).Start()
	}
}

// ProbeStatus sends one unauthenticated MCP request and returns the HTTP status.
func ProbeStatus(ctx context.Context, client *http.Client, server ServerConfig) (int, error) {
	if client == nil {
		client = http.DefaultClient
	}
	_, response, err := probe(ctx, client, server)
	if err != nil {
		return 0, err
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 64*1024))
	_ = response.Body.Close()
	return response.StatusCode, nil
}
