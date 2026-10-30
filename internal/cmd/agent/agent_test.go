package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	aiagent "github.com/rootlyhq/rootly-cli/internal/agent"
	"github.com/rootlyhq/rootly-cli/internal/api"
	"github.com/rootlyhq/rootly-cli/internal/config"
)

func TestPreflightAIFeatureFlag(t *testing.T) {
	tests := []struct {
		name               string
		status             int
		flags              map[string]bool
		wantError          string
		wantUser           bool
		wantAIChatDisabled bool
	}{
		{name: "disabled", status: http.StatusOK, flags: map[string]bool{"ai-chat": false}, wantError: "Rootly AI chat isn't enabled for your organization. Contact your Rootly account team or Rootly support to turn it on.", wantAIChatDisabled: true},
		{name: "enabled", status: http.StatusOK, flags: map[string]bool{"ai-chat": true}, wantUser: true},
		{name: "flag absent", status: http.StatusOK, flags: map[string]bool{"ai-native-home": true}, wantUser: true},
		{name: "unauthorized", status: http.StatusUnauthorized, wantError: "not authenticated: run `rootly login` or set ROOTLY_API_KEY"},
		{name: "other failure proceeds", status: http.StatusInternalServerError},
		{name: "disabled response", status: http.StatusForbidden, wantError: "Rootly AI chat isn't enabled for your organization. Contact your Rootly account team or Rootly support to turn it on.", wantAIChatDisabled: true},
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
				case http.StatusForbidden:
					_, _ = writer.Write([]byte(`{"errors":[{"title":"AI chat is not enabled for this team"}]}`))
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
				if got := aiagent.IsAIChatDisabled(err); got != test.wantAIChatDisabled {
					t.Errorf("IsAIChatDisabled(error) = %v, want %v", got, test.wantAIChatDisabled)
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

func TestAgentPreflightRunsBeforeIncidentLookup(t *testing.T) {
	var incidentLookups atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/v1/users/me" {
			writer.Header().Set("Content-Type", "application/vnd.api+json")
			_, _ = writer.Write([]byte(`{"data":{"id":"user-1","type":"users","attributes":{"feature_flags":{"ai-chat":false}}}}`))
			return
		}
		if strings.Contains(request.URL.Path, "/incidents") {
			incidentLookups.Add(1)
		}
		http.NotFound(writer, request)
	}))
	defer server.Close()

	oldAPIKey, oldAPIHost, oldDebug := viper.GetString("api_key"), viper.GetString("api_host"), viper.GetBool("debug")
	viper.Set("api_key", "test-key")
	viper.Set("api_host", server.URL)
	viper.Set("debug", false)
	t.Cleanup(func() {
		viper.Set("api_key", oldAPIKey)
		viper.Set("api_host", oldAPIHost)
		viper.Set("debug", oldDebug)
	})

	cmd := &cobra.Command{}
	cmd.Flags().StringP("print", "p", "", "")
	cmd.Flags().StringP("resume", "r", "", "")
	cmd.Flags().BoolP("continue", "c", false, "")
	cmd.Flags().String("incident", "", "")
	cmd.Flags().String("alert", "", "")
	cmd.Flags().String("output-format", "text", "")
	cmd.SetContext(context.Background())
	cmd.SetIn(bytes.NewReader(nil))
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	if err := cmd.Flags().Set("incident", "INC-8"); err != nil {
		t.Fatalf("set incident flag: %v", err)
	}

	err := AgentCmd.RunE(cmd, nil)
	wantError := "Rootly AI chat isn't enabled for your organization. Contact your Rootly account team or Rootly support to turn it on."
	if err == nil || err.Error() != wantError {
		t.Fatalf("AgentCmd.RunE() error = %v, want %q", err, wantError)
	}
	if got := incidentLookups.Load(); got != 0 {
		t.Errorf("incident lookup requests = %d, want none before AI preflight", got)
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
		{name: "no sessions", status: http.StatusOK, body: `{"meta":{"pagination":{"has_more":false,"next_before":null}},"sessions":[]}`, wantError: "no previous AI chat session to continue (incident/alert-bound chats are resumed with --resume <id>)"},
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

func TestContinueFlagHelp(t *testing.T) {
	flag := AgentCmd.Flags().Lookup("continue")
	if flag == nil {
		t.Fatal("continue flag is not registered")
	}
	want := "Continue the most recent AI chat session (incident/alert chats: use --resume <id>)"
	if flag.Usage != want {
		t.Errorf("continue help = %q, want %q", flag.Usage, want)
	}
}

func TestClearTerminalForAgent(t *testing.T) {
	tests := []struct {
		name     string
		enabled  bool
		terminal bool
		want     string
	}{
		{name: "terminal", enabled: true, terminal: true, want: "\x1b[2J\x1b[3J\x1b[H"},
		{name: "disabled", enabled: false, terminal: true},
		{name: "non-terminal", enabled: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var output bytes.Buffer
			if err := clearTerminalForAgent(&output, test.enabled, test.terminal); err != nil {
				t.Fatalf("clearTerminalForAgent() error = %v", err)
			}
			if got := output.String(); got != test.want {
				t.Errorf("clear output = %q, want %q", got, test.want)
			}
		})
	}
}

func TestAgentPrintModeDoesNotEmitTerminalClear(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/v1/users/me":
			writer.Header().Set("Content-Type", "application/vnd.api+json")
			_, _ = writer.Write([]byte(`{"data":{"id":"user-1","type":"users","attributes":{"email":"test@example.com","feature_flags":{"ai-chat":true}}}}`))
		case "/v1/ai/chat/stream":
			writer.Header().Set("Content-Type", "text/event-stream")
			_, _ = writer.Write([]byte("event: text\ndata: {\"content\":\"answer\"}\n\nevent: done\ndata: {\"status\":\"complete\"}\n\n"))
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	oldAPIKey, oldAPIHost := viper.GetString("api_key"), viper.GetString("api_host")
	oldNoColor, oldQuiet := viper.GetBool("no-color"), viper.GetBool("quiet")
	viper.Set("api_key", "test-key")
	viper.Set("api_host", server.URL)
	viper.Set("no-color", true)
	viper.Set("quiet", false)
	t.Cleanup(func() {
		viper.Set("api_key", oldAPIKey)
		viper.Set("api_host", oldAPIHost)
		viper.Set("no-color", oldNoColor)
		viper.Set("quiet", oldQuiet)
	})

	cmd := &cobra.Command{}
	cmd.Flags().StringP("print", "p", "", "")
	cmd.Flags().StringP("resume", "r", "", "")
	cmd.Flags().BoolP("continue", "c", false, "")
	cmd.Flags().String("incident", "", "")
	cmd.Flags().String("alert", "", "")
	cmd.Flags().String("output-format", "text", "")
	if err := cmd.Flags().Set("print", "question"); err != nil {
		t.Fatalf("set print flag: %v", err)
	}
	cmd.SetContext(context.Background())
	cmd.SetIn(bytes.NewBuffer(nil))
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)

	if err := AgentCmd.RunE(cmd, nil); err != nil {
		t.Fatalf("AgentCmd.RunE() error = %v", err)
	}
	if got := stdout.String(); got != "answer\n" {
		t.Errorf("stdout = %q, want print answer", got)
	}
	if strings.Contains(stdout.String(), "\x1b") || strings.Contains(stderr.String(), "\x1b") {
		t.Errorf("print output contains escape bytes: stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}
