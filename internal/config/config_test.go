package config

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

// setupTestEnv sets up a temporary home directory for test isolation.
// On Unix, sets HOME; on Windows, sets USERPROFILE (used by os.UserHomeDir).
// Returns the temp dir path and a cleanup function that should be deferred.
func setupTestEnv(t *testing.T) (tmpDir string, cleanup func()) {
	t.Helper()
	tmpDir = t.TempDir()

	if runtime.GOOS == "windows" {
		originalUserProfile := os.Getenv("USERPROFILE")
		os.Setenv("USERPROFILE", tmpDir)
		return tmpDir, func() {
			os.Setenv("USERPROFILE", originalUserProfile)
		}
	}

	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	return tmpDir, func() {
		os.Setenv("HOME", originalHome)
	}
}

func TestDefaultEndpoint(t *testing.T) {
	if DefaultEndpoint != "api.rootly.com" {
		t.Errorf("expected default endpoint to be 'api.rootly.com', got '%s'", DefaultEndpoint)
	}
}

func TestPath(t *testing.T) {
	path := Path()
	if path == "" {
		t.Error("expected config path to be non-empty")
	}

	// Should end with config.yaml
	if filepath.Base(path) != "config.yaml" {
		t.Errorf("expected config path to end with 'config.yaml', got '%s'", filepath.Base(path))
	}

	// Should contain .rootly-tui directory
	dir := filepath.Dir(path)
	if filepath.Base(dir) != ".rootly-tui" {
		t.Errorf("expected config dir to be '.rootly-tui', got '%s'", filepath.Base(dir))
	}
}

func TestDir(t *testing.T) {
	dir := Dir()
	if dir == "" {
		t.Error("expected config dir to be non-empty")
	}

	if filepath.Base(dir) != ".rootly-tui" {
		t.Errorf("expected config dir to end with '.rootly-tui', got '%s'", filepath.Base(dir))
	}
}

func TestConfigIsValid(t *testing.T) {
	tests := []struct {
		name     string
		config   Config
		expected bool
	}{
		{
			name:     "valid config",
			config:   Config{APIKey: "test-key", Endpoint: "api.rootly.com"},
			expected: true,
		},
		{
			name:     "missing api key",
			config:   Config{APIKey: "", Endpoint: "api.rootly.com"},
			expected: false,
		},
		{
			name:     "missing endpoint",
			config:   Config{APIKey: "test-key", Endpoint: ""},
			expected: false,
		},
		{
			name:     "empty config",
			config:   Config{},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.config.IsValid(); got != tt.expected {
				t.Errorf("Config.IsValid() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestSaveAndLoad(t *testing.T) {
	_, cleanup := setupTestEnv(t)
	defer cleanup()

	// Test saving config
	cfg := &Config{
		APIKey:   "test-api-key-12345",
		Endpoint: "api.test.rootly.com",
	}

	err := Save(cfg)
	if err != nil {
		t.Fatalf("failed to save config: %v", err)
	}

	// Verify file was created
	if !Exists() {
		t.Error("expected config file to exist after save")
	}

	// Test loading config
	loaded, err := Load()
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	if loaded.APIKey != cfg.APIKey {
		t.Errorf("expected APIKey '%s', got '%s'", cfg.APIKey, loaded.APIKey)
	}

	if loaded.Endpoint != cfg.Endpoint {
		t.Errorf("expected Endpoint '%s', got '%s'", cfg.Endpoint, loaded.Endpoint)
	}
}

func TestLoadDefaultEndpoint(t *testing.T) {
	_, cleanup := setupTestEnv(t)
	defer cleanup()

	// Save config without endpoint
	cfg := &Config{
		APIKey:   "test-key",
		Endpoint: "", // Empty endpoint
	}

	err := Save(cfg)
	if err != nil {
		t.Fatalf("failed to save config: %v", err)
	}

	// Load and verify default endpoint is set
	loaded, err := Load()
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	if loaded.Endpoint != DefaultEndpoint {
		t.Errorf("expected default endpoint '%s', got '%s'", DefaultEndpoint, loaded.Endpoint)
	}
}

func TestLoadNonExistent(t *testing.T) {
	_, cleanup := setupTestEnv(t)
	defer cleanup()

	// Try to load non-existent config
	_, err := Load()
	if err == nil {
		t.Error("expected error when loading non-existent config")
	}
}

func TestExistsWhenNotExists(t *testing.T) {
	_, cleanup := setupTestEnv(t)
	defer cleanup()

	if Exists() {
		t.Error("expected Exists() to return false for non-existent config")
	}
}

func TestDetectTimezone(t *testing.T) {
	tz := DetectTimezone()

	t.Logf("Detected timezone: %s", tz)

	// Should return a non-empty string
	if tz == "" {
		t.Error("expected DetectTimezone to return non-empty string")
	}

	// Should be a valid timezone that can be loaded
	_, err := time.LoadLocation(tz)
	if err != nil {
		t.Errorf("expected DetectTimezone to return valid timezone, got '%s' with error: %v", tz, err)
	}
}

func TestGetLocation(t *testing.T) {
	tests := []struct {
		name     string
		timezone string
		expected string
	}{
		{
			name:     "valid timezone",
			timezone: "America/Los_Angeles",
			expected: "America/Los_Angeles",
		},
		{
			name:     "UTC",
			timezone: "UTC",
			expected: "UTC",
		},
		{
			name:     "empty timezone falls back to UTC",
			timezone: "",
			expected: "UTC",
		},
		{
			name:     "invalid timezone falls back to UTC",
			timezone: "Invalid/Timezone",
			expected: "UTC",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{Timezone: tt.timezone}
			loc := cfg.GetLocation()
			if loc.String() != tt.expected {
				t.Errorf("expected location '%s', got '%s'", tt.expected, loc.String())
			}
		})
	}
}

func TestDefaultTimezone(t *testing.T) {
	if DefaultTimezone != "UTC" {
		t.Errorf("expected default timezone to be 'UTC', got '%s'", DefaultTimezone)
	}
}

func TestSaveAndLoadWithTimezone(t *testing.T) {
	_, cleanup := setupTestEnv(t)
	defer cleanup()

	// Test saving config with timezone
	cfg := &Config{
		APIKey:   "test-api-key",
		Endpoint: "api.test.rootly.com",
		Timezone: "America/New_York",
	}

	err := Save(cfg)
	if err != nil {
		t.Fatalf("failed to save config: %v", err)
	}

	// Load and verify timezone is preserved
	loaded, err := Load()
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	if loaded.Timezone != cfg.Timezone {
		t.Errorf("expected Timezone '%s', got '%s'", cfg.Timezone, loaded.Timezone)
	}
}

func TestLoadDefaultTimezone(t *testing.T) {
	_, cleanup := setupTestEnv(t)
	defer cleanup()

	// Save config without timezone
	cfg := &Config{
		APIKey:   "test-key",
		Endpoint: "api.rootly.com",
		Timezone: "", // Empty timezone
	}

	err := Save(cfg)
	if err != nil {
		t.Fatalf("failed to save config: %v", err)
	}

	// Load and verify default timezone is set
	loaded, err := Load()
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	if loaded.Timezone != DefaultTimezone {
		t.Errorf("expected default timezone '%s', got '%s'", DefaultTimezone, loaded.Timezone)
	}
}

func TestListTimezones(t *testing.T) {
	timezones := ListTimezones()

	// Should return non-empty list
	if len(timezones) == 0 {
		t.Error("expected ListTimezones to return non-empty list")
	}

	t.Logf("ListTimezones returned %d timezones", len(timezones))

	// All returned timezones should be valid
	for _, tz := range timezones {
		_, err := time.LoadLocation(tz)
		if err != nil {
			t.Errorf("invalid timezone in list: '%s' with error: %v", tz, err)
		}
	}

	// If we have a large list (from system), it should be sorted
	// Small lists (fallback) are hand-crafted and may not be alphabetically sorted
	if len(timezones) > 20 {
		for i := 1; i < len(timezones); i++ {
			if timezones[i] < timezones[i-1] {
				t.Errorf("system timezones not sorted: '%s' comes after '%s'", timezones[i], timezones[i-1])
			}
		}
	}
}

func TestDetectTimezoneWithTZEnv(t *testing.T) {
	// Save original TZ env
	originalTZ := os.Getenv("TZ")
	defer os.Setenv("TZ", originalTZ)

	// Set TZ env to a known timezone
	os.Setenv("TZ", "America/Chicago")
	tz := DetectTimezone()

	if tz != "America/Chicago" {
		t.Errorf("expected 'America/Chicago' from TZ env, got '%s'", tz)
	}
}

func TestDetectTimezoneWithInvalidTZEnv(t *testing.T) {
	// Save original TZ env
	originalTZ := os.Getenv("TZ")
	defer os.Setenv("TZ", originalTZ)

	// Set TZ env to an invalid timezone
	os.Setenv("TZ", "Invalid/NotATimezone")
	tz := DetectTimezone()

	// Should fall back to something valid
	_, err := time.LoadLocation(tz)
	if err != nil {
		t.Errorf("expected valid timezone after invalid TZ env, got '%s' with error: %v", tz, err)
	}
}

func TestLoadInvalidYAML(t *testing.T) {
	tmpDir, cleanup := setupTestEnv(t)
	defer cleanup()

	// Create config directory
	configDir := filepath.Join(tmpDir, ".rootly-tui")
	if err := os.MkdirAll(configDir, 0700); err != nil {
		t.Fatalf("failed to create config dir: %v", err)
	}

	// Write invalid YAML
	configPath := filepath.Join(configDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte("invalid: yaml: content:\n  - broken"), 0600); err != nil {
		t.Fatalf("failed to write invalid config: %v", err)
	}

	// Try to load invalid YAML config
	_, err := Load()
	if err == nil {
		t.Error("expected error when loading invalid YAML config")
	}
}

func TestSaveAndLoadWithLanguage(t *testing.T) {
	_, cleanup := setupTestEnv(t)
	defer cleanup()

	// Test saving config with language
	cfg := &Config{
		APIKey:   "test-api-key",
		Endpoint: "api.test.rootly.com",
		Timezone: "America/New_York",
		Language: "fr_FR",
	}

	err := Save(cfg)
	if err != nil {
		t.Fatalf("failed to save config: %v", err)
	}

	// Load and verify language is preserved
	loaded, err := Load()
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	if loaded.Language != cfg.Language {
		t.Errorf("expected Language '%s', got '%s'", cfg.Language, loaded.Language)
	}
}

func TestLoadDefaultLanguage(t *testing.T) {
	_, cleanup := setupTestEnv(t)
	defer cleanup()

	// Save config without language
	cfg := &Config{
		APIKey:   "test-key",
		Endpoint: "api.rootly.com",
		Timezone: "UTC",
		Language: "", // Empty language
	}

	err := Save(cfg)
	if err != nil {
		t.Fatalf("failed to save config: %v", err)
	}

	// Load and verify default language is set
	loaded, err := Load()
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	if loaded.Language != DefaultLanguage {
		t.Errorf("expected default language '%s', got '%s'", DefaultLanguage, loaded.Language)
	}
}

func TestDefaultLanguageConstant(t *testing.T) {
	if DefaultLanguage != "en_US" {
		t.Errorf("expected default language to be 'en_US', got '%s'", DefaultLanguage)
	}
}

func TestDetectTimezoneWithEmptyTZEnv(t *testing.T) {
	// Save original TZ env
	originalTZ := os.Getenv("TZ")
	defer os.Setenv("TZ", originalTZ)

	// Clear TZ env to test other detection methods
	os.Unsetenv("TZ")
	tz := DetectTimezone()

	// Should return a valid timezone
	_, err := time.LoadLocation(tz)
	if err != nil {
		t.Errorf("expected valid timezone, got '%s' with error: %v", tz, err)
	}
}

func TestListTimezonesContainsCommonTimezones(t *testing.T) {
	timezones := ListTimezones()

	// Create a set for fast lookup
	tzSet := make(map[string]bool)
	for _, tz := range timezones {
		tzSet[tz] = true
	}

	// Check some common timezones are in the list
	commonTimezones := []string{"UTC", "America/New_York", "America/Los_Angeles", "Europe/London"}
	for _, tz := range commonTimezones {
		if !tzSet[tz] {
			t.Errorf("expected common timezone '%s' to be in list", tz)
		}
	}
}

func TestConfigStruct(t *testing.T) {
	// Test that all fields can be set and retrieved
	cfg := Config{
		APIKey:   "my-api-key",
		Endpoint: "custom.endpoint.com",
		Timezone: "Asia/Tokyo",
		Language: "ja_JP",
		Layout:   "vertical",
	}

	if cfg.APIKey != "my-api-key" {
		t.Errorf("APIKey mismatch")
	}
	if cfg.Endpoint != "custom.endpoint.com" {
		t.Errorf("Endpoint mismatch")
	}
	if cfg.Timezone != "Asia/Tokyo" {
		t.Errorf("Timezone mismatch")
	}
	if cfg.Language != "ja_JP" {
		t.Errorf("Language mismatch")
	}
	if cfg.Layout != "vertical" {
		t.Errorf("Layout mismatch")
	}
}

func TestSaveAndLoadWithLayout(t *testing.T) {
	_, cleanup := setupTestEnv(t)
	defer cleanup()

	// Test saving config with layout
	cfg := &Config{
		APIKey:   "test-api-key",
		Endpoint: "api.test.rootly.com",
		Timezone: "America/New_York",
		Language: "en_US",
		Layout:   LayoutVertical,
	}

	err := Save(cfg)
	if err != nil {
		t.Fatalf("failed to save config: %v", err)
	}

	// Load and verify layout is preserved
	loaded, err := Load()
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	if loaded.Layout != LayoutVertical {
		t.Errorf("expected layout '%s', got '%s'", LayoutVertical, loaded.Layout)
	}
}

func TestLoadDefaultLayout(t *testing.T) {
	_, cleanup := setupTestEnv(t)
	defer cleanup()

	// Save config without layout
	cfg := &Config{
		APIKey:   "test-key",
		Endpoint: "api.rootly.com",
		Timezone: "UTC",
		Language: "en_US",
		Layout:   "", // Empty layout
	}

	err := Save(cfg)
	if err != nil {
		t.Fatalf("failed to save config: %v", err)
	}

	// Load and verify default layout is set
	loaded, err := Load()
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	if loaded.Layout != DefaultLayout {
		t.Errorf("expected default layout '%s', got '%s'", DefaultLayout, loaded.Layout)
	}
}

func TestDefaultLayoutConstant(t *testing.T) {
	if DefaultLayout != "horizontal" {
		t.Errorf("expected default layout to be 'horizontal', got '%s'", DefaultLayout)
	}
}

func TestLayoutConstants(t *testing.T) {
	if LayoutHorizontal != "horizontal" {
		t.Errorf("expected LayoutHorizontal to be 'horizontal', got '%s'", LayoutHorizontal)
	}
	if LayoutVertical != "vertical" {
		t.Errorf("expected LayoutVertical to be 'vertical', got '%s'", LayoutVertical)
	}
}
