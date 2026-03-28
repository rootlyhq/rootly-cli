package oauth

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"golang.org/x/oauth2"

	"github.com/rootlyhq/rootly-cli/internal/config"
)

const (
	CallbackPort = "19797"
	RedirectURI  = "http://localhost:" + CallbackPort + "/callback"
)

// NewConfig creates an oauth2.Config for the given auth base URL and client ID.
func NewConfig(authBaseURL, clientID string) *oauth2.Config {
	return &oauth2.Config{
		ClientID:    clientID,
		RedirectURL: RedirectURI,
		Scopes:      []string{"openid", "profile", "email", "all"},
		Endpoint: oauth2.Endpoint{
			AuthURL:   authBaseURL + "/oauth/authorize",
			TokenURL:  authBaseURL + "/oauth/token",
			AuthStyle: oauth2.AuthStyleInParams,
		},
	}
}

// registrationRequest is the payload for POST /oauth/register.
type registrationRequest struct {
	ClientName              string   `json:"client_name"`
	RedirectURIs            []string `json:"redirect_uris"`
	TokenEndpointAuthMethod string   `json:"token_endpoint_auth_method"`
	GrantTypes              []string `json:"grant_types"`
	ResponseTypes           []string `json:"response_types"`
}

// registrationResponse is the response from POST /oauth/register.
type registrationResponse struct {
	ClientID string `json:"client_id"`
}

// RegisterClient dynamically registers an OAuth client and returns the client_id.
func RegisterClient(ctx context.Context, authBaseURL string) (string, error) {
	reqBody := registrationRequest{
		ClientName:              "Rootly CLI",
		RedirectURIs:            []string{RedirectURI},
		TokenEndpointAuthMethod: "none",
		GrantTypes:              []string{"authorization_code"},
		ResponseTypes:           []string{"code"},
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal registration request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, authBaseURL+"/oauth/register", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("failed to create registration request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to register OAuth client: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusCreated {
		return "", fmt.Errorf("could not register OAuth client (status %d)", resp.StatusCode)
	}

	var regResp registrationResponse
	if err := json.NewDecoder(resp.Body).Decode(&regResp); err != nil {
		return "", fmt.Errorf("failed to parse registration response: %w", err)
	}

	if regResp.ClientID == "" {
		return "", fmt.Errorf("registration response missing client_id")
	}

	return regResp.ClientID, nil
}

// LoadCachedClientID reads the cached OAuth client_id from config.
func LoadCachedClientID() string {
	cfg, err := config.Load()
	if err != nil {
		return ""
	}
	return cfg.ClientID
}

// SaveClientID persists the OAuth client_id to config, preserving other fields.
func SaveClientID(clientID string) error {
	cfg, err := config.Load()
	if err != nil {
		cfg = &config.Config{}
	}
	cfg.ClientID = clientID
	return config.Save(cfg)
}

// ClearClientID removes the cached client_id from config.
func ClearClientID() error {
	cfg, err := config.Load()
	if err != nil {
		return nil // No config file means nothing to clear
	}
	cfg.ClientID = ""
	return config.Save(cfg)
}

// GenerateState creates a cryptographically random state parameter.
func GenerateState() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// ExchangeCode exchanges an authorization code for tokens using PKCE.
func ExchangeCode(ctx context.Context, cfg *oauth2.Config, code, codeVerifier string) (*oauth2.Token, error) {
	return cfg.Exchange(ctx, code, oauth2.VerifierOption(codeVerifier))
}

// DeriveAuthBaseURL builds the OAuth base URL from the API host.
// For api.rootly.com it returns https://app.rootly.com.
// For localhost it returns http://localhost:<port>.
func DeriveAuthBaseURL(apiHost string) string {
	// Strip scheme to normalize, then re-apply appropriate scheme
	scheme := ""
	host := apiHost
	if strings.HasPrefix(apiHost, "http://") {
		scheme = "http://"
		host = apiHost[7:]
	} else if strings.HasPrefix(apiHost, "https://") {
		scheme = "https://"
		host = apiHost[8:]
	}

	// Strip /api suffix (used for localhost API endpoints, not OAuth)
	host = strings.TrimSuffix(host, "/api")

	if strings.HasPrefix(host, "localhost") || strings.HasPrefix(host, "127.0.0.1") {
		if scheme == "" {
			scheme = "http://"
		}
		return scheme + host
	}
	if strings.HasPrefix(host, "api.") {
		return "https://app." + host[4:]
	}
	if scheme == "" {
		scheme = "https://"
	}
	return scheme + host
}

// TokenSourceFromStored creates a token source that auto-refreshes using stored tokens.
func TokenSourceFromStored(cfg *oauth2.Config) (oauth2.TokenSource, error) {
	stored, err := LoadTokens()
	if err != nil {
		return nil, err
	}
	tok := ToOAuth2Token(stored)
	return cfg.TokenSource(context.Background(), tok), nil
}
