package oauth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"golang.org/x/oauth2"
)

func TestNewUserAgentTransport(t *testing.T) {
	for _, original := range []string{"", "Go-http-client/1.1", "custom-agent"} {
		t.Run(original, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if got := r.UserAgent(); got != "rootly-cli/test" {
					t.Errorf("User-Agent = %q", got)
				}
				if got := r.Header.Get("X-Test"); got != "preserved" {
					t.Errorf("X-Test = %q", got)
				}
				w.WriteHeader(http.StatusNoContent)
			}))
			defer server.Close()
			for _, base := range []http.RoundTripper{nil, server.Client().Transport} {
				client := &http.Client{Transport: NewUserAgentTransport(base, "rootly-cli/test")}
				request, err := http.NewRequestWithContext(context.Background(), http.MethodGet, server.URL, http.NoBody)
				if err != nil {
					t.Fatal(err)
				}
				request.Header.Set("User-Agent", original)
				request.Header.Set("X-Test", "preserved")
				response, err := client.Do(request)
				if err != nil {
					t.Fatal(err)
				}
				response.Body.Close()
				if got := request.UserAgent(); got != original {
					t.Fatalf("original request mutated: User-Agent = %q, want %q", got, original)
				}
			}
		})
	}
}

func TestWithUserAgent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.UserAgent(); got != "rootly-cli/test" {
			t.Errorf("User-Agent = %q", got)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	for _, original := range []*http.Client{nil, {Timeout: time.Second, Transport: server.Client().Transport}} {
		client := WithUserAgent(original, "rootly-cli/test")
		if original != nil && (client == original || client.Timeout != original.Timeout || client.Transport == original.Transport) {
			t.Fatal("expected a copy preserving timeout with a wrapped transport")
		}
		request, err := http.NewRequestWithContext(context.Background(), http.MethodGet, server.URL, http.NoBody)
		if err != nil {
			t.Fatal(err)
		}
		response, err := client.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
	}
}

func TestNewHTTPClient_SetsAuthAndUserAgent(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	t.Setenv("USERPROFILE", tmpDir)

	tokens := &TokenData{
		AccessToken:  "my-token",
		RefreshToken: "my-refresh",
		ExpiresAt:    time.Now().Add(1 * time.Hour),
		TokenType:    "Bearer",
	}
	if err := SaveTokens(tokens); err != nil {
		t.Fatal(err)
	}

	var gotAuth, gotUA string
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotUA = r.Header.Get("User-Agent")
		w.WriteHeader(200)
	}))
	defer backend.Close()

	cfg := &oauth2.Config{
		ClientID: "test",
		Endpoint: oauth2.Endpoint{
			TokenURL:  backend.URL + "/oauth/token",
			AuthStyle: oauth2.AuthStyleInParams,
		},
	}

	client, err := NewHTTPClient(cfg, http.DefaultTransport, "rootly-cli/test")
	if err != nil {
		t.Fatalf("NewHTTPClient: %v", err)
	}

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, backend.URL+"/test", http.NoBody)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	resp.Body.Close()

	if gotAuth != "Bearer my-token" {
		t.Errorf("Authorization = %q, want %q", gotAuth, "Bearer my-token")
	}
	if gotUA != "rootly-cli/test" {
		t.Errorf("User-Agent = %q, want %q", gotUA, "rootly-cli/test")
	}
}

func TestNewHTTPClient_NoTokens(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	t.Setenv("USERPROFILE", tmpDir)

	cfg := &oauth2.Config{
		ClientID: "test",
		Endpoint: oauth2.Endpoint{TokenURL: "http://localhost/token"},
	}

	_, err := NewHTTPClient(cfg, http.DefaultTransport, "")
	if err == nil {
		t.Error("expected error when no tokens exist")
	}
}

func TestNewHTTPClient_RefreshesExpiredToken(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	t.Setenv("USERPROFILE", tmpDir)

	// Token server for refresh
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.UserAgent(); got != "rootly-cli/test" {
			t.Errorf("refresh User-Agent = %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"access_token":  "fresh-token",
			"refresh_token": "fresh-refresh",
			"expires_in":    3600,
			"token_type":    "Bearer",
		})
	}))
	defer tokenServer.Close()

	// Save expired tokens
	tokens := &TokenData{
		AccessToken:  "expired-token",
		RefreshToken: "valid-refresh",
		ExpiresAt:    time.Now().Add(-1 * time.Hour),
		TokenType:    "Bearer",
	}
	SaveTokens(tokens)

	cfg := &oauth2.Config{
		ClientID: "test",
		Endpoint: oauth2.Endpoint{
			TokenURL:  tokenServer.URL,
			AuthStyle: oauth2.AuthStyleInParams,
		},
	}

	client, err := NewHTTPClient(cfg, http.DefaultTransport, "rootly-cli/test")
	if err != nil {
		t.Fatalf("NewHTTPClient: %v", err)
	}

	var gotAuth string
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.WriteHeader(200)
	}))
	defer backend.Close()

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, backend.URL+"/test", http.NoBody)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	resp.Body.Close()

	if gotAuth != "Bearer fresh-token" {
		t.Errorf("Authorization = %q, want %q", gotAuth, "Bearer fresh-token")
	}

	// Verify refreshed tokens were persisted
	saved, _ := LoadTokens()
	if saved.AccessToken != "fresh-token" {
		t.Errorf("saved AccessToken = %q", saved.AccessToken)
	}
}

func TestNewHTTPClient_RefreshFailsSuggestsLogin(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	t.Setenv("USERPROFILE", tmpDir)

	// Token server rejects refresh with invalid_grant
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error":"invalid_grant","error_description":"refresh token revoked"}`))
	}))
	defer tokenServer.Close()

	// Save expired tokens so refresh is triggered
	tokens := &TokenData{
		AccessToken:  "expired-token",
		RefreshToken: "revoked-refresh",
		ExpiresAt:    time.Now().Add(-1 * time.Hour),
		TokenType:    "Bearer",
	}
	SaveTokens(tokens)

	cfg := &oauth2.Config{
		ClientID: "test",
		Endpoint: oauth2.Endpoint{
			TokenURL:  tokenServer.URL,
			AuthStyle: oauth2.AuthStyleInParams,
		},
	}

	client, err := NewHTTPClient(cfg, http.DefaultTransport, "")
	if err != nil {
		t.Fatalf("NewHTTPClient: %v", err)
	}

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, tokenServer.URL+"/test", http.NoBody)
	resp, err := client.Do(req)
	if resp != nil {
		resp.Body.Close()
	}
	if err == nil {
		t.Fatal("expected error when refresh token is revoked")
	}
	if !strings.Contains(err.Error(), "rootly login") {
		t.Errorf("error should suggest 'rootly login', got: %v", err)
	}
}
