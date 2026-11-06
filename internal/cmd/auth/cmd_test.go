package auth

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/rootlyhq/rootly-cli/internal/buildinfo"
	"github.com/rootlyhq/rootly-cli/internal/config"
	"github.com/rootlyhq/rootly-cli/internal/oauth"
)

func TestOAuthPreflightSendsUserAgent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.UserAgent(); got != buildinfo.UserAgent() {
			t.Errorf("User-Agent = %q, want %q", got, buildinfo.UserAgent())
		}
		if r.Method != http.MethodHead || r.URL.Path != "/oauth/authorize" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()
	if _, err := doOAuthFlow(t.Context(), LoginCmd, server.URL, "stale-client", nil); !isAuthorize404(err) {
		t.Fatalf("expected authorize 404, got %v", err)
	}
}

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
