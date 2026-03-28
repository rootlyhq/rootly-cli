package oauth

import (
	"fmt"
	"net/http"
	"strings"

	"golang.org/x/oauth2"
)

// NewHTTPClient creates an http.Client that uses stored OAuth tokens with auto-refresh.
// The userAgent is set on all requests. The base transport is used for underlying HTTP calls.
func NewHTTPClient(cfg *oauth2.Config, base http.RoundTripper, userAgent string) (*http.Client, error) {
	ts, err := TokenSourceFromStored(cfg)
	if err != nil {
		return nil, err
	}

	// Wrap the token source to save refreshed tokens
	ts = &persistingTokenSource{
		base: ts,
	}

	transport := &oauth2.Transport{
		Source: ts,
		Base:   base,
	}

	// Wrap with user-agent transport
	return &http.Client{
		Transport: &userAgentTransport{
			base:      transport,
			userAgent: userAgent,
		},
	}, nil
}

// persistingTokenSource wraps a TokenSource and saves refreshed tokens to disk.
type persistingTokenSource struct {
	base            oauth2.TokenSource
	lastAccessToken string
}

func (p *persistingTokenSource) Token() (*oauth2.Token, error) {
	tok, err := p.base.Token()
	if err != nil {
		// Surface a user-friendly message when refresh fails
		errMsg := err.Error()
		if strings.Contains(errMsg, "token") || strings.Contains(errMsg, "401") || strings.Contains(errMsg, "invalid_grant") {
			return nil, fmt.Errorf("session expired — run 'rootly login' to re-authenticate: %w", err)
		}
		return nil, err
	}
	// Only save when the token was actually refreshed (new access token)
	if tok.AccessToken != p.lastAccessToken {
		p.lastAccessToken = tok.AccessToken
		_ = SaveOAuth2Token(tok)
	}
	return tok, nil
}

// userAgentTransport sets User-Agent on all requests.
type userAgentTransport struct {
	base      http.RoundTripper
	userAgent string
}

func (t *userAgentTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req2 := req.Clone(req.Context())
	if t.userAgent != "" {
		req2.Header.Set("User-Agent", t.userAgent)
	}
	return t.base.RoundTrip(req2)
}
