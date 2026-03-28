package oauth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"strings"

	"golang.org/x/oauth2"
)

const (
	ClientID    = "rootly-cli"
	RedirectURI = "http://localhost:19797/callback"
)

// NewConfig creates an oauth2.Config for the given auth base URL.
func NewConfig(authBaseURL string) *oauth2.Config {
	return &oauth2.Config{
		ClientID:    ClientID,
		RedirectURL: RedirectURI,
		Scopes:      []string{"openid", "profile", "email", "all"},
		Endpoint: oauth2.Endpoint{
			AuthURL:   authBaseURL + "/oauth/authorize",
			TokenURL:  authBaseURL + "/oauth/token",
			AuthStyle: oauth2.AuthStyleInParams,
		},
	}
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
	if strings.HasPrefix(apiHost, "http://") || strings.HasPrefix(apiHost, "https://") {
		return apiHost
	}
	if strings.HasPrefix(apiHost, "localhost") || strings.HasPrefix(apiHost, "127.0.0.1") {
		return "http://" + apiHost
	}
	if strings.HasPrefix(apiHost, "api.") {
		return "https://app." + apiHost[4:]
	}
	return "https://" + apiHost
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
