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

	rootly "github.com/rootlyhq/rootly-go"

	"github.com/rootlyhq/rootly-cli/internal/api"
	"github.com/rootlyhq/rootly-cli/internal/config"
	"github.com/rootlyhq/rootly-cli/internal/mcp"
)

const localTestSessionID = "39fd7596-152c-43d3-afce-48b792ae386d"

type fakeLocalTools struct {
	mu      sync.Mutex
	tools   map[string]mcp.ToolRef
	omitted []mcp.ToolRef
	calls   []string
	output  string
}

func (f *fakeLocalTools) ClientTools() (tools []mcp.ClientTool, omitted []mcp.ToolRef) {
	out := []mcp.ClientTool{}
	for name, ref := range f.tools {
		out = append(out, mcp.ClientTool{Name: name, Description: ref.Tool, InputSchema: json.RawMessage(`{"type":"object"}`), ReadOnly: ref.ReadOnly})
	}
	return out, f.omitted
}

func (f *fakeLocalTools) OmittedClientTools() []mcp.ToolRef {
	return f.omitted
}

func (f *fakeLocalTools) Lookup(name string) (mcp.ToolRef, bool) {
	ref, ok := f.tools[name]
	return ref, ok
}

func (f *fakeLocalTools) NeedsApproval(name string, _ map[string]any) bool {
	ref, ok := f.tools[name]
	return !ok || !ref.ReadOnly
}

func (f *fakeLocalTools) Call(_ context.Context, name string, arguments map[string]any) mcp.CallResult {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, name)
	if f.output != "" {
		return mcp.CallResult{Output: f.output, IsError: true}
	}
	return mcp.CallResult{Output: fmt.Sprintf("ran %s with %v", name, arguments["path"])}
}

func TestFormatOmittedLocalTools(t *testing.T) {
	omitted := make([]mcp.ToolRef, 7)
	for index := range omitted {
		omitted[index] = mcp.ToolRef{ServerName: fmt.Sprintf("server-%d", index), Tool: fmt.Sprintf("tool-%d", index)}
	}

	got := formatOmittedLocalTools(omitted)
	for _, expected := range []string{
		"7 local tools left out (max 64 per turn)",
		"server-0 · tool-0",
		"server-4 · tool-4",
		"… +2 more",
		"Turn off servers or tools you don't need in /mcp.",
	} {
		if !strings.Contains(got, expected) {
			t.Errorf("formatOmittedLocalTools() = %q, missing %q", got, expected)
		}
	}
	if strings.Contains(got, "server-5 · tool-5") {
		t.Fatalf("formatted more than five names: %q", got)
	}

	singular := formatOmittedLocalTools([]mcp.ToolRef{{ServerName: "x", Tool: "y"}})
	if want := "1 local tool left out (max 64 per turn): x · y."; !strings.Contains(singular, want) {
		t.Fatalf("singular notice = %q, want it to contain %q", singular, want)
	}
}

func TestTUIOmittedLocalToolsWarningAppearsOnlyWhenSetChanges(t *testing.T) {
	model := &TUIModel{noColor: true, width: 120}
	model.resetTranscript()
	first := []mcp.ToolRef{{ServerID: "one", ServerName: "server", Tool: "tool"}}
	changed := []mcp.ToolRef{{ServerID: "two", ServerName: "server", Tool: "tool"}}

	model.warnOmittedLocalTools(first)
	model.warnOmittedLocalTools(first)
	model.warnOmittedLocalTools(changed)

	if got := strings.Count(model.Transcript(), "local tool left out"); got != 2 {
		t.Fatalf("warning appeared %d times, want 2:\n%s", got, model.Transcript())
	}
}

func TestLocalToolConfirmationPreservesExecutableCode(t *testing.T) {
	ref := mcp.ToolRef{ServerID: "srv", ServerName: "third-party", Tool: "run"}
	command := `TOKEN="$(rm -rf ./data)"`
	confirmation := localToolConfirmation(ref, &ClientToolCall{
		Name: "third_party_run",
		Arguments: map[string]any{
			"command":  command,
			"password": "x${y}",
		},
	})
	var preview map[string]any
	if err := json.Unmarshal(confirmation.Preview, &preview); err != nil {
		t.Fatalf("decode MCP approval preview: %v", err)
	}
	if preview["command"] != command || preview["password"] != "x${y}" {
		t.Fatalf("MCP approval preview masked executable code: %#v", preview)
	}

	confirmation = localToolConfirmation(ref, &ClientToolCall{
		Name:      "third_party_run",
		Arguments: map[string]any{"password": "hunter22"},
	})
	if err := json.Unmarshal(confirmation.Preview, &preview); err != nil {
		t.Fatalf("decode plain-secret preview: %v", err)
	}
	if preview["password"] != mcp.Mask {
		t.Fatalf("MCP approval preview did not mask plain secret: %#v", preview)
	}

	confirmation = localToolConfirmation(mcp.ToolRef{ServerID: "builtin", Tool: "kubectl"}, &ClientToolCall{
		Name:      "local_kubectl",
		Arguments: map[string]any{"args": []any{"--token", "a$(b)"}},
	})
	if !strings.Contains(string(confirmation.Preview), "a$(b)") {
		t.Fatalf("CLI approval preview masked executable code: %q", confirmation.Preview)
	}

	for _, command := range []string{`export TOKEN="<(rm x)"`, `export TOKEN=">(rm x)"`} {
		confirmation = localToolConfirmation(mcp.ToolRef{ServerID: "builtin"}, &ClientToolCall{
			Name:      "local_bash",
			Arguments: map[string]any{"command": command},
		})
		if !strings.Contains(string(confirmation.Preview), command) {
			t.Errorf("bash approval preview masked executable code: %q", confirmation.Preview)
		}
	}
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

func (trip *toolRoundTrip) run(t *testing.T, tools LocalTools, stdinTTY bool, stdin string, quiet ...bool) (stdout, stderr string, result api.ToolResult) {
	t.Helper()
	client, err := api.NewClient(&config.Config{APIKey: "test-token", Endpoint: trip.server.URL})
	if err != nil {
		t.Fatal(err)
	}
	var outBuf, errBuf bytes.Buffer
	options := newPrintOptions(&outBuf, &errBuf)
	options.Stdin = strings.NewReader(stdin)
	options.IsStdinTTY = func() bool { return stdinTTY }
	options.IsStderrTTY = func() bool { return stdinTTY }
	options.NoColor = true
	options.LocalTools = tools
	options.ToolResults = client
	if len(quiet) > 0 {
		options.Quiet = quiet[0]
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := RunPrint(ctx, client, options); err != nil {
		t.Fatalf("RunPrint() error = %v (stderr %q)", err, errBuf.String())
	}
	select {
	case result = <-trip.results:
	default:
	}
	return outBuf.String(), errBuf.String(), result
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

func TestPrintLocalToolResultControlsCompletionActivity(t *testing.T) {
	for _, test := range []struct {
		name      string
		output    string
		wantCross int
		wantCheck int
	}{
		{name: "error", output: "path is denied", wantCross: 1},
		{name: "success", wantCheck: 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			tools := &fakeLocalTools{
				tools:  map[string]mcp.ToolRef{"local_fs_read_file": {ServerID: "user:fs", ServerName: "fs", Tool: "read_file", ReadOnly: true}},
				output: test.output,
			}
			var stdout, stderr bytes.Buffer
			options := newPrintOptions(&stdout, &stderr)
			options.IsStderrTTY = func() bool { return true }
			options.OutputFormat = "text"
			options.LocalTools = tools
			run := &printRun{options: options, ctx: context.Background()}
			run.activities.Handle(activityStatusEvent("Local read file..."))
			run.handleClientToolCall(&ClientToolCall{CallID: "call-1", Name: "local_fs_read_file"})
			if test.wantCross > 0 {
				run.writeActivityLines(run.activities.Handle(Event{
					Type: rootly.AiChatStreamEventTaskUpdate,
					TaskUpdate: &rootly.AiChatStreamTaskUpdate{
						ID:     "local-read",
						Title:  "Local read file failed: path is denied",
						Status: "failed",
					},
				}))
			} else {
				run.writeActivityLines(run.activities.Finish(true))
			}
			if got := strings.Count(stderr.String(), "✗"); got != test.wantCross {
				t.Errorf("failure markers = %d, want %d:\n%s", got, test.wantCross, stderr.String())
			}
			if got := strings.Count(stderr.String(), "✓"); got != test.wantCheck {
				t.Errorf("success markers = %d, want %d:\n%s", got, test.wantCheck, stderr.String())
			}
		})
	}
}

func TestPrintOmittedLocalToolsNoticeUsesStderr(t *testing.T) {
	tools := &fakeLocalTools{omitted: []mcp.ToolRef{{ServerName: "braintrust", Tool: "list_datasets"}}}
	trip := newToolRoundTrip(t, 0, "")
	stdout, stderr, _ := trip.run(t, tools, false, "")

	if !strings.Contains(stderr, "1 local tool left out (max 64 per turn): braintrust · list_datasets") {
		t.Fatalf("stderr = %q", stderr)
	}
	if strings.Contains(stdout, "left out") {
		t.Fatalf("omission notice leaked to stdout: %q", stdout)
	}
}

func TestPrintOmittedLocalToolsNoticeQuietModeWritesNothingToStderr(t *testing.T) {
	tools := &fakeLocalTools{omitted: []mcp.ToolRef{{ServerName: "braintrust", Tool: "list_datasets"}}}
	trip := newToolRoundTrip(t, 0, "")
	_, stderr, _ := trip.run(t, tools, false, "", true)

	if stderr != "" {
		t.Fatalf("stderr = %q, want empty", stderr)
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
