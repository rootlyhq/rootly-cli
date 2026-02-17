package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/rootlyhq/rootly-cli/internal/config"
)

func TestNewClient(t *testing.T) {
	cfg := &config.Config{
		APIKey:   "test-api-key",
		Endpoint: "api.rootly.com",
	}

	client, err := NewClient(cfg)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	if client == nil {
		t.Fatal("expected client to be non-nil")
	}
}

func TestNewClientWithHTTPS(t *testing.T) {
	tests := []struct {
		name     string
		endpoint string
	}{
		{"hostname only", "api.rootly.com"},
		{"with https", "https://api.rootly.com"},
		{"with http", "http://localhost:8080"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{
				APIKey:   "test-key",
				Endpoint: tt.endpoint,
			}

			client, err := NewClient(cfg)
			if err != nil {
				t.Fatalf("failed to create client: %v", err)
			}

			if client == nil {
				t.Fatal("expected client to be non-nil")
			}
		})
	}
}

func TestUserAgentHeader(t *testing.T) {
	Version = "1.2.3"

	var receivedUserAgent string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedUserAgent = r.Header.Get("User-Agent")
		w.Header().Set("Content-Type", "application/vnd.api+json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"data": map[string]interface{}{
				"id":   "123",
				"type": "users",
				"attributes": map[string]interface{}{
					"name":  "Test User",
					"email": "test@example.com",
				},
			},
		})
	}))
	defer server.Close()

	cfg := &config.Config{
		APIKey:   "test-key",
		Endpoint: server.URL,
	}

	client, err := NewClient(cfg)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	_ = client.ValidateAPIKey(context.Background())

	expectedUserAgent := "rootly-cli/1.2.3"
	if receivedUserAgent != expectedUserAgent {
		t.Errorf("User-Agent = %q, want %q", receivedUserAgent, expectedUserAgent)
	}
}

func TestValidateAPIKey(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth != "Bearer valid-key" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		if r.URL.Path != "/v1/users/me" {
			w.WriteHeader(http.StatusNotFound)
			return
		}

		w.Header().Set("Content-Type", "application/vnd.api+json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"data": map[string]interface{}{
				"id":   "123",
				"type": "users",
				"attributes": map[string]interface{}{
					"name":  "Test User",
					"email": "test@example.com",
				},
			},
		})
	}))
	defer server.Close()

	tests := []struct {
		name    string
		apiKey  string
		wantErr bool
	}{
		{"valid key", "valid-key", false},
		{"invalid key", "invalid-key", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{
				APIKey:   tt.apiKey,
				Endpoint: server.URL,
			}

			client, err := NewClient(cfg)
			if err != nil {
				t.Fatalf("failed to create client: %v", err)
			}

			err = client.ValidateAPIKey(context.Background())
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateAPIKey() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateAPIKeyHTTPError(t *testing.T) {
	cfg := &config.Config{
		APIKey:   "test-key",
		Endpoint: "http://invalid.nonexistent.host:99999",
	}

	client, err := NewClient(cfg)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	err = client.ValidateAPIKey(context.Background())
	if err == nil {
		t.Error("expected error for unreachable host")
	}
}

func TestParseTimePtr(t *testing.T) {
	tests := []struct {
		name    string
		input   *string
		wantNil bool
	}{
		{"nil input", nil, true},
		{"empty string", strPtr(""), true},
		{"valid RFC3339", strPtr("2025-01-01T10:00:00Z"), false},
		{"invalid format", strPtr("not a date"), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseTimePtr(tt.input)
			if tt.wantNil && result != nil {
				t.Errorf("expected nil, got %v", result)
			}
			if !tt.wantNil && result == nil {
				t.Error("expected non-nil result")
			}
		})
	}
}

func strPtr(s string) *string {
	return &s
}
