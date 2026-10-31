package mcp

import (
	"context"
	"strings"
	"testing"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

func connectInMemory(t *testing.T, server *sdk.Server) *sdk.ClientSession {
	t.Helper()
	ctx := context.Background()
	clientTransport, serverTransport := sdk.NewInMemoryTransports()
	serverSession, err := server.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = serverSession.Close() })
	client := sdk.NewClient(&sdk.Implementation{Name: "test-client", Version: "1"}, nil)
	session, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = session.Close() })
	return session
}

func docsServer() *sdk.Server {
	server := sdk.NewServer(&sdk.Implementation{Name: "docs", Version: "1"}, nil)
	server.AddPrompt(&sdk.Prompt{Name: "triage", Description: "Triage an alert", Arguments: []*sdk.PromptArgument{{Name: "service", Required: true}}},
		func(_ context.Context, request *sdk.GetPromptRequest) (*sdk.GetPromptResult, error) {
			return &sdk.GetPromptResult{Messages: []*sdk.PromptMessage{{Role: "user", Content: &sdk.TextContent{Text: "Triage " + request.Params.Arguments["service"]}}}}, nil
		})
	server.AddResource(&sdk.Resource{URI: "file:///runbook.md", Name: "runbook", MIMEType: "text/markdown"},
		func(_ context.Context, request *sdk.ReadResourceRequest) (*sdk.ReadResourceResult, error) {
			return &sdk.ReadResourceResult{Contents: []*sdk.ResourceContents{{URI: request.Params.URI, Text: strings.Repeat("é", 40000)}}}, nil
		})
	server.AddResource(&sdk.Resource{URI: "file:///logo.png", Name: "logo", MIMEType: "image/png"},
		func(_ context.Context, request *sdk.ReadResourceRequest) (*sdk.ReadResourceResult, error) {
			return &sdk.ReadResourceResult{Contents: []*sdk.ResourceContents{{URI: request.Params.URI, Blob: []byte{1, 2, 3}}}}, nil
		})
	return server
}

func TestDiscoverExtrasListsPromptsAndResources(t *testing.T) {
	session := connectInMemory(t, docsServer())
	prompts, resources := discoverExtras(context.Background(), session)
	if len(prompts) != 1 || prompts[0].Name != "triage" || len(prompts[0].Arguments) != 1 || !prompts[0].Arguments[0].Required {
		t.Fatalf("prompts = %+v", prompts)
	}
	if len(resources) != 2 || resources[0].URI != "file:///logo.png" && resources[1].URI != "file:///logo.png" {
		t.Fatalf("resources = %+v", resources)
	}
}

func TestDiscoverExtrasSkipsUndeclaredCapabilities(t *testing.T) {
	server := sdk.NewServer(&sdk.Implementation{Name: "tools-only", Version: "1"}, nil)
	session := connectInMemory(t, server)
	if caps := session.InitializeResult().Capabilities; caps.Prompts != nil || caps.Resources != nil {
		t.Skip("SDK advertises prompts/resources without features")
	}
	prompts, resources := discoverExtras(context.Background(), session)
	if prompts != nil || resources != nil {
		t.Fatalf("prompts=%v resources=%v, want none", prompts, resources)
	}
}

func TestGetPromptAndReadResourceWithCap(t *testing.T) {
	session := connectInMemory(t, docsServer())
	manager := &Manager{servers: []*runtimeServer{
		{server: Server{Name: "off"}, session: session, sessionOff: true},
		{server: Server{Name: "docs"}, session: session, status: StatusConnected},
	}}
	ctx := context.Background()
	text, err := manager.GetPrompt(ctx, "docs", "triage", map[string]string{"service": "api"})
	if err != nil || text != "Triage api" {
		t.Fatalf("GetPrompt = %q, %v", text, err)
	}
	body, truncated, err := manager.ReadResource(ctx, "docs", "file:///runbook.md", 1001)
	if err != nil || !truncated || len(body) != 1000 || !strings.HasPrefix(body, "éé") {
		t.Fatalf("ReadResource = len %d truncated %v err %v", len(body), truncated, err)
	}
	full, truncated, err := manager.ReadResource(ctx, "docs", "file:///runbook.md", 0)
	if err != nil || truncated || len(full) != 80000 {
		t.Fatalf("uncapped ReadResource = len %d truncated %v err %v", len(full), truncated, err)
	}
	if _, _, err := manager.ReadResource(ctx, "docs", "file:///logo.png", ResourceByteCap); err == nil {
		t.Fatal("binary resource should be unreadable")
	}
	if _, _, err := manager.ReadResource(ctx, "off", "file:///runbook.md", ResourceByteCap); err == nil {
		t.Fatal("session-off server should not be readable")
	}
	if _, err := manager.GetPrompt(ctx, "missing", "triage", nil); err == nil {
		t.Fatal("unknown server should fail")
	}
}
