package oauth

import (
	"os"
	"path/filepath"
	"time"

	"golang.org/x/oauth2"
	"gopkg.in/yaml.v3"
)

const (
	tokenDir  = ".rootly-cli"
	tokenFile = "tokens.yaml"
)

type TokenData struct {
	AccessToken  string    `yaml:"access_token"`
	RefreshToken string    `yaml:"refresh_token"`
	ExpiresAt    time.Time `yaml:"expires_at"`
	TokenType    string    `yaml:"token_type"`
}

func tokenPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, tokenDir, tokenFile)
}

func LoadTokens() (*TokenData, error) {
	data, err := os.ReadFile(tokenPath())
	if err != nil {
		return nil, err
	}
	var t TokenData
	if err := yaml.Unmarshal(data, &t); err != nil {
		return nil, err
	}
	return &t, nil
}

func SaveTokens(t *TokenData) error {
	dir := filepath.Dir(tokenPath())
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	data, err := yaml.Marshal(t)
	if err != nil {
		return err
	}
	return os.WriteFile(tokenPath(), data, 0600)
}

// SaveOAuth2Token converts and persists an oauth2.Token.
func SaveOAuth2Token(tok *oauth2.Token) error {
	return SaveTokens(TokenDataFromOAuth2(tok))
}

// HasTokens returns true if a token file exists (cheap stat, no parsing).
func HasTokens() bool {
	_, err := os.Stat(tokenPath())
	return err == nil
}

func ClearTokens() error {
	path := tokenPath()
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil
	}
	return os.Remove(path)
}

func (t *TokenData) IsExpired() bool {
	return time.Now().After(t.ExpiresAt.Add(-30 * time.Second))
}

// ToOAuth2Token converts stored token data to an oauth2.Token.
func (t *TokenData) ToOAuth2Token() *oauth2.Token {
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
