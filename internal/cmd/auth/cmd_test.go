package auth

import (
	"bytes"
	"os"
	"testing"
	"time"

	"github.com/rootlyhq/rootly-cli/internal/oauth"
)

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
			got := deriveAuthBaseURL(tt.input)
			if got != tt.want {
				t.Errorf("deriveAuthBaseURL(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestLogoutCmd(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	// Save tokens first
	tokens := &oauth.TokenData{
		AccessToken:  "test",
		RefreshToken: "test",
		ExpiresAt:    time.Now().Add(time.Hour),
	}
	oauth.SaveTokens(tokens)

	// Run logout
	var buf bytes.Buffer
	LogoutCmd.SetOut(&buf)
	LogoutCmd.SetErr(&buf)
	LogoutCmd.SetArgs([]string{})

	if err := LogoutCmd.Execute(); err != nil {
		t.Fatalf("logout failed: %v", err)
	}

	// Verify tokens cleared
	if _, err := oauth.LoadTokens(); !os.IsNotExist(err) {
		t.Errorf("expected tokens to be cleared, got err: %v", err)
	}
}

func TestLogoutCmd_NoTokens(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	var buf bytes.Buffer
	LogoutCmd.SetOut(&buf)
	LogoutCmd.SetErr(&buf)
	LogoutCmd.SetArgs([]string{})

	// Should not error when no tokens exist
	if err := LogoutCmd.Execute(); err != nil {
		t.Fatalf("logout with no tokens should not fail: %v", err)
	}
}
