package oauth

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestIsExpired(t *testing.T) {
	tests := []struct {
		name    string
		expires time.Time
		want    bool
	}{
		{"future", time.Now().Add(10 * time.Minute), false},
		{"past", time.Now().Add(-10 * time.Minute), true},
		{"within 30s buffer", time.Now().Add(20 * time.Second), true},
		{"just outside buffer", time.Now().Add(60 * time.Second), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			td := &TokenData{ExpiresAt: tt.expires}
			if got := IsExpired(td); got != tt.want {
				t.Errorf("IsExpired() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSaveAndLoadTokens(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	tokens := &TokenData{
		AccessToken:  "test-access",
		RefreshToken: "test-refresh",
		ExpiresAt:    time.Now().Add(1 * time.Hour).Truncate(time.Second),
		TokenType:    "Bearer",
	}

	if err := SaveTokens(tokens); err != nil {
		t.Fatalf("SaveTokens: %v", err)
	}

	// Verify file permissions
	path := filepath.Join(tmpDir, ".rootly-cli", "config.yaml")
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat config file: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0600 {
		t.Errorf("file permissions = %o, want 0600", perm)
	}

	loaded, err := LoadTokens()
	if err != nil {
		t.Fatalf("LoadTokens: %v", err)
	}
	if loaded.AccessToken != tokens.AccessToken {
		t.Errorf("AccessToken = %q, want %q", loaded.AccessToken, tokens.AccessToken)
	}
	if loaded.RefreshToken != tokens.RefreshToken {
		t.Errorf("RefreshToken = %q, want %q", loaded.RefreshToken, tokens.RefreshToken)
	}
}

func TestSaveTokens_PreservesExistingConfig(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	// Write a config with an API key first
	dir := filepath.Join(tmpDir, ".rootly-cli")
	os.MkdirAll(dir, 0700)
	os.WriteFile(filepath.Join(dir, "config.yaml"), []byte("api_key: my-key\napi_host: custom.rootly.com\n"), 0600)

	// Save tokens
	tokens := &TokenData{AccessToken: "tok", RefreshToken: "ref", ExpiresAt: time.Now().Add(time.Hour)}
	if err := SaveTokens(tokens); err != nil {
		t.Fatalf("SaveTokens: %v", err)
	}

	// Verify existing fields preserved
	data, _ := os.ReadFile(filepath.Join(dir, "config.yaml"))
	content := string(data)
	if !strings.Contains(content, "api_key: my-key") {
		t.Errorf("api_key not preserved in config:\n%s", content)
	}
	if !strings.Contains(content, "api_host: custom.rootly.com") {
		t.Errorf("api_host not preserved in config:\n%s", content)
	}
	if !strings.Contains(content, "access_token: tok") {
		t.Errorf("oauth tokens not written:\n%s", content)
	}
}


func TestClearTokens(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	tokens := &TokenData{AccessToken: "x", RefreshToken: "y", ExpiresAt: time.Now().Add(time.Hour)}
	_ = SaveTokens(tokens)

	if err := ClearTokens(); err != nil {
		t.Fatalf("ClearTokens: %v", err)
	}

	_, err := LoadTokens()
	if err == nil {
		t.Error("expected error after clearing tokens")
	}
}

func TestClearTokens_PreservesConfig(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	// Write config with API key + tokens
	dir := filepath.Join(tmpDir, ".rootly-cli")
	os.MkdirAll(dir, 0700)
	os.WriteFile(filepath.Join(dir, "config.yaml"), []byte("api_key: my-key\noauth:\n  access_token: tok\n  refresh_token: ref\n"), 0600)

	if err := ClearTokens(); err != nil {
		t.Fatalf("ClearTokens: %v", err)
	}

	// API key should still be there
	data, _ := os.ReadFile(filepath.Join(dir, "config.yaml"))
	if !strings.Contains(string(data), "api_key: my-key") {
		t.Errorf("api_key not preserved after clear:\n%s", string(data))
	}
}

func TestClearTokens_NoFile(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	if err := ClearTokens(); err != nil {
		t.Fatalf("ClearTokens on missing file: %v", err)
	}
}

func TestHasTokens(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	if HasTokens() {
		t.Error("HasTokens should be false with no config")
	}

	SaveTokens(&TokenData{AccessToken: "x", RefreshToken: "y", ExpiresAt: time.Now().Add(time.Hour)})

	if !HasTokens() {
		t.Error("HasTokens should be true after saving tokens")
	}
}

func TestLoadTokens_NoOAuthSection(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	dir := filepath.Join(tmpDir, ".rootly-cli")
	os.MkdirAll(dir, 0700)
	os.WriteFile(filepath.Join(dir, "config.yaml"), []byte("api_key: my-key\n"), 0600)

	_, err := LoadTokens()
	if err == nil {
		t.Error("expected error when no oauth section exists")
	}
}
