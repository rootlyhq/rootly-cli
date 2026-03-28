package oauth

import (
	"testing"
)

func TestNewConfig(t *testing.T) {
	cfg := NewConfig("https://app.rootly.com")

	if cfg.ClientID != "rootly-cli" {
		t.Errorf("ClientID = %q", cfg.ClientID)
	}
	if cfg.RedirectURL != "http://localhost:19797/callback" {
		t.Errorf("RedirectURL = %q", cfg.RedirectURL)
	}
	if cfg.Endpoint.AuthURL != "https://app.rootly.com/oauth/authorize" {
		t.Errorf("AuthURL = %q", cfg.Endpoint.AuthURL)
	}
	if cfg.Endpoint.TokenURL != "https://app.rootly.com/oauth/token" {
		t.Errorf("TokenURL = %q", cfg.Endpoint.TokenURL)
	}
	if len(cfg.Scopes) != 4 {
		t.Errorf("Scopes = %v", cfg.Scopes)
	}
}

func TestNewConfig_Localhost(t *testing.T) {
	cfg := NewConfig("http://localhost:22166")

	if cfg.Endpoint.AuthURL != "http://localhost:22166/oauth/authorize" {
		t.Errorf("AuthURL = %q", cfg.Endpoint.AuthURL)
	}
	if cfg.Endpoint.TokenURL != "http://localhost:22166/oauth/token" {
		t.Errorf("TokenURL = %q", cfg.Endpoint.TokenURL)
	}
}

func TestDeriveAuthBaseURL(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"api.rootly.com", "https://app.rootly.com"},
		{"api.staging.rootly.com", "https://app.staging.rootly.com"},
		{"localhost:22166", "http://localhost:22166"},
		{"localhost:22166/api", "http://localhost:22166/api"},
		{"127.0.0.1:3000", "http://127.0.0.1:3000"},
		{"http://localhost:22166", "http://localhost:22166"},
		{"https://custom.example.com", "https://custom.example.com"},
		{"custom.example.com", "https://custom.example.com"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := DeriveAuthBaseURL(tt.input)
			if got != tt.want {
				t.Errorf("DeriveAuthBaseURL(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
