package oauth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func TestNewConfig(t *testing.T) {
	cfg := NewConfig("https://rootly.com", "test-client-id")

	if cfg.ClientID != "test-client-id" {
		t.Errorf("ClientID = %q, want %q", cfg.ClientID, "test-client-id")
	}
	if cfg.RedirectURL != "http://localhost:19797/callback" {
		t.Errorf("RedirectURL = %q", cfg.RedirectURL)
	}
	if cfg.Endpoint.AuthURL != "https://rootly.com/oauth/authorize" {
		t.Errorf("AuthURL = %q", cfg.Endpoint.AuthURL)
	}
	if cfg.Endpoint.TokenURL != "https://rootly.com/oauth/token" {
		t.Errorf("TokenURL = %q", cfg.Endpoint.TokenURL)
	}
	if len(cfg.Scopes) != 4 {
		t.Errorf("Scopes = %v", cfg.Scopes)
	}
}

func TestNewConfig_Localhost(t *testing.T) {
	cfg := NewConfig("http://localhost:22166", "my-client")

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
		{"api.rootly.com", "https://rootly.com"},
		{"api.staging.rootly.com", "https://staging.rootly.com"},
		{"https://api.rootly.com", "https://rootly.com"},
		{"https://api.staging.rootly.com", "https://staging.rootly.com"},
		{"localhost:22166", "http://localhost:22166"},
		{"localhost:22166/api", "http://localhost:22166"},
		{"http://localhost:22166/api", "http://localhost:22166"},
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

func TestRegisterClient(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/oauth/register" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Errorf("unexpected method: %s", r.Method)
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("unexpected content-type: %s", r.Header.Get("Content-Type"))
		}

		var req registrationRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("failed to decode request: %v", err)
		}
		if req.ClientName != "Rootly CLI" {
			t.Errorf("ClientName = %q", req.ClientName)
		}

		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(registrationResponse{ClientID: "dynamic-id-123"})
	}))
	defer srv.Close()

	clientID, err := RegisterClient(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("RegisterClient() error: %v", err)
	}
	if clientID != "dynamic-id-123" {
		t.Errorf("clientID = %q, want %q", clientID, "dynamic-id-123")
	}
}

func TestRegisterClient_NonCreated(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	_, err := RegisterClient(context.Background(), srv.URL)
	if err == nil {
		t.Fatal("expected error for non-201 status")
	}
}

func TestLoadSaveClearClientID(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	t.Setenv("USERPROFILE", tmpDir)

	// Initially empty
	if id := LoadCachedClientID(); id != "" {
		t.Errorf("expected empty, got %q", id)
	}

	// Save
	if err := SaveClientID("cached-id"); err != nil {
		t.Fatalf("SaveClientID: %v", err)
	}
	if id := LoadCachedClientID(); id != "cached-id" {
		t.Errorf("got %q, want %q", id, "cached-id")
	}

	// Clear
	if err := ClearClientID(); err != nil {
		t.Fatalf("ClearClientID: %v", err)
	}

	// Verify cleared — load raw config to check
	data, err := os.ReadFile(configPathForTest(tmpDir))
	if err != nil {
		// File may not exist after clear, that's ok
		return
	}
	if id := LoadCachedClientID(); id != "" {
		t.Errorf("expected empty after clear, got %q (raw: %s)", id, string(data))
	}
}

func configPathForTest(home string) string {
	return home + "/.rootly-cli/config.yaml"
}
