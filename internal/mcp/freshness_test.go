package mcp

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestServerTimeoutConfig(t *testing.T) {
	config := ServerConfig{Command: "srv"}
	if got := config.CallTimeout(45 * time.Second); got != 45*time.Second {
		t.Errorf("unset timeout = %s, want fallback", got)
	}
	config.Timeout = 5
	if got := config.CallTimeout(45 * time.Second); got != 5*time.Second {
		t.Errorf("timeout = %s, want 5s", got)
	}
	if err := config.Validate(); err != nil {
		t.Errorf("valid timeout rejected: %v", err)
	}
	for _, bad := range []int{-1, MaxTimeoutSeconds + 1} {
		config.Timeout = bad
		if err := config.Validate(); err == nil {
			t.Errorf("timeout %d accepted", bad)
		}
	}
	var parsed ServerConfig
	if err := json.Unmarshal([]byte(`{"command":"srv","timeout":30}`), &parsed); err != nil || parsed.Timeout != 30 {
		t.Errorf("timeout not parsed from config: %+v %v", parsed, err)
	}
}

func refreshFixture(t *testing.T, refreshToken string) (*httptest.Server, *TokenStore, *atomic.Int32) {
	t.Helper()
	var refreshes atomic.Int32
	mux := http.NewServeMux()
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		refreshes.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"access_token":"fresh","refresh_token":"r2","token_type":"Bearer","expires_in":3600}`)
	})
	mux.HandleFunc("/mcp", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer fresh" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		body, _ := io.ReadAll(r.Body)
		_, _ = w.Write(body)
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	store := &TokenStore{Path: filepath.Join(t.TempDir(), "mcp-oauth.json")}
	credential := StoredCredential{ClientID: "c", TokenURL: server.URL + "/token", AccessToken: "stale", RefreshToken: refreshToken, TokenType: "Bearer", Expiry: time.Now().Add(time.Hour)}
	if err := store.Put(server.URL+"/mcp", credential); err != nil {
		t.Fatal(err)
	}
	return server, store, &refreshes
}

func TestRemote401RefreshesTokenAndRetriesOnce(t *testing.T) {
	server, store, refreshes := refreshFixture(t, "r1")
	manager := &Manager{options: ManagerOptions{Tokens: store}}
	config := ServerConfig{Type: TransportHTTP, URL: server.URL + "/mcp"}
	bearer, refresh := manager.bearerFuncs(config)
	probe := &authProbe{}
	client := newServerHTTPClientWithRefresh(config, bearer, refresh, probe)

	request, _ := http.NewRequestWithContext(t.Context(), http.MethodPost, config.URL, strings.NewReader(`{"ping":1}`))
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if response.StatusCode != http.StatusOK || string(body) != `{"ping":1}` {
		t.Fatalf("retry = %d %q, want 200 with replayed body", response.StatusCode, body)
	}
	if refreshes.Load() != 1 || probe.seen() {
		t.Errorf("refreshes = %d, needs-login = %v; want one silent refresh", refreshes.Load(), probe.seen())
	}
	if stored, _ := store.Get(config.URL); stored.AccessToken != "fresh" {
		t.Errorf("refreshed token not persisted: %q", stored.AccessToken)
	}
}

func TestRemote401WithoutRefreshTokenNeedsLogin(t *testing.T) {
	server, store, refreshes := refreshFixture(t, "")
	manager := &Manager{options: ManagerOptions{Tokens: store}}
	config := ServerConfig{Type: TransportHTTP, URL: server.URL + "/mcp"}
	bearer, refresh := manager.bearerFuncs(config)
	probe := &authProbe{}
	client := newServerHTTPClientWithRefresh(config, bearer, refresh, probe)

	request, _ := http.NewRequestWithContext(t.Context(), http.MethodGet, config.URL, http.NoBody)
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusUnauthorized || !probe.seen() || refreshes.Load() != 0 {
		t.Errorf("status %d, needs-login %v, refreshes %d; want 401 + needs-login without refresh", response.StatusCode, probe.seen(), refreshes.Load())
	}
}

func TestExpandedKeepsTimeout(t *testing.T) {
	config := ServerConfig{Command: "x", Timeout: 5}.Expanded(func(string) (string, bool) { return "", false })
	if config.CallTimeout(time.Minute) != 5*time.Second {
		t.Fatalf("expanded timeout = %v, want 5s", config.CallTimeout(time.Minute))
	}
}

func TestRemote401RefreshStopsWithRequestContext(t *testing.T) {
	release := make(chan struct{})
	mux := http.NewServeMux()
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.ReadAll(r.Body)
		select {
		case <-r.Context().Done():
		case <-release:
		}
	})
	mux.HandleFunc("/mcp", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	t.Cleanup(func() { close(release) })
	store := &TokenStore{Path: filepath.Join(t.TempDir(), "mcp-oauth.json")}
	if err := store.Put(server.URL+"/mcp", StoredCredential{ClientID: "c", TokenURL: server.URL + "/token", AccessToken: "stale", RefreshToken: "r1", TokenType: "Bearer", Expiry: time.Now().Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}
	manager := &Manager{options: ManagerOptions{Tokens: store}}
	config := ServerConfig{Type: TransportHTTP, URL: server.URL + "/mcp"}
	bearer, refresh := manager.bearerFuncs(config)
	client := newServerHTTPClientWithRefresh(config, bearer, refresh, &authProbe{})

	ctx, cancel := context.WithTimeout(t.Context(), 200*time.Millisecond)
	defer cancel()
	request, _ := http.NewRequestWithContext(ctx, http.MethodGet, config.URL, http.NoBody)
	started := time.Now()
	response, err := client.Do(request)
	if err == nil {
		_ = response.Body.Close()
	}
	if elapsed := time.Since(started); elapsed > 3*time.Second {
		t.Fatalf("refresh ignored request cancellation, took %v", elapsed)
	}
}
