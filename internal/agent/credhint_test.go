package agent

import (
	"context"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/rootlyhq/rootly-cli/internal/mcp"
)

func TestLooksLikeExpiredCreds(t *testing.T) {
	cases := []struct {
		result mcp.CallResult
		want   bool
	}{
		{mcp.CallResult{Output: "Error when retrieving token from sso: Token has expired and refresh failed"}, true},
		{mcp.CallResult{Output: "The SSO session associated with this profile has expired or is otherwise invalid."}, true},
		{mcp.CallResult{Output: "An error occurred (ExpiredToken) when calling the GetCallerIdentity operation"}, true},
		{mcp.CallResult{Output: "HTTP 401 Unauthorized", IsError: true}, true},
		{mcp.CallResult{Output: "request failed: 403 Forbidden", IsError: true}, true},
		{mcp.CallResult{Output: "found 401 rows in table forbidden_words"}, false},
		{mcp.CallResult{Output: userDeclinedOutput, IsError: true}, false},
		{mcp.CallResult{Output: "file not found", IsError: true}, false},
	}
	for _, tc := range cases {
		if got := looksLikeExpiredCreds(tc.result); got != tc.want {
			t.Errorf("looksLikeExpiredCreds(%q, error=%v) = %v, want %v", tc.result.Output, tc.result.IsError, got, tc.want)
		}
	}
}

func TestCredHintShownOnceInTUI(t *testing.T) {
	model := NewTUIModel(context.Background(), &fakeStreamer{}, TUIOptions{NoColor: true})
	model.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	model.Init()
	model.Update(localToolDoneMsg{turn: model.turn, credHint: true})
	model.Update(localToolDoneMsg{turn: model.turn, credHint: true})
	model.Update(localToolDoneMsg{turn: model.turn})
	if got := strings.Count(model.Transcript(), localCredsHint); got != 1 {
		t.Fatalf("hint shown %d times, want once:\n%s", got, model.Transcript())
	}
}

func TestCredHintShownOnceInPrintMode(t *testing.T) {
	tools := &fakeLocalTools{
		tools:  map[string]mcp.ToolRef{"local_aws_whoami": {ServerID: "user:aws", ServerName: "aws", Tool: "whoami", ReadOnly: true}},
		output: "The SSO session associated with this profile has expired or is otherwise invalid.",
	}
	trip := newToolRoundTrip(t, 2, "local_aws_whoami")
	_, stderr, _ := trip.run(t, tools, true, "")
	if got := strings.Count(stderr, localCredsHint); got != 1 {
		t.Fatalf("hint shown %d times, want once:\n%s", got, stderr)
	}
}
