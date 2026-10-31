package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/rootlyhq/rootly-cli/internal/api"
	"github.com/rootlyhq/rootly-cli/internal/config"
	"github.com/rootlyhq/rootly-cli/internal/mcp"
)

const localTestSessionID = "39fd7596-152c-43d3-afce-48b792ae386d"

type fakeLocalTools struct {
	mu    sync.Mutex
	tools map[string]mcp.ToolRef
	calls []string
}

func (f *fakeLocalTools) ClientTools() ([]mcp.ClientTool, int) {
	out := []mcp.ClientTool{}
	for name, ref := range f.tools {
		out = append(out, mcp.ClientTool{Name: name, Description: ref.Tool, InputSchema: json.RawMessage(`{"type":"object"}`), ReadOnly: ref.ReadOnly})
	}
	return out, 0
}

func (f *fakeLocalTools) Lookup(name string) (mcp.ToolRef, bool) {
	ref, ok := f.tools[name]
	return ref, ok
}

func (f *fakeLocalTools) Call(_ context.Context, name string, arguments map[string]any) mcp.CallResult {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, name)
	return mcp.CallResult{Output: fmt.Sprintf("ran %s with %v", name, arguments["path"])}
}

type toolRoundTrip struct {
	server      *httptest.Server
	streamBody  map[string]any
	results     chan api.ToolResult
	accepted    int
	toolName    string
	streamCount int
	mu          sync.Mutex
}

func newToolRoundTrip(t *testing.T, accepted int, toolName string) *toolRoundTrip {
	t.Helper()
	trip := &toolRoundTrip{results: make(chan api.ToolResult, 1), accepted: accepted, toolName: toolName}
	trip.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/ai/chat/stream":
			body, _ := io.ReadAll(r.Body)
			trip.mu.Lock()
			trip.streamCount++
			_ = json.Unmarshal(body, &trip.streamBody)
			trip.mu.Unlock()
			w.Header().Set("Content-Type", "text/event-stream")
			flusher := w.(http.Flusher)
			_, _ = fmt.Fprintf(w, "event: session_id\ndata: {\"session_id\":%q,\"client_tools_accepted\":%d}\n\n", localTestSessionID, trip.accepted)
			if trip.toolName != "" {
				_, _ = fmt.Fprintf(w, "event: client_tool_call\ndata: {\"call_id\":\"call-1\",\"name\":%q,\"arguments\":{\"path\":\"/tmp/a\"}}\n\n", trip.toolName)
				flusher.Flush()
				select {
				case result := <-trip.results:
					trip.results <- result
					_, _ = fmt.Fprintf(w, "event: text\ndata: {\"content\":\"got %s\"}\n\n", strings.ReplaceAll(result.Output, `"`, ``))
				case <-time.After(5 * time.Second):
					_, _ = fmt.Fprint(w, "event: text\ndata: {\"content\":\"timed out\"}\n\n")
				}
			} else {
				_, _ = fmt.Fprint(w, "event: text\ndata: {\"content\":\"hello\"}\n\n")
			}
			_, _ = fmt.Fprint(w, "event: done\ndata: {\"status\":\"complete\"}\n\n")
		case "/v1/ai/chat/tool_results":
			if r.Method != http.MethodPost || r.Header.Get("Authorization") != "Bearer test-token" {
				t.Errorf("tool_results request = %s auth=%q", r.Method, r.Header.Get("Authorization"))
			}
			var result api.ToolResult
			if err := json.NewDecoder(r.Body).Decode(&result); err != nil {
				t.Errorf("decode tool result: %v", err)
			}
			trip.results <- result
			w.WriteHeader(http.StatusAccepted)
		default:
			t.Errorf("unexpected request %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(trip.server.Close)
	return trip
}

func (trip *toolRoundTrip) run(t *testing.T, tools LocalTools, stdinTTY bool, stdin string) (string, string, api.ToolResult) {
	t.Helper()
	client, err := api.NewClient(&config.Config{APIKey: "test-token", Endpoint: trip.server.URL})
	if err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	options := newPrintOptions(&stdout, &stderr)
	options.Stdin = strings.NewReader(stdin)
	options.IsStdinTTY = func() bool { return stdinTTY }
	options.IsStderrTTY = func() bool { return stdinTTY }
	options.NoColor = true
	options.LocalTools = tools
	options.ToolResults = client
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := RunPrint(ctx, client, options); err != nil {
		t.Fatalf("RunPrint() error = %v (stderr %q)", err, stderr.String())
	}
	var result api.ToolResult
	select {
	case result = <-trip.results:
	default:
	}
	return stdout.String(), stderr.String(), result
}

func TestPrintClientToolCallRoundTripReadOnly(t *testing.T) {
	tools := &fakeLocalTools{tools: map[string]mcp.ToolRef{"local_fs_read_file": {ServerID: "user:fs", ServerName: "fs", Tool: "read_file", ReadOnly: true}}}
	trip := newToolRoundTrip(t, 1, "local_fs_read_file")
	stdout, stderr, result := trip.run(t, tools, true, "")

	sent, _ := trip.streamBody["client_tools"].([]any)
	if len(sent) != 1 || sent[0].(map[string]any)["name"] != "local_fs_read_file" || sent[0].(map[string]any)["read_only"] != true {
		t.Fatalf("client_tools body = %v", trip.streamBody)
	}
	if result.SessionID != localTestSessionID || result.CallID != "call-1" || result.IsError || result.Output != "ran local_fs_read_file with /tmp/a" {
		t.Fatalf("result = %+v", result)
	}
	if !strings.Contains(stdout, "got ran local_fs_read_file") {
		t.Fatalf("stdout = %q", stdout)
	}
	if !strings.Contains(stderr, "⚙ local · fs · read_file") || strings.Contains(stderr, localToolsOffMessage) {
		t.Fatalf("stderr = %q", stderr)
	}
}

func TestPrintClientToolWriteDeclinedWithoutTTY(t *testing.T) {
	tools := &fakeLocalTools{tools: map[string]mcp.ToolRef{"local_fs_write_file": {ServerID: "user:fs", ServerName: "fs", Tool: "write_file"}}}
	trip := newToolRoundTrip(t, 1, "local_fs_write_file")
	_, _, result := trip.run(t, tools, false, "y\n")
	if !result.IsError || result.Output != "User declined" {
		t.Fatalf("result = %+v", result)
	}
	if len(tools.calls) != 0 {
		t.Fatalf("declined tool must not run, calls = %v", tools.calls)
	}
}

func TestPrintClientToolWriteApprovedOnTTY(t *testing.T) {
	tools := &fakeLocalTools{tools: map[string]mcp.ToolRef{"local_fs_write_file": {ServerID: "user:fs", ServerName: "fs", Tool: "write_file"}}}
	trip := newToolRoundTrip(t, 1, "local_fs_write_file")
	_, _, result := trip.run(t, tools, true, "y\n")
	if result.IsError || len(tools.calls) != 1 {
		t.Fatalf("result = %+v calls = %v", result, tools.calls)
	}

	declined := &fakeLocalTools{tools: tools.tools}
	trip = newToolRoundTrip(t, 1, "local_fs_write_file")
	_, _, result = trip.run(t, declined, true, "n\n")
	if !result.IsError || result.Output != "User declined" || len(declined.calls) != 0 {
		t.Fatalf("result = %+v calls = %v", result, declined.calls)
	}
}

func TestPrintUnknownClientToolReturnsError(t *testing.T) {
	tools := &fakeLocalTools{tools: map[string]mcp.ToolRef{"local_fs_read_file": {ServerName: "fs", Tool: "read_file", ReadOnly: true}}}
	trip := newToolRoundTrip(t, 1, "local_other_tool")
	_, _, result := trip.run(t, tools, false, "")
	if !result.IsError || result.CallID != "call-1" {
		t.Fatalf("result = %+v", result)
	}
}

func TestPrintLocalToolsOffForOrg(t *testing.T) {
	tools := &fakeLocalTools{tools: map[string]mcp.ToolRef{"local_fs_read_file": {ServerName: "fs", Tool: "read_file", ReadOnly: true}}}
	trip := newToolRoundTrip(t, 0, "")
	stdout, stderr, _ := trip.run(t, tools, false, "")
	if strings.Count(stderr, localToolsOffMessage) != 1 || stdout != "hello\n" {
		t.Fatalf("stdout = %q stderr = %q", stdout, stderr)
	}

	none := &fakeLocalTools{tools: map[string]mcp.ToolRef{}}
	trip = newToolRoundTrip(t, 0, "")
	_, stderr, _ = trip.run(t, none, false, "")
	if strings.Contains(stderr, localToolsOffMessage) {
		t.Fatalf("no tools sent, notice must not show: %q", stderr)
	}
	if _, ok := trip.streamBody["client_tools"]; ok {
		t.Fatalf("no client_tools key expected: %v", trip.streamBody)
	}
}

func TestDecodeClientToolCallRequiresIDs(t *testing.T) {
	if _, _, err := decodeEvent(eventOf(EventClientToolCall, `{"name":"local_x"}`)); err == nil {
		t.Fatal("missing call_id should fail")
	}
	event, _, err := decodeEvent(eventOf(EventClientToolCall, `{"call_id":"c","name":"local_x","arguments":{"a":1}}`))
	if err != nil || event.ClientToolCall == nil || event.ClientToolCall.Arguments["a"] != float64(1) {
		t.Fatalf("event = %+v err = %v", event, err)
	}
}
