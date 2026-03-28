package auth

import (
	"bytes"
	"os"
	"testing"
	"time"

	"github.com/rootlyhq/rootly-cli/internal/oauth"
)

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
