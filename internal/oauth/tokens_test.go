package oauth

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestTokenData_IsExpired(t *testing.T) {
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
			if got := td.IsExpired(); got != tt.want {
				t.Errorf("IsExpired() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSaveAndLoadTokens(t *testing.T) {
	// Use temp dir to avoid touching real config
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	t.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

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
	path := filepath.Join(tmpDir, tokenDir, tokenFile)
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat token file: %v", err)
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

func TestClearTokens(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	t.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

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

func TestClearTokens_NoFile(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	// Should not error when file doesn't exist
	if err := ClearTokens(); err != nil {
		t.Fatalf("ClearTokens on missing file: %v", err)
	}
}

func TestLoadTokens_InvalidYAML(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	dir := filepath.Join(tmpDir, tokenDir)
	os.MkdirAll(dir, 0700)
	os.WriteFile(filepath.Join(dir, tokenFile), []byte("not: [valid: yaml: {{"), 0600)

	_, err := LoadTokens()
	if err == nil {
		t.Error("expected error for invalid YAML")
	}
}
