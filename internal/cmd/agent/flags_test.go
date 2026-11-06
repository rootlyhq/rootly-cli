package agent

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	aiagent "github.com/rootlyhq/rootly-cli/internal/agent"
)

func agentFlagsCommand(t *testing.T, flags map[string]string) *cobra.Command {
	t.Helper()
	cmd := &cobra.Command{}
	cmd.Flags().String("print", "", "")
	cmd.Flags().String("resume", "", "")
	cmd.Flags().Bool("continue", false, "")
	cmd.Flags().String("incident", "", "")
	cmd.Flags().String("alert", "", "")
	cmd.Flags().String("output-format", "text", "")
	for name, value := range flags {
		if err := cmd.Flags().Set(name, value); err != nil {
			t.Fatal(err)
		}
	}
	cmd.SetIn(strings.NewReader(""))
	cmd.SetContext(context.Background())
	return cmd
}

func TestRunAgentRejectsBadFlagsBeforeAnyRequest(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	previous := viper.GetString("api_key")
	viper.Set("api_key", "test-key")
	viper.Set("api_host", "127.0.0.1:1")
	t.Cleanup(func() { viper.Set("api_key", previous); viper.Set("api_host", "") })
	cases := []struct {
		flags map[string]string
		want  string
	}{
		{map[string]string{"print": "hi", "output-format": "yaml"}, `invalid output format "yaml"`},
		{map[string]string{"print": "hi", "resume": "not-a-uuid"}, `invalid resume session UUID "not-a-uuid"`},
	}
	for _, tc := range cases {
		err := runAgent(agentFlagsCommand(t, tc.flags))
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("runAgent(%v) error = %v, want %q", tc.flags, err, tc.want)
		}
	}
}

func TestRunAgentNeedsAuthenticationInPrintMode(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	previous := viper.GetString("api_key")
	viper.Set("api_key", "")
	t.Cleanup(func() { viper.Set("api_key", previous) })
	err := runAgent(agentFlagsCommand(t, map[string]string{"print": "hi"}))
	if err == nil || !strings.Contains(err.Error(), "authentication required") {
		t.Fatalf("runAgent without credentials = %v", err)
	}
}

func TestAgentWorkingDir(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	got := agentWorkingDir()
	resolvedGot, _ := os.Stat(got)
	resolvedWant, _ := os.Stat(dir)
	if !os.SameFile(resolvedGot, resolvedWant) {
		t.Fatalf("agentWorkingDir() = %q, want %q", got, dir)
	}
}

func TestRunAgentReportsLookupFailures(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	unauthorized := false
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if unauthorized {
			writer.WriteHeader(http.StatusUnauthorized)
			return
		}
		http.NotFound(writer, request)
	}))
	defer server.Close()
	oldKey, oldHost := viper.GetString("api_key"), viper.GetString("api_host")
	viper.Set("api_key", "test-key")
	viper.Set("api_host", server.URL)
	t.Cleanup(func() { viper.Set("api_key", oldKey); viper.Set("api_host", oldHost) })
	cases := []struct {
		flags map[string]string
		want  string
	}{
		{map[string]string{"print": "hi", "incident": "INC-404"}, "incident not found"},
		{map[string]string{"print": "hi", "alert": "missing"}, "alert not found"},
		{map[string]string{"print": "hi", "continue": "true"}, "404"},
	}
	for _, tc := range cases {
		err := runAgent(agentFlagsCommand(t, tc.flags))
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("runAgent(%v) error = %v, want %q", tc.flags, err, tc.want)
		}
	}
	unauthorized = true
	err := runAgent(agentFlagsCommand(t, map[string]string{"print": "hi", "incident": "INC-1"}))
	if !aiagent.IsNotAuthenticated(err) {
		t.Fatalf("a 401 preflight must stop as not authenticated, got %v", err)
	}
}

func TestRunAgentPrintModeStartsLocalToolsAndSurfacesStreamErrors(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Chdir(t.TempDir())
	var streamed atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if strings.Contains(request.URL.Path, "/ai/") {
			streamed.Add(1)
			writer.WriteHeader(http.StatusInternalServerError)
			return
		}
		http.NotFound(writer, request)
	}))
	defer server.Close()
	oldKey, oldHost := viper.GetString("api_key"), viper.GetString("api_host")
	viper.Set("api_key", "test-key")
	viper.Set("api_host", server.URL)
	t.Cleanup(func() { viper.Set("api_key", oldKey); viper.Set("api_host", oldHost) })
	cmd := agentFlagsCommand(t, map[string]string{"print": "hello"})
	cmd.AddCommand(&cobra.Command{Use: "child"})
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	err := runAgent(cmd)
	if err == nil {
		t.Fatal("a failing chat stream must return an error")
	}
	if streamed.Load() == 0 {
		t.Fatalf("print mode never reached the chat API: %v", err)
	}
	if strings.Contains(stdout.String(), "\x1b[2J") {
		t.Fatal("print mode must not clear the terminal")
	}
}

func TestPreflightAIDebugReportsSoftFailures(t *testing.T) {
	var status atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/vnd.api+json")
		writer.WriteHeader(int(status.Load()))
		_, _ = writer.Write([]byte(`{"errors":[{"title":"Forbidden"}]}`))
	}))
	oldKey, oldHost, oldDebug := viper.GetString("api_key"), viper.GetString("api_host"), viper.GetBool("debug")
	viper.Set("api_key", "test-key")
	viper.Set("api_host", server.URL)
	viper.Set("debug", true)
	t.Cleanup(func() { viper.Set("api_key", oldKey); viper.Set("api_host", oldHost); viper.Set("debug", oldDebug) })
	client, err := getAPIClient()
	if err != nil {
		t.Fatal(err)
	}
	for _, code := range []int32{http.StatusForbidden, http.StatusBadGateway} {
		status.Store(code)
		cmd := agentFlagsCommand(t, nil)
		var stderr bytes.Buffer
		cmd.SetErr(&stderr)
		user, err := preflightAI(cmd, client)
		if user != nil || err != nil {
			t.Fatalf("HTTP %d preflight = %v, %v; want a soft failure", code, user, err)
		}
		if !strings.Contains(stderr.String(), "AI preflight returned HTTP") {
			t.Errorf("HTTP %d debug output = %q", code, stderr.String())
		}
	}
	server.Close()
	cmd := agentFlagsCommand(t, nil)
	var stderr bytes.Buffer
	cmd.SetErr(&stderr)
	if user, err := preflightAI(cmd, client); user != nil || err != nil || !strings.Contains(stderr.String(), "AI preflight failed") {
		t.Fatalf("unreachable preflight = %v, %v, %q", user, err, stderr.String())
	}
}

func TestGetAPIClientDefaultsTheEndpoint(t *testing.T) {
	oldKey, oldHost := viper.GetString("api_key"), viper.GetString("api_host")
	viper.Set("api_key", "test-key")
	viper.Set("api_host", "")
	t.Cleanup(func() { viper.Set("api_key", oldKey); viper.Set("api_host", oldHost) })
	client, err := getAPIClient()
	if err != nil || client.APIHost() == "" {
		t.Fatalf("getAPIClient() = %v, %v", client, err)
	}
}

func TestStartLocalMCPReportsBadConfigAndKeepsGoing(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Chdir(t.TempDir())
	if err := os.MkdirAll(filepath.Join(home, ".rootly-cli"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".rootly-cli", "mcp.json"), []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	cmd := agentFlagsCommand(t, nil)
	var stderr bytes.Buffer
	cmd.SetErr(&stderr)
	manager := startLocalMCP(cmd)
	defer manager.Close()
	if !strings.Contains(stderr.String(), "local MCP config") {
		t.Fatalf("bad config not reported: %q", stderr.String())
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	waitForLocalMCP(ctx, manager, time.Second)
	if tools, _ := manager.ClientTools(); tools == nil && manager.Connecting() != 0 {
		t.Fatal("manager should still be usable after a bad config")
	}
}
