package oauth

import (
	"os"
	"time"

	"golang.org/x/oauth2"
	"gopkg.in/yaml.v3"

	"github.com/rootlyhq/rootly-cli/internal/config"
)

// TokenData is an alias for config.OAuthData used within the oauth package.
type TokenData = config.OAuthData

// LoadTokens reads OAuth tokens from ~/.rootly-cli/config.yaml.
func LoadTokens() (*TokenData, error) {
	data, err := os.ReadFile(config.Path())
	if err != nil {
		return nil, err
	}
	var cfg config.Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	if cfg.OAuth == nil || cfg.OAuth.AccessToken == "" {
		return nil, os.ErrNotExist
	}
	return cfg.OAuth, nil
}

// SaveTokens writes OAuth tokens into ~/.rootly-cli/config.yaml,
// preserving all other config fields.
func SaveTokens(t *TokenData) error {
	path := config.Path()

	// Read existing config to preserve other fields
	var cfg config.Config
	if data, err := os.ReadFile(path); err == nil {
		_ = yaml.Unmarshal(data, &cfg)
	}

	cfg.OAuth = t

	if err := os.MkdirAll(config.Dir(), 0700); err != nil {
		return err
	}
	data, err := yaml.Marshal(&cfg)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}

// SaveOAuth2Token converts and persists an oauth2.Token.
func SaveOAuth2Token(tok *oauth2.Token) error {
	return SaveTokens(TokenDataFromOAuth2(tok))
}

// HasTokens returns true if OAuth tokens exist in the config file (cheap check).
func HasTokens() bool {
	data, err := os.ReadFile(config.Path())
	if err != nil {
		return false
	}
	// Quick check without full unmarshal
	var cfg struct {
		OAuth *struct {
			AccessToken string `yaml:"access_token"`
		} `yaml:"oauth"`
	}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return false
	}
	return cfg.OAuth != nil && cfg.OAuth.AccessToken != ""
}

// ClearTokens removes OAuth tokens from ~/.rootly-cli/config.yaml,
// preserving all other config fields.
func ClearTokens() error {
	path := config.Path()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	var cfg config.Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return err
	}

	if cfg.OAuth == nil {
		return nil
	}

	cfg.OAuth = nil

	out, err := yaml.Marshal(&cfg)
	if err != nil {
		return err
	}
	return os.WriteFile(path, out, 0600)
}

// IsExpired returns true if the token is expired or within 30s of expiring.
func IsExpired(t *TokenData) bool {
	return time.Now().After(t.ExpiresAt.Add(-30 * time.Second))
}

// ToOAuth2Token converts stored token data to an oauth2.Token.
func ToOAuth2Token(t *TokenData) *oauth2.Token {
	return &oauth2.Token{
		AccessToken:  t.AccessToken,
		RefreshToken: t.RefreshToken,
		TokenType:    t.TokenType,
		Expiry:       t.ExpiresAt,
	}
}

// TokenDataFromOAuth2 converts an oauth2.Token to TokenData for storage.
func TokenDataFromOAuth2(tok *oauth2.Token) *TokenData {
	return &TokenData{
		AccessToken:  tok.AccessToken,
		RefreshToken: tok.RefreshToken,
		ExpiresAt:    tok.Expiry,
		TokenType:    tok.TokenType,
	}
}
