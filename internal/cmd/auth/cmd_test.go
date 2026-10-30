package auth

import (
	"bytes"
	"os"
	"testing"
	"time"

	"github.com/rootlyhq/rootly-cli/internal/config"
	"github.com/rootlyhq/rootly-cli/internal/oauth"
)

func TestLogoutCmd(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	t.Setenv("USERPROFILE", tmpDir)
	t.Setenv("ROOTLY_API_KEY", "")

	if err := config.Save(&config.Config{
		APIKey:   "test-api-key",
		ClientID: "stale-client",
		Scopes:   []string{"openid", "ai.chat:read"},
	}); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

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
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("failed to load config after logout: %v", err)
	}
	if cfg.ClientID != "" {
		t.Errorf("client_id = %q, want empty", cfg.ClientID)
	}
	if len(cfg.Scopes) != 0 {
		t.Errorf("scopes = %v, want empty", cfg.Scopes)
	}
	if cfg.APIKey != "test-api-key" {
		t.Errorf("api_key = %q, want preserved API key", cfg.APIKey)
	}
}

func TestLogoutCmd_NoTokens(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	t.Setenv("USERPROFILE", tmpDir)

	var buf bytes.Buffer
	LogoutCmd.SetOut(&buf)
	LogoutCmd.SetErr(&buf)
	LogoutCmd.SetArgs([]string{})

	// Should not error when no tokens exist
	if err := LogoutCmd.Execute(); err != nil {
		t.Fatalf("logout with no tokens should not fail: %v", err)
	}
}
