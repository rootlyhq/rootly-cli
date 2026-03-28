package oauth

import (
	"context"
	"crypto/rand"
	"encoding/base64"

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

// TokenSourceFromStored creates a token source that auto-refreshes using stored tokens.
func TokenSourceFromStored(cfg *oauth2.Config) (oauth2.TokenSource, error) {
	stored, err := LoadTokens()
	if err != nil {
		return nil, err
	}
	tok := stored.ToOAuth2Token()
	return cfg.TokenSource(context.Background(), tok), nil
}
