package agent

import (
	"context"
	"encoding/json"
	"time"

	rootly "github.com/rootlyhq/rootly-go"

	"github.com/rootlyhq/rootly-cli/internal/api"
	"github.com/rootlyhq/rootly-cli/internal/mcp"
)

const (
	localToolsOffMessage = "Local MCP tools are off for your org"
	userDeclinedOutput   = "User declined"
)

// LocalTools is the agent's view of the local MCP manager.
type LocalTools interface {
	ClientTools() (tools []mcp.ClientTool, omitted int)
	Lookup(string) (mcp.ToolRef, bool)
	Call(context.Context, string, map[string]any) mcp.CallResult
}

type ToolResultPoster interface {
	PostAiChatToolResult(context.Context, api.ToolResult) error
}

func localToolActivity(ref mcp.ToolRef) string {
	return "⚙ local · " + sanitizeTerminalText(ref.ServerName) + " · " + sanitizeTerminalText(ref.Tool)
}

func resolveLocalCall(tools LocalTools, call *ClientToolCall) (mcp.ToolRef, bool) {
	if tools == nil || call == nil {
		return mcp.ToolRef{}, false
	}
	return tools.Lookup(call.Name)
}

func runLocalCall(ctx context.Context, tools LocalTools, call *ClientToolCall) mcp.CallResult {
	if _, ok := resolveLocalCall(tools, call); !ok {
		return mcp.CallResult{Output: "Unknown local tool " + call.Name, IsError: true}
	}
	return tools.Call(ctx, call.Name, call.Arguments)
}

func postLocalResult(ctx context.Context, poster ToolResultPoster, sessionID string, call *ClientToolCall, result mcp.CallResult) error {
	if poster == nil {
		return nil
	}
	return poster.PostAiChatToolResult(ctx, api.ToolResult{
		SessionID: sessionID,
		CallID:    call.CallID,
		Output:    mcp.TruncateOutput(result.Output),
		IsError:   result.IsError,
	})
}

func localToolConfirmation(ref mcp.ToolRef, call *ClientToolCall) *rootly.AiChatStreamConfirmationRequired {
	confirmation := &rootly.AiChatStreamConfirmationRequired{
		Question: "Run local tool " + ref.ServerName + " · " + ref.Tool + "? It can change things on this machine.",
	}
	if call != nil && len(call.Arguments) > 0 {
		if preview, err := json.MarshalIndent(call.Arguments, "", "  "); err == nil {
			confirmation.Preview = preview
		}
	}
	return confirmation
}

func renderLocalToolCard(ref mcp.ToolRef, call *ClientToolCall, width int, noColor bool) string {
	return renderConfirmationCard(localToolConfirmation(ref, call), width, noColor, time.Now())
}
