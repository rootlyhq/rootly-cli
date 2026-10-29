package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/spf13/cobra"

	"github.com/rootlyhq/rootly-cli/internal/api"
	"github.com/rootlyhq/rootly-cli/internal/config"
)

func TestPreflightAIFeatureFlag(t *testing.T) {
	tests := []struct {
		name      string
		status    int
		flags     map[string]bool
		wantError string
		wantUser  bool
	}{
		{name: "disabled", status: http.StatusOK, flags: map[string]bool{"ai-chat": false}, wantError: "Rootly AI chat isn't enabled for your organization. Contact your Rootly account team or Rootly support to turn it on."},
		{name: "enabled", status: http.StatusOK, flags: map[string]bool{"ai-chat": true}, wantUser: true},
		{name: "flag absent", status: http.StatusOK, flags: map[string]bool{"ai-native-home": true}, wantUser: true},
		{name: "unauthorized", status: http.StatusUnauthorized, wantError: "not authenticated: run `rootly login` or set ROOTLY_API_KEY"},
		{name: "other failure proceeds", status: http.StatusInternalServerError},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				if request.URL.Path != "/v1/users/me" {
					t.Errorf("preflight path = %q, want /v1/users/me", request.URL.Path)
				}
				writer.Header().Set("Content-Type", "application/vnd.api+json")
				writer.WriteHeader(test.status)
				switch test.status {
				case http.StatusOK:
					_ = json.NewEncoder(writer).Encode(map[string]any{
						"data": map[string]any{
							"id": "user-1", "type": "users",
							"attributes": map[string]any{"email": "test@example.com", "feature_flags": test.flags},
						},
					})
				case http.StatusUnauthorized:
					_, _ = writer.Write([]byte(`{"errors":[{"title":"Unauthorized"}]}`))
				}
			}))
			defer server.Close()

			client, err := api.NewClient(&config.Config{APIKey: "test-key", Endpoint: server.URL})
			if err != nil {
				t.Fatalf("NewClient() error = %v", err)
			}
			var stderr bytes.Buffer
			cmd := &cobra.Command{}
			cmd.SetContext(context.Background())
			cmd.SetErr(&stderr)
			currentUser, err := preflightAI(cmd, client)
			if test.wantError != "" {
				if err == nil || err.Error() != test.wantError {
					t.Fatalf("preflightAI() error = %v, want %q", err, test.wantError)
				}
				return
			}
			if err != nil {
				t.Fatalf("preflightAI() error = %v", err)
			}
			if (currentUser != nil) != test.wantUser {
				t.Errorf("current user returned = %v, want %v", currentUser != nil, test.wantUser)
			}
		})
	}
}

func TestLatestSessionRequestsMostRecentUnarchivedSession(t *testing.T) {
	sessionID := "8e9bd763-7c86-4741-9766-86a27d3c36d1"
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/v1/ai/chat/sessions" {
			t.Errorf("list path = %q, want /v1/ai/chat/sessions", request.URL.Path)
		}
		if request.URL.Query().Get("limit") != "1" || request.URL.Query().Get("archived") != "false" {
			t.Errorf("list query = %q, want limit=1 and archived=false", request.URL.RawQuery)
		}
		writer.Header().Set("Content-Type", "application/vnd.api+json")
		_, _ = writer.Write([]byte(`{"meta":{"pagination":{"has_more":false,"next_before":null}},"sessions":[{"id":"` + sessionID + `","title":"Recent","last_active_at":"2026-10-29T12:00:00Z"}]}`))
	}))
	defer server.Close()

	client, err := api.NewClient(&config.Config{APIKey: "test-key", Endpoint: server.URL})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	gotID, title, err := latestSession(context.Background(), client)
	if err != nil {
		t.Fatalf("latestSession() error = %v", err)
	}
	if gotID.String() != sessionID || title != "Recent" {
		t.Errorf("latestSession() = %s, %q, want %s, Recent", gotID, title, sessionID)
	}
}

func TestLatestSessionErrors(t *testing.T) {
	tests := []struct {
		name      string
		status    int
		body      string
		wantError string
	}{
		{name: "no sessions", status: http.StatusOK, body: `{"meta":{"pagination":{"has_more":false,"next_before":null}},"sessions":[]}`, wantError: "no previous AI chat session to continue"},
		{name: "disabled", status: http.StatusForbidden, body: `{"errors":[{"title":"AI chat is not enabled for this team"}]}`, wantError: "Rootly AI chat isn't enabled for your organization. Contact your Rootly account team or Rootly support to turn it on."},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				writer.Header().Set("Content-Type", "application/vnd.api+json")
				writer.WriteHeader(test.status)
				_, _ = writer.Write([]byte(test.body))
			}))
			defer server.Close()
			client, err := api.NewClient(&config.Config{APIKey: "test-key", Endpoint: server.URL})
			if err != nil {
				t.Fatalf("NewClient() error = %v", err)
			}
			_, _, err = latestSession(context.Background(), client)
			if err == nil || err.Error() != test.wantError {
				t.Errorf("latestSession() error = %v, want %q", err, test.wantError)
			}
		})
	}
}
