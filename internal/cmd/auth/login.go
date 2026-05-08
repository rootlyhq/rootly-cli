package auth

import (
	"context"
	"errors"
	"fmt"
	"html"
	"net"
	"net/http"
	"os/exec"
	"runtime"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"golang.org/x/oauth2"

	"github.com/rootlyhq/rootly-cli/internal/config"
	xoauth "github.com/rootlyhq/rootly-cli/internal/oauth"
)

var LoginCmd = &cobra.Command{
	Use:   "login",
	Short: "Authenticate with Rootly via browser-based OAuth2",
	Long: `Opens your browser to authenticate with Rootly using OAuth2 Authorization Code + PKCE.

No configuration is needed — just run "rootly login" and follow the browser prompts.
By default, connects to api.rootly.com. Use --api-host to target a different environment.`,
	Example: `  # Login to Rootly (production)
  rootly login

  # Login to a local dev server
  rootly login --api-host=localhost:22166`,
	Annotations: map[string]string{"skipAuth": "true"},
	RunE:        runLogin,
}

func init() {
	LoginCmd.Flags().String("client-id", "", "Override OAuth2 client ID (for debugging)")
	_ = LoginCmd.Flags().MarkHidden("client-id")
}

// resolveRegistration returns a client ID and scopes, registering dynamically if needed.
func resolveRegistration(ctx context.Context, authBaseURL string, cmd *cobra.Command) (clientID string, scopes []string, err error) {
	if clientID, _ := cmd.Flags().GetString("client-id"); clientID != "" {
		return clientID, nil, nil
	}
	if clientID, scopes := xoauth.LoadCachedRegistration(); clientID != "" {
		return clientID, scopes, nil
	}
	return registerAndCache(ctx, authBaseURL, cmd)
}

// registerAndCache calls POST /oauth/register and saves the client_id and scopes.
func registerAndCache(ctx context.Context, authBaseURL string, cmd *cobra.Command) (clientID string, scopes []string, err error) {
	_, _ = fmt.Fprintf(cmd.OutOrStderr(), "Registering OAuth client...\n")
	clientID, scopes, err = xoauth.RegisterClient(ctx, authBaseURL)
	if err != nil {
		return "", nil, err
	}
	if saveErr := xoauth.SaveRegistration(clientID, scopes); saveErr != nil {
		return "", nil, fmt.Errorf("failed to cache registration: %w", saveErr)
	}
	return clientID, scopes, nil
}

// httpClientWithTimeout is used for pre-flight checks (HEAD) and registration.
var httpClientWithTimeout = &http.Client{Timeout: 10 * time.Second}

func runLogin(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()

	apiHost := viper.GetString("api_host")
	if apiHost == "" {
		apiHost = config.DefaultEndpoint
	}

	authBaseURL := xoauth.DeriveAuthBaseURL(apiHost)

	clientID, scopes, err := resolveRegistration(ctx, authBaseURL, cmd)
	if err != nil {
		return err
	}

	tok, err := doOAuthFlow(ctx, cmd, authBaseURL, clientID, scopes)
	if err != nil {
		// If authorize returned 404, the cached client_id may be stale — re-register once
		if isAuthorize404(err) {
			_, _ = fmt.Fprintf(cmd.OutOrStderr(), "OAuth client not found, re-registering...\n")
			_ = xoauth.ClearRegistration()
			clientID, scopes, regErr := registerAndCache(ctx, authBaseURL, cmd)
			if regErr != nil {
				return regErr
			}
			tok, err = doOAuthFlow(ctx, cmd, authBaseURL, clientID, scopes)
			if err != nil {
				return err
			}
		} else {
			return err
		}
	}

	if err := xoauth.SaveOAuth2Token(tok); err != nil {
		return fmt.Errorf("failed to save tokens: %w", err)
	}

	_, _ = fmt.Fprintf(cmd.OutOrStderr(), "Login successful! Tokens saved.\n")
	return nil
}

// errAuthorize404 is returned when the authorize endpoint returns 404.
var errAuthorize404 = errors.New("authorization endpoint returned 404")

func isAuthorize404(err error) bool {
	return errors.Is(err, errAuthorize404)
}

func doOAuthFlow(ctx context.Context, cmd *cobra.Command, authBaseURL, clientID string, scopes []string) (*oauth2.Token, error) {
	cfg := xoauth.NewConfig(authBaseURL, clientID, scopes)

	verifier := oauth2.GenerateVerifier()

	state, err := xoauth.GenerateState()
	if err != nil {
		return nil, fmt.Errorf("failed to generate state: %w", err)
	}

	// Build authorization URL with PKCE (S256 challenge derived from verifier)
	authURL := cfg.AuthCodeURL(state, oauth2.S256ChallengeOption(verifier))

	// Pre-check: HEAD the authorize URL to detect 404 (stale client_id)
	// Done before starting the callback server to avoid binding a port unnecessarily.
	headReq, headReqErr := http.NewRequestWithContext(ctx, http.MethodHead, authURL, http.NoBody)
	if headReqErr == nil {
		if headResp, headErr := httpClientWithTimeout.Do(headReq); headErr == nil {
			_ = headResp.Body.Close()
			if headResp.StatusCode == http.StatusNotFound {
				return nil, errAuthorize404
			}
		}
	}

	// Channel to receive the authorization code
	codeCh := make(chan string, 1)
	errCh := make(chan error, 1)

	// Start local callback server
	mux := http.NewServeMux()
	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("state") != state {
			errCh <- fmt.Errorf("state mismatch")
			http.Error(w, "State mismatch", http.StatusBadRequest)
			return
		}
		if errMsg := r.URL.Query().Get("error"); errMsg != "" {
			desc := r.URL.Query().Get("error_description")
			errCh <- fmt.Errorf("authorization error: %s — %s", errMsg, desc)
			_, _ = fmt.Fprintf(w, "<html><body><h1>Authorization Failed</h1><p>%s</p><p>You can close this window.</p></body></html>", html.EscapeString(desc))
			return
		}
		code := r.URL.Query().Get("code")
		if code == "" {
			errCh <- fmt.Errorf("no code in callback")
			http.Error(w, "Missing code", http.StatusBadRequest)
			return
		}
		codeCh <- code
		_, _ = fmt.Fprint(w, "<html><body><h1>Login Successful!</h1><p>You can close this window and return to the terminal.</p></body></html>")
	})

	lc := net.ListenConfig{}
	listener, err := lc.Listen(ctx, "tcp", "localhost:"+xoauth.CallbackPort)
	if err != nil {
		return nil, fmt.Errorf("failed to start callback server on port %s: %w", xoauth.CallbackPort, err)
	}

	server := &http.Server{Handler: mux}
	go func() { _ = server.Serve(listener) }()
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()

	_, _ = fmt.Fprintf(cmd.OutOrStderr(), "Opening browser for authentication...\n")
	_, _ = fmt.Fprintf(cmd.OutOrStderr(), "If the browser doesn't open, visit:\n%s\n\n", authURL)

	if err := openBrowser(ctx, authURL); err != nil {
		_, _ = fmt.Fprintf(cmd.OutOrStderr(), "Failed to open browser: %v\n", err)
	}

	_, _ = fmt.Fprintf(cmd.OutOrStderr(), "Waiting for authorization...\n")

	// Wait for callback
	var code string
	select {
	case code = <-codeCh:
	case err := <-errCh:
		return nil, err
	case <-time.After(5 * time.Minute):
		return nil, fmt.Errorf("login timed out after 5 minutes")
	}

	// Exchange code for tokens
	tok, err := xoauth.ExchangeCode(ctx, cfg, code, verifier)
	if err != nil {
		return nil, err
	}

	return tok, nil
}

func openBrowser(ctx context.Context, url string) error {
	switch runtime.GOOS {
	case "darwin":
		return exec.CommandContext(ctx, "open", url).Start()
	case "linux":
		return exec.CommandContext(ctx, "xdg-open", url).Start()
	case "windows":
		return exec.CommandContext(ctx, "rundll32", "url.dll,FileProtocolHandler", url).Start()
	default:
		return fmt.Errorf("unsupported platform")
	}
}
