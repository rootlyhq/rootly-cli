package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
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

func TestNormalizeIncidentID(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"INC-42", "42"},
		{"INC-1", "1"},
		{"inc-99", "99"},
		{"Inc-123", "123"},
		{"e5923856-6fe8-4a2c-b0eb-cb783e811d06", "e5923856-6fe8-4a2c-b0eb-cb783e811d06"},
		{"some-slug", "some-slug"},
		{"INC-", "INC-"},
		{"INC-abc", "INC-abc"},
		{"42", "42"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := NormalizeIncidentID(tt.input)
			if got != tt.want {
				t.Errorf("NormalizeIncidentID(%q) = %q, want %q", tt.input, got, tt.want)
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

func TestDebugTransportRedactsAuthorization(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	var buf bytes.Buffer
	dt := &debugTransport{transport: http.DefaultTransport}

	req, err := http.NewRequestWithContext(context.Background(), "GET", server.URL+"/v1/incidents", http.NoBody)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer super-secret-token-12345")

	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	resp, err := dt.RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip failed: %v", err)
	}
	resp.Body.Close()

	w.Close()
	io.Copy(&buf, r)
	os.Stderr = oldStderr

	output := buf.String()
	if strings.Contains(output, "super-secret-token-12345") {
		t.Error("debug output contains raw API token — Authorization header must be redacted")
	}
	if !strings.Contains(output, "Bearer [REDACTED]") {
		t.Error("debug output should contain 'Bearer [REDACTED]'")
	}
	if !strings.Contains(output, "DEBUG REQUEST") {
		t.Error("debug output should contain request dump marker")
	}
}

func TestDebugTransportPreservesActualAuthHeader(t *testing.T) {
	var receivedAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	dt := &debugTransport{transport: http.DefaultTransport}

	req, err := http.NewRequestWithContext(context.Background(), "GET", server.URL+"/v1/incidents", http.NoBody)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer real-token")

	oldStderr := os.Stderr
	_, w, _ := os.Pipe()
	os.Stderr = w

	resp, err := dt.RoundTrip(req)
	w.Close()
	os.Stderr = oldStderr

	if err != nil {
		t.Fatalf("RoundTrip failed: %v", err)
	}
	resp.Body.Close()

	if receivedAuth != "Bearer real-token" {
		t.Errorf("server received Authorization = %q, want %q", receivedAuth, "Bearer real-token")
	}
}

func TestDebugTransportNoAuthHeader(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	var buf bytes.Buffer
	dt := &debugTransport{transport: http.DefaultTransport}

	req, err := http.NewRequestWithContext(context.Background(), "GET", server.URL+"/v1/incidents", http.NoBody)
	if err != nil {
		t.Fatal(err)
	}

	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	resp, err := dt.RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip failed: %v", err)
	}
	resp.Body.Close()

	w.Close()
	io.Copy(&buf, r)
	os.Stderr = oldStderr

	output := buf.String()
	if strings.Contains(output, "[REDACTED]") {
		t.Error("should not contain [REDACTED] when no Authorization header is set")
	}
	if !strings.Contains(output, "DEBUG REQUEST") {
		t.Error("debug output should still contain request dump marker")
	}
}
